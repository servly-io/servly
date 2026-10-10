package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/httpapi"
	"github.com/servly-io/servly/internal/payments/provider"
	"github.com/servly-io/servly/internal/payments/provider/sandbox"
	"github.com/servly-io/servly/internal/payments/provider/stripe"
	"github.com/servly-io/servly/internal/platform/secretbox"
	"github.com/servly-io/servly/internal/storage"
	"github.com/servly-io/servly/web"
)

const shutdownGrace = 10 * time.Second

func newServeCommand(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "run the server: API, live order events and the web frontend",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return serve(ctx, cmd, f, nil)
		},
	}
	cmd.Flags().String("listen-addr", "", "address to listen on (default :8080)")
	cmd.Flags().Bool("migrate-on-start", false, "apply pending migrations at startup (default true; pass --migrate-on-start=false to require `servly migrate up`)")
	addDatabaseFlags(cmd)
	return cmd
}

// serve runs until ctx is cancelled. When ready is not nil it receives the
// bound listener address once the server accepts connections (for tests).
func serve(ctx context.Context, cmd *cobra.Command, f *rootFlags, ready chan<- string) error {
	cfg, log, err := load(cmd, f)
	if err != nil {
		return err
	}
	log.Info("starting servly", "version", Version, "database", cfg.DatabaseDriver,
		"listen_addr", cfg.ListenAddr, "config_file", cfg.File)

	db, err := openDatabase(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer db.Close()

	schemaCurrent := true
	if cfg.MigrateOnStart {
		state, err := db.Migrate(ctx)
		if err != nil {
			return err
		}
		log.Info("schema current", "version", state.Current)
	} else if state, err := db.MigrationState(ctx); err != nil {
		return err
	} else if state.Pending() {
		// Serve anyway: /readyz reports unavailable until an operator migrates.
		log.Warn("schema migrations pending; run `servly migrate up`", "current", state.Current, "latest", state.Latest)
		schemaCurrent = false
	}

	stop := make(chan struct{})
	application := app.New(db, log)
	application.Files = storage.NewLocal(filepath.Join(cfg.DataDir, "assets"))
	application.Providers = map[string]provider.Provider{"stripe": stripe.New()}
	if cfg.PaymentsSandbox {
		application.Providers["sandbox"] = sandbox.New()
		log.Warn("payments sandbox is on: guests can 'pay' on a fake checkout page; never use it for real guests")
	}
	if cfg.SecretKey != "" {
		// Validated when the configuration was loaded.
		application.Secrets, _ = secretbox.Parse(cfg.SecretKey)
	} else {
		log.Info("online payments are off: set secret_key (SERVLY_SECRET_KEY) to enable them")
	}
	if schemaCurrent {
		required, err := application.SetupRequired(ctx)
		if err != nil {
			return err
		}
		if required {
			// shortcut: the token lives in this process only, so a restart prints
			// a new one; persist it if several replicas ever share a fresh database.
			application.SetupToken = app.NewSetupToken()
			log.Warn("first-run setup: open servly in a browser and enter this setup token",
				"setup_token", application.SetupToken, "path", "/setup")
		}
	}
	go application.RunMaintenance(ctx, time.Hour)
	if schemaCurrent {
		go application.RunPaymentReconciler(ctx, 5*time.Second)
	}

	handler := httpapi.NewHandler(httpapi.Deps{
		Log:           log,
		Version:       Version,
		Frontend:      web.Dist(),
		App:           application,
		CookieSecure:  cfg.CookieSecure,
		TrustProxy:    cfg.TrustProxy,
		PublicBaseURL: cfg.PublicBaseURL,
		Stop:          stop,
		Ready: func(ctx context.Context) error {
			if err := db.PingContext(ctx); err != nil {
				return err
			}
			state, err := db.MigrationState(ctx)
			if err != nil {
				return err
			}
			if state.Pending() {
				return fmt.Errorf("schema at version %d, latest %d", state.Current, state.Latest)
			}
			return nil
		},
	})

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return err
	}
	srv := httpapi.NewServer(cfg.ListenAddr, handler)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("listening", "addr", ln.Addr().String())
	if ready != nil {
		ready <- ln.Addr().String()
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	close(stop)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

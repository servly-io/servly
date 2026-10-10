package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
)

// openApp resolves the configuration, opens the database and makes sure the
// schema is current, the same way `serve` does.
func openApp(cmd *cobra.Command, f *rootFlags) (*app.App, func(), error) {
	cfg, log, err := load(cmd, f)
	if err != nil {
		return nil, nil, err
	}
	db, err := openDatabase(cmd.Context(), cfg, log)
	if err != nil {
		return nil, nil, err
	}
	if err := ensureSchema(cmd.Context(), db, cfg.MigrateOnStart); err != nil {
		db.Close()
		return nil, nil, err
	}
	return app.New(db, log), func() { db.Close() }, nil
}

func ensureSchema(ctx context.Context, db *database.DB, migrate bool) error {
	if migrate {
		_, err := db.Migrate(ctx)
		return err
	}
	state, err := db.MigrationState(ctx)
	if err != nil {
		return err
	}
	if state.Pending() {
		return fmt.Errorf("schema at version %d, latest %d: run `servly migrate up` first", state.Current, state.Latest)
	}
	return nil
}

// readPassword reads a password: the first line of stdin with
// --password-stdin, else twice from the terminal without echo.
func readPassword(cmd *cobra.Command, fromStdin bool, prompt string) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("stdin is not a terminal: pass the password on stdin with --password-stdin")
	}
	fmt.Fprint(cmd.ErrOrStderr(), prompt+": ")
	first, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", err
	}
	fmt.Fprint(cmd.ErrOrStderr(), "repeat: ")
	second, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

func newTenantCommand(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "manage restaurants (tenants)",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var in app.CreateTenantInput
	var pwStdin bool
	create := &cobra.Command{
		Use:   "create",
		Short: "create a restaurant with its first owner",
		Example: `  servly tenant create --name "Trattoria Roma" --currency EUR --timezone Europe/Berlin --owner-email anna@example.com
  echo "$PASSWORD" | servly tenant create --name Roma --currency EUR --timezone Europe/Rome --owner-email a@b.c --password-stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, closeDB, err := openApp(cmd, f)
			if err != nil {
				return err
			}
			defer closeDB()
			// Ask for a password only for a new user; an existing user keeps theirs.
			if _, err := a.LookupUser(cmd.Context(), in.OwnerEmail); err != nil {
				if in.OwnerPassword, err = readPassword(cmd, pwStdin, "password for "+in.OwnerEmail); err != nil {
					return err
				}
			}
			res, err := a.CreateTenant(cmd.Context(), in)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "created tenant %q (slug %s, id %s)\n", res.Tenant.Name, res.Tenant.Slug, res.Tenant.ID)
			if res.OwnerExists {
				fmt.Fprintf(out, "owner: existing user %s (password unchanged)\n", res.Owner.Email)
			} else {
				fmt.Fprintf(out, "owner: %s\n", res.Owner.Email)
			}
			fmt.Fprintln(out, "sign in at /login")
			return nil
		},
	}
	fl := create.Flags()
	fl.StringVar(&in.Name, "name", "", "restaurant name (required)")
	fl.StringVar(&in.Slug, "slug", "", "short unique id, a-z 0-9 - (default: from the name)")
	fl.StringVar(&in.Currency, "currency", "EUR", "ISO 4217 currency of all prices")
	fl.StringVar(&in.Timezone, "timezone", "", "IANA timezone, e.g. Europe/Berlin (required)")
	fl.StringVar(&in.OwnerEmail, "owner-email", "", "owner's sign-in email (required)")
	fl.StringVar(&in.OwnerName, "owner-name", "", "owner's display name (default: from the email)")
	fl.BoolVar(&pwStdin, "password-stdin", false, "read the owner's password from the first line of stdin")
	for _, req := range []string{"name", "timezone", "owner-email"} {
		_ = create.MarkFlagRequired(req)
	}
	addDatabaseFlags(create)
	cmd.AddCommand(create)
	return cmd
}

func newUserCommand(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "manage staff sign-ins",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var pwStdin bool
	setPassword := &cobra.Command{
		Use:   "set-password <email>",
		Short: "set a user's password and sign them out everywhere",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, closeDB, err := openApp(cmd, f)
			if err != nil {
				return err
			}
			defer closeDB()
			pw, err := readPassword(cmd, pwStdin, "new password for "+args[0])
			if err != nil {
				return err
			}
			if err := a.SetPassword(cmd.Context(), args[0], pw); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "password set for %s; existing sessions ended\n", args[0])
			return nil
		},
	}
	setPassword.Flags().BoolVar(&pwStdin, "password-stdin", false, "read the password from the first line of stdin")
	addDatabaseFlags(setPassword)
	cmd.AddCommand(setPassword)
	return cmd
}

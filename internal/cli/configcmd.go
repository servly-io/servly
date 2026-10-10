package cli

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/servly-io/servly/internal/platform/config"
)

func newConfigCommand(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "inspect the configuration",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	path := &cobra.Command{
		Use:   "path",
		Short: "print which config file is read (none is fine)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := load(cmd, f)
			if err != nil {
				return err
			}
			if cfg.File == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "no config file; looked in:")
				for _, c := range config.Candidates() {
					fmt.Fprintln(cmd.OutOrStdout(), "  "+c)
				}
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.File)
			return nil
		},
	}
	view := &cobra.Command{
		Use:   "view",
		Short: "print the resolved configuration (flags, environment, file, defaults), secrets redacted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, _, err := load(cmd, f)
			if err != nil {
				return err
			}
			out := map[string]any{
				"listen_addr":      cfg.ListenAddr,
				"data_dir":         cfg.DataDir,
				"migrate_on_start": cfg.MigrateOnStart,
				"database":         map[string]string{"driver": string(cfg.DatabaseDriver), "url": redactURL(cfg.DatabaseURL)},
				"log":              map[string]string{"level": cfg.LogLevel.String(), "format": cfg.LogFormat},
				"secret_key":       redactSecret(cfg.SecretKey),
				"setup_token":      redactSecret(cfg.SetupToken),
				"payments":         map[string]bool{"sandbox": cfg.PaymentsSandbox},
			}
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent(2)
			return enc.Encode(out)
		},
	}
	for _, c := range []*cobra.Command{path, view} {
		c.Flags().String("listen-addr", "", "address to listen on")
		c.Flags().Bool("migrate-on-start", false, "apply pending migrations at startup")
		addDatabaseFlags(c)
	}
	cmd.AddCommand(path, view)
	return cmd
}

// redactURL hides a password in a connection URL. A SQLite path passes as is.
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return s
	}
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), "REDACTED")
		}
	}
	q := u.Query()
	if q.Has("password") {
		q.Set("password", "REDACTED")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// redactSecret shows only whether a secret is set.
func redactSecret(s string) string {
	if s == "" {
		return ""
	}
	return "REDACTED"
}

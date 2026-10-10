// Package cli implements the servly command line
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/servly-io/servly/internal/platform/config"
	"github.com/servly-io/servly/internal/platform/logging"
)

// Version is set at build time with -ldflags "-X .../internal/cli.Version=...".
var Version = "dev"

// Main runs a command and returns a process exit code.
func Main(args []string) int {
	root := newRootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	return 0
}

// flagKeys maps each configuration flag to its config key.
var flagKeys = map[string]string{
	"listen-addr":      config.KeyListenAddr,
	"data-dir":         config.KeyDataDir,
	"database-driver":  config.KeyDatabaseDriver,
	"database-url":     config.KeyDatabaseURL,
	"migrate-on-start": config.KeyMigrateOnStart,
	"log-level":        config.KeyLogLevel,
	"log-format":       config.KeyLogFormat,
}

// rootFlags are the flags every command accepts.
type rootFlags struct {
	config string
}

func newRootCommand() *cobra.Command {
	f := &rootFlags{}
	root := &cobra.Command{
		Use:           "servly",
		Short:         "Restaurant order management: QR menu ordering, staff order board, menu administration",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetUsageTemplate(usageTemplate)

	pf := root.PersistentFlags()
	pf.StringVar(&f.config, "config", "", "config file (default: $SERVLY_CONFIG, ~/.config/servly-io/servly.yaml, /etc/servly-io/servly.yaml; none is fine)")
	pf.String("log-level", "", "debug, info, warn or error (default info)")
	pf.String("log-format", "", "json or text (default json)")

	root.AddCommand(
		newServeCommand(f),
		newMigrateCommand(f),
		newConfigCommand(f),
		newTenantCommand(f),
		newUserCommand(f),
		newVersionCommand(),
		newOptionsCommand(root),
	)
	root.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	return root
}

// addDatabaseFlags adds the flags of commands that open the database.
func addDatabaseFlags(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.String("data-dir", "", "directory for the SQLite database and uploaded assets (default ./data)")
	fl.String("database-driver", "", "sqlite or postgres (default sqlite)")
	fl.String("database-url", "", "PostgreSQL connection URL, or a SQLite file path overriding <data-dir>/servly.db")
}

// load resolves the configuration for cmd: its flags, the environment and
// the config file. It also returns the logger the configuration asks for.
func load(cmd *cobra.Command, f *rootFlags) (config.Config, *slog.Logger, error) {
	v := config.New()
	if err := bindFlags(v, cmd); err != nil {
		return config.Config{}, nil, err
	}
	cfg, err := config.Load(v, f.config)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, logging.New(cmd.ErrOrStderr(), cfg.LogFormat, cfg.LogLevel), nil
}

func bindFlags(v *viper.Viper, cmd *cobra.Command) error {
	for name, key := range flagKeys {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			if err := v.BindPFlag(key, fl); err != nil {
				return err
			}
		}
	}
	return nil
}

const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalNonPersistentFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
{{if .HasAvailableSubCommands}}
Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
Use "servly options" for a list of global command-line options (applies to all commands).
`

func newOptionsCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "options",
		Short: "print the list of flags inherited by all commands",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "The following options can be passed to any command:")
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprint(cmd.OutOrStdout(), root.PersistentFlags().FlagUsages())
		},
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), versionString())
		},
	}
}

func vcsRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return ""
}

func versionString() string {
	v := Version
	if rev := vcsRevision(); len(rev) >= 7 {
		v += " (" + rev[:7] + ")"
	}
	return "servly " + v
}

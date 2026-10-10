package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMigrateCommand(f *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "apply or inspect database schema migrations",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	up := &cobra.Command{
		Use:   "up",
		Short: "apply every pending migration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := load(cmd, f)
			if err != nil {
				return err
			}
			db, err := openDatabase(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer db.Close()
			state, err := db.Migrate(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "schema at version %d\n", state.Current)
			return nil
		},
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "print the applied and the latest schema version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, log, err := load(cmd, f)
			if err != nil {
				return err
			}
			db, err := openDatabase(cmd.Context(), cfg, log)
			if err != nil {
				return err
			}
			defer db.Close()
			state, err := db.MigrationState(cmd.Context())
			if err != nil {
				return err
			}
			pending := "up to date"
			if state.Pending() {
				pending = "migrations pending"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "current %d, latest %d: %s\n", state.Current, state.Latest, pending)
			return nil
		},
	}
	addDatabaseFlags(up)
	addDatabaseFlags(status)
	cmd.AddCommand(up, status)
	return cmd
}

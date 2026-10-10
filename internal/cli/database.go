package cli

import (
	"context"
	"log/slog"

	"github.com/servly-io/servly/internal/platform/config"
	"github.com/servly-io/servly/internal/platform/database"
)

func openDatabase(ctx context.Context, cfg config.Config, log *slog.Logger) (*database.DB, error) {
	if cfg.DatabaseDriver == config.DriverPostgres {
		return database.OpenPostgres(ctx, cfg.DatabaseURL, log)
	}
	return database.OpenSQLite(ctx, cfg.SQLitePath(), log)
}

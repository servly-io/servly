// Package app is the application layer: the use cases of servly. A use case
// opens a transaction, builds the stores it needs on it, applies the domain
// rules and records audit events. The HTTP layer and the CLI call use cases;
// they never open transactions themselves.
package app

import (
	"log/slog"

	"github.com/servly-io/servly/internal/platform/database"
)

// App holds what the use cases share.
type App struct {
	DB  *database.DB
	Log *slog.Logger
}

func New(db *database.DB, log *slog.Logger) *App { return &App{DB: db, Log: log} }

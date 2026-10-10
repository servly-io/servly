// Package app is the application layer: the use cases of servly. A use case
// opens a transaction, builds the stores it needs on it, applies the domain
// rules and records audit events. The HTTP layer and the CLI call use cases;
// they never open transactions themselves.
package app

import (
	"log/slog"

	"github.com/servly-io/servly/internal/notifications"
	"github.com/servly-io/servly/internal/payments/provider"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/secretbox"
	"github.com/servly-io/servly/internal/storage"
)

// App holds what the use cases share.
type App struct {
	DB  *database.DB
	Log *slog.Logger
	// Files stores uploaded assets; nil disables uploads.
	Files *storage.Local
	// Events receives order changes after commit, for the staff SSE stream.
	Events *notifications.Broker
	// Secrets seals payment provider credentials; nil disables online payments.
	Secrets *secretbox.Box
	// Providers are the payment providers this server offers, by id.
	Providers map[string]provider.Provider
	// SetupToken is the secret the first-run setup asks for (NewSetupToken);
	// empty keeps the setup in the browser closed.
	SetupToken string
}

func New(db *database.DB, log *slog.Logger) *App {
	return &App{DB: db, Log: log, Events: notifications.NewBroker()}
}

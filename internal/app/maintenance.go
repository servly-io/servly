package app

import (
	"context"
	"time"

	"github.com/servly-io/servly/internal/auth"
)

// RunMaintenance purges expired records every interval until ctx ends.
func (a *App) RunMaintenance(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		a.maintain(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) maintain(ctx context.Context, now time.Time) {
	if n, err := auth.NewSessions(a.DB).DeleteExpired(ctx, now); err != nil {
		if ctx.Err() == nil {
			a.Log.WarnContext(ctx, "purge expired sessions", "error", err)
		}
	} else if n > 0 {
		a.Log.InfoContext(ctx, "purged expired sessions", "count", n)
	}
}

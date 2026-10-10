package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

func healthz(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	}
}

// readyz is readiness: the database answers and the schema is current.
func readyz(log *slog.Logger, ready func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				log.WarnContext(r.Context(), "not ready", "error", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

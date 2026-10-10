// Package httpapi is the HTTP layer: routing, middleware, request decoding,
// JSON responses and errors, health endpoints and the embedded frontend.
// Business rules live in the domain packages and use cases in internal/app,
// never in handlers.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/servly-io/servly/internal/app"
)

// DefaultMaxBodyBytes caps every request body unless a route sets less.
const DefaultMaxBodyBytes = 1 << 20

// Deps are what the HTTP layer needs from the rest of the application.
type Deps struct {
	Log *slog.Logger
	// Ready reports whether the server can serve traffic: database reachable
	// and schema current. nil means always ready.
	Ready func(ctx context.Context) error
	// Frontend is the built web application (web/dist). nil serves a notice
	// that the frontend was not built.
	Frontend fs.FS
	Version  string

	// App serves the API. nil serves health and the frontend only.
	App *app.App
	// CookieSecure: "auto" (Secure on HTTPS requests), "true" or "false".
	CookieSecure string
	// TrustProxy believes X-Forwarded-For and X-Forwarded-Proto.
	TrustProxy bool
	// PublicBaseURL is the guest-facing address for QR codes; empty derives
	// it from the request.
	PublicBaseURL string
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Stop is closed when the server shuts down, ending long-lived SSE
	// streams so a graceful shutdown does not wait for them.
	Stop <-chan struct{}
}

type server struct {
	Deps
	loginPerIP    *limiter
	loginPerEmail *limiter
	public        publicLimits
}

// NewHandler returns the root handler of the server.
func NewHandler(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.CookieSecure == "" {
		d.CookieSecure = "auto"
	}
	s := &server{
		Deps:          d,
		loginPerIP:    newLimiter(3*time.Second, 20),
		loginPerEmail: newLimiter(12*time.Second, 5),
		public:        newPublicLimits(),
	}

	r := chi.NewRouter()
	r.Use(requestID)
	r.Use(requestLogger(d.Log))
	r.Use(recoverer(d.Log))
	r.Use(maxBody(DefaultMaxBodyBytes))

	r.Get("/healthz", healthz(d.Version))
	r.Get("/readyz", readyz(d.Log, d.Ready))

	r.Route("/api", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, errRouteNotFound) })
		r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusMethodNotAllowed, errorBody{errorDetail{
				Code: errMethodNotAllowed.Code, Message: errMethodNotAllowed.Message,
				RequestID: middleware.GetReqID(r.Context()),
			}})
		})
		if d.App == nil {
			return
		}
		r.Get("/setup", s.setupStatus)
		r.Post("/setup", s.completeSetup)
		r.Post("/setup/verify", s.verifySetupToken)
		r.Route("/auth", s.authRoutes)
		r.Route("/public", func(r chi.Router) {
			s.publicRoutes(r)
			s.paymentPublicRoutes(r)
		})
		r.Route("/payments", s.paymentProviderRoutes)
		r.Route("/admin", func(r chi.Router) {
			r.Use(s.authenticated, s.csrf, s.requireTenant)
			s.adminRoutes(r)
			// Unknown admin paths answer 401 to anonymous callers too, so
			// the route list cannot be probed without a session. (A catch-all
			// route, because chi runs no middleware for NotFound.)
			r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, errRouteNotFound) })
		})
	})

	r.Handle("/*", frontend(d.Frontend))
	return r
}

// adminRoutes registers the staff API; each feature adds its routes here.
func (s *server) adminRoutes(r chi.Router) {
	s.settingsRoutes(r)
	s.menuRoutes(r)
	s.diningRoutes(r)
	s.assetRoutes(r)
	s.orderRoutes(r)
	s.paymentAdminRoutes(r)
}

// NewServer wraps a handler in an http.Server with conservative timeouts.
// WriteTimeout stays 0: SSE responses are long-lived.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, s.Log, err)
}

func itoa(i int) string { return strconv.Itoa(i) }

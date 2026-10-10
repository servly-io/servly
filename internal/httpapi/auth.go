package httpapi

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/tenant"
)

const sessionCookie = "servly_session"

type ctxKey int

const principalKey ctxKey = iota

// principal returns the request's authenticated principal; only valid behind
// the authenticated middleware.
func principal(r *http.Request) *app.Principal {
	p, _ := r.Context().Value(principalKey).(*app.Principal)
	return p
}

func (s *server) authRoutes(r chi.Router) {
	r.Post("/login", s.login)
	r.Group(func(r chi.Router) {
		r.Use(s.authenticated, s.csrf)
		r.Get("/session", s.getSession)
		r.Post("/logout", s.logout)
		r.Put("/session/tenant", s.switchTenant)
	})
}

// authenticated resolves the session cookie into a principal or answers 401.
func (s *server) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			s.fail(w, r, auth.ErrUnauthenticated)
			return
		}
		p, err := s.App.Authenticate(r.Context(), c.Value, s.Now())
		if err != nil {
			if e, ok := asUnauthenticated(err); ok {
				s.clearCookie(w, r)
				s.fail(w, r, e)
				return
			}
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

func asUnauthenticated(err error) (error, bool) {
	if err == auth.ErrUnauthenticated {
		return err, true
	}
	return nil, false
}

// csrf requires the session's CSRF token on every state-changing request.
func (s *server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			got := r.Header.Get("X-CSRF-Token")
			want := principal(r).Session.CSRFToken
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
				s.fail(w, r, auth.ErrCSRF)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireTenant answers 403 when no restaurant is active in the session.
func (s *server) requireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal(r).Tenant == nil {
			s.fail(w, r, auth.ErrNoTenant)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// can wraps a handler with a permission check in the active tenant.
func (s *server) can(perm tenant.Permission, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !principal(r).Can(perm) {
			s.fail(w, r, errForbidden)
			return
		}
		h(w, r)
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	if !s.loginPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.loginPerIP)
		return
	}
	if !s.loginPerEmail.allow(strings.ToLower(strings.TrimSpace(req.Email))) {
		s.limited(w, r, s.loginPerEmail)
		return
	}
	p, token, err := s.App.Login(r.Context(), req.Email, req.Password, s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookie(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  p.Session.CreatedAt.Add(auth.AbsoluteTimeout),
	})
	writeJSON(w, http.StatusOK, sessionView(p))
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.App.Logout(r.Context(), principal(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	s.clearCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, sessionView(principal(r)))
}

func (s *server) switchTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID uuid.UUID `json:"tenant_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.App.SwitchTenant(r.Context(), principal(r), req.TenantID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sessionView(p))
}

func (s *server) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteLaxMode})
}

func (s *server) secureCookie(r *http.Request) bool {
	switch s.CookieSecure {
	case "true":
		return true
	case "false":
		return false
	}
	return s.isHTTPS(r)
}

func (s *server) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return s.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP is the address rate limits key on. Behind a trusted proxy it is
// the last X-Forwarded-For entry: the one the proxy itself appended.
func (s *server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Session JSON (docs/docs/api.md#get-apiauthsession).

type sessionUser struct {
	ID              uuid.UUID `json:"id"`
	Email           string    `json:"email"`
	DisplayName     string    `json:"display_name"`
	IsPlatformAdmin bool      `json:"is_platform_admin"`
}

type activeTenant struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Role        tenant.Role         `json:"role"`
	Permissions []tenant.Permission `json:"permissions"`
}

type membershipView struct {
	TenantID   uuid.UUID   `json:"tenant_id"`
	TenantName string      `json:"tenant_name"`
	Role       tenant.Role `json:"role"`
}

type sessionBody struct {
	User         sessionUser      `json:"user"`
	CSRFToken    string           `json:"csrf_token"`
	ActiveTenant *activeTenant    `json:"active_tenant"`
	Memberships  []membershipView `json:"memberships"`
	ExpiresAt    time.Time        `json:"expires_at"`
}

func sessionView(p *app.Principal) sessionBody {
	b := sessionBody{
		User:        sessionUser{ID: p.User.ID, Email: p.User.Email, DisplayName: p.User.DisplayName, IsPlatformAdmin: p.User.IsPlatformAdmin},
		CSRFToken:   p.Session.CSRFToken,
		Memberships: []membershipView{},
		ExpiresAt:   p.Session.ExpiresAt.Time,
	}
	for _, m := range p.Memberships {
		if m.TenantStatus == tenant.StatusActive {
			b.Memberships = append(b.Memberships, membershipView{TenantID: m.TenantID, TenantName: m.TenantName, Role: m.Role})
		}
	}
	if p.Tenant != nil {
		b.ActiveTenant = &activeTenant{ID: p.Tenant.TenantID, Name: p.Tenant.TenantName, Role: p.Tenant.Role,
			Permissions: tenant.PermissionsOf(p.Tenant.Role)}
	}
	return b
}

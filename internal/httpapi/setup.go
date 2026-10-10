package httpapi

import (
	"net/http"

	"github.com/servly-io/servly/internal/app"
)

// First-run setup (docs/docs/api.md#first-run-setup). Both routes are public:
// the setup token in the body is what authorizes the claim.

func (s *server) setupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := s.App.SetupRequired(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"required": required})
}

type setupRequest struct {
	SetupToken     string `json:"setup_token"`
	RestaurantName string `json:"restaurant_name"`
	Currency       string `json:"currency"`
	Timezone       string `json:"timezone"`
	OwnerName      string `json:"owner_name"`
	OwnerEmail     string `json:"owner_email"`
	OwnerPassword  string `json:"owner_password"`
}

func (s *server) completeSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	// The login limiter: guessing the token costs the same as guessing a password.
	if !s.loginPerIP.allow(s.clientIP(r)) {
		s.limited(w, r, s.loginPerIP)
		return
	}
	p, token, err := s.App.CompleteSetup(r.Context(), req.SetupToken, app.CreateTenantInput{
		Name: req.RestaurantName, Currency: req.Currency, Timezone: req.Timezone,
		OwnerName: req.OwnerName, OwnerEmail: req.OwnerEmail, OwnerPassword: req.OwnerPassword,
	}, s.Now())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, r, p, token)
	writeJSON(w, http.StatusCreated, sessionView(p))
}

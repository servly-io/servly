package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/dining"
	"github.com/servly-io/servly/internal/qrcode"
	"github.com/servly-io/servly/internal/tenant"
)

func (s *server) diningRoutes(r chi.Router) {
	manage := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermManageDiningLocations, h) }
	r.Get("/dining-locations", s.listDining)
	r.Post("/dining-locations", manage(s.createDining))
	r.Patch("/dining-locations/{id}", manage(s.updateDining))
	r.Post("/dining-locations/{id}/rotate-token", manage(s.rotateDining))
	r.Get("/dining-locations/{id}/qr", s.diningQR)
}

type diningJSON struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Kind        dining.Kind `json:"kind"`
	PublicToken string      `json:"public_token"`
	MenuURL     string      `json:"menu_url"`
	IsEnabled   bool        `json:"is_enabled"`
}

func (s *server) diningView(r *http.Request, l dining.Location) diningJSON {
	return diningJSON{ID: l.ID, Name: l.Name, Kind: l.Kind, PublicToken: l.PublicToken,
		MenuURL: s.publicBase(r) + dining.MenuPath(l.PublicToken), IsEnabled: l.IsEnabled}
}

// publicBase is the address guests reach this server at: the configured
// public base URL, else the scheme and host of the request.
func (s *server) publicBase(r *http.Request) string {
	if s.PublicBaseURL != "" {
		return s.PublicBaseURL
	}
	scheme := "http"
	if s.isHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *server) listDining(w http.ResponseWriter, r *http.Request) {
	ls, err := s.App.ListDiningLocations(r.Context(), principal(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]diningJSON, 0, len(ls))
	for _, l := range ls {
		out = append(out, s.diningView(r, l))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *server) createDining(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string      `json:"name"`
		Kind dining.Kind `json:"kind"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.App.CreateDiningLocation(r.Context(), principal(r), req.Name, req.Kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.diningView(r, *l))
}

func (s *server) updateDining(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Name      optional[string] `json:"name"`
		IsEnabled optional[bool]   `json:"is_enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	var in app.DiningLocationInput
	var e1, e2 error
	in.Name, e1 = req.Name.ptr()
	in.IsEnabled, e2 = req.IsEnabled.ptr()
	if err := firstErr(e1, e2); err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.App.UpdateDiningLocation(r.Context(), principal(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.diningView(r, *l))
}

func (s *server) rotateDining(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.App.UpdateDiningLocation(r.Context(), principal(r), id, app.DiningLocationInput{RotateToken: true})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.diningView(r, *l))
}

func (s *server) diningQR(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.App.GetDiningLocation(r.Context(), principal(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	url := s.publicBase(r) + dining.MenuPath(l.PublicToken)
	var body []byte
	ctype := "image/svg+xml"
	if r.URL.Query().Get("format") == "png" {
		ctype = "image/png"
		body, err = qrcode.PNG(url, 10)
	} else {
		body, err = qrcode.SVG(url)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(body)
}

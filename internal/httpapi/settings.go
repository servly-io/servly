package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/tenant"
)

func (s *server) settingsRoutes(r chi.Router) {
	r.Get("/settings", s.getSettings)
	r.Patch("/settings", s.can(tenant.PermManageSettings, s.updateSettings))
	staff := func(h http.HandlerFunc) http.HandlerFunc { return s.can(tenant.PermManageStaff, h) }
	r.Get("/staff", staff(s.listStaff))
	r.Post("/staff", staff(s.addStaff))
	r.Patch("/staff/{user_id}", staff(s.updateStaff))
	r.Delete("/staff/{user_id}", staff(s.removeStaff))
}

type settingsJSON struct {
	Name                string         `json:"name"`
	Currency            money.Currency `json:"currency"`
	Timezone            string         `json:"timezone"`
	OrderNotesEnabled   bool           `json:"order_notes_enabled"`
	PublicStatusEnabled bool           `json:"public_status_enabled"`
	AcceptingOrders     bool           `json:"accepting_orders"`
	// MaxImageBytes is the upload limit for menu photos (read-only, server config).
	MaxImageBytes int64 `json:"max_image_bytes"`
}

func (s *server) settingsView(l *tenant.Location) settingsJSON {
	return settingsJSON{Name: l.Name, Currency: l.Currency, Timezone: l.Timezone,
		OrderNotesEnabled: l.OrderNotesEnabled, PublicStatusEnabled: l.PublicStatusEnabled,
		AcceptingOrders: l.AcceptingOrders, MaxImageBytes: s.App.ImageLimit()}
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	l, err := s.App.Settings(r.Context(), principal(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.settingsView(l))
}

func (s *server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                optional[string] `json:"name"`
		Currency            optional[string] `json:"currency"`
		Timezone            optional[string] `json:"timezone"`
		OrderNotesEnabled   optional[bool]   `json:"order_notes_enabled"`
		PublicStatusEnabled optional[bool]   `json:"public_status_enabled"`
		AcceptingOrders     optional[bool]   `json:"accepting_orders"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	var in app.SettingsInput
	var errs [6]error
	in.Name, errs[0] = req.Name.ptr()
	in.Currency, errs[1] = req.Currency.ptr()
	in.Timezone, errs[2] = req.Timezone.ptr()
	in.OrderNotesEnabled, errs[3] = req.OrderNotesEnabled.ptr()
	in.PublicStatusEnabled, errs[4] = req.PublicStatusEnabled.ptr()
	in.AcceptingOrders, errs[5] = req.AcceptingOrders.ptr()
	if err := firstErr(errs[:]...); err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.App.UpdateSettings(r.Context(), principal(r), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.settingsView(l))
}

type staffJSON struct {
	UserID      uuid.UUID         `json:"user_id"`
	Email       string            `json:"email"`
	DisplayName string            `json:"display_name"`
	Role        tenant.Role       `json:"role"`
	Status      tenant.UserStatus `json:"status"`
}

func staffView(m tenant.Member) staffJSON {
	return staffJSON{UserID: m.UserID, Email: m.Email, DisplayName: m.DisplayName, Role: m.Role, Status: m.UserStatus}
}

func (s *server) listStaff(w http.ResponseWriter, r *http.Request) {
	ms, err := s.App.ListStaff(r.Context(), principal(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]staffJSON, 0, len(ms))
	for _, m := range ms {
		out = append(out, staffView(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *server) addStaff(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string      `json:"email"`
		DisplayName string      `json:"display_name"`
		Role        tenant.Role `json:"role"`
		Password    string      `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.App.AddStaff(r.Context(), principal(r), app.AddStaffInput{Email: req.Email, DisplayName: req.DisplayName, Role: req.Role, Password: req.Password})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, staffView(*m))
}

func (s *server) updateStaff(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "user_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var req struct {
		Role     optional[tenant.Role] `json:"role"`
		Password optional[string]      `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.fail(w, r, err)
		return
	}
	var in app.UpdateStaffInput
	var e1, e2 error
	in.Role, e1 = req.Role.ptr()
	in.Password, e2 = req.Password.ptr()
	if err := firstErr(e1, e2); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.App.UpdateStaff(r.Context(), principal(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, staffView(*m))
}

func (s *server) removeStaff(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "user_id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.App.RemoveStaff(r.Context(), principal(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

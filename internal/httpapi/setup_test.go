package httpapi

import (
	"net/http"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

func TestFirstRunSetup(t *testing.T) {
	db := dbtest.Open(t, database.SQLite)
	e := &env{t: t, db: db, app: app.New(db, quiet), now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	e.app.SetupToken = "0a1b2-c3d4e-5f6a7-b8c9d"
	e.h = NewHandler(Deps{Log: quiet, App: e.app, Now: func() time.Time { return e.now }})
	c := e.client()

	if r := c.do(http.MethodGet, "/api/setup", nil); r.Code != http.StatusOK || r.json(t)["required"] != true {
		t.Fatalf("status: %d %s", r.Code, r.Body.String())
	}
	if r := c.do(http.MethodPost, "/api/setup/verify", map[string]string{"setup_token": "wrong"}); r.Code != http.StatusForbidden || r.code() != "setup_token_invalid" {
		t.Fatalf("verify wrong token: %d %s", r.Code, r.Body.String())
	}
	if r := c.do(http.MethodPost, "/api/setup/verify", map[string]string{"setup_token": " 0A1B2-C3D4E-5F6A7-B8C9D "}); r.Code != http.StatusNoContent {
		t.Fatalf("verify right token: %d %s", r.Code, r.Body.String())
	}
	body := map[string]string{
		"setup_token": "wrong", "restaurant_name": "Trattoria Roma", "currency": "EUR", "timezone": "Europe/Berlin",
		"owner_name": "Anna", "owner_email": "anna@example.com", "owner_password": annaPW,
	}
	if r := c.do(http.MethodPost, "/api/setup", body); r.Code != http.StatusForbidden || r.code() != "setup_token_invalid" {
		t.Fatalf("wrong token: %d %s", r.Code, r.Body.String())
	}

	body["setup_token"] = e.app.SetupToken
	r := c.do(http.MethodPost, "/api/setup", body)
	if r.Code != http.StatusCreated || c.cookie == nil {
		t.Fatalf("setup: %d %s", r.Code, r.Body.String())
	}
	// The owner is signed in, with the new restaurant active.
	c.csrf, _ = r.json(t)["csrf_token"].(string)
	if r := c.do(http.MethodPatch, "/api/admin/settings", map[string]bool{"order_notes_enabled": false}); r.Code != http.StatusOK {
		t.Fatalf("settings after setup: %d %s", r.Code, r.Body.String())
	}

	if r := c.do(http.MethodGet, "/api/setup", nil); r.json(t)["required"] != false {
		t.Fatalf("status after setup: %s", r.Body.String())
	}
	if r := e.client().do(http.MethodPost, "/api/setup", body); r.Code != http.StatusConflict || r.code() != "setup_complete" {
		t.Fatalf("second setup: %d %s", r.Code, r.Body.String())
	}
	if r := e.client().do(http.MethodPost, "/api/setup/verify", map[string]string{"setup_token": e.app.SetupToken}); r.Code != http.StatusConflict || r.code() != "setup_complete" {
		t.Fatalf("verify after setup: %d %s", r.Code, r.Body.String())
	}
}

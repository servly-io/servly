package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

func TestSettings(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	s := mustCode(t, c.do(http.MethodGet, "/api/admin/settings", nil), http.StatusOK)
	if s["name"] != "Roma" || s["currency"] != "EUR" || s["timezone"] != "Europe/Berlin" || s["order_notes_enabled"] != true {
		t.Fatalf("settings: %v", s)
	}
	s = mustCode(t, c.do(http.MethodPatch, "/api/admin/settings", map[string]any{
		"name": "Trattoria Roma", "timezone": "Europe/Rome", "order_notes_enabled": false, "currency": "CHF"}), http.StatusOK)
	if s["name"] != "Trattoria Roma" || s["timezone"] != "Europe/Rome" || s["order_notes_enabled"] != false || s["currency"] != "CHF" {
		t.Fatalf("patched: %v", s)
	}
	if at := mustCode(t, c.do(http.MethodGet, "/api/auth/session", nil), http.StatusOK)["active_tenant"].(map[string]any); at["name"] != "Trattoria Roma" {
		t.Fatalf("tenant name not updated: %v", at)
	}
	for body, code := range map[string]string{`{"timezone":"Mars/Base"}`: "invalid_timezone", `{"currency":"eur"}`: "invalid_currency", `{"name":"  "}`: "name_required"} {
		r := c.doRaw(http.MethodPatch, "/api/admin/settings", body)
		if r.Code != http.StatusUnprocessableEntity || r.code() != code {
			t.Errorf("%s: %d %s", body, r.Code, r.Body.String())
		}
	}
	// The currency is fixed once orders exist.
	referenceItemless(t, e)
	if r := c.do(http.MethodPatch, "/api/admin/settings", map[string]any{"currency": "EUR"}); r.Code != http.StatusConflict || r.code() != "currency_fixed" {
		t.Fatalf("currency change with orders: %d %s", r.Code, r.Body.String())
	}
}

func referenceItemless(t *testing.T, e *env) {
	t.Helper()
	cat := mustCode(t, e.owner().do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "C"}), http.StatusCreated)
	item := mustCode(t, e.owner().do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": cat["id"], "name": "I", "price_minor": 1}), http.StatusCreated)
	referenceItem(t, e, item["id"].(string))
}

func TestStaffManagement(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db)
		owner := e.owner()

		bob := mustCode(t, owner.do(http.MethodPost, "/api/admin/staff", map[string]any{
			"email": "Bob@Example.com", "display_name": "Bob", "role": "staff", "password": "bob's long password"}), http.StatusCreated)
		if bob["email"] != "bob@example.com" || bob["role"] != "staff" {
			t.Fatalf("added: %v", bob)
		}
		bc := e.client()
		at := mustCode(t, bc.login("bob@example.com", "bob's long password"), http.StatusOK)["active_tenant"].(map[string]any)
		if at["role"] != "staff" {
			t.Fatalf("bob's session: %v", at)
		}
		// Staff can read the menu and settings, not change them or manage staff.
		mustCode(t, bc.do(http.MethodGet, "/api/admin/menu/items", nil), http.StatusOK)
		mustCode(t, bc.do(http.MethodGet, "/api/admin/settings", nil), http.StatusOK)
		for _, req := range []struct{ m, p string }{
			{http.MethodPost, "/api/admin/menu/categories"}, {http.MethodPatch, "/api/admin/settings"},
			{http.MethodGet, "/api/admin/staff"}, {http.MethodPost, "/api/admin/staff"},
		} {
			if r := bc.do(req.m, req.p, map[string]any{"name": "x"}); r.Code != http.StatusForbidden || r.code() != "forbidden" {
				t.Errorf("staff %s %s: %d %s", req.m, req.p, r.Code, r.Body.String())
			}
		}

		// Managers manage the menu but not settings or staff.
		bobPath := "/api/admin/staff/" + bob["user_id"].(string)
		mustCode(t, owner.do(http.MethodPatch, bobPath, map[string]any{"role": "manager"}), http.StatusOK)
		mustCode(t, bc.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pasta"}), http.StatusCreated)
		mustCode(t, bc.do(http.MethodPatch, "/api/admin/settings", map[string]any{"name": "x"}), http.StatusForbidden)

		// The last owner can be neither demoted nor removed.
		annaPath := "/api/admin/staff/" + e.roma.Owner.ID.String()
		if r := owner.do(http.MethodPatch, annaPath, map[string]any{"role": "staff"}); r.Code != http.StatusConflict || r.code() != "last_owner" {
			t.Fatalf("demote last owner: %d %s", r.Code, r.Body.String())
		}
		mustCode(t, owner.do(http.MethodDelete, annaPath, nil), http.StatusConflict)

		// An existing user from another restaurant joins without a password;
		// supplying one is refused, and so is resetting theirs.
		if _, err := e.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "carla@example.com", OwnerPassword: "carla's long pw"}); err != nil {
			t.Fatal(err)
		}
		if r := owner.do(http.MethodPost, "/api/admin/staff", map[string]any{"email": "carla@example.com", "role": "staff", "password": "takeover attempt"}); r.Code != http.StatusUnprocessableEntity || r.code() != "existing_user" {
			t.Fatalf("existing user with password: %d %s", r.Code, r.Body.String())
		}
		carla := mustCode(t, owner.do(http.MethodPost, "/api/admin/staff", map[string]any{"email": "carla@example.com", "role": "staff"}), http.StatusCreated)
		carlaPath := "/api/admin/staff/" + carla["user_id"].(string)
		if r := owner.do(http.MethodPatch, carlaPath, map[string]any{"password": "takeover attempt"}); r.Code != http.StatusUnprocessableEntity || r.code() != "shared_user" {
			t.Fatalf("reset shared user's password: %d %s", r.Code, r.Body.String())
		}
		mustCode(t, owner.do(http.MethodPost, "/api/admin/staff", map[string]any{"email": "carla@example.com", "role": "staff"}), http.StatusConflict)

		// Resetting Bob's password (he works only here) ends his sessions.
		mustCode(t, owner.do(http.MethodPatch, bobPath, map[string]any{"password": "bob's new password"}), http.StatusOK)
		mustCode(t, bc.do(http.MethodGet, "/api/auth/session", nil), http.StatusUnauthorized)
		mustCode(t, bc.login("bob@example.com", "bob's new password"), http.StatusOK)

		// Removing Bob takes effect at once.
		mustCode(t, owner.do(http.MethodDelete, bobPath, nil), http.StatusNoContent)
		if r := bc.do(http.MethodGet, "/api/admin/menu/items", nil); r.Code != http.StatusForbidden || r.code() != "no_active_tenant" {
			t.Fatalf("removed member: %d %s", r.Code, r.Body.String())
		}
		list := mustCode(t, owner.do(http.MethodGet, "/api/admin/staff", nil), http.StatusOK)
		if len(list["items"].([]any)) != 2 { // anna, carla
			t.Fatalf("staff: %v", list)
		}
	})
}

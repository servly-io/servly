package httpapi

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

func TestDiningLocations(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db, func(d *Deps) { d.PublicBaseURL = "http://192.168.1.20:8080" })
		c := e.owner()

		t4 := mustCode(t, c.do(http.MethodPost, "/api/admin/dining-locations", map[string]any{"name": "Table 4", "kind": "table"}), http.StatusCreated)
		mustCode(t, c.do(http.MethodPost, "/api/admin/dining-locations", map[string]any{"name": "Pickup", "kind": "pickup"}), http.StatusCreated)
		token := t4["public_token"].(string)
		if !ids.IsPublicToken(token) || t4["menu_url"] != "http://192.168.1.20:8080/m/"+token || t4["is_enabled"] != true {
			t.Fatalf("created: %v", t4)
		}
		for _, bad := range []map[string]any{{"name": "x", "kind": "bar"}, {"name": " ", "kind": "table"}, {"name": strings.Repeat("x", 61), "kind": "table"}} {
			if r := c.do(http.MethodPost, "/api/admin/dining-locations", bad); r.Code != http.StatusUnprocessableEntity {
				t.Errorf("%v: %d", bad, r.Code)
			}
		}

		path := "/api/admin/dining-locations/" + t4["id"].(string)
		upd := mustCode(t, c.do(http.MethodPatch, path, map[string]any{"name": "Terrace 4", "is_enabled": false}), http.StatusOK)
		if upd["name"] != "Terrace 4" || upd["is_enabled"] != false || upd["public_token"] != token {
			t.Fatalf("updated: %v", upd)
		}
		rot := mustCode(t, c.do(http.MethodPost, path+"/rotate-token", nil), http.StatusOK)
		if rot["public_token"] == token || !ids.IsPublicToken(rot["public_token"].(string)) {
			t.Fatalf("rotate: %v", rot)
		}

		svg := c.do(http.MethodGet, path+"/qr", nil)
		if svg.Code != http.StatusOK || svg.Header().Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(svg.Body.String(), "<svg") {
			t.Fatalf("svg: %d %s", svg.Code, svg.Header())
		}
		png := c.do(http.MethodGet, path+"/qr?format=png", nil)
		if png.Code != http.StatusOK || png.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(png.Body.String(), "\x89PNG") {
			t.Fatalf("png: %d %s", png.Code, png.Header())
		}

		list := mustCode(t, c.do(http.MethodGet, "/api/admin/dining-locations", nil), http.StatusOK)
		if got := names(t, list); len(got) != 2 || got[0] != "Terrace 4" || got[1] != "Pickup" {
			t.Fatalf("list: %v", got)
		}

		// Isolation: another restaurant sees none of it.
		if _, err := e.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"}); err != nil {
			t.Fatal(err)
		}
		m := e.client()
		mustCode(t, m.login("mallory@example.com", "another long pw"), http.StatusOK)
		mustCode(t, m.do(http.MethodGet, path+"/qr", nil), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPatch, path, map[string]any{"name": "x"}), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPost, path+"/rotate-token", nil), http.StatusNotFound)
		if got := names(t, mustCode(t, m.do(http.MethodGet, "/api/admin/dining-locations", nil), http.StatusOK)); len(got) != 0 {
			t.Fatalf("other tenant lists: %v", got)
		}
	})
}

func TestQRUsesRequestHostWithoutPublicBaseURL(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	l := c.do(http.MethodPost, "/api/admin/dining-locations", map[string]any{"name": "T1", "kind": "table"}, "Host", "roma.local:8080")
	// httptest requests carry Host example.com; the header set above does
	// not change r.Host, so check the default.
	if u := l.json(t)["menu_url"].(string); !strings.HasPrefix(u, "http://example.com/m/") {
		t.Fatalf("menu_url: %s", u)
	}
}

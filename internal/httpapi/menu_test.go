package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

func mustCode(t *testing.T, r resp, want int) map[string]any {
	t.Helper()
	if r.Code != want {
		t.Fatalf("status %d, want %d: %s", r.Code, want, r.Body.String())
	}
	if r.Code == http.StatusNoContent {
		return nil
	}
	return r.json(t)
}

func names(t *testing.T, body map[string]any) []string {
	t.Helper()
	var out []string
	for _, x := range body["items"].([]any) {
		out = append(out, x.(map[string]any)["name"].(string))
	}
	return out
}

func TestMenuAdministration(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db)
		c := e.owner()

		pizza := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pizza"}), http.StatusCreated)
		drinks := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Drinks", "description": "cold"}), http.StatusCreated)
		mustCode(t, c.do(http.MethodPatch, "/api/admin/menu/categories/"+drinks["id"].(string), map[string]any{"position": 0}), http.StatusOK)
		if got := names(t, mustCode(t, c.do(http.MethodGet, "/api/admin/menu/categories", nil), http.StatusOK)); len(got) != 2 || got[0] != "Drinks" {
			t.Fatalf("categories after move: %v", got)
		}

		item := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{
			"category_id": pizza["id"], "name": " Margherita ", "description": "Tomato, mozzarella", "price_minor": 950,
			"option_groups": []any{
				map[string]any{"name": "Size", "min_select": 1, "max_select": 1, "options": []any{
					map[string]any{"name": "Normal", "price_delta_minor": 0},
					map[string]any{"name": "Large", "price_delta_minor": 300},
				}},
				map[string]any{"name": "Extras", "min_select": 0, "max_select": 3, "options": []any{
					map[string]any{"name": "Basil", "price_delta_minor": 50, "is_available": false},
				}},
			},
		}), http.StatusCreated)
		if item["name"] != "Margherita" || item["price_minor"].(float64) != 950 || item["is_enabled"] != true {
			t.Fatalf("created item: %v", item)
		}
		groups := item["option_groups"].([]any)
		size := groups[0].(map[string]any)
		sizeOpts := size["options"].([]any)
		basil := groups[1].(map[string]any)["options"].([]any)[0].(map[string]any)
		if len(groups) != 2 || len(sizeOpts) != 2 || basil["is_available"] != false {
			t.Fatalf("groups: %v", groups)
		}
		itemPath := "/api/admin/menu/items/" + item["id"].(string)

		// Edit: keep Size (rename Large), drop Normal and the Extras group,
		// add a new option. Ids of kept entries stay stable.
		large := sizeOpts[1].(map[string]any)
		upd := mustCode(t, c.do(http.MethodPatch, itemPath, map[string]any{
			"price_minor": 1000,
			"option_groups": []any{map[string]any{"id": size["id"], "name": "Size", "min_select": 1, "max_select": 1, "options": []any{
				map[string]any{"id": large["id"], "name": "XL", "price_delta_minor": 400},
				map[string]any{"name": "Kids", "price_delta_minor": -200},
			}}},
		}), http.StatusOK)
		ug := upd["option_groups"].([]any)
		uo := ug[0].(map[string]any)["options"].([]any)
		if upd["price_minor"].(float64) != 1000 || len(ug) != 1 || len(uo) != 2 || uo[0].(map[string]any)["id"] != large["id"] ||
			uo[0].(map[string]any)["name"] != "XL" || uo[1].(map[string]any)["price_delta_minor"].(float64) != -200 {
			t.Fatalf("updated item: %v", upd)
		}
		// Removed options are archived, not deleted: past orders keep their reference.
		var archived int
		if err := db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM options WHERE tenant_id = $1 AND archived_at IS NOT NULL`, e.roma.Tenant.ID).Scan(&archived); err != nil {
			t.Fatal(err)
		}
		if archived != 2 { // Normal and Basil
			t.Fatalf("archived options: %d", archived)
		}

		// An option id that is not in this group cannot be smuggled in.
		r := c.do(http.MethodPatch, itemPath, map[string]any{"option_groups": []any{map[string]any{"name": "G", "min_select": 0, "max_select": 1,
			"options": []any{map[string]any{"id": basil["id"], "name": "x"}}}}})
		mustCode(t, r, http.StatusNotFound)

		dup := mustCode(t, c.do(http.MethodPost, itemPath+"/duplicate", nil), http.StatusCreated)
		if dup["name"] != "Margherita (copy)" || dup["is_enabled"] != false || dup["id"] == item["id"] ||
			dup["option_groups"].([]any)[0].(map[string]any)["id"] == size["id"] {
			t.Fatalf("duplicate: %v", dup)
		}
		list := mustCode(t, c.do(http.MethodGet, "/api/admin/menu/items?category_id="+pizza["id"].(string), nil), http.StatusOK)
		if got := names(t, list); len(got) != 2 || got[0] != "Margherita" || got[1] != "Margherita (copy)" {
			t.Fatalf("items: %v", got)
		}
		if got := names(t, mustCode(t, c.do(http.MethodGet, "/api/admin/menu/items?category_id="+drinks["id"].(string), nil), http.StatusOK)); len(got) != 0 {
			t.Fatalf("drinks: %v", got)
		}

		// Delete: unreferenced items go, referenced items are archived.
		mustCode(t, c.do(http.MethodDelete, "/api/admin/menu/items/"+dup["id"].(string), nil), http.StatusNoContent)
		referenceItem(t, e, item["id"].(string))
		if body := mustCode(t, c.do(http.MethodDelete, itemPath, nil), http.StatusOK); body["archived"] != true {
			t.Fatalf("delete referenced: %v", body)
		}
		mustCode(t, c.do(http.MethodGet, itemPath, nil), http.StatusNotFound)
		mustCode(t, c.do(http.MethodPatch, itemPath, map[string]any{"name": "zombie"}), http.StatusNotFound)
		if got := names(t, mustCode(t, c.do(http.MethodGet, "/api/admin/menu/items", nil), http.StatusOK)); len(got) != 0 {
			t.Fatalf("items after delete: %v", got)
		}
	})
}

// referenceItem makes an order line point at an item, as a placed order would.
func referenceItem(t *testing.T, e *env, itemID string) {
	t.Helper()
	ctx := context.Background()
	now := database.Now()
	tid, lid, oid := e.roma.Tenant.ID, e.roma.Location.ID, ids.New()
	if _, err := e.db.ExecContext(ctx, `INSERT INTO orders (id, tenant_id, location_id, order_number, business_date, public_token, mode,
		status, payment_status, currency, subtotal_minor, total_minor, created_at, updated_at)
		VALUES ($1, $2, $3, 1, '2026-10-10', $4, 'pickup', 'new', 'unpaid', 'EUR', 950, 950, $5, $5)`,
		oid, tid, lid, ids.PublicToken(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.ExecContext(ctx, `INSERT INTO order_items (id, tenant_id, order_id, menu_item_id, position, name, quantity,
		unit_price_minor, options_total_minor, line_total_minor) VALUES ($1, $2, $3, $4, 0, 'Margherita', 1, 950, 0, 950)`,
		ids.New(), tid, oid, itemID); err != nil {
		t.Fatal(err)
	}
}

func TestMenuValidation(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	cat := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pizza"}), http.StatusCreated)
	cases := []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"name": "x", "price_minor": 100}, "category_required"},
		{map[string]any{"category_id": ids.New(), "name": "x", "price_minor": 100}, "category_required"},
		{map[string]any{"category_id": cat["id"], "name": " ", "price_minor": 100}, "name_required"},
		{map[string]any{"category_id": cat["id"], "name": "x", "price_minor": -1}, "invalid_price"},
		{map[string]any{"category_id": cat["id"], "name": "x", "price_minor": 1, "option_groups": []any{
			map[string]any{"name": "g", "min_select": 2, "max_select": 1}}}, "invalid_option_group"},
		{map[string]any{"category_id": cat["id"], "name": "x", "price_minor": 9.5}, "malformed_json"},
		{map[string]any{"category_id": cat["id"], "name": "x", "price_minor": 1, "secret": true}, "malformed_json"},
		{map[string]any{"category_id": cat["id"], "name": nil, "price_minor": 1}, "malformed_json"},
	}
	for _, tc := range cases {
		r := c.do(http.MethodPost, "/api/admin/menu/items", tc.body)
		if r.Code != http.StatusUnprocessableEntity || r.code() != tc.code {
			t.Errorf("%v: %d %s, want %s", tc.body, r.Code, r.Body.String(), tc.code)
		}
	}
	c.csrf = ""
	if r := c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "x"}); r.Code != http.StatusForbidden {
		t.Errorf("no csrf: %d", r.Code)
	}
}

// Another restaurant's owner cannot see or touch roma's menu, by id or by
// listing, and cannot attach items to roma's categories.
func TestMenuTenantIsolation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db)
		roma := e.owner()
		cat := mustCode(t, roma.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pizza"}), http.StatusCreated)
		item := mustCode(t, roma.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": cat["id"], "name": "Margherita", "price_minor": 950}), http.StatusCreated)

		if _, err := e.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"}); err != nil {
			t.Fatal(err)
		}
		m := e.client()
		mustCode(t, m.login("mallory@example.com", "another long pw"), http.StatusOK)

		itemPath := "/api/admin/menu/items/" + item["id"].(string)
		mustCode(t, m.do(http.MethodGet, itemPath, nil), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPatch, itemPath, map[string]any{"price_minor": 1}), http.StatusNotFound)
		mustCode(t, m.do(http.MethodDelete, itemPath, nil), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPost, itemPath+"/duplicate", nil), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPatch, "/api/admin/menu/categories/"+cat["id"].(string), map[string]any{"name": "x"}), http.StatusNotFound)
		r := m.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": cat["id"], "name": "Trojan", "price_minor": 1})
		if r.Code != http.StatusUnprocessableEntity || r.code() != "category_required" {
			t.Fatalf("item in foreign category: %d %s", r.Code, r.Body.String())
		}
		if got := names(t, mustCode(t, m.do(http.MethodGet, "/api/admin/menu/items", nil), http.StatusOK)); len(got) != 0 {
			t.Fatalf("other tenant lists roma's items: %v", got)
		}
		if got := names(t, mustCode(t, m.do(http.MethodGet, "/api/admin/menu/categories", nil), http.StatusOK)); len(got) != 0 {
			t.Fatalf("other tenant lists roma's categories: %v", got)
		}

		// Roma's data is untouched.
		got := mustCode(t, roma.do(http.MethodGet, itemPath, nil), http.StatusOK)
		if got["price_minor"].(float64) != 950 || got["name"] != "Margherita" {
			t.Fatalf("roma's item changed: %v", got)
		}
	})
}

package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

// shop is roma with a menu and two dining locations.
type shop struct {
	*env
	owner                        *client
	pizza, cola, soldOut         map[string]any
	normal, large, basil         string
	table, pickup, hiddenItemCat string
	tableToken, pickupToken      string
	mu                           sync.Mutex
	guests                       int
}

func newShop(t *testing.T, db *database.DB) *shop {
	t.Helper()
	s := &shop{env: newEnv(t, db)}
	s.owner = s.env.owner()
	c := s.owner
	pizzas := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Pizza"}), http.StatusCreated)
	drinks := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Drinks"}), http.StatusCreated)
	hidden := mustCode(t, c.do(http.MethodPost, "/api/admin/menu/categories", map[string]any{"name": "Secret", "is_enabled": false}), http.StatusCreated)
	s.hiddenItemCat = hidden["id"].(string)
	s.pizza = mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{
		"category_id": pizzas["id"], "name": "Margherita", "price_minor": 950,
		"option_groups": []any{
			map[string]any{"name": "Size", "min_select": 1, "max_select": 1, "options": []any{
				map[string]any{"name": "Normal", "price_delta_minor": 0}, map[string]any{"name": "Large", "price_delta_minor": 300}}},
			map[string]any{"name": "Extras", "min_select": 0, "max_select": 2, "options": []any{
				map[string]any{"name": "Basil", "price_delta_minor": 50}}},
		}}), http.StatusCreated)
	g := s.pizza["option_groups"].([]any)
	s.normal = g[0].(map[string]any)["options"].([]any)[0].(map[string]any)["id"].(string)
	s.large = g[0].(map[string]any)["options"].([]any)[1].(map[string]any)["id"].(string)
	s.basil = g[1].(map[string]any)["options"].([]any)[0].(map[string]any)["id"].(string)
	s.cola = mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": drinks["id"], "name": "Cola", "price_minor": 300}), http.StatusCreated)
	s.soldOut = mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": drinks["id"], "name": "Lemonade", "price_minor": 350, "is_available": false}), http.StatusCreated)
	mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": hidden["id"], "name": "Hidden", "price_minor": 1}), http.StatusCreated)
	mustCode(t, c.do(http.MethodPost, "/api/admin/menu/items", map[string]any{"category_id": pizzas["id"], "name": "Draft", "price_minor": 1, "is_enabled": false}), http.StatusCreated)
	tbl := mustCode(t, c.do(http.MethodPost, "/api/admin/dining-locations", map[string]any{"name": "Table 4", "kind": "table"}), http.StatusCreated)
	pk := mustCode(t, c.do(http.MethodPost, "/api/admin/dining-locations", map[string]any{"name": "Counter", "kind": "pickup"}), http.StatusCreated)
	s.table, s.tableToken = tbl["id"].(string), tbl["public_token"].(string)
	s.pickup, s.pickupToken = pk["id"].(string), pk["public_token"].(string)
	return s
}

// order submits as a new guest (own address), so tests do not trip the
// per-address rate limit; orderFrom submits from a given address.
func (s *shop) order(key string, body map[string]any) resp {
	s.t.Helper()
	s.mu.Lock()
	s.guests++
	ip := fmt.Sprintf("10.0.%d.%d", s.guests/250, s.guests%250+1)
	s.mu.Unlock()
	return s.orderFrom(ip, key, body)
}

func (s *shop) orderFrom(ip, key string, body map[string]any) resp {
	s.t.Helper()
	c := s.client()
	c.ip = ip
	return c.do(http.MethodPost, "/api/public/orders", body, "Idempotency-Key", key)
}

func (s *shop) basicOrder() map[string]any {
	return map[string]any{
		"dining_location_token": s.tableToken,
		"notes":                 "by the window",
		"items": []any{
			map[string]any{"menu_item_id": s.pizza["id"], "quantity": 2, "option_ids": []any{s.large, s.basil}, "notes": "well done"},
			map[string]any{"menu_item_id": s.cola["id"], "quantity": 1},
		},
	}
}

func (s *shop) countOrders() int {
	s.t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM orders`).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

func TestPublicMenu(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		m := mustCode(t, s.client().do(http.MethodGet, "/api/public/menu/"+s.tableToken, nil), http.StatusOK)
		if r := m["restaurant"].(map[string]any); r["name"] != "Roma" || r["currency"] != "EUR" || r["order_notes_enabled"] != true {
			t.Fatalf("restaurant: %v", r)
		}
		if d := m["dining_location"].(map[string]any); d["name"] != "Table 4" || d["kind"] != "table" {
			t.Fatalf("dining: %v", d)
		}
		var got []string
		for _, c := range m["categories"].([]any) {
			for _, it := range c.(map[string]any)["items"].([]any) {
				i := it.(map[string]any)
				got = append(got, fmt.Sprintf("%s:%v", i["name"], i["available"]))
			}
		}
		if strings.Join(got, ",") != "Margherita:true,Cola:true,Lemonade:false" {
			t.Fatalf("items: %v", got)
		}
		if strings.Contains(s.client().do(http.MethodGet, "/api/public/menu/"+s.tableToken, nil).Body.String(), s.roma.Tenant.ID.String()) {
			t.Fatal("public menu reveals the tenant id")
		}

		for _, tok := range []string{ids.PublicToken(), "short", "../../etc/passwd"} {
			if r := s.client().do(http.MethodGet, "/api/public/menu/"+tok, nil); r.Code != http.StatusNotFound {
				t.Errorf("token %q: %d %s", tok, r.Code, r.Body.String())
			}
		}
		mustCode(t, s.owner.do(http.MethodPatch, "/api/admin/dining-locations/"+s.table, map[string]any{"is_enabled": false}), http.StatusOK)
		mustCode(t, s.client().do(http.MethodGet, "/api/public/menu/"+s.tableToken, nil), http.StatusNotFound)
		mustCode(t, s.order("key-disabled-table", s.basicOrder()), http.StatusNotFound)
	})
}

func TestPlaceOrder(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		o := mustCode(t, s.order("checkout-0001", s.basicOrder()), http.StatusCreated)
		// (950 + 300 + 50) × 2 + 300 = 2900, computed by the server.
		if o["number"].(float64) != 1 || o["status"] != "new" || o["payment_status"] != "unpaid" || o["mode"] != "dine_in" ||
			o["total_minor"].(float64) != 2900 || o["subtotal_minor"].(float64) != 2900 || o["currency"] != "EUR" ||
			o["dining_location"] != "Table 4" || o["notes"] != "by the window" || !ids.IsPublicToken(o["order_token"].(string)) {
			t.Fatalf("order: %v", o)
		}
		line := o["items"].([]any)[0].(map[string]any)
		if line["name"] != "Margherita" || line["unit_price_minor"].(float64) != 950 || line["options_total_minor"].(float64) != 350 ||
			line["line_total_minor"].(float64) != 2600 || len(line["options"].([]any)) != 2 || line["notes"] != "well done" {
			t.Fatalf("line: %v", line)
		}
		if strings.Contains(s.order("checkout-0001", s.basicOrder()).Body.String(), s.roma.Tenant.ID.String()) {
			t.Fatal("order response reveals the tenant id")
		}

		// Retry with the same key and body: the same order, nothing new.
		again := mustCode(t, s.order("checkout-0001", s.basicOrder()), http.StatusOK)
		if again["order_token"] != o["order_token"] || s.countOrders() != 1 {
			t.Fatalf("replay: %v, orders %d", again, s.countOrders())
		}
		// Same key, different order: refused.
		changed := s.basicOrder()
		changed["notes"] = "different"
		if r := s.order("checkout-0001", changed); r.Code != http.StatusConflict || r.code() != "idempotency_key_reused" {
			t.Fatalf("reused key: %d %s", r.Code, r.Body.String())
		}
		if r := s.order("", s.basicOrder()); r.Code != http.StatusUnprocessableEntity || r.code() != "idempotency_key_required" {
			t.Fatalf("no key: %d %s", r.Code, r.Body.String())
		}

		// A pickup point makes a pickup order, numbered on.
		p := s.basicOrder()
		p["dining_location_token"], p["notes"] = s.pickupToken, ""
		po := mustCode(t, s.order("checkout-0002", p), http.StatusCreated)
		if po["mode"] != "pickup" || po["number"].(float64) != 2 {
			t.Fatalf("pickup order: %v", po)
		}

		// The public status shows the order, and only with its token.
		st := mustCode(t, s.client().do(http.MethodGet, "/api/public/orders/"+o["order_token"].(string), nil), http.StatusOK)
		if st["number"].(float64) != 1 || st["total_minor"].(float64) != 2900 {
			t.Fatalf("status: %v", st)
		}
		mustCode(t, s.client().do(http.MethodGet, "/api/public/orders/"+ids.PublicToken(), nil), http.StatusNotFound)

		// Editing the menu never changes a placed order.
		mustCode(t, s.owner.do(http.MethodPatch, "/api/admin/menu/items/"+s.pizza["id"].(string), map[string]any{"price_minor": 1200, "name": "Margherita DOP"}), http.StatusOK)
		st = mustCode(t, s.client().do(http.MethodGet, "/api/public/orders/"+o["order_token"].(string), nil), http.StatusOK)
		if l := st["items"].([]any)[0].(map[string]any); l["name"] != "Margherita" || l["unit_price_minor"].(float64) != 950 || st["total_minor"].(float64) != 2900 {
			t.Fatalf("historical order changed: %v", st)
		}

		// Restaurants may hide the status page.
		mustCode(t, s.owner.do(http.MethodPatch, "/api/admin/settings", map[string]any{"public_status_enabled": false}), http.StatusOK)
		mustCode(t, s.client().do(http.MethodGet, "/api/public/orders/"+o["order_token"].(string), nil), http.StatusNotFound)
	})
}

func TestPlaceOrderRejectsAndRollsBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		bad := func(items ...any) map[string]any {
			return map[string]any{"dining_location_token": s.tableToken, "items": items}
		}
		pizza := func(qty int, opts ...any) map[string]any {
			return map[string]any{"menu_item_id": s.pizza["id"], "quantity": qty, "option_ids": opts}
		}
		cases := []struct {
			body map[string]any
			code string
		}{
			{bad(), "empty_order"},
			{bad(pizza(1)), "invalid_option_selection"},
			{bad(pizza(1, s.normal, s.large)), "invalid_option_selection"},
			{bad(pizza(0, s.normal)), "invalid_quantity"},
			{bad(pizza(100, s.normal)), "invalid_quantity"},
			{bad(map[string]any{"menu_item_id": s.soldOut["id"], "quantity": 1}), "item_unavailable"},
			{bad(map[string]any{"menu_item_id": ids.New(), "quantity": 1}), "item_unavailable"},
			// A valid first line, then a bad one: nothing may be stored.
			{bad(pizza(1, s.normal), map[string]any{"menu_item_id": s.cola["id"], "quantity": 1, "option_ids": []any{s.basil}}), "invalid_option_selection"},
			// Prices cannot be sent at all.
			{bad(map[string]any{"menu_item_id": s.cola["id"], "quantity": 1, "price_minor": 1}), "malformed_json"},
			{map[string]any{"dining_location_token": s.tableToken, "items": []any{pizza(1, s.normal)}, "total_minor": 1}, "malformed_json"},
		}
		for i, c := range cases {
			r := s.order(fmt.Sprintf("bad-order-%04d", i), c.body)
			if r.Code != http.StatusUnprocessableEntity || r.code() != c.code {
				t.Errorf("case %d: %d %s, want %s", i, r.Code, r.Body.String(), c.code)
			}
		}
		r := s.order("bad-order-line", bad(pizza(1, s.normal), map[string]any{"menu_item_id": s.soldOut["id"], "quantity": 1}))
		if d, _ := r.json(t)["error"].(map[string]any)["details"].(map[string]any); d["line"].(float64) != 1 {
			t.Fatalf("details: %v", d)
		}
		if n := s.countOrders(); n != 0 {
			t.Fatalf("rejected orders stored: %d", n)
		}
		// The failed attempts did not use up order numbers.
		if o := mustCode(t, s.order("good-order-0001", s.basicOrder()), http.StatusCreated); o["number"].(float64) != 1 {
			t.Fatalf("first good order number: %v", o["number"])
		}
		// Notes are refused when the restaurant turned them off.
		mustCode(t, s.owner.do(http.MethodPatch, "/api/admin/settings", map[string]any{"order_notes_enabled": false}), http.StatusOK)
		if r := s.order("notes-off-0001", s.basicOrder()); r.code() != "notes_disabled" {
			t.Fatalf("notes disabled: %d %s", r.Code, r.Body.String())
		}
	})
}

func TestOrderNumbersPerBusinessDay(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	// Roma is in Europe/Berlin (UTC+2 in October).
	s.now = time.Date(2026, 10, 10, 21, 30, 0, 0, time.UTC) // 23:30 local
	a := mustCode(t, s.order("day-one-0001", s.basicOrder()), http.StatusCreated)
	s.now = time.Date(2026, 10, 10, 22, 30, 0, 0, time.UTC) // 00:30 local, next day
	b := mustCode(t, s.order("day-two-0001", s.basicOrder()), http.StatusCreated)
	c := mustCode(t, s.order("day-two-0002", s.basicOrder()), http.StatusCreated)
	if a["number"].(float64) != 1 || b["number"].(float64) != 1 || c["number"].(float64) != 2 {
		t.Fatalf("numbers: %v %v %v", a["number"], b["number"], c["number"])
	}
	var dates []string
	rows, _ := s.db.QueryContext(context.Background(), `SELECT business_date FROM orders ORDER BY created_at`)
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		dates = append(dates, d)
	}
	rows.Close()
	if strings.Join(dates, ",") != "2026-10-10,2026-10-11,2026-10-11" {
		t.Fatalf("business dates: %v", dates)
	}
}

func TestConcurrentOrders(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		const n = 8
		var wg sync.WaitGroup
		codes := make([]int, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = s.order(fmt.Sprintf("concurrent-%04d", i), s.basicOrder()).Code
			}()
		}
		wg.Wait()
		for i, c := range codes {
			if c != http.StatusCreated {
				t.Fatalf("order %d: %d", i, c)
			}
		}
		var distinct, maxN int
		if err := db.QueryRowContext(context.Background(), `SELECT COUNT(DISTINCT order_number), MAX(order_number) FROM orders`).Scan(&distinct, &maxN); err != nil {
			t.Fatal(err)
		}
		if distinct != n || maxN != n {
			t.Fatalf("numbers: %d distinct, max %d", distinct, maxN)
		}
	})
}

func TestConcurrentSameKey(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		const n = 6
		var wg sync.WaitGroup
		tokens := make([]string, n)
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := s.order("double-click-0001", s.basicOrder())
				if r.Code == http.StatusCreated || r.Code == http.StatusOK {
					tokens[i] = r.json(t)["order_token"].(string)
				}
			}()
		}
		wg.Wait()
		for i := range tokens {
			if tokens[i] == "" || tokens[i] != tokens[0] {
				t.Fatalf("tokens: %v", tokens)
			}
		}
		if c := s.countOrders(); c != 1 {
			t.Fatalf("orders: %d", c)
		}
	})
}

func TestPublicLimits(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	big := strings.Repeat("x", publicMaxBodyBytes)
	r := s.client().doRaw(http.MethodPost, "/api/public/orders", `{"dining_location_token":"`+s.tableToken+`","notes":"`+big+`","items":[]}`)
	if r.Code != http.StatusRequestEntityTooLarge || r.code() != "request_too_large" {
		t.Fatalf("oversized: %d %s", r.Code, r.Body.String())
	}
	var last resp
	for i := range 11 {
		last = s.orderFrom("10.9.9.9", fmt.Sprintf("flood-%04d", i), s.basicOrder())
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("11th order from one address: %d", last.Code)
	}
}

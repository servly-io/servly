package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
)

// placed submits a basic order as a guest and returns the staff view of it.
func (s *shop) placed(t *testing.T, key string) map[string]any {
	t.Helper()
	mustCode(t, s.order(key, s.basicOrder()), http.StatusCreated)
	list := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?limit=1", nil), http.StatusOK)
	return list["items"].([]any)[0].(map[string]any)
}

func (s *shop) staffClient(t *testing.T, email, role string) *client {
	t.Helper()
	mustCode(t, s.owner.do(http.MethodPost, "/api/admin/staff", map[string]any{"email": email, "role": role, "password": "a long staff password"}), http.StatusCreated)
	c := s.client()
	mustCode(t, c.login(email, "a long staff password"), http.StatusOK)
	return c
}

func status(c *client, id string, to string, version float64, reason string) resp {
	return c.do(http.MethodPatch, "/api/admin/orders/"+id+"/status", map[string]any{"status": to, "version": version, "reason": reason})
}

func TestOrderLifecycle(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		staff := s.staffClient(t, "bob@example.com", "staff")
		o := s.placed(t, "life-0001")
		id := o["id"].(string)
		if o["status"] != "new" || o["dining_location"].(map[string]any)["name"] != "Table 4" || o["version"].(float64) != 1 {
			t.Fatalf("board order: %v", o)
		}

		if r := status(staff, id, "ready", 1, ""); r.Code != http.StatusUnprocessableEntity || r.code() != "invalid_transition" {
			t.Fatalf("skip to ready: %d %s", r.Code, r.Body.String())
		}
		v := 1.0
		for _, to := range []string{"accepted", "preparing", "ready", "completed"} {
			d := mustCode(t, status(staff, id, to, v, ""), http.StatusOK)
			if d["status"] != to || d["version"].(float64) != v+1 {
				t.Fatalf("to %s: %v", to, d)
			}
			v++
		}
		if r := status(staff, id, "cancelled", v, "too late"); r.code() != "invalid_transition" {
			t.Fatalf("cancel completed: %d %s", r.Code, r.Body.String())
		}
		d := mustCode(t, staff.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusOK)
		evs := d["events"].([]any)
		if len(evs) != 5 || evs[0].(map[string]any)["from"] != nil || evs[0].(map[string]any)["actor"] != nil {
			t.Fatalf("events: %v", evs)
		}
		last := evs[4].(map[string]any)
		if last["from"] != "ready" || last["to"] != "completed" || last["actor"].(map[string]any)["name"] != "bob" {
			t.Fatalf("last event: %v", last)
		}
	})
}

func TestCancellationRules(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		staff := s.staffClient(t, "bob@example.com", "staff")
		manager := s.staffClient(t, "mia@example.com", "manager")

		a := s.placed(t, "cancel-0001")
		if r := status(staff, a["id"].(string), "cancelled", 1, " "); r.code() != "reason_required" {
			t.Fatalf("no reason: %d %s", r.Code, r.Body.String())
		}
		mustCode(t, status(staff, a["id"].(string), "cancelled", 1, "guest left"), http.StatusOK)

		b := s.placed(t, "cancel-0002")
		id := b["id"].(string)
		mustCode(t, status(staff, id, "accepted", 1, ""), http.StatusOK)
		mustCode(t, status(staff, id, "preparing", 2, ""), http.StatusOK)
		if r := status(staff, id, "cancelled", 3, "burnt"); r.Code != http.StatusForbidden {
			t.Fatalf("staff late cancel: %d %s", r.Code, r.Body.String())
		}
		d := mustCode(t, status(manager, id, "cancelled", 3, "burnt"), http.StatusOK)
		if d["status"] != "cancelled" {
			t.Fatalf("manager cancel: %v", d)
		}
		var n int
		_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action = 'order.cancelled'`).Scan(&n)
		if n != 2 {
			t.Fatalf("cancel audit events: %d", n)
		}
	})
}

func TestConcurrentStatusChange(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		staff := []*client{s.staffClient(t, "a@example.com", "staff"), s.staffClient(t, "b@example.com", "staff"), s.owner}
		id := s.placed(t, "race-0001")["id"].(string)

		var wg sync.WaitGroup
		codes := make([]int, len(staff))
		for i, c := range staff {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = status(c, id, "accepted", 1, "").Code
			}()
		}
		wg.Wait()
		ok, conflict := 0, 0
		for _, c := range codes {
			switch c {
			case http.StatusOK:
				ok++
			case http.StatusConflict:
				conflict++
			}
		}
		if ok != 1 || conflict != len(staff)-1 {
			t.Fatalf("codes: %v", codes)
		}
		d := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusOK)
		if d["version"].(float64) != 2 || len(d["events"].([]any)) != 2 {
			t.Fatalf("after race: %v", d)
		}
	})
}

func TestPayments(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		staff := s.staffClient(t, "bob@example.com", "staff")
		manager := s.staffClient(t, "mia@example.com", "manager")
		o := s.placed(t, "pay-0001")
		id, total := o["id"].(string), o["total_minor"].(float64)
		pay := func(c *client, body map[string]any) resp {
			return c.do(http.MethodPost, "/api/admin/orders/"+id+"/payments", body)
		}

		for body, code := range map[string]string{
			`{"method":"cash","status":"paid","amount_minor":100}`:                                                 "invalid_payment_amount",
			`{"method":"card","status":"paid","amount_minor":2900}`:                                                "invalid_payment_method",
			`{"method":"cash","status":"refunded","amount_minor":2900}`:                                            "invalid_payment_status",
			`{"method":"external_terminal","status":"paid","amount_minor":2900,"reference":"4111 1111 1111 1111"}`: "reference_looks_like_card",
		} {
			if r := staff.doRaw(http.MethodPost, "/api/admin/orders/"+id+"/payments", body); r.code() != code {
				t.Errorf("%s: %d %s", body, r.Code, r.Body.String())
			}
		}

		// A pending terminal payment, then it fails, then cash.
		d := mustCode(t, pay(staff, map[string]any{"method": "external_terminal", "status": "pending", "amount_minor": total, "reference": "TID 4411"}), http.StatusOK)
		if d["payment_status"] != "pending" || d["status"] != "new" {
			t.Fatalf("pending: %v", d)
		}
		pid := d["payments"].([]any)[0].(map[string]any)["id"].(string)
		d = mustCode(t, staff.do(http.MethodPatch, "/api/admin/payments/"+pid, map[string]any{"status": "failed"}), http.StatusOK)
		if d["payment_status"] != "unpaid" || d["outstanding_minor"].(float64) != total {
			t.Fatalf("failed: %v", d)
		}
		if r := staff.do(http.MethodPatch, "/api/admin/payments/"+pid, map[string]any{"status": "paid"}); r.code() != "invalid_payment_transition" {
			t.Fatalf("settle failed payment: %d %s", r.Code, r.Body.String())
		}
		d = mustCode(t, pay(staff, map[string]any{"method": "cash", "status": "paid", "amount_minor": total}), http.StatusOK)
		if d["payment_status"] != "paid" || d["outstanding_minor"].(float64) != 0 || d["status"] != "new" {
			t.Fatalf("paid: %v", d)
		}
		if r := pay(staff, map[string]any{"method": "cash", "status": "paid", "amount_minor": total}); r.code() != "invalid_payment_amount" {
			t.Fatalf("pay twice: %d %s", r.Code, r.Body.String())
		}
		cash := d["payments"].([]any)[1].(map[string]any)
		if cash["recorded_by"].(map[string]any)["name"] != "bob" {
			t.Fatalf("recorded_by: %v", cash)
		}

		// Refunds need a manager, a reason and a valid amount.
		rp := "/api/admin/payments/" + cash["id"].(string) + "/refund"
		mustCode(t, staff.do(http.MethodPost, rp, map[string]any{"amount_minor": 100, "reason": "cold"}), http.StatusForbidden)
		if r := manager.do(http.MethodPost, rp, map[string]any{"amount_minor": 100}); r.code() != "reason_required" {
			t.Fatalf("refund without reason: %d %s", r.Code, r.Body.String())
		}
		if r := manager.do(http.MethodPost, rp, map[string]any{"amount_minor": total + 1, "reason": "x"}); r.code() != "invalid_refund_amount" {
			t.Fatalf("over-refund: %d %s", r.Code, r.Body.String())
		}
		d = mustCode(t, manager.do(http.MethodPost, rp, map[string]any{"amount_minor": 500, "reason": "cold pizza"}), http.StatusOK)
		if d["payment_status"] != "partially_refunded" {
			t.Fatalf("partial refund: %v", d)
		}
		d = mustCode(t, manager.do(http.MethodPost, rp, map[string]any{"amount_minor": total - 500, "reason": "rest"}), http.StatusOK)
		if d["payment_status"] != "refunded" {
			t.Fatalf("full refund: %v", d)
		}

		// Payment status stays independent of order status: complete an unpaid order.
		o2 := s.placed(t, "pay-0002")
		v := 1.0
		for _, to := range []string{"accepted", "preparing", "ready", "completed"} {
			mustCode(t, status(staff, o2["id"].(string), to, v, ""), http.StatusOK)
			v++
		}
		if d := mustCode(t, staff.do(http.MethodGet, "/api/admin/orders/"+o2["id"].(string), nil), http.StatusOK); d["status"] != "completed" || d["payment_status"] != "unpaid" {
			t.Fatalf("completed unpaid: %v", d)
		}

		var n int
		_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events WHERE action LIKE 'payment.%'`).Scan(&n)
		if n != 5 { // pending, failed, cash, 2 refunds
			t.Fatalf("payment audit events: %d", n)
		}
	})
}

func TestOrderListFilters(t *testing.T) {
	s := newShop(t, dbtest.Open(t, database.SQLite))
	for i := range 3 {
		s.now = s.now.Add(time.Minute)
		mustCode(t, s.order(fmt.Sprintf("list-%04d", i), s.basicOrder()), http.StatusCreated)
	}
	p := s.basicOrder()
	p["dining_location_token"], p["notes"] = s.pickupToken, ""
	s.now = s.now.Add(time.Minute)
	mustCode(t, s.order("list-pickup", p), http.StatusCreated)

	page := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?limit=3", nil), http.StatusOK)
	items := page["items"].([]any)
	if len(items) != 3 || items[0].(map[string]any)["number"].(float64) != 4 || page["next_cursor"] == nil {
		t.Fatalf("page 1: %v", page)
	}
	page2 := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?limit=3&cursor="+page["next_cursor"].(string), nil), http.StatusOK)
	if items := page2["items"].([]any); len(items) != 1 || items[0].(map[string]any)["number"].(float64) != 1 || page2["next_cursor"] != nil {
		t.Fatalf("page 2: %v", page2)
	}
	if got := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?mode=pickup", nil), http.StatusOK)["items"].([]any); len(got) != 1 {
		t.Fatalf("pickup filter: %v", got)
	}
	first := items[2].(map[string]any)
	mustCode(t, status(s.owner, first["id"].(string), "accepted", 1, ""), http.StatusOK)
	if got := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders?status=accepted&status=preparing", nil), http.StatusOK)["items"].([]any); len(got) != 1 {
		t.Fatalf("status filter: %v", got)
	}
	for _, q := range []string{"?status=bogus", "?mode=delivery", "?limit=0", "?cursor=@@@"} {
		if r := s.owner.do(http.MethodGet, "/api/admin/orders"+q, nil); r.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", q, r.Code)
		}
	}
}

func TestOrderTenantIsolation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		o := s.placed(t, "iso-0001")
		id := o["id"].(string)
		d := mustCode(t, s.owner.do(http.MethodPost, "/api/admin/orders/"+id+"/payments",
			map[string]any{"method": "cash", "status": "paid", "amount_minor": o["total_minor"]}), http.StatusOK)
		pid := d["payments"].([]any)[0].(map[string]any)["id"].(string)

		if _, err := s.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"}); err != nil {
			t.Fatal(err)
		}
		m := s.client()
		mustCode(t, m.login("mallory@example.com", "another long pw"), http.StatusOK)
		mustCode(t, m.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusNotFound)
		mustCode(t, status(m, id, "accepted", 1, ""), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPost, "/api/admin/orders/"+id+"/payments", map[string]any{"method": "cash", "status": "paid", "amount_minor": 1}), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPatch, "/api/admin/payments/"+pid, map[string]any{"status": "failed"}), http.StatusNotFound)
		mustCode(t, m.do(http.MethodPost, "/api/admin/payments/"+pid+"/refund", map[string]any{"amount_minor": 1, "reason": "x"}), http.StatusNotFound)
		if got := mustCode(t, m.do(http.MethodGet, "/api/admin/orders", nil), http.StatusOK)["items"].([]any); len(got) != 0 {
			t.Fatalf("other tenant lists orders: %v", got)
		}
		if d := mustCode(t, s.owner.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusOK); d["status"] != "new" || d["payment_status"] != "paid" {
			t.Fatalf("roma's order changed: %v", d)
		}
	})
}

// sse opens the staff event stream on a real server and returns a channel of
// event names with their data.
func sse(t *testing.T, srv *httptest.Server, c *client) <-chan string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/admin/events", nil)
	req.AddCookie(c.cookie)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	resp, err := srv.Client().Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("events: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	out := make(chan string, 16)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		name := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				out <- name + " " + strings.TrimPrefix(line, "data: ")
			}
		}
		close(out)
	}()
	return out
}

func next(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return ""
	}
}

func TestEventStream(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		s := newShop(t, db)
		srv := httptest.NewServer(s.h)
		t.Cleanup(srv.Close)

		if _, err := s.app.CreateTenant(context.Background(), app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
			OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"}); err != nil {
			t.Fatal(err)
		}
		m := s.client()
		mustCode(t, m.login("mallory@example.com", "another long pw"), http.StatusOK)

		roma := sse(t, srv, s.owner)
		other := sse(t, srv, m)
		if ev := next(t, roma); ev != "ready {}" {
			t.Fatalf("first event: %q", ev)
		}
		next(t, other)

		o := s.placed(t, "sse-0001")
		if ev := next(t, roma); ev != fmt.Sprintf(`order.created {"order_id":%q,"version":1}`, o["id"]) {
			t.Fatalf("created: %q", ev)
		}
		mustCode(t, status(s.owner, o["id"].(string), "accepted", 1, ""), http.StatusOK)
		if ev := next(t, roma); ev != fmt.Sprintf(`order.updated {"order_id":%q,"version":2}`, o["id"]) {
			t.Fatalf("updated: %q", ev)
		}
		select {
		case ev := <-other:
			t.Fatalf("other tenant received %q", ev)
		case <-time.After(200 * time.Millisecond):
		}
	})
}

// TestMVPEndToEnd is MASTER_PLAN.md §16's end-to-end scenario: tenant and
// owner, menu, table and QR token, guest order, staff board, lifecycle,
// external payment, restart, history intact.
func TestMVPEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servly.db")
	ctx := context.Background()
	open := func() *database.DB {
		db, err := database.OpenSQLite(ctx, path, quiet)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		return db
	}
	db := open()
	s := newShop(t, db)          // steps 1-3
	o := s.placed(t, "mvp-0001") // 4-5
	id := o["id"].(string)
	staff := s.staffClient(t, "bob@example.com", "staff")
	for i, to := range []string{"accepted", "preparing", "ready", "completed"} { // 6-7
		mustCode(t, status(staff, id, to, float64(i+1), ""), http.StatusOK)
	}
	mustCode(t, staff.do(http.MethodPost, "/api/admin/orders/"+id+"/payments", map[string]any{ // 8
		"method": "external_terminal", "status": "paid", "amount_minor": o["total_minor"], "reference": "TID 4411 #0193"}), http.StatusOK)
	db.Close()

	db = open() // 9: a new process on the same data
	defer db.Close()
	a := app.New(db, quiet)
	h := NewHandler(Deps{Log: quiet, App: a})
	e2 := &env{t: t, db: db, app: a, h: h, now: time.Now()}
	c := e2.client()
	mustCode(t, c.login("anna@example.com", annaPW), http.StatusOK)
	d := mustCode(t, c.do(http.MethodGet, "/api/admin/orders/"+id, nil), http.StatusOK) // 10
	if d["status"] != "completed" || d["payment_status"] != "paid" || len(d["events"].([]any)) != 5 || len(d["payments"].([]any)) != 1 ||
		d["total_minor"].(float64) != 2900 || d["items"].([]any)[0].(map[string]any)["name"] != "Margherita" {
		t.Fatalf("after restart: %v", d)
	}
}

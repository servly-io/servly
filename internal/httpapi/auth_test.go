package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

func TestLoginSessionLogout(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		e := newEnv(t, db)
		c := e.client()

		for _, bad := range [][2]string{{"anna@example.com", "wrong password!"}, {"nobody@example.com", annaPW}, {"not-an-email", annaPW}} {
			if r := c.login(bad[0], bad[1]); r.Code != http.StatusUnauthorized || r.code() != "invalid_credentials" {
				t.Fatalf("login %v: %d %s", bad, r.Code, r.Body.String())
			}
		}

		r := c.login("ANNA@example.com", annaPW)
		if r.Code != http.StatusOK {
			t.Fatalf("login: %d %s", r.Code, r.Body.String())
		}
		ck := c.cookie
		if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Secure || ck.Path != "/" || len(ck.Value) < 40 {
			t.Fatalf("cookie: %+v", ck)
		}
		body := r.json(t)
		at, _ := body["active_tenant"].(map[string]any)
		if at["name"] != "Roma" || at["role"] != "owner" || c.csrf == "" {
			t.Fatalf("session body: %v", body)
		}
		if perms, _ := at["permissions"].([]any); len(perms) != len(tenant.PermissionsOf(tenant.RoleOwner)) {
			t.Fatalf("permissions: %v", at["permissions"])
		}

		// The cookie value is never stored: only its hash.
		var stored string
		if err := db.QueryRowContext(context.Background(), `SELECT token_hash FROM sessions`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == ck.Value || stored != auth.HashToken(ck.Value) {
			t.Fatal("session token stored in clear")
		}

		if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusOK {
			t.Fatalf("session: %d", r.Code)
		}

		// State changes need the CSRF token.
		csrf := c.csrf
		c.csrf = ""
		if r := c.do(http.MethodPost, "/api/auth/logout", nil); r.Code != http.StatusForbidden || r.code() != "csrf_failed" {
			t.Fatalf("logout without csrf: %d %s", r.Code, r.Body.String())
		}
		c.csrf = "wrong"
		if r := c.do(http.MethodPost, "/api/auth/logout", nil); r.Code != http.StatusForbidden {
			t.Fatalf("logout with wrong csrf: %d", r.Code)
		}
		c.csrf = csrf
		old := *c.cookie
		if r := c.do(http.MethodPost, "/api/auth/logout", nil); r.Code != http.StatusNoContent || c.cookie != nil {
			t.Fatalf("logout: %d cookie=%v", r.Code, c.cookie)
		}
		c.cookie = &old // replaying the old cookie must not work
		if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusUnauthorized {
			t.Fatalf("session after logout: %d", r.Code)
		}
	})
}

func TestUnauthenticatedAdminRequests(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.client()
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		if r := c.do(m, "/api/admin/anything", nil); r.Code != http.StatusUnauthorized || r.code() != "unauthenticated" {
			t.Errorf("%s: %d %s", m, r.Code, r.Body.String())
		}
	}
	c.cookie = &http.Cookie{Name: sessionCookie, Value: "forged-token"}
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusUnauthorized {
		t.Errorf("forged cookie: %d", r.Code)
	}
}

func TestSecureCookie(t *testing.T) {
	cases := []struct {
		name   string
		mod    func(*Deps)
		header []string
		want   bool
	}{
		{"auto http", nil, nil, false},
		{"forced", func(d *Deps) { d.CookieSecure = "true" }, nil, true},
		{"proxy https untrusted", nil, []string{"X-Forwarded-Proto", "https"}, false},
		{"proxy https trusted", func(d *Deps) { d.TrustProxy = true }, []string{"X-Forwarded-Proto", "https"}, true},
		{"forced off", func(d *Deps) { d.CookieSecure = "false"; d.TrustProxy = true }, []string{"X-Forwarded-Proto", "https"}, false},
	}
	for _, tc := range cases {
		var mods []func(*Deps)
		if tc.mod != nil {
			mods = append(mods, tc.mod)
		}
		e := newSQLiteEnv(t, mods...)
		c := e.client()
		c.do(http.MethodPost, "/api/auth/login", map[string]string{"email": "anna@example.com", "password": annaPW}, tc.header...)
		if c.cookie == nil || c.cookie.Secure != tc.want {
			t.Errorf("%s: cookie %+v, want Secure=%v", tc.name, c.cookie, tc.want)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.client()
	var last resp
	for range 6 {
		last = c.login("anna@example.com", "wrong password!")
	}
	if last.Code != http.StatusTooManyRequests || last.code() != "rate_limited" || last.Header().Get("Retry-After") == "" {
		t.Fatalf("6th attempt: %d %s", last.Code, last.Body.String())
	}
	// The limit is per email: another account is unaffected.
	if r := c.login("other@example.com", "wrong password!"); r.Code != http.StatusUnauthorized {
		t.Fatalf("other email: %d", r.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	e.now = e.now.Add(auth.IdleTimeout - time.Minute)
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusOK {
		t.Fatalf("before idle timeout: %d", r.Code)
	}
	// That request extended the idle window.
	e.now = e.now.Add(auth.IdleTimeout - time.Minute)
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusOK {
		t.Fatalf("after activity: %d", r.Code)
	}
	e.now = e.now.Add(auth.IdleTimeout + time.Second)
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("after idle timeout: %d", r.Code)
	}
}

func TestSessionAbsoluteExpiry(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	for range 20 { // keep it busy for more than 7 days
		e.now = e.now.Add(10 * time.Hour)
		if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusOK {
			if e.now.Sub(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)) < auth.AbsoluteTimeout {
				t.Fatalf("expired early at %v", e.now)
			}
			return
		}
	}
	t.Fatal("session outlived the absolute timeout")
}

func TestDisabledUserLosesSession(t *testing.T) {
	e := newSQLiteEnv(t)
	c := e.owner()
	st := tenantsql.New(e.db)
	u, _ := st.GetUser(context.Background(), e.roma.Owner.ID)
	u.Status = tenant.UserDisabled
	if err := st.UpdateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user: %d", r.Code)
	}
	if r := e.client().login("anna@example.com", annaPW); r.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user login: %d", r.Code)
	}
}

func TestSwitchTenant(t *testing.T) {
	e := newSQLiteEnv(t)
	ctx := context.Background()
	due, err := e.app.CreateTenant(ctx, app.CreateTenantInput{Name: "Due", Currency: "EUR", Timezone: "Europe/Rome", OwnerEmail: "anna@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := e.app.CreateTenant(ctx, app.CreateTenantInput{Name: "Other", Currency: "EUR", Timezone: "Europe/Rome",
		OwnerEmail: "mallory@example.com", OwnerPassword: "another long pw"})
	if err != nil {
		t.Fatal(err)
	}

	c := e.client()
	body := c.login("anna@example.com", annaPW).json(t)
	if body["active_tenant"] != nil || len(body["memberships"].([]any)) != 2 {
		t.Fatalf("two memberships: want none active: %v", body)
	}

	switchTo := func(id string) resp {
		return c.do(http.MethodPut, "/api/auth/session/tenant", map[string]string{"tenant_id": id})
	}
	if r := switchTo(other.Tenant.ID.String()); r.Code != http.StatusNotFound {
		t.Fatalf("switch to foreign tenant: %d %s", r.Code, r.Body.String())
	}
	r := switchTo(due.Tenant.ID.String())
	if at, _ := r.json(t)["active_tenant"].(map[string]any); r.Code != http.StatusOK || at["name"] != "Due" {
		t.Fatalf("switch: %d %s", r.Code, r.Body.String())
	}
	if at, _ := c.do(http.MethodGet, "/api/auth/session", nil).json(t)["active_tenant"].(map[string]any); at["name"] != "Due" {
		t.Fatalf("switch not persisted: %v", at)
	}
}

func TestRequireTenantAndPermission(t *testing.T) {
	s := &server{Deps: Deps{Log: quiet}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	run := func(p *app.Principal, h http.Handler) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), principalKey, p))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	noTenant := &app.Principal{}
	staff := &app.Principal{Tenant: &tenant.TenantMembership{Membership: tenant.Membership{Role: tenant.RoleStaff}}}
	if c := run(noTenant, s.requireTenant(ok)); c != http.StatusForbidden {
		t.Errorf("no tenant: %d", c)
	}
	if c := run(staff, s.requireTenant(ok)); c != http.StatusTeapot {
		t.Errorf("tenant: %d", c)
	}
	if c := run(staff, s.can(tenant.PermManageMenu, ok)); c != http.StatusForbidden {
		t.Errorf("staff managing menu: %d", c)
	}
	if c := run(staff, s.can(tenant.PermViewOrders, ok)); c != http.StatusTeapot {
		t.Errorf("staff viewing orders: %d", c)
	}
}

func TestClientIPGroupsIPv6(t *testing.T) {
	s := &server{Deps: Deps{TrustProxy: true}}
	key := func(remote, xff string) string {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return s.clientIP(r)
	}
	a, b := key("[2001:db8:1:2:aaaa::1]:4000", ""), key("[2001:db8:1:2:bbbb::9]:4000", "")
	if a != b || a != "2001:db8:1:2::/64" {
		t.Fatalf("same /64: %q %q", a, b)
	}
	if key("[2001:db8:1:3::1]:4000", "") == a {
		t.Fatal("other /64 shares a key")
	}
	if got := key("10.0.0.1:4000", "203.0.113.7, 2001:db8:9:9::5"); got != "2001:db8:9:9::/64" {
		t.Fatalf("forwarded v6: %q", got)
	}
	if got := key("10.0.0.1:4000", ""); got != "10.0.0.1" {
		t.Fatalf("v4: %q", got)
	}
}

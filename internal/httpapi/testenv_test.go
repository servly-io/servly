package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/storage"
)

// env is a server on a fresh database with one restaurant, "roma", owned by
// anna@example.com.
type env struct {
	t    *testing.T
	db   *database.DB
	app  *app.App
	h    http.Handler
	now  time.Time
	roma *app.CreateTenantResult
}

const annaPW = "correct horse battery"

func newEnv(t *testing.T, db *database.DB, mod ...func(*Deps)) *env {
	t.Helper()
	e := &env{t: t, db: db, app: app.New(db, quiet), now: time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)}
	e.app.Files = storage.NewLocal(t.TempDir())
	d := Deps{Log: quiet, App: e.app, Now: func() time.Time { return e.now }}
	for _, m := range mod {
		m(&d)
	}
	e.h = NewHandler(d)
	var err error
	e.roma, err = e.app.CreateTenant(context.Background(), app.CreateTenantInput{
		Name: "Roma", Currency: "EUR", Timezone: "Europe/Berlin", OwnerEmail: "anna@example.com", OwnerPassword: annaPW,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func newSQLiteEnv(t *testing.T, mod ...func(*Deps)) *env {
	return newEnv(t, dbtest.Open(t, database.SQLite), mod...)
}

// client is a browser: it keeps the session cookie and the CSRF token.
type client struct {
	e      *env
	cookie *http.Cookie
	csrf   string
	ip     string // RemoteAddr host; empty keeps httptest's default
}

func (e *env) client() *client { return &client{e: e} }

type resp struct {
	*httptest.ResponseRecorder
}

func (r resp) json(t *testing.T) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
		t.Fatalf("body %q: %v", r.Body.String(), err)
	}
	return v
}

func (r resp) code() string {
	var v struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(r.Body.Bytes(), &v)
	return v.Error.Code
}

func (c *client) do(method, path string, body any, hdr ...string) resp {
	c.e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.ip != "" {
		req.RemoteAddr = c.ip + ":40000"
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	return resp{rec}
}

// login signs in and keeps the CSRF token.
func (c *client) login(email, pw string) resp {
	c.e.t.Helper()
	r := c.do(http.MethodPost, "/api/auth/login", map[string]string{"email": email, "password": pw})
	if r.Code == http.StatusOK {
		c.csrf, _ = r.json(c.e.t)["csrf_token"].(string)
	}
	return r
}

// owner returns a client signed in as the owner of roma.
func (e *env) owner() *client {
	e.t.Helper()
	c := e.client()
	if r := c.login("anna@example.com", annaPW); r.Code != http.StatusOK {
		e.t.Fatalf("owner login: %d %s", r.Code, r.Body.String())
	}
	return c
}

// doRaw sends a raw JSON body.
func (c *client) doRaw(method, path, body string) resp {
	c.e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	return resp{rec}
}

// upload posts body as the multipart field "file".
func (c *client) upload(field string, body []byte) resp {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, "photo.png")
	if err != nil {
		c.e.t.Fatal(err)
	}
	_, _ = fw.Write(body)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	req.Header.Set("X-CSRF-Token", c.csrf)
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	return resp{rec}
}

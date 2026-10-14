package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func do(t *testing.T, h http.Handler, method, target string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, body))
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestHealthz(t *testing.T) {
	h := NewHandler(Deps{Log: quiet, Version: "1.2.3", Ready: func(context.Context) error { return errors.New("db down") }})
	rec := do(t, h, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status %d", rec.Code)
	}
	if v := decode(t, rec); v["status"] != "ok" || v["version"] != "1.2.3" {
		t.Fatalf("healthz body %v", v)
	}
}

func TestReadyz(t *testing.T) {
	var readyErr error
	h := NewHandler(Deps{Log: quiet, Ready: func(context.Context) error { return readyErr }})
	if rec := do(t, h, http.MethodGet, "/readyz", nil); rec.Code != http.StatusOK {
		t.Fatalf("ready: status %d", rec.Code)
	}
	readyErr = errors.New("secret connection string in error")
	rec := do(t, h, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready: status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("readyz leaked the error: %s", rec.Body.String())
	}
}

func TestUnknownAPIRouteIsJSONError(t *testing.T) {
	h := NewHandler(Deps{Log: quiet})
	rec := do(t, h, http.MethodGet, "/api/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	body := decode(t, rec)
	e, _ := body["error"].(map[string]any)
	id, _ := e["request_id"].(string)
	if e["code"] != "route_not_found" || len(id) != 24 || rec.Header().Get("X-Request-Id") != id {
		t.Fatalf("error body %v, header %q", body, rec.Header().Get("X-Request-Id"))
	}
	if host, _ := os.Hostname(); host != "" && strings.Contains(id, host) {
		t.Fatalf("request id %q reveals the hostname", id)
	}
}

func TestRequestIDFromProxy(t *testing.T) {
	h := NewHandler(Deps{Log: quiet})
	for in, keep := range map[string]bool{"abc-123_X": true, "bad id\nx": false, strings.Repeat("a", 65): false} {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("X-Request-Id", in)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("X-Request-Id"); (got == in) != keep {
			t.Errorf("incoming %q: response id %q, keep=%v", in, got, keep)
		}
	}
}

func TestPanicIsInternalErrorWithoutDetail(t *testing.T) {
	h := recoverer(quiet)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("db password=hunter2") }))
	rec := do(t, h, http.MethodGet, "/", nil)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}
	run := func(body string, limit int64) error {
		var v req
		var err error
		h := maxBody(limit, limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { err = decodeJSON(r, &v) }))
		do(t, h, http.MethodPost, "/", strings.NewReader(body))
		return err
	}
	if err := run(`{"name":"x"}`, 1024); err != nil {
		t.Fatalf("valid body: %v", err)
	}
	for _, body := range []string{`{"name":"x","extra":1}`, `{"name":"x"} {}`, `not json`, ``} {
		if err := run(body, 1024); !errors.Is(err, errMalformedJSON) {
			t.Errorf("%q: err = %v", body, err)
		}
	}
	if err := run(`{"name":"`+strings.Repeat("x", 100)+`"}`, 16); !errors.Is(err, errBodyTooLarge) {
		t.Errorf("oversized body: err = %v", err)
	}
}

func TestFrontend(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"assets/app-1.js":      {Data: []byte("js")},
		"manifest.webmanifest": {Data: []byte("{}")},
	}
	h := NewHandler(Deps{Log: quiet, Frontend: dist})

	for _, p := range []string{"/", "/staff", "/m/abcdef", "/admin/menu/items"} {
		rec := do(t, h, http.MethodGet, p, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "<html>app</html>" || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: status %d cache %q body %q", p, rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
		}
	}
	rec := do(t, h, http.MethodGet, "/assets/app-1.js", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: status %d cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(t, h, http.MethodGet, "/assets/missing.js", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset: status %d", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/../../etc/passwd", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("traversal: status %d body %q", rec.Code, rec.Body.String())
	}
}

func TestFrontendNotBuilt(t *testing.T) {
	h := NewHandler(Deps{Log: quiet, Frontend: fstest.MapFS{".gitkeep": {}}})
	if rec := do(t, h, http.MethodGet, "/", nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
}

package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "SERVLY_") {
			t.Setenv(k, "")
		}
	}
}

func startServe(t *testing.T, args ...string) string {
	t.Helper()
	f := &rootFlags{}
	cmd := newServeCommand(f)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.ParseFlags(append([]string{"--listen-addr=127.0.0.1:0"}, args...)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serve(ctx, cmd, f, ready) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	select {
	case addr := <-ready:
		return "http://" + addr
	case err := <-done:
		t.Fatalf("serve exited: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not start")
	}
	return ""
}

func status(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestServeSQLite(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	base := startServe(t, "--data-dir="+dir)
	if s := status(t, base+"/healthz"); s != http.StatusOK {
		t.Fatalf("healthz %d", s)
	}
	if s := status(t, base+"/readyz"); s != http.StatusOK {
		t.Fatalf("readyz %d", s)
	}
	if _, err := os.Stat(filepath.Join(dir, "servly.db")); err != nil {
		t.Fatalf("database file: %v", err)
	}
}

func TestServeNotReadyWhileMigrationsPending(t *testing.T) {
	isolate(t)
	base := startServe(t, "--data-dir="+t.TempDir(), "--migrate-on-start=false")
	if s := status(t, base+"/healthz"); s != http.StatusOK {
		t.Fatalf("healthz %d", s)
	}
	if s := status(t, base+"/readyz"); s != http.StatusServiceUnavailable {
		t.Fatalf("readyz %d, want 503", s)
	}
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestMigrateCommands(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	out, err := run(t, "migrate", "status", "--data-dir="+dir)
	if err != nil || !strings.Contains(out, "current 0") || !strings.Contains(out, "pending") {
		t.Fatalf("status before: %q %v", out, err)
	}
	if out, err := run(t, "migrate", "up", "--data-dir="+dir); err != nil || !strings.Contains(out, "schema at version") {
		t.Fatalf("up: %q %v", out, err)
	}
	if out, err := run(t, "migrate", "status", "--data-dir="+dir); err != nil || !strings.Contains(out, "up to date") {
		t.Fatalf("status after: %q %v", out, err)
	}
}

func TestConfigViewRedactsAndAppliesPrecedence(t *testing.T) {
	isolate(t)
	file := filepath.Join(t.TempDir(), "servly.yaml")
	if err := os.WriteFile(file, []byte("listen_addr: ':7000'\ndatabase:\n  driver: postgres\n  url: postgres://servly:s3cret@db/servly?sslmode=disable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVLY_LISTEN_ADDR", ":7001")
	out, err := run(t, "config", "view", "--config="+file, "--data-dir=/srv/servly")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen_addr: :7001", "data_dir: /srv/servly", "driver: postgres", "REDACTED"} {
		if !strings.Contains(out, want) {
			t.Errorf("config view lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "s3cret") {
		t.Errorf("config view leaked the password:\n%s", out)
	}
}

func TestBadConfigFails(t *testing.T) {
	isolate(t)
	if _, err := run(t, "config", "view", "--database-driver=mysql"); err == nil {
		t.Fatal("want error for unsupported driver")
	}
}

func TestRedactURL(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"./data/x.db":         "./data/x.db",
		"postgres://u:p@h/db": "postgres://u:REDACTED@h/db",
		"postgres://u@h/db":   "postgres://u@h/db",
		"postgres://h/db?password=p&sslmode=require": "postgres://h/db?password=REDACTED&sslmode=require",
	}
	for in, want := range cases {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func runWithStdin(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	root := newRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestTenantCreateAndSetPassword(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	out, err := runWithStdin(t, "correct horse battery\n", "tenant", "create", "--data-dir="+dir,
		"--name=Trattoria Roma", "--timezone=Europe/Berlin", "--owner-email=anna@example.com", "--password-stdin")
	if err != nil || !strings.Contains(out, "trattoria-roma") || !strings.Contains(out, "owner: anna@example.com") {
		t.Fatalf("tenant create: %q %v", out, err)
	}
	// Existing owner: no password needed, none read.
	out, err = runWithStdin(t, "", "tenant", "create", "--data-dir="+dir,
		"--name=Due", "--timezone=Europe/Rome", "--owner-email=anna@example.com", "--password-stdin")
	if err != nil || !strings.Contains(out, "password unchanged") {
		t.Fatalf("second tenant: %q %v", out, err)
	}
	if _, err := runWithStdin(t, "short\n", "user", "set-password", "anna@example.com", "--data-dir="+dir, "--password-stdin"); err == nil {
		t.Fatal("weak password accepted")
	}
	out, err = runWithStdin(t, "another long secret\n", "user", "set-password", "anna@example.com", "--data-dir="+dir, "--password-stdin")
	if err != nil || !strings.Contains(out, "password set") {
		t.Fatalf("set-password: %q %v", out, err)
	}
}

// An open staff event stream must not hold up a graceful shutdown.
func TestShutdownEndsEventStreams(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if _, err := runWithStdin(t, "correct horse battery\n", "tenant", "create", "--data-dir="+dir,
		"--name=Roma", "--timezone=Europe/Berlin", "--owner-email=anna@example.com", "--password-stdin"); err != nil {
		t.Fatal(err)
	}
	f := &rootFlags{}
	cmd := newServeCommand(f)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.ParseFlags([]string{"--listen-addr=127.0.0.1:0", "--data-dir=" + dir}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serve(ctx, cmd, f, ready) }()
	base := "http://" + <-ready

	resp, err := http.Post(base+"/api/auth/login", "application/json",
		strings.NewReader(`{"email":"anna@example.com","password":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/admin/events", nil)
	for _, c := range resp.Cookies() {
		req.AddCookie(c)
	}
	stream, err := http.DefaultClient.Do(req)
	if err != nil || stream.StatusCode != http.StatusOK {
		t.Fatalf("events: %v %v", err, stream)
	}
	defer stream.Body.Close()

	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("shutdown took %v", d)
		}
	case <-time.After(shutdownGrace + 2*time.Second):
		t.Fatal("shutdown hung on the event stream")
	}
}

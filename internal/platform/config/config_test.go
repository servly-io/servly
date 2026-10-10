package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// isolate keeps the developer's real config files and SERVLY_ variables out.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "SERVLY_") {
			t.Setenv(k, "")
		}
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "servly.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	isolate(t)
	c, err := Load(New(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.CookieSecure != "auto" || c.TrustProxy || c.PublicBaseURL != "" {
		t.Fatalf("unexpected http defaults: %+v", c)
	}
	if c.ListenAddr != ":8080" || c.DatabaseDriver != DriverSQLite || !c.MigrateOnStart ||
		c.LogLevel != slog.LevelInfo || c.LogFormat != "json" || c.File != "" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if got, want := c.SQLitePath(), filepath.Join("data", "servly.db"); got != want {
		t.Fatalf("SQLitePath = %q, want %q", got, want)
	}
}

func TestPrecedenceFlagOverEnvOverFile(t *testing.T) {
	isolate(t)
	file := writeFile(t, "listen_addr: ':1'\ndata_dir: /from/file\nlog:\n  level: debug\ndatabase:\n  driver: postgres\n  url: postgres://file\n")
	t.Setenv("SERVLY_DATA_DIR", "/from/env")
	t.Setenv("SERVLY_LOG_LEVEL", "warn")

	v := New()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.String("log-level", "", "")
	if err := v.BindPFlag(KeyLogLevel, fs.Lookup("log-level")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Parse([]string{"--log-level=error"}); err != nil {
		t.Fatal(err)
	}

	c, err := Load(v, file)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":1" || c.DatabaseDriver != DriverPostgres || c.DatabaseURL != "postgres://file" {
		t.Errorf("file values not applied: %+v", c)
	}
	if c.DataDir != "/from/env" {
		t.Errorf("env did not override file: data dir %q", c.DataDir)
	}
	if c.LogLevel != slog.LevelError {
		t.Errorf("flag did not override env: level %v", c.LogLevel)
	}
	if c.File != file {
		t.Errorf("File = %q", c.File)
	}
}

func TestFileFoundThroughEnv(t *testing.T) {
	isolate(t)
	t.Setenv(ConfigEnv, writeFile(t, "listen_addr: ':9'\n"))
	c, err := Load(New(), "")
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddr != ":9" {
		t.Fatalf("ListenAddr = %q", c.ListenAddr)
	}
}

func TestExplicitFileMustExist(t *testing.T) {
	isolate(t)
	if _, err := Load(New(), filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("want error for missing explicit config file")
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	isolate(t)
	_, err := Load(New(), writeFile(t, "databse:\n  driver: sqlite\n"))
	if err == nil || !strings.Contains(err.Error(), "databse.driver") {
		t.Fatalf("want unknown key error, got %v", err)
	}
}

func TestPostgresNeedsURL(t *testing.T) {
	isolate(t)
	t.Setenv("SERVLY_DATABASE_DRIVER", "postgres")
	_, err := Load(New(), "")
	if err == nil || !strings.Contains(err.Error(), KeyDatabaseURL) {
		t.Fatalf("want missing URL error, got %v", err)
	}
}

func TestInvalidValuesAreAllReported(t *testing.T) {
	isolate(t)
	t.Setenv("SERVLY_DATABASE_DRIVER", "mysql")
	t.Setenv("SERVLY_MIGRATE_ON_START", "maybe")
	t.Setenv("SERVLY_LOG_LEVEL", "loud")
	t.Setenv("SERVLY_LOG_FORMAT", "xml")
	t.Setenv("SERVLY_HTTP_COOKIE_SECURE", "sometimes")
	t.Setenv("SERVLY_HTTP_TRUST_PROXY_HEADERS", "yes please")
	t.Setenv("SERVLY_HTTP_PUBLIC_BASE_URL", "ftp://x")
	t.Setenv("SERVLY_SECRET_KEY", "too-short")
	t.Setenv("SERVLY_SETUP_TOKEN", "short")
	t.Setenv("SERVLY_PAYMENTS_SANDBOX", "kind of")
	_, err := Load(New(), "")
	if err == nil {
		t.Fatal("want error")
	}
	for _, key := range []string{KeyDatabaseDriver, KeyMigrateOnStart, KeyLogLevel, KeyLogFormat, KeyCookieSecure, KeyTrustProxy, KeyPublicBaseURL,
		KeySecretKey, KeySetupToken, KeySandbox} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestHTTPSettingsFromFile(t *testing.T) {
	isolate(t)
	c, err := Load(New(), writeFile(t, "http:\n  cookie_secure: \"true\"\n  trust_proxy_headers: true\n  public_base_url: https://roma.example.com/\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.CookieSecure != "true" || !c.TrustProxy || c.PublicBaseURL != "https://roma.example.com" {
		t.Fatalf("%+v", c)
	}
}

// A PostgreSQL URL alone selects PostgreSQL; another URL is no SQLite path.
func TestDatabaseURLSelectsDriver(t *testing.T) {
	isolate(t)
	t.Setenv("SERVLY_DATABASE_URL", "postgresql://neondb_owner:pw@ep-x.neon.tech/neondb?sslmode=require&channel_binding=require")
	c, err := Load(New(), "")
	if err != nil || c.DatabaseDriver != DriverPostgres {
		t.Fatalf("driver %q, err %v", c.DatabaseDriver, err)
	}
	t.Setenv("SERVLY_DATABASE_URL", "mysql://root:secret@db/servly")
	_, err = Load(New(), "")
	if err == nil || !strings.Contains(err.Error(), "not a SQLite file path") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("mysql URL: %v", err)
	}
}

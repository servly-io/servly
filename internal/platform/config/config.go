// Package config resolves the server configuration. Sources, highest
// precedence first: command-line flags, SERVLY_* environment variables, the
// config file, built-in defaults. The config file is optional; a container
// can run on environment variables alone. See docs/docs/local-deployment.md.
package config

import (
	"github.com/servly-io/servly/internal/platform/secretbox"

	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Driver names a supported database backend.
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "postgres"
)

// Keys of the configuration. In the YAML file a dot is nesting
// (database.driver is database: {driver: ...}); in the environment it is an
// underscore after the SERVLY_ prefix (SERVLY_DATABASE_DRIVER).
const (
	KeyListenAddr     = "listen_addr"
	KeyDataDir        = "data_dir"
	KeyDatabaseDriver = "database.driver"
	KeyDatabaseURL    = "database.url"
	KeyMigrateOnStart = "migrate_on_start"
	KeyLogLevel       = "log.level"
	KeyLogFormat      = "log.format"
	KeyCookieSecure   = "http.cookie_secure"
	KeyTrustProxy     = "http.trust_proxy_headers"
	KeyPublicBaseURL  = "http.public_base_url"
	KeySecretKey      = "secret_key"
	KeySandbox        = "payments.sandbox"
	KeySetupToken     = "setup_token"
)

var defaults = map[string]any{
	KeyListenAddr:     ":8080",
	KeyDataDir:        "./data",
	KeyDatabaseDriver: string(DriverSQLite),
	KeyDatabaseURL:    "",
	KeyMigrateOnStart: true,
	KeyLogLevel:       "info",
	KeyLogFormat:      "json",
	KeyCookieSecure:   "auto",
	KeyTrustProxy:     false,
	KeyPublicBaseURL:  "",
	KeySecretKey:      "",
	KeySandbox:        false,
	KeySetupToken:     "",
}

// ConfigEnv names the variable that points at the config file.
const ConfigEnv = "SERVLY_CONFIG"

// Config is the complete server configuration.
type Config struct {
	ListenAddr     string
	DataDir        string
	DatabaseDriver Driver
	// DatabaseURL is the PostgreSQL connection string, or for SQLite an
	// optional database file path that overrides <DataDir>/servly.db.
	DatabaseURL    string
	MigrateOnStart bool
	LogLevel       slog.Level
	LogFormat      string
	// CookieSecure is "auto" (Secure when the request came over HTTPS),
	// "true" or "false".
	CookieSecure string
	// TrustProxy makes the server believe X-Forwarded-For and
	// X-Forwarded-Proto. Enable it only behind a reverse proxy that sets them.
	TrustProxy bool
	// PublicBaseURL is the address guests reach the server at, used in QR
	// codes, e.g. "http://192.168.1.20:8080". Empty: taken from the request.
	PublicBaseURL string
	// SecretKey (base64, 32 bytes) encrypts payment provider credentials.
	// Empty disables online payments.
	SecretKey string
	// PaymentsSandbox offers the built-in sandbox provider: a fake hosted
	// checkout for demos and tests. Never for real guests.
	PaymentsSandbox bool
	// SetupToken fixes the first-run setup token instead of a random one per
	// process: for platforms that run several instances or restart them.
	SetupToken string
	// File is the config file that was read, or empty.
	File string
}

// SQLitePath is the SQLite database file the configuration selects.
func (c Config) SQLitePath() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return filepath.Join(c.DataDir, "servly.db")
}

// New returns a viper instance with the defaults and the SERVLY_ environment
// bound. Bind command-line flags to it, then call Load.
func New() *viper.Viper {
	v := viper.New()
	for k, d := range defaults {
		v.SetDefault(k, d)
	}
	v.SetEnvPrefix("SERVLY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}

// Candidates lists where the config file may live, in lookup order:
// $SERVLY_CONFIG, the user's config dir, then /etc/servly.
func Candidates() []string {
	var c []string
	if p := os.Getenv(ConfigEnv); p != "" {
		c = append(c, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		c = append(c, filepath.Join(home, ".config", "servly", "servly.yaml"))
	}
	return append(c, "/etc/servly-io/servly.yaml")
}

// Load reads the config file into v and resolves the configuration. An
// explicit path (the --config flag) must exist; otherwise the first existing
// candidate is read, and no file at all is fine.
func Load(v *viper.Viper, explicit string) (Config, error) {
	file := explicit
	if file == "" {
		for _, c := range Candidates() {
			if _, err := os.Stat(c); err == nil {
				file = c
				break
			}
		}
	}
	if file != "" {
		v.SetConfigFile(file)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return Config{}, fmt.Errorf("read config %s: %w", file, err)
		}
		if err := checkKeys(v); err != nil {
			return Config{}, fmt.Errorf("config %s: %w", file, err)
		}
	}
	c, err := resolve(v)
	c.File = file
	return c, err
}

// checkKeys rejects unknown keys, so a typo in the file fails loudly instead
// of silently using a default.
func checkKeys(v *viper.Viper) error {
	var unknown []string
	for _, k := range v.AllKeys() {
		if _, ok := defaults[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return fmt.Errorf("unknown keys: %s", strings.Join(unknown, ", "))
	}
	return nil
}

func resolve(v *viper.Viper) (Config, error) {
	c := Config{
		ListenAddr:     v.GetString(KeyListenAddr),
		DataDir:        v.GetString(KeyDataDir),
		DatabaseDriver: Driver(v.GetString(KeyDatabaseDriver)),
		DatabaseURL:    v.GetString(KeyDatabaseURL),
		LogFormat:      v.GetString(KeyLogFormat),
		CookieSecure:   v.GetString(KeyCookieSecure),
		PublicBaseURL:  strings.TrimRight(v.GetString(KeyPublicBaseURL), "/"),
		SecretKey:      v.GetString(KeySecretKey),
		SetupToken:     strings.TrimSpace(v.GetString(KeySetupToken)),
	}
	var errs []error

	switch c.DatabaseDriver {
	case DriverSQLite:
	case DriverPostgres:
		if c.DatabaseURL == "" {
			errs = append(errs, fmt.Errorf("%s is required when %s is postgres", KeyDatabaseURL, KeyDatabaseDriver))
		}
	default:
		errs = append(errs, fmt.Errorf("%s: unsupported driver %q (want sqlite or postgres)", KeyDatabaseDriver, c.DatabaseDriver))
	}

	// Parsed here, not with viper's GetBool, which turns "maybe" into false.
	migrate, err := strconv.ParseBool(v.GetString(KeyMigrateOnStart))
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %q is not a boolean", KeyMigrateOnStart, v.GetString(KeyMigrateOnStart)))
	}
	c.MigrateOnStart = migrate

	if err := c.LogLevel.UnmarshalText([]byte(v.GetString(KeyLogLevel))); err != nil {
		errs = append(errs, fmt.Errorf("%s: %q is not debug, info, warn or error", KeyLogLevel, v.GetString(KeyLogLevel)))
	}
	trust, err := strconv.ParseBool(v.GetString(KeyTrustProxy))
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %q is not a boolean", KeyTrustProxy, v.GetString(KeyTrustProxy)))
	}
	c.TrustProxy = trust
	sandbox, err := strconv.ParseBool(v.GetString(KeySandbox))
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %q is not a boolean", KeySandbox, v.GetString(KeySandbox)))
	}
	c.PaymentsSandbox = sandbox
	if c.SetupToken != "" && len(c.SetupToken) < 16 {
		errs = append(errs, fmt.Errorf("%s: at least 16 characters (openssl rand -hex 16)", KeySetupToken))
	}
	if c.SecretKey != "" {
		if _, err := secretbox.Parse(c.SecretKey); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", KeySecretKey, err))
		}
	}
	if c.CookieSecure != "auto" && c.CookieSecure != "true" && c.CookieSecure != "false" {
		errs = append(errs, fmt.Errorf("%s: %q is not auto, true or false", KeyCookieSecure, c.CookieSecure))
	}
	if c.PublicBaseURL != "" {
		if u, err := url.Parse(c.PublicBaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s: %q is not an http(s) URL", KeyPublicBaseURL, c.PublicBaseURL))
		}
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		errs = append(errs, fmt.Errorf("%s: unsupported format %q (want json or text)", KeyLogFormat, c.LogFormat))
	}
	return c, errors.Join(errs...)
}

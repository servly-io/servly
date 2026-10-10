package app

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/platform/database"
)

var (
	ErrSetupComplete     = apperr.Conflict("setup_complete", "this installation is already set up; sign in instead")
	ErrSetupTokenInvalid = apperr.Forbidden("setup_token_invalid", "the setup token is not correct; copy it from the server log")
)

// NewSetupToken returns a one-time secret that claims a fresh installation,
// grouped for reading off a terminal: 4 groups of 5 hex digits (80 bits).
func NewSetupToken() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[0:5] + "-" + h[5:10] + "-" + h[10:15] + "-" + h[15:20]
}

// SetupRequired reports whether the installation still waits for its
// first-run setup: nobody has created a restaurant yet.
func (a *App) SetupRequired(ctx context.Context) (bool, error) {
	var n int
	err := a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM installation`).Scan(&n)
	return n == 0, err
}

// CompleteSetup is the first-run setup in the browser: it checks the setup
// token, creates the first restaurant and its owner, claims the installation
// and signs the owner in. Only one call ever succeeds; every later one fails
// with ErrSetupComplete.
func (a *App) CompleteSetup(ctx context.Context, token string, in CreateTenantInput, now time.Time) (*Principal, string, error) {
	required, err := a.SetupRequired(ctx)
	if err != nil {
		return nil, "", err
	}
	if !required {
		return nil, "", ErrSetupComplete
	}
	if a.SetupToken == "" || subtle.ConstantTimeCompare([]byte(normalizeSetupToken(token)), []byte(a.SetupToken)) != 1 {
		return nil, "", ErrSetupTokenInvalid
	}
	res, err := a.createTenant(ctx, in, true)
	if err != nil {
		return nil, "", err
	}
	a.Log.InfoContext(ctx, "installation set up in the browser", "tenant_id", res.Tenant.ID, "owner_id", res.Owner.ID)
	return a.Login(ctx, in.OwnerEmail, in.OwnerPassword, now)
}

func normalizeSetupToken(t string) string {
	return strings.ToLower(strings.Join(strings.Fields(t), ""))
}

// markInstalled records the installation as set up. A claim must be the
// first; otherwise an existing record is kept.
func markInstalled(ctx context.Context, tx *sql.Tx, now database.Timestamp, claim bool) error {
	q := `INSERT INTO installation (id, initialized_at) VALUES (1, $1)`
	if !claim {
		q += ` ON CONFLICT (id) DO NOTHING`
	}
	_, err := tx.ExecContext(ctx, q, now)
	if database.IsUniqueViolation(err) {
		return ErrSetupComplete
	}
	return err
}

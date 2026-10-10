package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/platform/database"
)

const (
	IdleTimeout     = 12 * time.Hour
	AbsoluteTimeout = 7 * 24 * time.Hour
	// touchEvery limits how often a busy session's last_seen is written.
	touchEvery = time.Minute
)

var (
	ErrUnauthenticated    = apperr.New(apperr.KindUnauthenticated, "unauthenticated", "sign in to continue")
	ErrInvalidCredentials = apperr.New(apperr.KindUnauthenticated, "invalid_credentials", "email or password is wrong")
	ErrCSRF               = apperr.Forbidden("csrf_failed", "missing or wrong CSRF token; reload the page")
	ErrNoTenant           = apperr.Forbidden("no_active_tenant", "choose a restaurant first")
)

// Session is a signed-in browser. The cookie holds the token; only its
// SHA-256 is stored.
type Session struct {
	TokenHash      string
	UserID         uuid.UUID
	ActiveTenantID uuid.NullUUID
	CSRFToken      string
	CreatedAt      database.Timestamp
	LastSeenAt     database.Timestamp
	ExpiresAt      database.Timestamp
}

// NewSession starts a session for userID and returns it with the cookie token.
func NewSession(userID uuid.UUID, now time.Time) (Session, string) {
	token := randomToken()
	ts := database.Timestamp{Time: now.UTC().Truncate(time.Microsecond)}
	return Session{
		TokenHash:  HashToken(token),
		UserID:     userID,
		CSRFToken:  randomToken(),
		CreatedAt:  ts,
		LastSeenAt: ts,
		ExpiresAt:  database.Timestamp{Time: ts.Add(IdleTimeout)},
	}, token
}

// Expired reports whether the session is no longer valid at now.
func (s *Session) Expired(now time.Time) bool { return !now.Before(s.ExpiresAt.Time) }

// NeedsTouch reports whether activity at now should be written back.
func (s *Session) NeedsTouch(now time.Time) bool { return now.Sub(s.LastSeenAt.Time) >= touchEvery }

// Touch extends the idle expiry, never past the absolute limit.
func (s *Session) Touch(now time.Time) {
	s.LastSeenAt = database.Timestamp{Time: now.UTC().Truncate(time.Microsecond)}
	exp := s.LastSeenAt.Add(IdleTimeout)
	if limit := s.CreatedAt.Add(AbsoluteTimeout); exp.After(limit) {
		exp = limit
	}
	s.ExpiresAt = database.Timestamp{Time: exp}
}

// HashToken is the stored form of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Sessions persists sessions.
type Sessions struct{ q database.Querier }

func NewSessions(q database.Querier) *Sessions { return &Sessions{q: q} }

func (s *Sessions) Create(ctx context.Context, ss *Session) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, active_tenant_id, csrf_token, created_at, last_seen_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ss.TokenHash, ss.UserID, ss.ActiveTenantID, ss.CSRFToken, ss.CreatedAt, ss.LastSeenAt, ss.ExpiresAt)
	return err
}

// Get returns the session for a token hash, or ErrUnauthenticated.
func (s *Sessions) Get(ctx context.Context, tokenHash string) (*Session, error) {
	var ss Session
	err := s.q.QueryRowContext(ctx,
		`SELECT token_hash, user_id, active_tenant_id, csrf_token, created_at, last_seen_at, expires_at
		   FROM sessions WHERE token_hash = $1`, tokenHash).
		Scan(&ss.TokenHash, &ss.UserID, &ss.ActiveTenantID, &ss.CSRFToken, &ss.CreatedAt, &ss.LastSeenAt, &ss.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	return &ss, err
}

// Update writes the activity times and the active tenant.
func (s *Sessions) Update(ctx context.Context, ss *Session) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE sessions SET active_tenant_id = $2, last_seen_at = $3, expires_at = $4 WHERE token_hash = $1`,
		ss.TokenHash, ss.ActiveTenantID, ss.LastSeenAt, ss.ExpiresAt)
	return err
}

func (s *Sessions) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Sessions) DeleteForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

// DeleteExpired removes sessions that expired before now.
func (s *Sessions) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < $1`, database.Timestamp{Time: now})
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Package dining holds dining locations: the tables and pickup points guests
// order from. Each has an unguessable public token, printed as a QR code.
package dining

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/platform/database"
)

type Kind string

const (
	KindTable  Kind = "table"
	KindPickup Kind = "pickup"
)

func (k Kind) Valid() bool { return k == KindTable || k == KindPickup }

// Location is a table or pickup point.
type Location struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	LocationID  uuid.UUID // the restaurant
	Name        string
	Kind        Kind
	PublicToken string
	IsEnabled   bool
	CreatedAt   database.Timestamp
	UpdatedAt   database.Timestamp
}

const MaxNameLen = 60

var (
	ErrNotFound    = apperr.NotFound("not_found", "not found")
	ErrInvalidKind = apperr.Invalid("invalid_kind", "kind must be table or pickup")
	ErrNameInvalid = apperr.Invalid("name_required", "name must be 1-60 characters")
)

// Validate checks name and kind.
func (l *Location) Validate() error {
	if n := utf8.RuneCountInString(strings.TrimSpace(l.Name)); n == 0 || n > MaxNameLen {
		return ErrNameInvalid
	}
	if !l.Kind.Valid() {
		return ErrInvalidKind
	}
	return nil
}

// MenuPath is the guest-facing path a QR code points to.
func MenuPath(token string) string { return "/m/" + token }

// Repository persists dining locations. GetByToken is the public lookup: the
// token is the only access check, so it takes no tenant id.
type Repository interface {
	List(ctx context.Context, tenantID, locationID uuid.UUID) ([]Location, error)
	Get(ctx context.Context, tenantID, id uuid.UUID) (*Location, error)
	GetByToken(ctx context.Context, token string) (*Location, error)
	Create(ctx context.Context, l *Location) error
	// Update writes name, enabled flag and token.
	Update(ctx context.Context, l *Location) error
}

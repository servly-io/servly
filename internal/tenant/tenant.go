// Package tenant holds the tenancy and access model: tenants, users, their
// memberships and roles, restaurant locations and public domains.
//
// Contract: docs/docs/domain-model.md and docs/docs/security.md.
package tenant

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
)

var (
	ErrNotFound        = apperr.NotFound("not_found", "not found")
	ErrSlugTaken       = apperr.Conflict("slug_taken", "a tenant with this slug already exists")
	ErrEmailTaken      = apperr.Conflict("email_taken", "a user with this email already exists")
	ErrAlreadyMember   = apperr.Conflict("already_member", "the user is already a member of this tenant")
	ErrInvalidSlug     = apperr.Invalid("invalid_slug", "slug must be 2-40 characters of a-z, 0-9 and -")
	ErrInvalidEmail    = apperr.Invalid("invalid_email", "email address is not valid")
	ErrInvalidTimezone = apperr.Invalid("invalid_timezone", "timezone must be an IANA name such as Europe/Berlin")
	ErrInvalidRole     = apperr.Invalid("invalid_role", "role must be owner, manager or staff")
	ErrNameRequired    = apperr.Invalid("name_required", "name must not be empty")
	ErrLastOwner       = apperr.Conflict("last_owner", "a tenant needs at least one active owner")
	ErrCurrencyFixed   = apperr.Conflict("currency_fixed", "the currency cannot change once orders exist")
)

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,38}[a-z0-9]$`)

// ValidSlug reports whether s is a usable tenant slug.
func ValidSlug(s string) bool { return slugRE.MatchString(s) }

// Slugify turns a name into a slug candidate ("Trattoria Roma" -> "trattoria-roma").
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimSuffix(b.String(), "-")
	if len(s) > 40 {
		s = strings.TrimSuffix(s[:40], "-")
	}
	return s
}

// NormalizeEmail lowercases and trims an address and checks its basic shape.
func NormalizeEmail(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	at := strings.LastIndexByte(e, '@')
	if at < 1 || at == len(e)-1 || len(e) > 254 || strings.ContainsAny(e, " \t\r\n") {
		return "", ErrInvalidEmail
	}
	return e, nil
}

// LoadTimezone validates an IANA timezone name.
func LoadTimezone(name string) (*time.Location, error) {
	if name == "" || name == "Local" {
		return nil, ErrInvalidTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrInvalidTimezone
	}
	return loc, nil
}

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
)

// Tenant is the customer/business account. It owns every other record.
type Tenant struct {
	ID        uuid.UUID
	Slug      string
	Name      string
	Status    Status
	CreatedAt database.Timestamp
	UpdatedAt database.Timestamp
}

type UserStatus string

const (
	UserActive   UserStatus = "active"
	UserDisabled UserStatus = "disabled"
)

// User is a staff or platform login. Users are global; a Membership grants
// access to one tenant.
type User struct {
	ID              uuid.UUID
	Email           string // stored lowercased
	PasswordHash    string
	DisplayName     string
	IsPlatformAdmin bool
	Status          UserStatus
	CreatedAt       database.Timestamp
	UpdatedAt       database.Timestamp
}

// Membership grants a user a role within one tenant.
type Membership struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Role      Role
	CreatedAt database.Timestamp
	UpdatedAt database.Timestamp
}

type LocationStatus string

const (
	LocationActive LocationStatus = "active"
	LocationClosed LocationStatus = "closed"
)

// Location is a physical restaurant. The MVP allows one per tenant.
type Location struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	Name                string
	Currency            money.Currency
	Timezone            string // IANA name; the business date of an order is taken in it
	OrderNotesEnabled   bool
	PublicStatusEnabled bool
	// AcceptingOrders is the restaurant-wide switch: off, guests can browse
	// the menu but not order.
	AcceptingOrders bool
	Status          LocationStatus
	CreatedAt       database.Timestamp
	UpdatedAt       database.Timestamp
}

// Domain maps a public hostname to a tenant. A hostname is a routing hint,
// never proof of access.
type Domain struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	LocationID uuid.NullUUID
	Hostname   string
	CreatedAt  database.Timestamp
}

// Member is a membership together with the user it belongs to, for staff
// management.
type Member struct {
	Membership
	Email       string
	DisplayName string
	UserStatus  UserStatus
}

// TenantMembership is a membership together with its tenant, for the session.
type TenantMembership struct {
	Membership
	TenantName   string
	TenantStatus Status
}

// Repository persists the tenancy model. Every method on tenant-owned data
// takes the tenant id explicitly; a record of another tenant is reported as
// ErrNotFound.
type Repository interface {
	CreateTenant(ctx context.Context, t *Tenant) error
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	UpdateTenant(ctx context.Context, t *Tenant) error

	CreateUser(ctx context.Context, u *User) error
	GetUser(ctx context.Context, id uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	// UpdateUser writes display name, password hash and status.
	UpdateUser(ctx context.Context, u *User) error

	CreateMembership(ctx context.Context, m *Membership) error
	GetMembership(ctx context.Context, tenantID, userID uuid.UUID) (*Membership, error)
	// Memberships lists the tenants a user may act in.
	Memberships(ctx context.Context, userID uuid.UUID) ([]TenantMembership, error)
	ListMembers(ctx context.Context, tenantID uuid.UUID) ([]Member, error)
	UpdateMembership(ctx context.Context, m *Membership) error
	// CountActiveOwners counts owners whose user is active.
	CountActiveOwners(ctx context.Context, tenantID uuid.UUID) (int, error)

	CreateLocation(ctx context.Context, l *Location) error
	GetLocation(ctx context.Context, tenantID, id uuid.UUID) (*Location, error)
	// PrimaryLocation is the tenant's first location; the MVP has exactly one.
	PrimaryLocation(ctx context.Context, tenantID uuid.UUID) (*Location, error)
	UpdateLocation(ctx context.Context, l *Location) error
}

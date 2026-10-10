package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/audit"
	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

// CreateTenantInput describes a new restaurant and its first owner.
type CreateTenantInput struct {
	Name          string
	Slug          string // derived from Name when empty
	Currency      string
	Timezone      string
	OwnerEmail    string
	OwnerName     string
	OwnerPassword string // required unless the owner already exists
}

// CreateTenantResult names what was created.
type CreateTenantResult struct {
	Tenant      tenant.Tenant
	Location    tenant.Location
	Owner       tenant.User
	OwnerExists bool // the owner was an existing user and kept their password
}

// CreateTenant provisions a tenant with one location and an owner. An
// existing user with the owner's email becomes the owner of the new tenant.
func (a *App) CreateTenant(ctx context.Context, in CreateTenantInput) (*CreateTenantResult, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, tenant.ErrNameRequired
	}
	slug := in.Slug
	if slug == "" {
		slug = tenant.Slugify(name)
	}
	if !tenant.ValidSlug(slug) {
		return nil, tenant.ErrInvalidSlug
	}
	currency, err := money.ParseCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if _, err := tenant.LoadTimezone(in.Timezone); err != nil {
		return nil, err
	}
	email, err := tenant.NormalizeEmail(in.OwnerEmail)
	if err != nil {
		return nil, err
	}

	now := database.Now()
	res := &CreateTenantResult{
		Tenant: tenant.Tenant{ID: ids.New(), Slug: slug, Name: name, Status: tenant.StatusActive, CreatedAt: now, UpdatedAt: now},
	}
	res.Location = tenant.Location{
		ID: ids.New(), TenantID: res.Tenant.ID, Name: name, Currency: currency, Timezone: in.Timezone,
		OrderNotesEnabled: true, PublicStatusEnabled: true, Status: tenant.LocationActive, CreatedAt: now, UpdatedAt: now,
	}

	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		if err := st.CreateTenant(ctx, &res.Tenant); err != nil {
			return err
		}
		if err := st.CreateLocation(ctx, &res.Location); err != nil {
			return err
		}
		owner, err := st.GetUserByEmail(ctx, email)
		switch {
		case err == nil:
			res.OwnerExists = true
		case errors.Is(err, tenant.ErrNotFound):
			hash, err := auth.HashPassword(in.OwnerPassword)
			if err != nil {
				return err
			}
			displayName := strings.TrimSpace(in.OwnerName)
			if displayName == "" {
				displayName = email[:strings.IndexByte(email, '@')]
			}
			owner = &tenant.User{ID: ids.New(), Email: email, PasswordHash: hash, DisplayName: displayName,
				Status: tenant.UserActive, CreatedAt: now, UpdatedAt: now}
			if err := st.CreateUser(ctx, owner); err != nil {
				return err
			}
		default:
			return err
		}
		res.Owner = *owner
		if err := st.CreateMembership(ctx, &tenant.Membership{ID: ids.New(), TenantID: res.Tenant.ID, UserID: owner.ID,
			Role: tenant.RoleOwner, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		tid, _ := audit.Tenant(res.Tenant.ID, uuid.Nil)
		return audit.Record(ctx, tx, audit.Event{TenantID: tid, Action: "tenant.created", EntityType: "tenant",
			EntityID: res.Tenant.ID, Data: map[string]any{"slug": slug, "owner_email": email}})
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// SetPassword sets a user's password and signs them out everywhere. It is the
// operator's recovery path (`servly user set-password`).
func (a *App) SetPassword(ctx context.Context, email, password string) error {
	email, err := tenant.NormalizeEmail(email)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		u, err := st.GetUserByEmail(ctx, email)
		if err != nil {
			return err
		}
		u.PasswordHash = hash
		u.UpdatedAt = database.Now()
		if err := st.UpdateUser(ctx, u); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, u.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorUserID: uuid.NullUUID{}, Action: "user.password_set",
			EntityType: "user", EntityID: u.ID})
	})
}

// LookupUser finds a user by email.
func (a *App) LookupUser(ctx context.Context, email string) (*tenant.User, error) {
	email, err := tenant.NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return tenantsql.New(a.DB).GetUserByEmail(ctx, email)
}

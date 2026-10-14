package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/apperr"
	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

// SettingsInput patches the restaurant settings; nil fields are unchanged.
type SettingsInput struct {
	Name                *string
	Currency            *string
	Timezone            *string
	OrderNotesEnabled   *bool
	PublicStatusEnabled *bool
	AcceptingOrders     *bool
}

// Settings returns the active location.
func (a *App) Settings(ctx context.Context, p *Principal) (*tenant.Location, error) {
	return tenantsql.New(a.DB).GetLocation(ctx, p.TenantID(), p.Location.ID)
}

// UpdateSettings changes the restaurant settings. The name is the tenant's
// and the location's (the MVP has one location). The currency is fixed once
// an order exists, because orders store amounts in it.
func (a *App) UpdateSettings(ctx context.Context, p *Principal, in SettingsInput) (*tenant.Location, error) {
	var loc *tenant.Location
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		var err error
		if loc, err = st.GetLocation(ctx, p.TenantID(), p.Location.ID); err != nil {
			return err
		}
		now := database.Now()
		changed := map[string]any{}
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if name == "" {
				return tenant.ErrNameRequired
			}
			t, err := st.GetTenant(ctx, p.TenantID())
			if err != nil {
				return err
			}
			t.Name, t.UpdatedAt = name, now
			if err := st.UpdateTenant(ctx, t); err != nil {
				return err
			}
			loc.Name = name
			changed["name"] = name
		}
		if in.Currency != nil && *in.Currency != string(loc.Currency) {
			c, err := money.ParseCurrency(*in.Currency)
			if err != nil {
				return err
			}
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders WHERE tenant_id = $1 AND location_id = $2`,
				loc.TenantID, loc.ID).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return tenant.ErrCurrencyFixed
			}
			loc.Currency = c
			changed["currency"] = c
		}
		if in.Timezone != nil {
			if _, err := tenant.LoadTimezone(*in.Timezone); err != nil {
				return err
			}
			loc.Timezone = *in.Timezone
			changed["timezone"] = loc.Timezone
		}
		if in.OrderNotesEnabled != nil {
			loc.OrderNotesEnabled = *in.OrderNotesEnabled
			changed["order_notes_enabled"] = loc.OrderNotesEnabled
		}
		if in.PublicStatusEnabled != nil {
			loc.PublicStatusEnabled = *in.PublicStatusEnabled
			changed["public_status_enabled"] = loc.PublicStatusEnabled
		}
		if in.AcceptingOrders != nil {
			loc.AcceptingOrders = *in.AcceptingOrders
			changed["accepting_orders"] = loc.AcceptingOrders
		}
		loc.UpdatedAt = now
		if err := st.UpdateLocation(ctx, loc); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "settings.updated", "restaurant_location", loc.ID, changed)
	})
	if err != nil {
		return nil, err
	}
	return loc, nil
}

func (a *App) ListStaff(ctx context.Context, p *Principal) ([]tenant.Member, error) {
	return tenantsql.New(a.DB).ListMembers(ctx, p.TenantID())
}

// AddStaffInput adds a member. A new user needs a password; an existing user
// (by email) keeps theirs and Password must be empty.
type AddStaffInput struct {
	Email       string
	DisplayName string
	Role        tenant.Role
	Password    string
}

// ErrExistingUserPassword is returned when a password is given for a user
// who already exists: another restaurant's account must not be taken over.
var ErrExistingUserPassword = apperr.Invalid("existing_user", "this person already has a servly sign-in; add them without a password")

func (a *App) AddStaff(ctx context.Context, p *Principal, in AddStaffInput) (*tenant.Member, error) {
	if !in.Role.Valid() {
		return nil, tenant.ErrInvalidRole
	}
	email, err := tenant.NormalizeEmail(in.Email)
	if err != nil {
		return nil, err
	}
	var m *tenant.Member
	err = a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		now := database.Now()
		u, err := st.GetUserByEmail(ctx, email)
		switch {
		case err == nil:
			if in.Password != "" {
				return ErrExistingUserPassword
			}
		case errors.Is(err, tenant.ErrNotFound):
			hash, err := auth.HashPassword(in.Password)
			if err != nil {
				return err
			}
			name := strings.TrimSpace(in.DisplayName)
			if name == "" {
				name = email[:strings.IndexByte(email, '@')]
			}
			u = &tenant.User{ID: ids.New(), Email: email, PasswordHash: hash, DisplayName: name, Status: tenant.UserActive, CreatedAt: now, UpdatedAt: now}
			if err := st.CreateUser(ctx, u); err != nil {
				return err
			}
		default:
			return err
		}
		ms := tenant.Membership{ID: ids.New(), TenantID: p.TenantID(), UserID: u.ID, Role: in.Role, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateMembership(ctx, &ms); err != nil {
			return err
		}
		m = &tenant.Member{Membership: ms, Email: u.Email, DisplayName: u.DisplayName, UserStatus: u.Status}
		return a.audit(ctx, tx, p, "staff.added", "user", u.ID, map[string]any{"email": email, "role": in.Role})
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateStaffInput changes a member; nil fields are unchanged.
type UpdateStaffInput struct {
	Role     *tenant.Role
	Password *string // only for users who belong to this tenant alone
}

// ErrSharedUserPassword protects users who also belong to other restaurants:
// only they, or the operator, can change their password.
var ErrSharedUserPassword = apperr.Invalid("shared_user", "this person also works for another restaurant; they must change their password themselves")

func (a *App) UpdateStaff(ctx context.Context, p *Principal, userID uuid.UUID, in UpdateStaffInput) (*tenant.Member, error) {
	var out *tenant.Member
	err := a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		m, err := st.GetMembership(ctx, p.TenantID(), userID)
		if err != nil {
			return err
		}
		now := database.Now()
		if in.Role != nil && *in.Role != m.Role {
			if !in.Role.Valid() {
				return tenant.ErrInvalidRole
			}
			if m.Role == tenant.RoleOwner {
				if err := a.keepOneOwner(ctx, st, p.TenantID()); err != nil {
					return err
				}
			}
			m.Role, m.UpdatedAt = *in.Role, now
			if err := st.UpdateMembership(ctx, m); err != nil {
				return err
			}
			if err := a.audit(ctx, tx, p, "staff.role_changed", "user", userID, map[string]any{"role": m.Role}); err != nil {
				return err
			}
		}
		u, err := st.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		if in.Password != nil {
			ms, err := st.Memberships(ctx, userID)
			if err != nil {
				return err
			}
			if len(ms) != 1 {
				return ErrSharedUserPassword
			}
			if u.PasswordHash, err = auth.HashPassword(*in.Password); err != nil {
				return err
			}
			u.UpdatedAt = now
			if err := st.UpdateUser(ctx, u); err != nil {
				return err
			}
			if err := auth.NewSessions(tx).DeleteForUser(ctx, userID); err != nil {
				return err
			}
			if err := a.audit(ctx, tx, p, "staff.password_set", "user", userID, nil); err != nil {
				return err
			}
		}
		out = &tenant.Member{Membership: *m, Email: u.Email, DisplayName: u.DisplayName, UserStatus: u.Status}
		return nil
	})
	return out, err
}

// RemoveStaff ends a membership. The user's sign-in survives for their other
// restaurants; their sessions end so the removal takes effect at once.
func (a *App) RemoveStaff(ctx context.Context, p *Principal, userID uuid.UUID) error {
	return a.DB.InTx(ctx, func(tx *sql.Tx) error {
		st := tenantsql.New(tx)
		m, err := st.GetMembership(ctx, p.TenantID(), userID)
		if err != nil {
			return err
		}
		if m.Role == tenant.RoleOwner {
			if err := a.keepOneOwner(ctx, st, p.TenantID()); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tenant_memberships WHERE tenant_id = $1 AND id = $2`, m.TenantID, m.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET active_tenant_id = NULL WHERE user_id = $1 AND active_tenant_id = $2`,
			userID, m.TenantID); err != nil {
			return err
		}
		return a.audit(ctx, tx, p, "staff.removed", "user", userID, nil)
	})
}

// keepOneOwner fails when the tenant would be left without an active owner
// after one owner is demoted or removed.
func (a *App) keepOneOwner(ctx context.Context, st *tenantsql.Store, tenantID uuid.UUID) error {
	n, err := st.CountActiveOwners(ctx, tenantID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return tenant.ErrLastOwner
	}
	return nil
}

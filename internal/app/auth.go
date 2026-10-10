package app

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

// Principal is the authenticated staff member of a request, with the tenant
// and location they act in. Tenant and Location are nil until a tenant is
// active.
type Principal struct {
	Session     auth.Session
	User        tenant.User
	Memberships []tenant.TenantMembership
	Tenant      *tenant.TenantMembership
	Location    *tenant.Location
}

// Can reports whether the principal holds p in the active tenant.
func (p *Principal) Can(perm tenant.Permission) bool {
	return p.Tenant != nil && p.Tenant.Role.Can(perm)
}

// TenantID is the active tenant's id; callers check Tenant != nil first.
func (p *Principal) TenantID() uuid.UUID { return p.Tenant.TenantID }

// Login checks credentials and starts a session. Unknown email, disabled user
// and wrong password are indistinguishable, also in timing.
func (a *App) Login(ctx context.Context, email, password string, now time.Time) (*Principal, string, error) {
	email, err := tenant.NormalizeEmail(email)
	if err != nil {
		auth.VerifyDummy(password)
		return nil, "", auth.ErrInvalidCredentials
	}
	st := tenantsql.New(a.DB)
	u, err := st.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, tenant.ErrNotFound):
		auth.VerifyDummy(password)
		return nil, "", auth.ErrInvalidCredentials
	case err != nil:
		return nil, "", err
	}
	if !auth.VerifyPassword(u.PasswordHash, password) || u.Status != tenant.UserActive {
		return nil, "", auth.ErrInvalidCredentials
	}

	ms, err := st.Memberships(ctx, u.ID)
	if err != nil {
		return nil, "", err
	}
	sess, token := auth.NewSession(u.ID, now)
	// One usable restaurant: select it. Several: the user picks.
	if active := activeMemberships(ms); len(active) == 1 {
		sess.ActiveTenantID = uuid.NullUUID{UUID: active[0].TenantID, Valid: true}
	}
	if err := auth.NewSessions(a.DB).Create(ctx, &sess); err != nil {
		return nil, "", err
	}
	p, err := a.principal(ctx, &sess, u, ms)
	if err != nil {
		return nil, "", err
	}
	a.Log.InfoContext(ctx, "staff signed in", "user_id", u.ID)
	return p, token, nil
}

func activeMemberships(ms []tenant.TenantMembership) []tenant.TenantMembership {
	var out []tenant.TenantMembership
	for _, m := range ms {
		if m.TenantStatus == tenant.StatusActive {
			out = append(out, m)
		}
	}
	return out
}

// Authenticate resolves a session token into a principal. It re-reads the
// user and memberships every time, so disabling a user or removing a
// membership takes effect on the next request.
func (a *App) Authenticate(ctx context.Context, token string, now time.Time) (*Principal, error) {
	if token == "" {
		return nil, auth.ErrUnauthenticated
	}
	sessions := auth.NewSessions(a.DB)
	sess, err := sessions.Get(ctx, auth.HashToken(token))
	if err != nil {
		return nil, err
	}
	if sess.Expired(now) {
		_ = sessions.Delete(ctx, sess.TokenHash)
		return nil, auth.ErrUnauthenticated
	}
	st := tenantsql.New(a.DB)
	u, err := st.GetUser(ctx, sess.UserID)
	if err != nil || u.Status != tenant.UserActive {
		_ = sessions.Delete(ctx, sess.TokenHash)
		return nil, auth.ErrUnauthenticated
	}
	ms, err := st.Memberships(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if sess.NeedsTouch(now) {
		sess.Touch(now)
		if err := sessions.Update(ctx, sess); err != nil {
			return nil, err
		}
	}
	return a.principal(ctx, sess, u, ms)
}

// principal assembles a Principal; an active tenant that is no longer usable
// (membership removed, tenant suspended) is simply not active.
func (a *App) principal(ctx context.Context, sess *auth.Session, u *tenant.User, ms []tenant.TenantMembership) (*Principal, error) {
	p := &Principal{Session: *sess, User: *u, Memberships: ms}
	if !sess.ActiveTenantID.Valid {
		return p, nil
	}
	for i := range ms {
		if ms[i].TenantID == sess.ActiveTenantID.UUID && ms[i].TenantStatus == tenant.StatusActive {
			loc, err := tenantsql.New(a.DB).PrimaryLocation(ctx, ms[i].TenantID)
			if err != nil {
				return nil, err
			}
			p.Tenant, p.Location = &ms[i], loc
			break
		}
	}
	return p, nil
}

// Logout ends the session.
func (a *App) Logout(ctx context.Context, p *Principal) error {
	return auth.NewSessions(a.DB).Delete(ctx, p.Session.TokenHash)
}

// SwitchTenant makes tenantID the session's active tenant. A tenant the user
// is not an active member of is reported as not found.
func (a *App) SwitchTenant(ctx context.Context, p *Principal, tenantID uuid.UUID) (*Principal, error) {
	found := false
	for _, m := range p.Memberships {
		if m.TenantID == tenantID && m.TenantStatus == tenant.StatusActive {
			found = true
		}
	}
	if !found {
		return nil, tenant.ErrNotFound
	}
	sess := p.Session
	sess.ActiveTenantID = uuid.NullUUID{UUID: tenantID, Valid: true}
	if err := a.DB.InTx(ctx, func(tx *sql.Tx) error { return auth.NewSessions(tx).Update(ctx, &sess) }); err != nil {
		return nil, err
	}
	u := p.User
	return a.principal(ctx, &sess, &u, p.Memberships)
}

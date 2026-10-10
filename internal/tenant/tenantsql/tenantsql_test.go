package tenantsql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/servly-io/servly/internal/ids"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

func seed(t *testing.T, st *tenantsql.Store, slug string) (*tenant.Tenant, *tenant.Location) {
	t.Helper()
	ctx := context.Background()
	now := database.Now()
	tn := &tenant.Tenant{ID: ids.New(), Slug: slug, Name: slug, Status: tenant.StatusActive, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateTenant(ctx, tn); err != nil {
		t.Fatal(err)
	}
	loc := &tenant.Location{ID: ids.New(), TenantID: tn.ID, Name: slug, Currency: "EUR", Timezone: "Europe/Berlin",
		OrderNotesEnabled: true, Status: tenant.LocationActive, CreatedAt: now, UpdatedAt: now}
	if err := st.CreateLocation(ctx, loc); err != nil {
		t.Fatal(err)
	}
	return tn, loc
}

func TestTenantStore(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		st := tenantsql.New(db)
		a, locA := seed(t, st, "alpha")
		b, locB := seed(t, st, "beta")

		dup := *a
		dup.ID = ids.New()
		if err := st.CreateTenant(ctx, &dup); !errors.Is(err, tenant.ErrSlugTaken) {
			t.Fatalf("duplicate slug: %v", err)
		}

		now := database.Now()
		u := &tenant.User{ID: ids.New(), Email: "anna@example.com", PasswordHash: "h", DisplayName: "Anna",
			Status: tenant.UserActive, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		u2 := *u
		u2.ID = ids.New()
		if err := st.CreateUser(ctx, &u2); !errors.Is(err, tenant.ErrEmailTaken) {
			t.Fatalf("duplicate email: %v", err)
		}
		got, err := st.GetUserByEmail(ctx, "anna@example.com")
		if err != nil || got.ID != u.ID || got.DisplayName != "Anna" || !got.CreatedAt.Equal(now.Time) {
			t.Fatalf("GetUserByEmail: %+v %v", got, err)
		}

		m := &tenant.Membership{ID: ids.New(), TenantID: a.ID, UserID: u.ID, Role: tenant.RoleOwner, CreatedAt: now, UpdatedAt: now}
		if err := st.CreateMembership(ctx, m); err != nil {
			t.Fatal(err)
		}
		if err := st.CreateMembership(ctx, &tenant.Membership{ID: ids.New(), TenantID: a.ID, UserID: u.ID, Role: tenant.RoleStaff, CreatedAt: now, UpdatedAt: now}); !errors.Is(err, tenant.ErrAlreadyMember) {
			t.Fatalf("duplicate membership: %v", err)
		}
		ms, err := st.Memberships(ctx, u.ID)
		if err != nil || len(ms) != 1 || ms[0].TenantName != "alpha" || ms[0].Role != tenant.RoleOwner {
			t.Fatalf("Memberships: %+v %v", ms, err)
		}
		if _, err := st.GetMembership(ctx, b.ID, u.ID); !errors.Is(err, tenant.ErrNotFound) {
			t.Fatalf("membership in other tenant: %v", err)
		}
		if n, err := st.CountActiveOwners(ctx, a.ID); err != nil || n != 1 {
			t.Fatalf("owners: %d %v", n, err)
		}
		u.Status = tenant.UserDisabled
		u.UpdatedAt = database.Now()
		if err := st.UpdateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if n, _ := st.CountActiveOwners(ctx, a.ID); n != 0 {
			t.Fatalf("disabled owner counted: %d", n)
		}

		// Locations are tenant-scoped: another tenant's id is not found,
		// for reading and for writing.
		if _, err := st.GetLocation(ctx, a.ID, locB.ID); !errors.Is(err, tenant.ErrNotFound) {
			t.Fatalf("cross-tenant GetLocation: %v", err)
		}
		hijack := *locB
		hijack.TenantID = a.ID
		hijack.Name = "pwned"
		if err := st.UpdateLocation(ctx, &hijack); !errors.Is(err, tenant.ErrNotFound) {
			t.Fatalf("cross-tenant UpdateLocation: %v", err)
		}
		if l, _ := st.GetLocation(ctx, b.ID, locB.ID); l.Name != "beta" {
			t.Fatalf("other tenant's location changed: %q", l.Name)
		}
		p, err := st.PrimaryLocation(ctx, a.ID)
		if err != nil || p.ID != locA.ID || p.Currency != "EUR" || !p.OrderNotesEnabled || p.PublicStatusEnabled {
			t.Fatalf("PrimaryLocation: %+v %v", p, err)
		}
	})
}

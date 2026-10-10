package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/auth"
	"github.com/servly-io/servly/internal/money"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/tenant"
	"github.com/servly-io/servly/internal/tenant/tenantsql"
)

func newApp(t *testing.T, db *database.DB) *app.App {
	t.Helper()
	return app.New(db, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

var roma = app.CreateTenantInput{
	Name: "Trattoria Roma", Currency: "EUR", Timezone: "Europe/Berlin",
	OwnerEmail: " Anna@Example.com ", OwnerPassword: "correct horse battery",
}

func TestCreateTenant(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		a := newApp(t, db)
		res, err := a.CreateTenant(ctx, roma)
		if err != nil {
			t.Fatal(err)
		}
		if res.Tenant.Slug != "trattoria-roma" || res.Owner.Email != "anna@example.com" || res.Owner.DisplayName != "anna" ||
			res.Location.Currency != money.Currency("EUR") || res.OwnerExists {
			t.Fatalf("result: %+v", res)
		}
		if !auth.VerifyPassword(res.Owner.PasswordHash, "correct horse battery") {
			t.Fatal("owner password not stored")
		}

		// A second restaurant for the same owner reuses the user.
		second := roma
		second.Name, second.OwnerPassword = "Pizzeria Due", ""
		res2, err := a.CreateTenant(ctx, second)
		if err != nil {
			t.Fatal(err)
		}
		if !res2.OwnerExists || res2.Owner.ID != res.Owner.ID {
			t.Fatalf("existing owner not reused: %+v", res2)
		}
		ms, _ := tenantsql.New(db).Memberships(ctx, res.Owner.ID)
		if len(ms) != 2 {
			t.Fatalf("memberships: %+v", ms)
		}

		if _, err := a.CreateTenant(ctx, roma); !errors.Is(err, tenant.ErrSlugTaken) {
			t.Fatalf("duplicate: %v", err)
		}
	})
}

func TestCreateTenantValidation(t *testing.T) {
	db := dbtest.Open(t, database.SQLite)
	a := newApp(t, db)
	cases := map[string]struct {
		mod  func(*app.CreateTenantInput)
		want error
	}{
		"no name":      {func(in *app.CreateTenantInput) { in.Name = " " }, tenant.ErrNameRequired},
		"bad slug":     {func(in *app.CreateTenantInput) { in.Slug = "Bad Slug" }, tenant.ErrInvalidSlug},
		"bad currency": {func(in *app.CreateTenantInput) { in.Currency = "euro" }, money.ErrInvalidCurrency},
		"bad timezone": {func(in *app.CreateTenantInput) { in.Timezone = "Mars/Olympus" }, tenant.ErrInvalidTimezone},
		"bad email":    {func(in *app.CreateTenantInput) { in.OwnerEmail = "anna" }, tenant.ErrInvalidEmail},
		"weak pw":      {func(in *app.CreateTenantInput) { in.OwnerPassword = "short" }, auth.ErrPasswordTooShort},
	}
	for name, c := range cases {
		in := roma
		c.mod(&in)
		if _, err := a.CreateTenant(context.Background(), in); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	// Nothing half-created: the weak-password case rolled back its tenant.
	if _, err := a.CreateTenant(context.Background(), roma); err != nil {
		t.Fatalf("valid create after failures: %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	db := dbtest.Open(t, database.SQLite)
	ctx := context.Background()
	a := newApp(t, db)
	if _, err := a.CreateTenant(ctx, roma); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPassword(ctx, "ANNA@example.com", "a brand new secret"); err != nil {
		t.Fatal(err)
	}
	u, _ := a.LookupUser(ctx, "anna@example.com")
	if !auth.VerifyPassword(u.PasswordHash, "a brand new secret") {
		t.Fatal("password not changed")
	}
	if err := a.SetPassword(ctx, "nobody@example.com", "a brand new secret"); !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}

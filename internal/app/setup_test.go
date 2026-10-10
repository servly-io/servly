package app_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/servly-io/servly/internal/app"
	"github.com/servly-io/servly/internal/platform/database"
	"github.com/servly-io/servly/internal/platform/database/dbtest"
	"github.com/servly-io/servly/internal/tenant"
)

const setupToken = "0a1b2-c3d4e-5f6a7-b8c9d"

func TestCompleteSetup(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := context.Background()
		a := newApp(t, db)
		a.SetupToken = setupToken
		if req, err := a.SetupRequired(ctx); err != nil || !req {
			t.Fatalf("fresh database: required=%v err=%v", req, err)
		}
		if _, _, err := a.CompleteSetup(ctx, "wrong", roma, time.Now()); !errors.Is(err, app.ErrSetupTokenInvalid) {
			t.Fatalf("wrong token: %v", err)
		}
		// Case and stray whitespace from copying the log line do not matter.
		p, token, err := a.CompleteSetup(ctx, " 0A1B2-C3D4E-5F6A7-B8C9D\n", roma, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if token == "" || p.Tenant == nil || p.Tenant.Role != tenant.RoleOwner || p.Tenant.TenantName != "Trattoria Roma" {
			t.Fatalf("principal: %+v", p)
		}
		if req, _ := a.SetupRequired(ctx); req {
			t.Fatal("still required after setup")
		}
		second := roma
		second.Name, second.OwnerEmail = "Pizzeria Due", "mallory@example.com"
		if _, _, err := a.CompleteSetup(ctx, setupToken, second, time.Now()); !errors.Is(err, app.ErrSetupComplete) {
			t.Fatalf("second setup: %v", err)
		}
	})
}

func TestCompleteSetupRace(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		a := newApp(t, db)
		a.SetupToken = setupToken
		inputs := []app.CreateTenantInput{roma, roma}
		inputs[1].Name, inputs[1].OwnerEmail = "Pizzeria Due", "mallory@example.com"
		errs := make([]error, len(inputs))
		var wg sync.WaitGroup
		for i, in := range inputs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, errs[i] = a.CompleteSetup(context.Background(), setupToken, in, time.Now())
			}()
		}
		wg.Wait()
		ok := 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case !errors.Is(err, app.ErrSetupComplete):
				t.Fatalf("losing claim: %v", err)
			}
		}
		var tenants int
		_ = db.QueryRow(`SELECT COUNT(*) FROM tenants`).Scan(&tenants)
		if ok != 1 || tenants != 1 {
			t.Fatalf("successful claims %d, tenants %d; want 1 and 1", ok, tenants)
		}
	})
}

func TestCreateTenantClosesSetup(t *testing.T) {
	db := dbtest.Open(t, database.SQLite)
	ctx := context.Background()
	a := newApp(t, db)
	a.SetupToken = setupToken
	if _, err := a.CreateTenant(ctx, roma); err != nil {
		t.Fatal(err)
	}
	if req, _ := a.SetupRequired(ctx); req {
		t.Fatal("setup still required after tenant create")
	}
	// A second tenant from the CLI is fine; the installation stays claimed.
	second := roma
	second.Name, second.OwnerPassword = "Pizzeria Due", ""
	if _, err := a.CreateTenant(ctx, second); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.CompleteSetup(ctx, setupToken, roma, time.Now()); !errors.Is(err, app.ErrSetupComplete) {
		t.Fatalf("setup after tenant create: %v", err)
	}
}

func TestSetupClosedWithoutToken(t *testing.T) {
	a := newApp(t, dbtest.Open(t, database.SQLite))
	if _, _, err := a.CompleteSetup(context.Background(), "", roma, time.Now()); !errors.Is(err, app.ErrSetupTokenInvalid) {
		t.Fatalf("no token configured: %v", err)
	}
}

// A configured token (SERVLY_SETUP_TOKEN) is compared like a generated one.
func TestConfiguredSetupToken(t *testing.T) {
	a := newApp(t, dbtest.Open(t, database.SQLite))
	a.SetupToken = app.NormalizeSetupToken(" Demo-Setup-Token-0001 ")
	if err := a.CheckSetupToken(context.Background(), "demo-setup-token-0001"); err != nil {
		t.Fatalf("configured token: %v", err)
	}
	if err := a.CheckSetupToken(context.Background(), "demo-setup-token-0002"); !errors.Is(err, app.ErrSetupTokenInvalid) {
		t.Fatalf("wrong token: %v", err)
	}
}

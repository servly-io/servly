package tenant

import "testing"

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		perm                  Permission
		staff, manager, owner bool
	}{
		{PermViewOrders, true, true, true},
		{PermUpdateOrderStatus, true, true, true},
		{PermRecordPayment, true, true, true},
		{PermCancelStartedOrder, false, true, true},
		{PermRefundPayment, false, true, true},
		{PermManageMenu, false, true, true},
		{PermManageDiningLocations, false, true, true},
		{PermManageStaff, false, false, true},
		{PermManageSettings, false, false, true},
	}
	for _, c := range cases {
		for role, want := range map[Role]bool{RoleStaff: c.staff, RoleManager: c.manager, RoleOwner: c.owner} {
			if got := role.Can(c.perm); got != want {
				t.Errorf("%s.Can(%s) = %v, want %v", role, c.perm, got, want)
			}
		}
	}
}

func TestUnknownRoleGrantsNothing(t *testing.T) {
	r := Role("admin")
	if r.Valid() || r.Can(PermViewOrders) {
		t.Fatal("unknown role must be invalid and grant nothing")
	}
}

func TestSlugAndEmail(t *testing.T) {
	if got := Slugify("  Trattoria Roma - Café #1 "); got != "trattoria-roma-caf-1" {
		t.Errorf("Slugify = %q", got)
	}
	for _, s := range []string{"ab", "roma-1", "a1"} {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false", s)
		}
	}
	for _, s := range []string{"a", "-ab", "ab-", "AB", "a b", "a_b"} {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true", s)
		}
	}
	if e, err := NormalizeEmail(" Anna@Example.COM "); err != nil || e != "anna@example.com" {
		t.Errorf("NormalizeEmail = %q %v", e, err)
	}
	for _, s := range []string{"", "anna", "@x", "anna@", "a b@c"} {
		if _, err := NormalizeEmail(s); err == nil {
			t.Errorf("NormalizeEmail(%q) accepted", s)
		}
	}
}

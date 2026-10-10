package tenant

import "slices"

// Role is a user's role within one tenant. Platform administration is a
// separate flag on User, not a role.
type Role string

const (
	RoleOwner   Role = "owner"
	RoleManager Role = "manager"
	RoleStaff   Role = "staff"
)

// Permission is one protected capability within a tenant.
type Permission string

const (
	PermViewOrders            Permission = "orders.view"
	PermUpdateOrderStatus     Permission = "orders.update_status"
	PermCancelStartedOrder    Permission = "orders.cancel_started" // cancel once preparation began
	PermRecordPayment         Permission = "payments.record"
	PermRefundPayment         Permission = "payments.refund"
	PermManageMenu            Permission = "menu.manage"
	PermManageDiningLocations Permission = "dining_locations.manage"
	PermManageStaff           Permission = "staff.manage"
	PermManageSettings        Permission = "settings.manage"
)

var staffPerms = []Permission{
	PermViewOrders,
	PermUpdateOrderStatus,
	PermRecordPayment,
}

var managerPerms = append(slices.Clone(staffPerms),
	PermCancelStartedOrder,
	PermRefundPayment,
	PermManageMenu,
	PermManageDiningLocations,
)

var ownerPerms = append(slices.Clone(managerPerms),
	PermManageStaff,
	PermManageSettings,
)

var rolePerms = map[Role][]Permission{
	RoleStaff:   staffPerms,
	RoleManager: managerPerms,
	RoleOwner:   ownerPerms,
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	_, ok := rolePerms[r]
	return ok
}

// Can reports whether the role grants p. Unknown roles grant nothing.
func (r Role) Can(p Permission) bool {
	return slices.Contains(rolePerms[r], p)
}

// PermissionsOf lists what a role grants, for the UI to hide what it cannot use.
func PermissionsOf(r Role) []Permission { return slices.Clone(rolePerms[r]) }

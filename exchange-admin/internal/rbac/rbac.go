package rbac

// Permission represents a single allowed action in the admin system.
type Permission string

const (
	// PermissionAll grants all permissions.
	PermissionAll Permission = "*"

	// Common read-only permissions.
	PermissionDashboardRead Permission = "dashboard:read"
	PermissionAuditRead     Permission = "audit:read"

	// Operations permissions.
	PermissionUserRead  Permission = "user:read"
	PermissionUserWrite Permission = "user:write"
	PermissionOrderRead Permission = "order:read"

	// Support permissions.
	PermissionTicketRead  Permission = "ticket:read"
	PermissionTicketWrite Permission = "ticket:write"
)

// Role is a named collection of permissions.
type Role struct {
	Name        string
	Permissions map[Permission]struct{}
}

func NewRole(name string, perms ...Permission) Role {
	m := make(map[Permission]struct{}, len(perms))
	for _, p := range perms {
		m[p] = struct{}{}
	}
	return Role{Name: name, Permissions: m}
}

var (
	// SuperAdmin has full access.
	SuperAdmin = NewRole("SuperAdmin", PermissionAll)

	// Operator can perform core operational actions.
	Operator = NewRole(
		"Operator",
		PermissionDashboardRead,
		PermissionUserRead, PermissionUserWrite,
		PermissionOrderRead,
		PermissionAuditRead,
	)

	// Support can handle user-facing tickets and view related data.
	Support = NewRole(
		"Support",
		PermissionDashboardRead,
		PermissionUserRead,
		PermissionTicketRead, PermissionTicketWrite,
		PermissionAuditRead,
	)

	// Auditor is strictly read-only for audit/compliance.
	Auditor = NewRole(
		"Auditor",
		PermissionDashboardRead,
		PermissionAuditRead,
		PermissionUserRead,
		PermissionOrderRead,
	)
)

// HasPermission returns true if the role grants the given permission.
// PermissionAll ("*") is treated as a wildcard.
func HasPermission(role Role, perm Permission) bool {
	if role.Permissions == nil {
		return false
	}
	if _, ok := role.Permissions[PermissionAll]; ok {
		return true
	}
	_, ok := role.Permissions[perm]
	return ok
}

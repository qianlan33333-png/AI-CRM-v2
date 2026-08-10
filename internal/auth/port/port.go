// Package port freezes authentication output consumed by HTTP and domain apps.
package port

import "context"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleOps   Role = "ops"
	RoleSales Role = "sales"
)

type Principal struct {
	AdminUserID int64
	Role        Role
	StaffID     *int64
}

type SessionRef string

// Service authenticates an opaque session and authorizes a named capability.
// Callers must not log, persist, or expose SessionRef.
type Service interface {
	Authenticate(context.Context, SessionRef) (Principal, error)
	Authorize(context.Context, Principal, string) error
	Invalidate(context.Context, SessionRef) error
}

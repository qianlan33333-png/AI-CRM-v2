package http

import (
	"context"
	"errors"
	stdhttp "net/http"
)

type Permission string

const (
	PermissionAdminRead  Permission = "admin.read"
	PermissionAdminWrite Permission = "admin.write"
)

type Actor struct {
	ID int64
}

var (
	ErrUnauthenticated   = errors.New("radar authentication required")
	ErrForbidden         = errors.New("radar permission denied")
	ErrCSRFInvalid       = errors.New("radar csrf invalid")
	ErrAccessUnavailable = errors.New("radar access dependency unavailable")
)

// Authorizer is the only identity/permission dependency accepted by this
// isolated package. The central composition layer owns adapting the canonical
// session and RBAC implementation to these two closed permissions.
type Authorizer interface {
	Authorize(context.Context, Permission) (Actor, error)
}

// CSRFVerifier is invoked only after admin.write authorization succeeds.
type CSRFVerifier interface {
	Verify(*stdhttp.Request) error
}

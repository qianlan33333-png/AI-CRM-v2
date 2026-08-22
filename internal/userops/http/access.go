// Package http exposes User Ops local-only operation seams. Route
// registration, canonical authorization adaptation, OpenAPI and generated
// clients remain in the shared composition window.
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
	ErrUnauthenticated   = errors.New("user ops authentication required")
	ErrForbidden         = errors.New("user ops permission denied")
	ErrCSRFInvalid       = errors.New("user ops csrf invalid")
	ErrAccessUnavailable = errors.New("user ops access dependency unavailable")
)

// Authorizer is intentionally narrow. The root later adapts the canonical
// session and RBAC implementation to these two frozen permissions.
type Authorizer interface {
	Authorize(context.Context, Permission) (Actor, error)
}

// CSRFVerifier runs only after an authorized local write request.
type CSRFVerifier interface {
	Verify(*stdhttp.Request) error
}

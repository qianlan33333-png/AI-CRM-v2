// Package port freezes authentication output consumed by HTTP and domain apps.
package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnauthenticated           = errors.New("authentication required")
	ErrUnauthorized              = errors.New("permission denied")
	ErrCSRFInvalid               = errors.New("csrf validation failed")
	ErrInvalidVerifiedLogin      = errors.New("invalid verified login")
	ErrAuthenticationUnavailable = errors.New("authentication unavailable")
)

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
type CSRFToken string

type Provider string

const ProviderWeCom Provider = "wecom"

// VerifiedLogin may only be constructed after a trusted provider adapter has
// verified the OAuth result. It is never accepted directly from an HTTP body.
type VerifiedLogin struct {
	Provider  Provider
	TenantID  string
	SubjectID string
}

// BrowserSession contains raw bearer material. Callers must only place it in
// secure browser cookies and must never log or persist it.
type BrowserSession struct {
	Session   SessionRef
	CSRF      CSRFToken
	ExpiresAt time.Time
}

type principalContextKey struct{}
type sessionContextKey struct{}

func WithAuthenticatedSession(ctx context.Context, principal Principal, session SessionRef) context.Context {
	ctx = context.WithValue(ctx, principalContextKey{}, principal)
	return context.WithValue(ctx, sessionContextKey{}, session)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func SessionFromContext(ctx context.Context) (SessionRef, bool) {
	if ctx == nil {
		return "", false
	}
	session, ok := ctx.Value(sessionContextKey{}).(SessionRef)
	return session, ok && session != ""
}

type Issuer interface {
	IssueVerified(context.Context, VerifiedLogin) (BrowserSession, error)
}

// Service authenticates an opaque session and authorizes a named capability.
// Callers must not log, persist, or expose SessionRef.
type Service interface {
	Authenticate(context.Context, SessionRef) (Principal, error)
	Authorize(context.Context, Principal, string) error
	Invalidate(context.Context, SessionRef, CSRFToken) error
}

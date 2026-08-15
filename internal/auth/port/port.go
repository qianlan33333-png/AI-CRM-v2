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
	ErrOAuthStateInvalid         = errors.New("oauth state is invalid")
	ErrOAuthStateUnavailable     = errors.New("oauth state unavailable")
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

// Capability is a stable operation permission. Unknown values must be denied.
type Capability string

const (
	CapabilityAuthSessionRead      Capability = "auth.session.read"
	CapabilityAuthSessionLogout    Capability = "auth.session.logout"
	CapabilityCustomersRead        Capability = "customers.read"
	CapabilityCustomersWrite       Capability = "customers.write"
	CapabilityCustomerEventsRead   Capability = "customer.events.read"
	CapabilityIdentityResolve      Capability = "identity.resolve"
	CapabilityIdentityBind         Capability = "identity.bind"
	CapabilityIdentityIngest       Capability = "identity.ingest"
	CapabilityIdentityReviewRead   Capability = "identity.review.read"
	CapabilityIdentityReviewWrite  Capability = "identity.review.write"
	CapabilityConfigOverviewRead   Capability = "config.overview.read"
	CapabilityConfigSettingsManage Capability = "config.settings.manage"
	CapabilityStagesRead           Capability = "stages.read"
	CapabilityStagesWrite          Capability = "stages.write"
	CapabilitySegmentsRead         Capability = "segments.read"
	CapabilitySegmentsWrite        Capability = "segments.write"
	CapabilityOutboundRead         Capability = "outbound.read"
	CapabilityOutboundControl      Capability = "outbound.control"
	CapabilityProductsRead         Capability = "products.read"
	CapabilityProductsWrite        Capability = "products.write"
	CapabilityMediaImagesWrite     Capability = "media.images.write"
	CapabilityMediaLibraryRead     Capability = "media.library.read"
	CapabilityMediaLibraryWrite    Capability = "media.library.write"
	CapabilityQuestionnairesRead   Capability = "questionnaires.read"
	CapabilityQuestionnairesWrite  Capability = "questionnaires.write"
	CapabilityChannelsRead         Capability = "channels.read"
	CapabilityChannelsWrite        Capability = "channels.write"
	CapabilityCouponsRead          Capability = "coupons.read"
	CapabilityCouponsWrite         Capability = "coupons.write"
)

func (capability Capability) Known() bool {
	switch capability {
	case CapabilityAuthSessionRead, CapabilityAuthSessionLogout,
		CapabilityCustomersRead, CapabilityCustomersWrite, CapabilityCustomerEventsRead,
		CapabilityIdentityResolve, CapabilityIdentityBind, CapabilityIdentityIngest,
		CapabilityIdentityReviewRead, CapabilityIdentityReviewWrite,
		CapabilityConfigOverviewRead, CapabilityConfigSettingsManage, CapabilityStagesRead, CapabilityStagesWrite,
		CapabilitySegmentsRead, CapabilitySegmentsWrite,
		CapabilityOutboundRead, CapabilityOutboundControl,
		CapabilityProductsRead, CapabilityProductsWrite,
		CapabilityMediaImagesWrite, CapabilityMediaLibraryRead, CapabilityMediaLibraryWrite,
		CapabilityQuestionnairesRead, CapabilityQuestionnairesWrite,
		CapabilityChannelsRead, CapabilityChannelsWrite, CapabilityCouponsRead, CapabilityCouponsWrite:
		return true
	default:
		return false
	}
}

type ScopeKind string

const (
	ScopeSelf       ScopeKind = "self"
	ScopeGlobal     ScopeKind = "global"
	ScopeOwnerStaff ScopeKind = "owner_staff"
)

// Authorization is produced only after both the principal and capability are
// accepted. OwnerStaffID is populated only for ScopeOwnerStaff.
type Authorization struct {
	Capability   Capability
	Scope        ScopeKind
	OwnerStaffID int64
}

func (authorization Authorization) AllowsOwner(ownerStaffID int64) bool {
	if ownerStaffID < 1 {
		return false
	}
	switch authorization.Scope {
	case ScopeGlobal:
		return true
	case ScopeOwnerStaff:
		return authorization.OwnerStaffID > 0 && authorization.OwnerStaffID == ownerStaffID
	default:
		return false
	}
}

type SessionRef string
type CSRFToken string

type Provider string

const ProviderWeCom Provider = "wecom"

// VerifiedLogin may only be constructed after a trusted provider adapter has
// verified the OAuth result. It is never accepted directly from an HTTP body.
type VerifiedLogin struct {
	Provider  Provider
	CorpID    string
	SubjectID string
}

// BrowserSession contains raw bearer material. Callers must only place it in
// secure browser cookies and must never log or persist it.
type BrowserSession struct {
	Session   SessionRef
	CSRF      CSRFToken
	ExpiresAt time.Time
}

type OAuthState string

type OAuthAttempt struct {
	State     OAuthState
	ExpiresAt time.Time
}

type OAuthClaim struct {
	Provider Provider
	NextPath string
}

type principalContextKey struct{}
type sessionContextKey struct{}
type authorizationContextKey struct{}

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

func WithAuthorization(ctx context.Context, authorization Authorization) (context.Context, error) {
	if ctx == nil || !validAuthorization(authorization) {
		return nil, ErrUnauthorized
	}
	return context.WithValue(ctx, authorizationContextKey{}, authorization), nil
}

func AuthorizationFromContext(ctx context.Context) (Authorization, bool) {
	if ctx == nil {
		return Authorization{}, false
	}
	authorization, ok := ctx.Value(authorizationContextKey{}).(Authorization)
	return authorization, ok && validAuthorization(authorization)
}

func validAuthorization(authorization Authorization) bool {
	if !authorization.Capability.Known() {
		return false
	}
	switch authorization.Capability {
	case CapabilityAuthSessionRead, CapabilityAuthSessionLogout:
		return authorization.Scope == ScopeSelf && authorization.OwnerStaffID == 0
	case CapabilityCustomersRead, CapabilityCustomersWrite, CapabilityCustomerEventsRead,
		CapabilityOutboundRead:
		if authorization.Scope == ScopeGlobal {
			return authorization.OwnerStaffID == 0
		}
		return authorization.Scope == ScopeOwnerStaff && authorization.OwnerStaffID > 0
	case CapabilityIdentityResolve, CapabilityIdentityBind, CapabilityIdentityIngest,
		CapabilityIdentityReviewRead, CapabilityIdentityReviewWrite, CapabilityConfigOverviewRead, CapabilityConfigSettingsManage,
		CapabilityStagesRead, CapabilityStagesWrite,
		CapabilitySegmentsRead, CapabilitySegmentsWrite, CapabilityOutboundControl,
		CapabilityProductsRead, CapabilityProductsWrite,
		CapabilityMediaImagesWrite, CapabilityMediaLibraryRead, CapabilityMediaLibraryWrite,
		CapabilityQuestionnairesRead, CapabilityQuestionnairesWrite,
		CapabilityChannelsRead, CapabilityChannelsWrite, CapabilityCouponsRead, CapabilityCouponsWrite:
		return authorization.Scope == ScopeGlobal && authorization.OwnerStaffID == 0
	default:
		return false
	}
}

type Issuer interface {
	IssueVerified(context.Context, VerifiedLogin) (BrowserSession, error)
}

// OAuthStateManager owns one-time, server-side OAuth state. The raw state is
// browser-only material; storage receives only its hash.
type OAuthStateManager interface {
	Begin(context.Context, Provider, string) (OAuthAttempt, error)
	Claim(context.Context, Provider, OAuthState) (OAuthClaim, error)
}

// Service authenticates an opaque session and authorizes a named capability.
// Callers must not log, persist, or expose SessionRef.
type Service interface {
	Authenticate(context.Context, SessionRef) (Principal, error)
	Authorize(context.Context, Principal, Capability) (Authorization, error)
	ValidateCSRF(context.Context, SessionRef, CSRFToken) error
	Invalidate(context.Context, SessionRef, CSRFToken) error
}

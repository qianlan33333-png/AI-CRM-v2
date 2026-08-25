package main

import (
	"context"
	"errors"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

// productExternalPushAuthorizer only consumes the authorization fact installed
// by the canonical router. It cannot grant Product capability itself.
type productExternalPushAuthorizer struct{}

func (productExternalPushAuthorizer) Authorize(ctx context.Context, capability authport.Capability) (authport.Principal, error) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return authport.Principal{}, authport.ErrUnauthenticated
	}
	if !authorizationOK || authorization.Scope != authport.ScopeGlobal || authorization.Capability != capability {
		return authport.Principal{}, authport.ErrUnauthorized
	}
	return principal, nil
}

// productExternalPushCSRF rejects direct fragment use; the canonical router
// has already performed the session-bound token validation before this check.
type productExternalPushCSRF struct{}

func (productExternalPushCSRF) Verify(request *http.Request, _ authport.Principal) error {
	if request == nil {
		return authport.ErrCSRFInvalid
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Scope != authport.ScopeGlobal || authorization.Capability != authport.CapabilityProductsWrite {
		return errors.Join(authport.ErrCSRFInvalid, authport.ErrUnauthorized)
	}
	return nil
}

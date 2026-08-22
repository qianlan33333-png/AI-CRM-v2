package main

import (
	"context"
	"errors"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/product/membergrid"
)

// legacyMemberGridManagementAuthorizer verifies the canonical authorization
// context installed by registerLegacy before the domain fragment is invoked.
// It never grants a capability from collaborator metadata.
type legacyMemberGridManagementAuthorizer struct{}

func (legacyMemberGridManagementAuthorizer) Authorize(ctx context.Context, capability string) (membergrid.ManagementActor, error) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return membergrid.ManagementActor{}, membergrid.ErrAuthenticationRequired
	}
	if !authorizationOK || authorization.Scope != authport.ScopeGlobal || string(authorization.Capability) != capability {
		return membergrid.ManagementActor{}, membergrid.ErrPermissionDenied
	}
	return membergrid.ManagementActor{ID: principal.AdminUserID}, nil
}

// registerLegacy performs the canonical session-bound CSRF validation before
// this adapter is reached. The adapter fails closed if the expected canonical
// authorization context is absent, which also prevents direct unwrapped use.
type legacyMemberGridManagementCSRF struct{}

func (legacyMemberGridManagementCSRF) Verify(request *http.Request) error {
	if request == nil {
		return membergrid.ErrCSRFRejected
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Scope != authport.ScopeGlobal || authorization.Capability != authport.CapabilityProductsWrite {
		return errors.Join(membergrid.ErrCSRFRejected, membergrid.ErrPermissionDenied)
	}
	return nil
}

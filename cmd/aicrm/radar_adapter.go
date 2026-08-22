package main

import (
	"context"
	"errors"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	radarthttp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/http"
)

// legacyRadarAuthorizer maps the package-local permissions onto the existing
// canonical capabilities. Radar mutation is an operations-management action;
// no new admin.write capability is introduced.
type legacyRadarAuthorizer struct{}

func (legacyRadarAuthorizer) Authorize(ctx context.Context, permission radarthttp.Permission) (radarthttp.Actor, error) {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return radarthttp.Actor{}, radarthttp.ErrUnauthenticated
	}
	if !authorizationOK || authorization.Scope != authport.ScopeGlobal {
		return radarthttp.Actor{}, radarthttp.ErrForbidden
	}
	expected := authport.CapabilityAdminRead
	if permission == radarthttp.PermissionAdminWrite {
		expected = authport.CapabilityOperationsManage
	} else if permission != radarthttp.PermissionAdminRead {
		return radarthttp.Actor{}, radarthttp.ErrForbidden
	}
	if authorization.Capability != expected {
		return radarthttp.Actor{}, radarthttp.ErrForbidden
	}
	return radarthttp.Actor{ID: principal.AdminUserID}, nil
}

// registerLegacy has already performed the canonical session-bound CSRF
// validation. This second check prevents direct, unwrapped mutation use.
type legacyRadarCSRF struct{}

func (legacyRadarCSRF) Verify(request *http.Request) error {
	if request == nil {
		return radarthttp.ErrCSRFInvalid
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Scope != authport.ScopeGlobal || authorization.Capability != authport.CapabilityOperationsManage {
		return errors.Join(radarthttp.ErrCSRFInvalid, radarthttp.ErrForbidden)
	}
	return nil
}

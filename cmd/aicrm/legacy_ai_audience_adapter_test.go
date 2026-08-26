package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudience"
)

func TestLegacyAIAudienceSecurityAcceptsAuthorizedOperationMemberSync(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, legacyaudience.OperationMembersSyncRoute, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{
		AdminUserID: 17,
		Role:        authport.RoleAdmin,
	}, authport.SessionRef("ai-audience-operation-member-sync"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{
		Capability: authport.CapabilityOperationsManage,
		Scope:      authport.ScopeGlobal,
	})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := (legacyAIAudienceSecurity{}).Authorize(request.WithContext(ctx), legacyaudience.AccessRequirement{
		Capability:  legacyaudience.CapabilityOperationsManage,
		RequireCSRF: true,
	})
	if err != nil || actor.AdminUserID != 17 {
		t.Fatalf("actor=%+v err=%v", actor, err)
	}
}

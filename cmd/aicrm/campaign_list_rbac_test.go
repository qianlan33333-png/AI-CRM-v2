package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
)

type campaignListRBACAuth struct {
	principal     authport.Principal
	allowed       authport.Capability
	authorization *authport.Authorization
	seen          []authport.Capability
}

func (stub *campaignListRBACAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return stub.principal, nil
}

func (stub *campaignListRBACAuth) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	stub.seen = append(stub.seen, capability)
	if stub.authorization != nil {
		return *stub.authorization, nil
	}
	if capability != stub.allowed {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (*campaignListRBACAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func (*campaignListRBACAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func campaignListFinalRouter(t *testing.T, auth authport.Service) http.Handler {
	t.Helper()
	now := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	store := campaign.NewMemoryStore(campaign.Campaign{
		Code: "spring", Name: "Spring", ApprovalStatus: campaign.ApprovalDraft,
		RuntimeStatus: campaign.RuntimeIdle, Version: 1, CreatedBy: 7, UpdatedBy: 7,
		CreatedAt: now, UpdatedAt: now,
	})
	service, err := campaign.NewService(store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := campaign.NewRouteFragment(service, legacyCampaignAuthorizer{}, legacyCampaignCSRF{})
	if err != nil {
		t.Fatal(err)
	}
	legacy := &Handler{auth: auth, campaign: fragment}
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func campaignListRequest() *http.Request {
	request := httptest.NewRequest(http.MethodGet, campaign.RoutePrefix+"?approval_status=draft&runtime_status=idle", nil)
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(71)})
	return request
}

func TestCampaignListFinalRouterAllowsOnlyGlobalOperationsRead(t *testing.T) {
	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role), func(t *testing.T) {
			auth := &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 7, Role: role}, allowed: authport.CapabilityOperationsRead}
			response := httptest.NewRecorder()
			campaignListFinalRouter(t, auth).ServeHTTP(response, campaignListRequest())
			if response.Code != http.StatusOK || len(auth.seen) != 1 || auth.seen[0] != authport.CapabilityOperationsRead || !strings.Contains(response.Body.String(), `"campaign_code":"spring"`) {
				t.Fatalf("status=%d capabilities=%v body=%s", response.Code, auth.seen, response.Body.String())
			}
		})
	}

	owner := int64(9)
	tests := []struct {
		name string
		auth *campaignListRBACAuth
		path string
	}{
		{"sales", &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &owner}}, campaign.RoutePrefix},
		{"owner scope", &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, authorization: &authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner}}, campaign.RoutePrefix},
		{"wrong capability", &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, authorization: &authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}, campaign.RoutePrefix},
		{"admin-only detail sibling", &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, allowed: authport.CapabilityOperationsRead}, campaign.RoutePrefix + "/spring"},
		{"manage sibling", &campaignListRBACAuth{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, allowed: authport.CapabilityOperationsRead}, campaign.RoutePrefix + "/batch-start"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.name == "manage sibling" {
				request = httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{"items":[]}`))
			}
			request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(72)})
			response := httptest.NewRecorder()
			campaignListFinalRouter(t, test.auth).ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status=%d capabilities=%v body=%s", response.Code, test.auth.seen, response.Body.String())
			}
		})
	}
}

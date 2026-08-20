package automationhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestCloudOrchestratorPagesCarryOnlyApprovedWorkspaces(t *testing.T) {
	tests := []struct {
		path     string
		location string
	}{
		{path: CloudOrchestratorRootPath, location: CloudOrchestratorPlansPath},
		{path: CloudOrchestratorPlansPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans"},
		{path: CloudOrchestratorPlansPath + "/plan_A-42", location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans%2Fplan_A-42"},
		{path: CloudOrchestratorCampaignsPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns"},
		{path: CloudOrchestratorObservabilityPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fobservability"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewCloudOrchestratorPages().ServeHTTP(response, authorizedCloudOrchestratorRequest(http.MethodGet, test.path))
			if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertCloudOrchestratorHeaders(t, response)
		})
	}
}

func TestCloudOrchestratorPagesFailClosedForIdentityAndScopeDrift(t *testing.T) {
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
	}{
		{name: "missing principal", authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "ops", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "wrong capability", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminShellRead, Scope: authport.ScopeGlobal}},
		{name: "owner scope", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, CloudOrchestratorPlansPath, nil)
			ctx := request.Context()
			if test.principal.AdminUserID > 0 {
				ctx = authport.WithAuthenticatedSession(ctx, test.principal, authport.SessionRef("session"))
			}
			if test.authorization.Capability != "" {
				if authorizedContext, err := authport.WithAuthorization(ctx, test.authorization); err == nil {
					ctx = authorizedContext
				}
			}
			response := httptest.NewRecorder()
			NewCloudOrchestratorPages().ServeHTTP(response, request.WithContext(ctx))
			if response.Code != http.StatusForbidden || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertCloudOrchestratorError(t, response, "UNAUTHORIZED")
		})
	}
}

func TestCloudOrchestratorPagesRejectMethodsBeforeAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		NewCloudOrchestratorPages().ServeHTTP(response, httptest.NewRequest(method, CloudOrchestratorPlansPath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("method/status/allow=%s/%d/%q", method, response.Code, response.Header().Get("Allow"))
		}
		assertCloudOrchestratorHeaders(t, response)
	}
}

func TestCloudOrchestratorPagesRejectUnknownAndNestedDetailPaths(t *testing.T) {
	for _, path := range []string{
		"/admin/cloud-orchestrator/unknown",
		CloudOrchestratorPlansPath + "/",
		CloudOrchestratorPlansPath + "/plan/nested",
		CloudOrchestratorPlansPath + "/plan%0Aheader",
	} {
		request := authorizedCloudOrchestratorRequest(http.MethodGet, path)
		response := httptest.NewRecorder()
		NewCloudOrchestratorPages().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Fatalf("path/status/location=%q/%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		assertCloudOrchestratorError(t, response, "NOT_FOUND")
	}
}

func TestCloudOrchestratorPageTargetDoesNotInventBusinessState(t *testing.T) {
	target, root, matched := cloudOrchestratorPageTarget(CloudOrchestratorPlansPath + "/pending-review-1")
	if target != CloudOrchestratorPlansPath+"/pending-review-1" || root || !matched {
		t.Fatalf("target/root/matched=%q/%t/%t", target, root, matched)
	}
	for _, forbidden := range []string{"approved", "sent", "provider", "audience", "quality"} {
		if strings.Contains(strings.ToLower(target[:len(CloudOrchestratorPlansPath)]), forbidden) {
			t.Fatalf("carrier invented %q", forbidden)
		}
	}
}

func authorizedCloudOrchestratorRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func assertCloudOrchestratorHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertCloudOrchestratorError(t *testing.T, response *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("error security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != wantCode {
		t.Fatalf("body/code=%q/%q err=%v", response.Body.String(), body.Code, err)
	}
}

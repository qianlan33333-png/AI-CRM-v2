package automationhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestGroupOpsPagesCarryOnlyApprovedWorkspaces(t *testing.T) {
	tests := []struct {
		path     string
		location string
	}{
		{path: GroupOpsPlansPath, location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fui"},
		{path: "/admin/automation-conversion/group-ops/plans/42", location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fplans%2F42"},
		{path: "/admin/automation-conversion/group-ops/plans/9007199254740993", location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fgroup-ops%2Fplans%2F9007199254740993"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewGroupOpsPages().ServeHTTP(response, authorizedGroupOpsRequest(http.MethodGet, test.path))
			if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertGroupOpsHeaders(t, response)
		})
	}
}

func TestGroupOpsPagesFailClosedForIdentityAndScopeDrift(t *testing.T) {
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
	}{
		{name: "missing principal", authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "ops", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "wrong capability", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityConfigOverviewRead, Scope: authport.ScopeGlobal}},
		{name: "owner scope", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, GroupOpsPlansPath, nil)
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
			NewGroupOpsPages().ServeHTTP(response, request.WithContext(ctx))
			if response.Code != http.StatusForbidden || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertGroupOpsError(t, response, "UNAUTHORIZED")
		})
	}
}

func TestGroupOpsPagesRejectMethodsBeforeAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		NewGroupOpsPages().ServeHTTP(response, httptest.NewRequest(method, GroupOpsPlansPath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("method/status/allow=%s/%d/%q", method, response.Code, response.Header().Get("Allow"))
		}
		assertGroupOpsHeaders(t, response)
	}
}

func TestGroupOpsPagesRejectUnknownOrNonCanonicalPlanIDs(t *testing.T) {
	for _, path := range []string{
		"/admin/automation-conversion/group-ops",
		"/admin/automation-conversion/group-ops/groups/ui",
		"/admin/automation-conversion/group-ops/unknown",
		"/admin/automation-conversion/group-ops/plans/",
		"/admin/automation-conversion/group-ops/plans/0",
		"/admin/automation-conversion/group-ops/plans/-1",
		"/admin/automation-conversion/group-ops/plans/01",
		"/admin/automation-conversion/group-ops/plans/9223372036854775808",
		"/admin/automation-conversion/group-ops/plans/42/nodes",
		"/admin/automation-conversion/group-ops/plans/42\\nodes",
		"/admin/automation-conversion/group-ops/plans/42%0Aheader",
	} {
		response := httptest.NewRecorder()
		NewGroupOpsPages().ServeHTTP(response, authorizedGroupOpsRequest(http.MethodGet, path))
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Fatalf("path/status/location=%q/%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		assertGroupOpsError(t, response, "NOT_FOUND")
	}
}

func authorizedGroupOpsRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func assertGroupOpsHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertGroupOpsError(t *testing.T, response *httptest.ResponseRecorder, wantCode string) {
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

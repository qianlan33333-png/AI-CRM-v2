package automationhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestUserOpsPagesCarryOnlyApprovedReviewWorkspaces(t *testing.T) {
	for _, path := range []string{UserOpsPath, UserOpsUIPath} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewUserOpsPages().ServeHTTP(response, authorizedUserOpsRequest(http.MethodGet, path))
			wantLocation := "/?legacy_admin_path=" + map[string]string{
				UserOpsPath:   "%2Fadmin%2Fuser-ops",
				UserOpsUIPath: "%2Fadmin%2Fuser-ops%2Fui",
			}[path]
			if response.Code != http.StatusFound || response.Header().Get("Location") != wantLocation {
				t.Fatalf("status/location=%d/%q want %q", response.Code, response.Header().Get("Location"), wantLocation)
			}
			assertUserOpsHeaders(t, response)
		})
	}
}

func TestUserOpsPagesFailClosedForIdentityAndScopeDrift(t *testing.T) {
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
			request := httptest.NewRequest(http.MethodGet, UserOpsPath, nil)
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
			NewUserOpsPages().ServeHTTP(response, request.WithContext(ctx))
			if response.Code != http.StatusForbidden || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertUserOpsError(t, response, "UNAUTHORIZED")
		})
	}
}

func TestUserOpsPagesRejectMethodsBeforeAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		NewUserOpsPages().ServeHTTP(response, httptest.NewRequest(method, UserOpsPath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("method/status/allow=%s/%d/%q", method, response.Code, response.Header().Get("Allow"))
		}
		assertUserOpsHeaders(t, response)
	}
}

func TestUserOpsPagesRejectUnknownOrActionPaths(t *testing.T) {
	for _, path := range []string{
		UserOpsPath + "/",
		UserOpsPath + "/unknown",
		UserOpsPath + "/batch-send/preview",
		UserOpsPath + "/batch-send/execute",
		UserOpsPath + "/send-records",
		UserOpsPath + "/do-not-disturb",
		"/api/admin/user-ops",
	} {
		response := httptest.NewRecorder()
		NewUserOpsPages().ServeHTTP(response, authorizedUserOpsRequest(http.MethodGet, path))
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Fatalf("path/status/location=%q/%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		assertUserOpsError(t, response, "NOT_FOUND")
	}
}

func TestUserOpsPagePatternRegistryIsExact(t *testing.T) {
	if !IsUserOpsPagePattern(UserOpsPath) || !IsUserOpsPagePattern(UserOpsUIPath) {
		t.Fatal("approved user ops page pattern is missing")
	}
	for _, pattern := range []string{UserOpsPath + "/", UserOpsPath + "/{action}", "/api/admin/user-ops"} {
		if IsUserOpsPagePattern(pattern) {
			t.Fatalf("unexpected user ops page pattern %q", pattern)
		}
	}
}

func authorizedUserOpsRequest(method, path string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func assertUserOpsHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertUserOpsError(t *testing.T, response *httptest.ResponseRecorder, wantCode string) {
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

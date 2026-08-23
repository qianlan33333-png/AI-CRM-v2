package automationhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestAudiencePackagePagesCarryOnlyApprovedWorkspaces(t *testing.T) {
	tests := []struct {
		path     string
		location string
	}{
		{path: AudiencePackagesPath, location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion"},
		{path: "/admin/automation-conversion/packages/42", location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fpackages%2F42"},
		{path: "/admin/automation-conversion/packages/9007199254740993", location: "/?legacy_admin_path=%2Fadmin%2Fautomation-conversion%2Fpackages%2F9007199254740993"},
	}
	for _, test := range tests {
		for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
			t.Run(test.path+"/"+string(role), func(t *testing.T) {
				response := httptest.NewRecorder()
				NewAudiencePackagePages().ServeHTTP(response, authorizedAudiencePackageRequest(http.MethodGet, test.path, role))
				if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
					t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
				}
				assertAudiencePackageHeaders(t, response)
			})
		}
	}
}

func TestAudiencePackagePagesFailClosedForIdentityAndScopeDrift(t *testing.T) {
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
	}{
		{name: "missing principal", authorization: authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeGlobal}},
		{name: "sales", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales}, authorization: authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeGlobal}},
		{name: "wrong capability", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "owner scope", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, AudiencePackagesPath, nil)
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
			NewAudiencePackagePages().ServeHTTP(response, request.WithContext(ctx))
			if response.Code != http.StatusForbidden || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertAudiencePackageError(t, response, "UNAUTHORIZED")
		})
	}
}

func TestAudiencePackagePagesRejectMethodsBeforeAuthorization(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		NewAudiencePackagePages().ServeHTTP(response, httptest.NewRequest(method, AudiencePackagesPath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("method/status/allow=%s/%d/%q", method, response.Code, response.Header().Get("Allow"))
		}
		assertAudiencePackageHeaders(t, response)
	}
}

func TestAudiencePackagePagesRejectUnknownOrNonCanonicalPackageIDs(t *testing.T) {
	for _, path := range []string{
		AudiencePackagesPath + "/",
		AudiencePackagesPath + "/programs/retired",
		AudiencePackagesPath + "/packages/",
		AudiencePackagesPath + "/packages/0",
		AudiencePackagesPath + "/packages/-1",
		AudiencePackagesPath + "/packages/01",
		AudiencePackagesPath + "/packages/9223372036854775808",
		AudiencePackagesPath + "/packages/42/members",
		AudiencePackagesPath + "/packages/42\\members",
		AudiencePackagesPath + "/packages/42%0Aheader",
	} {
		response := httptest.NewRecorder()
		NewAudiencePackagePages().ServeHTTP(response, authorizedAudiencePackageRequest(http.MethodGet, path, authport.RoleAdmin))
		if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
			t.Fatalf("path/status/location=%q/%d/%q", path, response.Code, response.Header().Get("Location"))
		}
		assertAudiencePackageError(t, response, "NOT_FOUND")
	}
}

func TestAudiencePackagePagePatternRegistryIsExact(t *testing.T) {
	if !IsAudiencePackagePagePattern(AudiencePackagesPath) || !IsAudiencePackagePagePattern(AudiencePackageDetailPattern) {
		t.Fatal("approved audience package page pattern is missing")
	}
	for _, pattern := range []string{AudiencePackagesPath + "/", AudiencePackagesPath + "/packages/{package_id}/members", "/api/admin/ai-audience/packages"} {
		if IsAudiencePackagePagePattern(pattern) {
			t.Fatalf("unexpected audience package page pattern %q", pattern)
		}
	}
}

func authorizedAudiencePackageRequest(method, path string, role authport.Role) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: role}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOperationsRead, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func assertAudiencePackageHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers=%q/%q", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
	}
}

func assertAudiencePackageError(t *testing.T, response *httptest.ResponseRecorder, wantCode string) {
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

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
		name     string
		path     string
		location string
		role     authport.Role
		cap      authport.Capability
	}{
		{name: "root admin", path: CloudOrchestratorRootPath, location: CloudOrchestratorPlansPath, role: authport.RoleAdmin, cap: authport.CapabilityAdminRead},
		{name: "plans admin", path: CloudOrchestratorPlansPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans", role: authport.RoleAdmin, cap: authport.CapabilityAdminRead},
		{name: "plan detail admin", path: CloudOrchestratorPlansPath + "/plan_A-42", location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fplans%2Fplan_A-42", role: authport.RoleAdmin, cap: authport.CapabilityAdminRead},
		{name: "campaigns admin", path: CloudOrchestratorCampaignsPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns", role: authport.RoleAdmin, cap: authport.CapabilityOperationsRead},
		{name: "campaigns ops", path: CloudOrchestratorCampaignsPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns", role: authport.RoleOps, cap: authport.CapabilityOperationsRead},
		{name: "observability admin", path: CloudOrchestratorObservabilityPath, location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fobservability", role: authport.RoleAdmin, cap: authport.CapabilityAdminRead},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			NewCloudOrchestratorPages().ServeHTTP(response, authorizedCloudOrchestratorRequest(http.MethodGet, test.path, test.role, test.cap))
			if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertCloudOrchestratorHeaders(t, response)
		})
	}
}

func TestCloudOrchestratorCampaignsPageCarriesClosedLaunchContext(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		location string
	}{
		{
			name:     "customer selection",
			query:    "source_kind=customer_selection&source_id=42",
			location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dcustomer_selection%26source_id%3D42",
		},
		{
			name:     "segment members",
			query:    "source_kind=segment_members&source_id=7",
			location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dsegment_members%26source_id%3D7",
		},
		{
			name:     "pair order is normalized",
			query:    "source_id=7&source_kind=segment_members",
			location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dsegment_members%26source_id%3D7",
		},
		{
			name:     "audience package at int64 maximum",
			query:    "source_kind=ai_audience_package_members&source_id=9223372036854775807",
			location: "/?legacy_admin_path=%2Fadmin%2Fcloud-orchestrator%2Fcampaigns%3Fsource_kind%3Dai_audience_package_members%26source_id%3D9223372036854775807",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := authorizedCloudOrchestratorRequest(http.MethodGet, CloudOrchestratorCampaignsPath, authport.RoleOps, authport.CapabilityOperationsRead)
			request.URL.RawQuery = test.query
			response := httptest.NewRecorder()

			NewCloudOrchestratorPages().ServeHTTP(response, request)

			if response.Code != http.StatusFound || response.Header().Get("Location") != test.location {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertCloudOrchestratorHeaders(t, response)
		})
	}
}

func TestCloudOrchestratorCampaignsPageRejectsNonCanonicalLaunchContext(t *testing.T) {
	queries := map[string]string{
		"half pair kind":         "source_kind=customer_selection",
		"half pair id":           "source_id=7",
		"duplicate kind":         "source_kind=customer_selection&source_kind=segment_members&source_id=7",
		"duplicate id":           "source_kind=segment_members&source_id=7&source_id=8",
		"extra query":            "source_kind=segment_members&source_id=7&return_to=customers",
		"unknown kind":           "source_kind=customer_filter&source_id=7",
		"empty id":               "source_kind=customer_selection&source_id=",
		"zero id":                "source_kind=customer_selection&source_id=0",
		"negative id":            "source_kind=customer_selection&source_id=-1",
		"leading zero":           "source_kind=customer_selection&source_id=01",
		"raw plus":               "source_kind=customer_selection&source_id=+1",
		"encoded plus":           "source_kind=customer_selection&source_id=%2B1",
		"leading whitespace":     "source_kind=customer_selection&source_id=%201",
		"trailing whitespace":    "source_kind=customer_selection&source_id=1%20",
		"above int64 maximum":    "source_kind=ai_audience_package_members&source_id=9223372036854775808",
		"too many digits":        "source_kind=segment_members&source_id=100000000000000000000",
		"invalid percent escape": "source_kind=segment_members&source_id=%zz",
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			request := authorizedCloudOrchestratorRequest(http.MethodGet, CloudOrchestratorCampaignsPath, authport.RoleAdmin, authport.CapabilityOperationsRead)
			request.URL.RawQuery = query
			response := httptest.NewRecorder()

			NewCloudOrchestratorPages().ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			assertCloudOrchestratorError(t, response, "MALFORMED_REQUEST")
		})
	}
}

func TestCloudOrchestratorCampaignsPageFailsClosedForRoleCapabilityAndScopeDrift(t *testing.T) {
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
			request := httptest.NewRequest(http.MethodGet, CloudOrchestratorCampaignsPath, nil)
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
		CloudOrchestratorPlansPath + "/..",
		CloudOrchestratorPlansPath + "/plan\\nested",
		CloudOrchestratorPlansPath + "/plan%0Aheader",
	} {
		request := authorizedCloudOrchestratorRequest(http.MethodGet, path, authport.RoleAdmin, authport.CapabilityAdminRead)
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

func authorizedCloudOrchestratorRequest(method, path string, role authport.Role, capability authport.Capability) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 7, Role: role}, authport.SessionRef("session"))
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
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

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestAdminShellPageAllowsOnlyAdminAndOps(t *testing.T) {
	for _, testCase := range []struct {
		name string
		role authport.Role
	}{
		{name: "admin", role: authport.RoleAdmin},
		{name: "ops", role: authport.RoleOps},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := mustAdminShellHandler(t, &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: testCase.role}})
			response := httptest.NewRecorder()
			adminShellRoute(handler, handler.Page).ServeHTTP(response, adminShellRequest(http.MethodGet, "/admin", legacyToken(31)))
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
			}
			for _, want := range []string{"<title>快捷入口</title>", `href="/admin/customers"`, `href="/admin/cloud-orchestrator/plans"`, `id="aicrmAdminActionGrants"`, "{}"} {
				if !strings.Contains(response.Body.String(), want) {
					t.Fatalf("page missing %q: %s", want, response.Body.String())
				}
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}
}

func TestAdminShellFailsClosed(t *testing.T) {
	staffID := int64(9)
	for _, testCase := range []struct {
		name         string
		request      func() *http.Request
		service      *adminShellAuthStub
		wantStatus   int
		wantLocation string
		wantError    string
	}{
		{
			name:         "missing browser session redirects to login",
			request:      func() *http.Request { return httptest.NewRequest(http.MethodGet, "/admin", nil) },
			service:      &adminShellAuthStub{},
			wantStatus:   http.StatusFound,
			wantLocation: "/login?next=%2Fadmin",
		},
		{
			name:       "expired browser session redirects to login",
			request:    func() *http.Request { return adminShellRequest(http.MethodGet, "/admin", legacyToken(32)) },
			service:    &adminShellAuthStub{authenticateErr: authport.ErrUnauthenticated},
			wantStatus: http.StatusFound, wantLocation: "/login?next=%2Fadmin",
		},
		{
			name: "ambiguous paired sessions redirect to login",
			request: func() *http.Request {
				request := adminShellRequest(http.MethodGet, "/admin", legacyToken(33))
				request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(34)})
				return request
			},
			service: &adminShellAuthStub{}, wantStatus: http.StatusFound, wantLocation: "/login?next=%2Fadmin",
		},
		{
			name:       "sales is denied even with a staff identity",
			request:    func() *http.Request { return adminShellRequest(http.MethodGet, "/admin", legacyToken(35)) },
			service:    &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}},
			wantStatus: http.StatusForbidden, wantError: "admin_capability_required",
		},
		{
			name:       "unknown role is denied",
			request:    func() *http.Request { return adminShellRequest(http.MethodGet, "/admin", legacyToken(36)) },
			service:    &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.Role("unknown")}},
			wantStatus: http.StatusForbidden, wantError: "admin_capability_required",
		},
		{
			name:       "missing principal identity is denied",
			request:    func() *http.Request { return adminShellRequest(http.MethodGet, "/admin", legacyToken(37)) },
			service:    &adminShellAuthStub{principal: authport.Principal{Role: authport.RoleAdmin}},
			wantStatus: http.StatusForbidden, wantError: "admin_capability_required",
		},
		{
			name: "bearer principal is forbidden",
			request: func() *http.Request {
				request := adminShellRequest(http.MethodGet, "/admin", legacyToken(38))
				request.Header.Set("Authorization", "Bearer service-token")
				return request
			},
			service:    &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}},
			wantStatus: http.StatusForbidden, wantError: "principal_type_forbidden",
		},
		{
			name: "ambiguous authorization fields are forbidden",
			request: func() *http.Request {
				request := adminShellRequest(http.MethodGet, "/admin", legacyToken(38))
				request.Header.Add("Authorization", "")
				request.Header.Add("Authorization", "Bearer service-token")
				return request
			},
			service:    &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}},
			wantStatus: http.StatusForbidden, wantError: "principal_type_forbidden",
		},
		{
			name:    "owner scoped grant is denied",
			request: func() *http.Request { return adminShellRequest(http.MethodGet, "/admin", legacyToken(39)) },
			service: &adminShellAuthStub{
				principal:     authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin},
				authorization: authport.Authorization{Capability: authport.CapabilityAdminShellRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 7},
			},
			wantStatus: http.StatusForbidden, wantError: "admin_capability_required",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := mustAdminShellHandler(t, testCase.service)
			response := httptest.NewRecorder()
			adminShellRoute(handler, handler.Page).ServeHTTP(response, testCase.request())
			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", response.Code, testCase.wantStatus, response.Body.String())
			}
			if testCase.wantLocation != "" && response.Header().Get("Location") != testCase.wantLocation {
				t.Fatalf("Location = %q, want %q", response.Header().Get("Location"), testCase.wantLocation)
			}
			if testCase.wantError != "" {
				var body struct {
					OK            bool   `json:"ok"`
					Error         string `json:"error"`
					Capability    string `json:"required_capability"`
					RouteOwner    string `json:"route_owner"`
					ExternalCalls bool   `json:"real_external_call_executed"`
				}
				if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body.OK || body.Error != testCase.wantError || body.Capability != adminShellRequiredCapability || body.RouteOwner != "ai_crm_next" || body.ExternalCalls {
					t.Fatalf("denial body=%#v err=%v", body, err)
				}
			}
		})
	}
}

func TestAdminShellLogoutAliasPreservesExistingLogoutOwner(t *testing.T) {
	handler := mustAdminShellHandler(t, &adminShellAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}})
	response := httptest.NewRecorder()
	adminShellRoute(handler, handler.LogoutAlias).ServeHTTP(response, adminShellRequest(http.MethodGet, "/admin/logout", legacyToken(40)))
	if response.Code != http.StatusFound || response.Header().Get("Location") != legacyLogoutPath {
		t.Fatalf("logout alias response=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("logout alias set cookies=%#v; the existing /logout handler must own mutation", cookies)
	}
}

func TestFinalRouterMountsAdminShellBeforeRootCompatibilityRoute(t *testing.T) {
	service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleOps}}
	legacy, err := NewHandler(service, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy,
	)
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	router.ServeHTTP(page, adminShellRequest(http.MethodGet, "/admin", legacyToken(41)))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "快捷入口") {
		t.Fatalf("admin router response=%d body=%s", page.Code, page.Body.String())
	}
	logout := httptest.NewRecorder()
	router.ServeHTTP(logout, adminShellRequest(http.MethodGet, "/admin/logout", legacyToken(41)))
	if logout.Code != http.StatusFound || logout.Header().Get("Location") != legacyLogoutPath {
		t.Fatalf("admin logout router response=%d location=%q", logout.Code, logout.Header().Get("Location"))
	}
}

func mustAdminShellHandler(t *testing.T, auth authport.Service) *adminShellHandler {
	t.Helper()
	handler, err := newAdminShellHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func adminShellRoute(handler *adminShellHandler, endpoint http.HandlerFunc) http.Handler {
	return handler.Authenticate(endpoint)
}

func adminShellRequest(method, path, session string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: session})
	return request
}

type adminShellAuthStub struct {
	principal       authport.Principal
	authenticateErr error
	authorization   authport.Authorization
	authorizeErr    error
}

func (stub *adminShellAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	if stub.authenticateErr != nil {
		return authport.Principal{}, stub.authenticateErr
	}
	return stub.principal, nil
}

func (stub *adminShellAuthStub) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if stub.authorizeErr != nil {
		return authport.Authorization{}, stub.authorizeErr
	}
	if capability != authport.CapabilityAdminShellRead || principal.AdminUserID < 1 {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	if stub.authorization != (authport.Authorization{}) {
		return stub.authorization, nil
	}
	if principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	return authport.Authorization{}, authport.ErrUnauthorized
}

func (*adminShellAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}
func (*adminShellAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

var _ authport.Service = (*adminShellAuthStub)(nil)

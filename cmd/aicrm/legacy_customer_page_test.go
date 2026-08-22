package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

func TestLegacyCustomerPageCarriersReuseClosedCapabilitiesAndScopes(t *testing.T) {
	staffID := int64(31)
	principals := []authport.Principal{
		{AdminUserID: 7, Role: authport.RoleAdmin},
		{AdminUserID: 8, Role: authport.RoleOps},
		{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staffID},
	}
	routes := []struct {
		name       string
		path       string
		location   string
		capability authport.Capability
	}{
		{
			name:       "list",
			path:       legacyCustomerListPagePath,
			location:   "/?legacy_admin_path=%2Fadmin%2Fcustomers",
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "detail",
			path:       "/admin/customers/42",
			location:   "/?legacy_admin_path=%2Fadmin%2Fcustomers%2F42",
			capability: authport.CapabilityCustomersRead,
		},
		{
			name:       "context",
			path:       "/admin/customer-360/42",
			location:   "/?legacy_admin_path=%2Fadmin%2Fcustomer-360%2F42",
			capability: authport.CapabilityCustomerEventsRead,
		},
	}

	for _, principal := range principals {
		for _, route := range routes {
			t.Run(string(principal.Role)+"/"+route.name, func(t *testing.T) {
				service := &customerPageAuthSpy{principal: principal}
				request := legacyRequest(http.MethodGet, route.path, legacyToken(81))
				response := httptest.NewRecorder()
				customerPageRouter(t, service).ServeHTTP(response, request)

				if response.Code != http.StatusFound || response.Header().Get("Location") != route.location {
					t.Fatalf("status/location=%d/%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
				}
				if response.Header().Get("Cache-Control") != "private, no-store" ||
					response.Header().Get("Referrer-Policy") != "no-referrer" ||
					response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatalf(
						"security headers=%q/%q/%q",
						response.Header().Get("Cache-Control"),
						response.Header().Get("Referrer-Policy"),
						response.Header().Get("X-Content-Type-Options"),
					)
				}
				if service.authenticateCalls != 1 ||
					service.authorizeCalls != 1 ||
					service.csrfCalls != 0 ||
					len(service.capabilities) != 1 ||
					service.capabilities[0] != route.capability {
					t.Fatalf(
						"authenticate/authorize/csrf/capabilities=%d/%d/%d/%v",
						service.authenticateCalls,
						service.authorizeCalls,
						service.csrfCalls,
						service.capabilities,
					)
				}
				if len(service.authorizations) != 1 {
					t.Fatalf("authorizations=%v", service.authorizations)
				}
				got := service.authorizations[0]
				if principal.Role == authport.RoleSales {
					if got.Scope != authport.ScopeOwnerStaff || got.OwnerStaffID != staffID {
						t.Fatalf("sales authorization=%+v", got)
					}
				} else if got.Scope != authport.ScopeGlobal || got.OwnerStaffID != 0 {
					t.Fatalf("global authorization=%+v", got)
				}
			})
		}
	}
}

func TestLegacyCustomerPageCarriersRejectAnonymousAndIncompleteSales(t *testing.T) {
	tests := []struct {
		name       string
		principal  authport.Principal
		token      bool
		wantStatus int
	}{
		{
			name:       "anonymous",
			principal:  authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "sales_without_owner",
			principal:  authport.Principal{AdminUserID: 9, Role: authport.RoleSales},
			token:      true,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &customerPageAuthSpy{principal: test.principal}
			request := httptest.NewRequest(http.MethodGet, "/admin/customers/42", nil)
			if test.token {
				request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(82)})
			}
			response := httptest.NewRecorder()
			customerPageRouter(t, service).ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
			if service.csrfCalls != 0 {
				t.Fatalf("csrf=%d", service.csrfCalls)
			}
		})
	}
}

func TestLegacyCustomerPageCarriersFailClosedForMalformedPaths(t *testing.T) {
	paths := []string{
		"/admin/customers/",
		"/admin/customers/0",
		"/admin/customers/-1",
		"/admin/customers/01",
		"/admin/customers/9007199254740992",
		"/admin/customers/18446744073709551615",
		"/admin/customers/42/extra",
		"/admin/customers/42\\extra",
		"/admin\\customers\\42",
		"/admin/customers/42%2Fextra",
		"/admin%2Fcustomers%2F42",
		"/admin%252Fcustomers%252F42",
		"/admin%5Ccustomers%5C42",
		"/admin/customers/42%5Cextra",
		"/admin/customers/42/",
		"/admin/customers?",
		"/admin/customers?extra=1",
		"/admin/customers/42?extra=1",
		"/admin/customer-360",
		"/admin/customer-360/",
		"/admin/customer-360/0",
		"/admin/customer-360/-1",
		"/admin/customer-360/01",
		"/admin/customer-360/9007199254740992",
		"/admin/customer-360/18446744073709551615",
		"/admin/customer-360/legacy-text-key",
		"/admin/customer-360/42/extra",
		"/admin/customer-360/42\\extra",
		"/admin/customer-360/42%2Fextra",
		"/admin%2Fcustomer-360%2F42",
		"/admin%252Fcustomer-360%252F42",
		"/admin/customer-360/42%5Cextra",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
			router := customerPageRouter(t, service)
			request := legacyRequest(http.MethodGet, path, legacyToken(83))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound || response.Header().Get("Location") != "" {
				t.Fatalf("status/location=%d/%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
			if response.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
				!strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
				!strings.Contains(response.Body.String(), `href="/admin/customers"`) {
				t.Fatalf(
					"content-type/csp/body=%q/%q/%s",
					response.Header().Get("Content-Type"),
					response.Header().Get("Content-Security-Policy"),
					response.Body.String(),
				)
			}
			if strings.Contains(response.Body.String(), "legacy-text-key") || strings.Contains(response.Body.String(), path) {
				t.Fatal("fixed missing response reflected request-controlled path text")
			}
			if service.authenticateCalls != 0 || service.authorizeCalls != 0 || service.csrfCalls != 0 {
				t.Fatalf(
					"malformed path reached auth chain: %d/%d/%d",
					service.authenticateCalls,
					service.authorizeCalls,
					service.csrfCalls,
				)
			}
		})
	}
}

func TestLegacyCustomerPageCarriersRejectAllOtherMethodsBeforeAuth(t *testing.T) {
	service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	router := customerPageRouter(t, service)
	for _, path := range []string{legacyCustomerListPagePath, "/admin/customers/42", "/admin/customer-360/42"} {
		for _, method := range []string{
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("path/method/status/allow=%s/%s/%d/%q", path, method, response.Code, response.Header().Get("Allow"))
			}
			if response.Header().Get("Cache-Control") != "private, no-store" ||
				response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf(
					"path/method/headers=%s/%s/%q/%q",
					path,
					method,
					response.Header().Get("Cache-Control"),
					response.Header().Get("X-Content-Type-Options"),
				)
			}
		}
	}
	if service.authenticateCalls != 0 || service.authorizeCalls != 0 || service.csrfCalls != 0 {
		t.Fatalf("authenticate/authorize/csrf=%d/%d/%d", service.authenticateCalls, service.authorizeCalls, service.csrfCalls)
	}
}

func TestLegacyCustomerPagePathParserAcceptsOnlyCanonicalSafeIDs(t *testing.T) {
	for _, path := range []string{
		legacyCustomerListPagePath,
		"/admin/customers/1",
		"/admin/customers/9007199254740991",
		"/admin/customers/legacy-text-key",
		"/admin/customer-360/1",
		"/admin/customer-360/9007199254740991",
	} {
		if _, ok := parseLegacyCustomerPagePath(path); !ok {
			t.Fatalf("expected accepted path %q", path)
		}
	}
	for _, path := range []string{
		"/admin/customers/0",
		"/admin/customers/01",
		"/admin/customers/9007199254740992",
		"/admin/customers/18446744073709551615",
		"/admin/customers/1/extra",
		"/admin/customers/1\\extra",
		"/admin/customer-360",
		"/admin/customer-360/0",
		"/admin/customer-360/01",
		"/admin/customer-360/9007199254740992",
		"/admin/customer-360/18446744073709551615",
		"/admin/customer-360/legacy-text-key",
		"/admin/customer-360/1/extra",
	} {
		if _, ok := parseLegacyCustomerPagePath(path); ok {
			t.Fatalf("expected rejected path %q", path)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/admin/customers/42", nil)
	request.URL.RawPath = "/admin/customers/%34%32"
	if _, ok := legacyCustomerPageRouteForRequest(request); ok {
		t.Fatal("encoded raw path must be rejected")
	}
	request = httptest.NewRequest(http.MethodGet, "/admin/customers/42?extra=1", nil)
	if _, ok := legacyCustomerPageRouteForRequest(request); ok {
		t.Fatal("query-bearing carrier must be rejected")
	}
}

func TestLegacyCustomerUnionIDRedirectResolvesAfterAuthorization(t *testing.T) {
	resolver := &customerPageUnionResolver{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 42}}
	service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	response := httptest.NewRecorder()
	customerPageRouter(t, service, resolver).ServeHTTP(response, legacyRequest(http.MethodGet, "/admin/customers/legacy-union-key", legacyToken(84)))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin/customers/42" {
		t.Fatalf("status/location/body=%d/%q/%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if resolver.calls != 1 || resolver.value != "legacy-union-key" || strings.Contains(response.Body.String(), "legacy-union-key") || strings.Contains(response.Header().Get("Location"), "legacy-union-key") {
		t.Fatalf("resolver/response=%d/%q/%q/%s", resolver.calls, resolver.value, response.Header().Get("Location"), response.Body.String())
	}
}

func TestLegacyCustomerUnionIDRedirectFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		result     identityport.ResolveResult
		err        error
		wantStatus int
	}{
		{name: "not_found", result: identityport.ResolveResult{Status: identityport.ResolveNotFound}, wantStatus: http.StatusNotFound},
		{name: "conflict", result: identityport.ResolveResult{Status: identityport.ResolveConflict}, wantStatus: http.StatusServiceUnavailable},
		{name: "invalid_found", result: identityport.ResolveResult{Status: identityport.ResolveFound}, wantStatus: http.StatusServiceUnavailable},
		{name: "dependency", err: errors.New("identity unavailable"), wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &customerPageUnionResolver{result: test.result, err: test.err}
			service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
			response := httptest.NewRecorder()
			customerPageRouter(t, service, resolver).ServeHTTP(response, legacyRequest(http.MethodGet, "/admin/customers/secret-union-key", legacyToken(85)))
			if response.Code != test.wantStatus || response.Header().Get("Location") != "" || strings.Contains(response.Body.String(), "secret-union-key") {
				t.Fatalf("status/location/body=%d/%q/%s", response.Code, response.Header().Get("Location"), response.Body.String())
			}
		})
	}
}

func TestLegacyCustomerUnionIDIsNotResolvedBeforeAuthorization(t *testing.T) {
	resolver := &customerPageUnionResolver{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 42}}
	service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	response := httptest.NewRecorder()
	customerPageRouter(t, service, resolver).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/customers/legacy-union-key", nil))
	if response.Code != http.StatusUnauthorized || resolver.calls != 0 {
		t.Fatalf("status/resolve_calls=%d/%d", response.Code, resolver.calls)
	}
}

func TestLegacyCustomerPageNotFoundOwnsOnlyItsClosedNamespace(t *testing.T) {
	service := &customerPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	router := customerPageRouter(t, service)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin/customers-other/42", nil))
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), `href="/admin/customers"`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}

type customerPageAuthSpy struct {
	principal         authport.Principal
	authenticateCalls int
	authorizeCalls    int
	csrfCalls         int
	capabilities      []authport.Capability
	authorizations    []authport.Authorization
}

func (spy *customerPageAuthSpy) Authenticate(_ context.Context, _ authport.SessionRef) (authport.Principal, error) {
	spy.authenticateCalls++
	return spy.principal, nil
}

func (spy *customerPageAuthSpy) Authorize(
	_ context.Context,
	principal authport.Principal,
	capability authport.Capability,
) (authport.Authorization, error) {
	spy.authorizeCalls++
	spy.capabilities = append(spy.capabilities, capability)
	if principal.AdminUserID < 1 ||
		(capability != authport.CapabilityCustomersRead &&
			capability != authport.CapabilityCustomerEventsRead) {
		return authport.Authorization{}, authport.ErrUnauthorized
	}

	authorization := authport.Authorization{Capability: capability}
	switch principal.Role {
	case authport.RoleAdmin, authport.RoleOps:
		authorization.Scope = authport.ScopeGlobal
	case authport.RoleSales:
		if principal.StaffID == nil || *principal.StaffID < 1 {
			return authport.Authorization{}, authport.ErrUnauthorized
		}
		authorization.Scope = authport.ScopeOwnerStaff
		authorization.OwnerStaffID = *principal.StaffID
	default:
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	spy.authorizations = append(spy.authorizations, authorization)
	return authorization, nil
}

func (spy *customerPageAuthSpy) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	spy.csrfCalls++
	return nil
}

func (*customerPageAuthSpy) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type customerPageUnionResolver struct {
	result identityport.ResolveResult
	err    error
	calls  int
	value  string
}

func (resolver *customerPageUnionResolver) ResolveUnionID(_ context.Context, value string) (identityport.ResolveResult, error) {
	resolver.calls++
	resolver.value = value
	return resolver.result, resolver.err
}

func customerPageRouter(t *testing.T, service authport.Service, resolvers ...legacyMessageArchiveUnionResolver) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolvers) > 0 {
		legacy.messageArchiveUnionID = resolvers[0]
	}
	router, err := newAPIHandlerWithAll(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

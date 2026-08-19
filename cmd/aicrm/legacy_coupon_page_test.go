package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

func TestLegacyCouponListCarrierUsesGlobalCouponReadWithoutCSRF(t *testing.T) {
	for _, principal := range []authport.Principal{
		{AdminUserID: 7, Role: authport.RoleAdmin},
		{AdminUserID: 8, Role: authport.RoleOps},
	} {
		t.Run(string(principal.Role), func(t *testing.T) {
			service := &couponPageAuthSpy{principal: principal}
			response := httptest.NewRecorder()
			couponPageRouter(t, service).ServeHTTP(response, legacyRequest(http.MethodGet, legacyCouponPagePath, legacyToken(171)))
			if response.Code != http.StatusFound || response.Header().Get("Location") != "/?legacy_admin_path=%2Fadmin%2Fcoupons" {
				t.Fatalf("status/location=%d/%q", response.Code, response.Header().Get("Location"))
			}
			if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || service.authenticateCalls != 1 || service.authorizeCalls != 1 || service.csrfCalls != 0 || len(service.capabilities) != 1 || service.capabilities[0] != authport.CapabilityCouponsRead {
				t.Fatalf("headers/auth/csrf/capabilities=%q/%q/%d/%d/%d/%v", response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), service.authenticateCalls, service.authorizeCalls, service.csrfCalls, service.capabilities)
			}
		})
	}
}

func TestLegacyCouponListCarrierFailsClosedForSalesAndAnonymous(t *testing.T) {
	for _, test := range []struct {
		name      string
		principal authport.Principal
		token     bool
		want      int
	}{
		{name: "sales", principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales}, token: true, want: http.StatusForbidden},
		{name: "anonymous", principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, want: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &couponPageAuthSpy{principal: test.principal}
			request := httptest.NewRequest(http.MethodGet, legacyCouponPagePath, nil)
			if test.token {
				request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(172)})
			}
			response := httptest.NewRecorder()
			couponPageRouter(t, service).ServeHTTP(response, request)
			if response.Code != test.want || service.csrfCalls != 0 {
				t.Fatalf("status/csrf=%d/%d body=%s", response.Code, service.csrfCalls, response.Body.String())
			}
		})
	}
}

func TestLegacyCouponListCarrierRejectsAllOtherMethodsBeforeAuth(t *testing.T) {
	service := &couponPageAuthSpy{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}
	router := couponPageRouter(t, service)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, legacyCouponPagePath, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("method/status/headers=%s/%d/%q/%q/%q", method, response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"))
		}
	}
	if service.authenticateCalls != 0 || service.authorizeCalls != 0 || service.csrfCalls != 0 {
		t.Fatalf("authenticate/authorize/csrf=%d/%d/%d", service.authenticateCalls, service.authorizeCalls, service.csrfCalls)
	}
}

type couponPageAuthSpy struct {
	principal         authport.Principal
	authenticateCalls int
	authorizeCalls    int
	csrfCalls         int
	capabilities      []authport.Capability
}

func (spy *couponPageAuthSpy) Authenticate(_ context.Context, _ authport.SessionRef) (authport.Principal, error) {
	spy.authenticateCalls++
	return spy.principal, nil
}

func (spy *couponPageAuthSpy) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	spy.authorizeCalls++
	spy.capabilities = append(spy.capabilities, capability)
	if principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) || capability != authport.CapabilityCouponsRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (spy *couponPageAuthSpy) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	spy.csrfCalls++
	return nil
}

func (*couponPageAuthSpy) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func couponPageRouter(t *testing.T, service authport.Service) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

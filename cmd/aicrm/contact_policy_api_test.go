package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
)

func TestContactPolicyCanonicalRouteRegistrationAndRootCSRF(t *testing.T) {
	source, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	registrations := string(source)
	for _, exact := range []string{
		`{http.MethodGet, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsRead, false, http.HandlerFunc(wrapper.GetCustomerContactPolicy)}`,
		`{http.MethodPut, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.PutCustomerContactPolicy)}`,
		`{http.MethodDelete, "/api/v1/customers/{customer_id}/contact-policy", authport.CapabilityOperationsManage, true, http.HandlerFunc(wrapper.DeleteCustomerContactPolicy)}`,
	} {
		if strings.Count(registrations, exact) != 1 {
			t.Fatalf("canonical route registration count for %q != 1", exact)
		}
	}
	if strings.Contains(registrations, `/api/admin/customers/{customer_id}/contact-policy`) {
		t.Fatal("non-canonical admin contact-policy route is registered")
	}
	contract, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(contract), "Contact eligibility checker accepts at most 1000 unique customer IDs per preview or dispatch check") != 1 {
		t.Fatal("OpenAPI does not freeze the Contact eligibility batch maximum at 1000")
	}

	authService := &recordingAuth{}
	authHandler, err := authhttp.NewHandler(authService)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), authHandler, authHandler)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/api/v1/customers/1/contact-policy", strings.NewReader(`{"expected_version":1}`))
		request.AddCookie(&http.Cookie{Name: authhttp.SessionCookieName, Value: "router-test-session"})
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "contact-policy-route-test")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || len(authService.capabilities()) != 0 {
			t.Fatalf("%s missing CSRF status/capabilities=%d/%v, want 403/none", method, response.Code, authService.capabilities())
		}
	}
}

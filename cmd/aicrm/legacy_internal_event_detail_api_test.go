package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type legacyInternalEventDetailRepositoryStub struct {
	snapshot eventport.AdminDetailSnapshot
	err      error
	calls    int
}

func (stub *legacyInternalEventDetailRepositoryStub) Read(context.Context, eventport.EventID) (eventport.AdminDetailSnapshot, error) {
	stub.calls++
	return stub.snapshot, stub.err
}

func TestLegacyInternalEventDetailSuccessIsExactAndSafe(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	completed := stamp.Add(time.Minute)
	repository := &legacyInternalEventDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{
		Found: true,
		Event: eventport.AdminReadEvent{EventID: 42, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: false},
		Deliveries: []eventport.AdminReadDelivery{{
			EventID: 42, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: &completed,
		}},
	}}
	request := internalEventsAuthorizedRequest(http.MethodGet, "/api/admin/internal-events/42", authport.RoleAdmin)
	response := httptest.NewRecorder()
	newLegacyInternalEventDetailHandler(repository).Get(response, request)
	if response.Code != http.StatusOK || repository.calls != 1 {
		t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
	}
	for key, want := range map[string]string{"Content-Type": "application/json", "Cache-Control": "private, no-store", "X-Content-Type-Options": "nosniff"} {
		if got := response.Header().Get(key); got != want {
			t.Fatalf("header %s=%q want %q", key, got, want)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 9 || payload["ok"] != true || payload["registry_id"] != eventport.AdminReadRegistryID || payload["source_status"] != "local_read_model" || payload["delivery_observation_available"] != true || payload["external_delivery"] != "unknown" || payload["route_owner"] != "ai_crm_next" || payload["real_external_call_executed"] != false {
		t.Fatalf("payload=%v", payload)
	}
	item, ok := payload["item"].(map[string]any)
	if !ok || len(item) != 5 || item["event_id"] != float64(42) || item["event_type"] != eventport.EvTagApplied || item["occurred_at"] != stamp.Format(time.RFC3339Nano) {
		t.Fatalf("item=%v", payload["item"])
	}
	if deliveries, ok := item["deliveries"].([]any); !ok || len(deliveries) != 1 {
		t.Fatalf("deliveries=%v", item["deliveries"])
	}
	body := response.Body.String()
	for _, forbidden := range []string{"payload", "customer_id", "idempotency_key", "lease_owner", "river_job_id", "last_error_code", "provider"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forbidden field %q in body %s", forbidden, body)
		}
	}
}

func TestLegacyInternalEventDetailNoRowAndRepositoryFailure(t *testing.T) {
	request := internalEventsAuthorizedRequest(http.MethodGet, "/api/admin/internal-events/99", authport.RoleAdmin)
	repository := &legacyInternalEventDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{Deliveries: make([]eventport.AdminReadDelivery, 0)}}
	response := httptest.NewRecorder()
	newLegacyInternalEventDetailHandler(repository).Get(response, request)
	assertLegacyInternalDetailErrorBody(t, response, http.StatusNotFound, "internal_event_not_found")
	if repository.calls != 1 {
		t.Fatalf("not-found calls=%d", repository.calls)
	}
	repository = &legacyInternalEventDetailRepositoryStub{err: errors.New("database unavailable")}
	response = httptest.NewRecorder()
	newLegacyInternalEventDetailHandler(repository).Get(response, request)
	assertLegacyInternalDetailErrorBody(t, response, http.StatusServiceUnavailable, "internal_event_observation_unavailable")
}

func TestLegacyInternalEventDetailPathAndQueryGrammar(t *testing.T) {
	for _, value := range []string{"", "0", "00", "01", "+1", "-1", "1.0", "1e2", " 1", "1 ", "１２", "9223372036854775808"} {
		request := internalEventsAuthorizedRequest(http.MethodGet, "/api/admin/internal-events/1", authport.RoleAdmin)
		request.URL.Path = "/api/admin/internal-events/" + value
		response := httptest.NewRecorder()
		newLegacyInternalEventDetailHandler(&legacyInternalEventDetailRepositoryStub{}).Get(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("id=%q status=%d body=%s", value, response.Code, response.Body.String())
		}
	}
	for _, query := range []string{"?x=1", "?x", "?x=1&x=2", "?%ZZ=1", "?%ff=1"} {
		request := internalEventsAuthorizedRequest(http.MethodGet, "/api/admin/internal-events/1", authport.RoleAdmin)
		request.URL.RawQuery = strings.TrimPrefix(query, "?")
		response := httptest.NewRecorder()
		newLegacyInternalEventDetailHandler(&legacyInternalEventDetailRepositoryStub{}).Get(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("query=%q status=%d body=%s", query, response.Code, response.Body.String())
		}
	}
	if got, err := parseLegacyInternalEventDetailID("9223372036854775807"); err != nil || got != 9223372036854775807 {
		t.Fatalf("max id got=%d err=%v", got, err)
	}
}

func TestLegacyInternalEventDetailOpenAPIUsesCanonicalSignedInt64EventID(t *testing.T) {
	const signedInt64Pattern = `^(?:[1-9][0-9]{0,17}|[1-8][0-9]{18}|9[0-1][0-9]{17}|92[0-1][0-9]{16}|922[0-2][0-9]{15}|9223[0-2][0-9]{14}|92233[0-6][0-9]{13}|922337[0-1][0-9]{12}|92233720[0-2][0-9]{10}|922337203[0-5][0-9]{9}|9223372036[0-7][0-9]{8}|92233720368[0-4][0-9]{7}|922337203685[0-3][0-9]{6}|9223372036854[0-6][0-9]{5}|92233720368547[0-6][0-9]{4}|922337203685477[0-4][0-9]{3}|9223372036854775[0-7][0-9]{2}|922337203685477580[0-7])$`
	contents, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wantSchema := "schema: { type: string, minLength: 1, maxLength: 19, pattern: '" + signedInt64Pattern + "' }"
	if !strings.Contains(string(contents), wantSchema) {
		t.Fatal("0371 OpenAPI event_id schema does not use the canonical signed-int64 pattern")
	}
	if strings.Contains(string(contents), "schema: { type: string, minLength: 1, maxLength: 19, pattern: '^[1-9][0-9]*$' }") {
		t.Fatal("0371 OpenAPI event_id schema still accepts signed-int64 overflow")
	}
	pattern := regexp.MustCompile(signedInt64Pattern)
	for _, test := range []struct {
		value string
		valid bool
	}{
		{value: "1", valid: true},
		{value: "9223372036854775807", valid: true},
		{value: "9223372036854775806", valid: true},
		{value: "9223372036854775808", valid: false},
		{value: "9223372036854775810", valid: false},
		{value: "0001", valid: false},
		{value: "+1", valid: false},
		{value: " 1", valid: false},
	} {
		if got := pattern.MatchString(test.value); got != test.valid {
			t.Fatalf("OpenAPI event_id pattern value=%q got=%v want=%v", test.value, got, test.valid)
		}
	}
}

func TestLegacyInternalEventDetailNoDeliveryUsesEmptyArray(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	repository := &legacyInternalEventDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{Found: true, Event: eventport.AdminReadEvent{EventID: 5, EventType: "custom.local_fact", OccurredAt: stamp}}}
	response := httptest.NewRecorder()
	newLegacyInternalEventDetailHandler(repository).Get(response, internalEventsAuthorizedRequest(http.MethodGet, "/api/admin/internal-events/5?", authport.RoleAdmin))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"deliveries":[]`) {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
}

func TestLegacyInternalEventDetailAuthAndMethodContract(t *testing.T) {
	repository := &legacyInternalEventDetailRepositoryStub{}
	response := httptest.NewRecorder()
	newLegacyInternalEventDetailHandler(repository).Get(response, httptest.NewRequest(http.MethodGet, "/api/admin/internal-events/1", nil))
	assertLegacyInternalDetailErrorBody(t, response, http.StatusUnauthorized, "authentication_required")
	response = httptest.NewRecorder()
	writeLegacyInternalEventsMethodNotAllowed(response, httptest.NewRequest(http.MethodPost, "/api/admin/internal-events/1", nil))
	if response.Header().Get("Allow") != http.MethodGet || response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status/headers=%d/%v", response.Code, response.Header())
	}
}

func TestLegacyInternalEventDetailRouteRejectsMethodBeforeAuth(t *testing.T) {
	service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	detailRepository := &legacyInternalEventDetailRepositoryStub{}
	router, err := newAPIHandlerWithAdminRead(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil, &legacyInternalEventsRepositoryStub{}, nil, detailRepository)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/admin/internal-events/42", nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || detailRepository.calls != 0 {
		t.Fatalf("status/allow/calls=%d/%q/%d body=%s", response.Code, response.Header().Get("Allow"), detailRepository.calls, response.Body.String())
	}
	assertLegacyInternalDetailErrorBody(t, response, http.StatusMethodNotAllowed, "method_not_allowed")
	assertLegacyInternalDetailRouteSecurityHeaders(t, response)
}

func TestLegacyInternalEventDetailRouteAuthMatrixAndNoCSRF(t *testing.T) {
	validRepository := func() *legacyInternalEventDetailRepositoryStub {
		return &legacyInternalEventDetailRepositoryStub{snapshot: eventport.AdminDetailSnapshot{
			Found:      true,
			Event:      eventport.AdminReadEvent{EventID: 42, EventType: eventport.EvTagApplied, OccurredAt: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)},
			Deliveries: []eventport.AdminReadDelivery{},
		}}
	}
	routerFor := func(service authport.Service, repository *legacyInternalEventDetailRepositoryStub) http.Handler {
		authHandler, err := authhttp.NewHandler(service)
		if err != nil {
			t.Fatal(err)
		}
		legacy, err := NewHandler(service, &legacyCustomerStub{})
		if err != nil {
			t.Fatal(err)
		}
		router, err := newAPIHandlerWithAdminRead(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil, &legacyInternalEventsRepositoryStub{}, nil, repository)
		if err != nil {
			t.Fatal(err)
		}
		return router
	}

	unauthorized := []struct {
		name    string
		request func() *http.Request
	}{
		{name: "anonymous", request: func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/admin/internal-events/42", nil) }},
		{name: "bearer-only", request: func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/api/admin/internal-events/42", nil)
			request.Header.Set("Authorization", "Bearer "+legacyToken(31))
			return request
		}},
		{name: "service-only", request: func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/api/admin/internal-events/42", nil)
			request.Header.Set("X-AICRM-Service-Token", "service-only-token")
			return request
		}},
	}
	for _, test := range unauthorized {
		t.Run("401/"+test.name, func(t *testing.T) {
			repository := validRepository()
			service := &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}
			response := httptest.NewRecorder()
			routerFor(service, repository).ServeHTTP(response, test.request())
			if response.Code != http.StatusUnauthorized || repository.calls != 0 {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
			}
			assertLegacyInternalDetailRouteSecurityHeaders(t, response)
			assertLegacyInternalDetailErrorBody(t, response, http.StatusUnauthorized, "authentication_required")
		})
	}

	staffID := int64(7)
	for _, test := range []struct {
		name    string
		service authport.Service
	}{
		{name: "non-admin", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.Role("viewer")}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}},
		{name: "ops", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}},
		{name: "sales", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleSales, StaffID: &staffID}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}},
		{name: "non-global", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeSelf}}},
		{name: "owner-scoped", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 7}}},
		{name: "missing-capability", service: &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}}},
	} {
		t.Run("403/"+test.name, func(t *testing.T) {
			repository := validRepository()
			request := legacyRequest(http.MethodGet, "/api/admin/internal-events/42", legacyToken(32))
			response := httptest.NewRecorder()
			routerFor(test.service, repository).ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || repository.calls != 0 {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
			}
			assertLegacyInternalDetailRouteSecurityHeaders(t, response)
			assertLegacyInternalDetailErrorBody(t, response, http.StatusForbidden, "forbidden")
		})
	}

	t.Run("GET has no CSRF", func(t *testing.T) {
		repository := validRepository()
		service := &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}
		response := httptest.NewRecorder()
		request := legacyRequest(http.MethodGet, "/api/admin/internal-events/42", legacyToken(33))
		// Deliberately omit X-CSRF-Token: the frozen GET is session-authenticated
		// but has no CSRF requirement.
		routerFor(service, repository).ServeHTTP(response, request)
		if response.Code != http.StatusOK || repository.calls != 1 {
			t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
		}
		assertLegacyInternalDetailRouteSecurityHeaders(t, response)
	})
}

func assertLegacyInternalDetailRouteSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for key, want := range map[string]string{
		"Content-Type":           "application/json",
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header().Get(key); got != want {
			t.Fatalf("header %s=%q want %q", key, got, want)
		}
	}
}

func assertLegacyInternalDetailErrorBody(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assertLegacyInternalEventsAuthErrorBody(t, response, status, code)
}

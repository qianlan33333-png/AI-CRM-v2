package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/platform/readiness"
)

func TestLegacyDataHealthRoutesUseOneLocalObservationEach(t *testing.T) {
	source := &dataHealthSourceStub{input: dataHealthReadyInput()}
	router := dataHealthRouter(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, source)
	for _, target := range []string{legacyDataHealthChecksPath, legacyDataHealthCheckPath[:len(legacyDataHealthCheckPath)-len("{check_id}")] + "database_readiness", legacyDataHealthSummaryPath} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(1)))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", target, response.Code, response.Body.String())
		}
		var body map[string]any
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil || body["real_external_call_executed"] != false {
			t.Fatalf("GET %s body=%v err=%v", target, body, err)
		}
		if target == legacyDataHealthSummaryPath {
			counts, gates := body["counts"].(map[string]any), body["gate_counts"].(map[string]any)
			if body["overall_status"] != "ok" || counts["ok"] != float64(4) || counts["warn"] != float64(0) || counts["fail"] != float64(0) || counts["not_applicable"] != float64(0) || gates["pass"] != float64(4) || gates["warn"] != float64(0) || gates["block"] != float64(0) {
				t.Fatalf("summary=%v", body)
			}
		}
		assertNoDataHealthSensitiveKeys(t, body)
	}
	if source.calls != 3 {
		t.Fatalf("Observe calls=%d, want 3", source.calls)
	}
}

func TestLegacyDataHealthUnknownAndUnavailableContracts(t *testing.T) {
	source := &dataHealthSourceStub{input: dataHealthReadyInput()}
	router := dataHealthRouter(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, source)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/data-health/checks/not-a-check", legacyToken(2)))
	if response.Code != http.StatusNotFound || response.Body.String() != "{\"check_id\":\"not-a-check\",\"error_code\":\"data_health_check_not_found\",\"ok\":false,\"real_external_call_executed\":false,\"status_code\":404}\n" {
		t.Fatalf("not found=%d %s", response.Code, response.Body.String())
	}
	if source.calls != 0 {
		t.Fatalf("unknown source calls=%d", source.calls)
	}
	unavailable := dataHealthRouter(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin})
	response = httptest.NewRecorder()
	unavailable.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/data-health/checks/not-a-check", legacyToken(3)))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown unavailable=%d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	unavailable.ServeHTTP(response, legacyRequest(http.MethodGet, legacyDataHealthChecksPath, legacyToken(3)))
	if response.Code != http.StatusServiceUnavailable || !contains(response.Body.String(), "data_health_observation_unavailable") {
		t.Fatalf("unavailable=%d %s", response.Code, response.Body.String())
	}
}

func TestLegacyDataHealthRequiresAdminAndGet(t *testing.T) {
	unauthenticated := httptest.NewRecorder()
	anonymousSource := &dataHealthSourceStub{input: dataHealthReadyInput()}
	dataHealthRouter(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, anonymousSource).ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, legacyDataHealthChecksPath, nil))
	if unauthenticated.Code != http.StatusUnauthorized || anonymousSource.calls != 0 {
		t.Fatalf("anonymous status/calls=%d/%d", unauthenticated.Code, anonymousSource.calls)
	}
	for _, principal := range []authport.Principal{{AdminUserID: 7, Role: authport.RoleOps}, {AdminUserID: 7, Role: authport.RoleSales}} {
		response := httptest.NewRecorder()
		source := &dataHealthSourceStub{input: dataHealthReadyInput()}
		dataHealthRouter(t, principal, source).ServeHTTP(response, legacyRequest(http.MethodGet, legacyDataHealthChecksPath, legacyToken(4)))
		if (response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden) || source.calls != 0 {
			t.Fatalf("principal=%+v status/calls=%d/%d", principal, response.Code, source.calls)
		}
	}
	for _, test := range []struct {
		name    string
		request *http.Request
		service authport.Service
	}{
		{"owner scoped", legacyRequest(http.MethodGet, legacyDataHealthChecksPath, legacyToken(6)), &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 7}}},
		{"bearer", httptest.NewRequest(http.MethodGet, legacyDataHealthChecksPath, nil), &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}},
		{"service principal", legacyRequest(http.MethodGet, legacyDataHealthChecksPath, legacyToken(7)), &dataHealthAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.Role("service")}, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "bearer" {
				test.request.Header.Set("Authorization", "Bearer opaque-service-token")
			}
			source := &dataHealthSourceStub{input: dataHealthReadyInput()}
			response := httptest.NewRecorder()
			dataHealthRouterWithService(t, test.service, source).ServeHTTP(response, test.request)
			if (response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden) || source.calls != 0 {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, source.calls, response.Body.String())
			}
		})
	}
	for _, target := range []string{legacyDataHealthChecksPath, "/api/admin/data-health/checks/database_readiness", legacyDataHealthSummaryPath} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			source := &dataHealthSourceStub{input: dataHealthReadyInput()}
			response := httptest.NewRecorder()
			dataHealthRouter(t, authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}, source).ServeHTTP(response, legacyRequest(method, target, legacyToken(5)))
			if response.Code != http.StatusMethodNotAllowed || source.calls != 0 {
				t.Fatalf("%s %s status=%d calls=%d", method, target, response.Code, source.calls)
			}
		}
	}
}

func dataHealthRouter(t *testing.T, principal authport.Principal, sources ...legacyDataHealthObservationSource) http.Handler {
	t.Helper()
	service := &legacyAuthStub{principal: principal}
	return dataHealthRouterWithService(t, service, sources...)
}

func dataHealthRouterWithService(t *testing.T, service authport.Service, sources ...legacyDataHealthObservationSource) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithAll(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil, sources...)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type dataHealthAuthStub struct {
	principal     authport.Principal
	authorization authport.Authorization
}

func (stub *dataHealthAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return stub.principal, nil
}
func (stub *dataHealthAuthStub) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	return stub.authorization, nil
}
func (*dataHealthAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}
func (*dataHealthAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func assertNoDataHealthSensitiveKeys(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{"dsn": true, "token": true, "error": true, "customer": true, "event": true, "job": true, "receipt": true, "payload": true, "provider": true}
	var walk func(any)
	walk = func(node any) {
		switch current := node.(type) {
		case map[string]any:
			for key, child := range current {
				if forbidden[key] {
					t.Fatalf("sensitive key %q in %#v", key, current)
				}
				walk(child)
			}
		case []any:
			for _, child := range current {
				walk(child)
			}
		}
	}
	walk(value)
}

type dataHealthSourceStub struct {
	input readiness.Input
	calls int
}

func (stub *dataHealthSourceStub) Observe(context.Context) readiness.Input {
	stub.calls++
	return stub.input
}
func dataHealthReadyInput() readiness.Input {
	return readiness.Input{Database: readiness.DatabaseObservation{Kind: readiness.DatabasePostgres, Probe: readiness.ProbeHealthy}, Migration: readiness.MigrationObservation{Compatibility: readiness.MigrationCompatible}, Release: readiness.ReleaseObservation{SHAComplete: true}, Queues: readiness.QueueObservation{Probe: readiness.ProbeHealthy}}
}
func contains(value, fragment string) bool {
	return len(value) >= len(fragment) && (value == fragment || index(value, fragment) >= 0)
}
func index(value, fragment string) int {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}

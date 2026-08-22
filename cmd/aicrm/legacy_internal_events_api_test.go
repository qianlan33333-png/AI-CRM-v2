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
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type legacyInternalEventsRepositoryStub struct {
	snapshot eventport.AdminReadSnapshot
	err      error
	calls    int
}

func (stub *legacyInternalEventsRepositoryStub) Read(context.Context, string) (eventport.AdminReadSnapshot, error) {
	stub.calls++
	return stub.snapshot, stub.err
}

func TestLegacyInternalEventsListContractAndRedaction(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	completed := stamp.Add(time.Minute)
	repository := &legacyInternalEventsRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events: []eventport.AdminReadEvent{{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: false}},
		Deliveries: []eventport.AdminReadDelivery{
			{EventID: 1, Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), AttemptCount: 1, CompletedAt: &completed},
		},
	}}
	handler := newLegacyInternalEventsHandler(repository)
	request := internalEventsAuthorizedRequest(http.MethodGet, legacyInternalEventsPath+"?event_type=customer.tag_applied&limit=1", authport.RoleAdmin)
	response := httptest.NewRecorder()
	handler.List(response, request)
	if response.Code != http.StatusOK || repository.calls != 1 {
		t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v", response.Header())
	}
	var payload legacyInternalEventListResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Total != 1 || payload.Limit != 1 || payload.Offset != 0 || payload.RegistryID != eventport.AdminReadRegistryID || payload.SourceStatus != "local_read_model" || !payload.DeliveryObservationAvailable || payload.ExternalDelivery != "unknown" || payload.RouteOwner != "ai_crm_next" || payload.RealExternalCallExecuted || len(payload.Items) != 1 {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Items[0].OccurredAt != stamp.Format(time.RFC3339Nano) || len(payload.Items[0].Deliveries) != 1 || payload.Items[0].Deliveries[0].CompletedAt == nil || *payload.Items[0].Deliveries[0].CompletedAt != completed.Format(time.RFC3339Nano) {
		t.Fatalf("item=%+v", payload.Items[0])
	}
	body := response.Body.String()
	for _, forbidden := range []string{"payload", "customer_id", "idempotency_key", "lease_owner", "river_job_id", "last_error_code", "provider"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forbidden field %q in body %s", forbidden, body)
		}
	}
}

func TestLegacyInternalEventDiagnosticsContract(t *testing.T) {
	stamp := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	repository := &legacyInternalEventsRepositoryStub{snapshot: eventport.AdminReadSnapshot{
		Events:     []eventport.AdminReadEvent{{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Dispatched: false}},
		Deliveries: []eventport.AdminReadDelivery{{EventID: 1, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending)}},
	}}
	response := httptest.NewRecorder()
	newLegacyInternalEventsHandler(repository).Diagnostics(response, internalEventsAuthorizedRequest(http.MethodGet, legacyInternalEventsDiagnosticsPath+"?status=pending", authport.RoleAdmin))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload legacyInternalEventDiagnosticsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Filters.Status != string(eventport.DeliveryPending) || payload.EventCount != 1 || payload.UndispatchedEventCount != 1 || payload.DeliveryCounts.Pending != 1 || payload.DeliveryCounts.Completed != 0 || len(payload.ConsumerRegistry) != 4 || len(payload.ObservedDomains) != 2 || len(payload.UnobservedDomains) != 3 || payload.RealExternalCallExecuted {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestLegacyInternalEventQueryGrammar(t *testing.T) {
	valid := []struct {
		query string
		want  eventport.AdminReadQuery
	}{
		{"", eventport.AdminReadQuery{Limit: 50}},
		{"event_type=%20customer.tag_applied%20&consumer=stats.tag-applied.v1&status=completed&limit=200&offset=100000", eventport.AdminReadQuery{EventType: "customer.tag_applied", Consumer: eventport.ConsumerStatsTagApplied, Status: string(eventport.DeliveryCompleted), Limit: 200, Offset: 100000}},
	}
	for _, test := range valid {
		got, err := parseLegacyInternalEventQuery(test.query, true)
		if err != nil || got != test.want {
			t.Fatalf("query=%q got=%+v err=%v want=%+v", test.query, got, err, test.want)
		}
	}
	for _, consumer := range []string{eventport.ConsumerAutomationTagTrigger, eventport.ConsumerStatsTagApplied, eventport.ConsumerOperationCycleFact, eventport.ConsumerCloudCampaignFact} {
		query, err := parseLegacyInternalEventQuery("consumer="+consumer, true)
		if err != nil || query.Consumer != consumer {
			t.Fatalf("consumer=%q parsed as=%+v err=%v", consumer, query, err)
		}
	}
	for _, status := range []string{string(eventport.DeliveryPending), string(eventport.DeliveryProcessing), string(eventport.DeliveryCompleted), string(eventport.DeliveryFinalFailed), string(eventport.DeliveryOutcomeUnknown)} {
		query, err := parseLegacyInternalEventQuery("status="+status, true)
		if err != nil || query.Status != status {
			t.Fatalf("status=%q parsed as=%+v err=%v", status, query, err)
		}
	}
	for _, query := range []string{"limit=1&offset=0", "limit=200&offset=100000"} {
		if _, err := parseLegacyInternalEventQuery(query, true); err != nil {
			t.Fatalf("boundary query=%q rejected: %v", query, err)
		}
	}
	invalid := []string{
		"unknown=1", "=value", "event_type=", "event_type=a&event_type=b", "event_type=" + strings.Repeat("x", 201),
		"consumer=", "consumer=" + strings.Repeat("x", 201), "consumer=unknown.consumer", "status=", "status=" + strings.Repeat("x", 201), "status=bad",
		"limit=01", "limit=0", "limit=201", "limit=+1", "limit=%201%20", "limit=9223372036854775808",
		"offset=01", "offset=-1", "offset=+1", "offset=%201%20", "offset=100001", "offset=9223372036854775808",
		"limit=1.0", "limit=1&offset=0&trace_id=x", "limit=%ZZ", "limit", "event_type=%ff", "%ff=value",
	}
	for _, query := range invalid {
		query := query
		t.Run("reject/"+query, func(t *testing.T) {
			if _, err := parseLegacyInternalEventQuery(query, true); err == nil {
				t.Fatalf("query=%q accepted", query)
			}
		})
	}
	if _, err := parseLegacyInternalEventQuery("limit=1", false); err == nil {
		t.Fatal("diagnostics accepted pagination key")
	}
}

func TestLegacyInternalEventsRepositoryErrorsAre503AndStableEnvelope(t *testing.T) {
	for _, test := range []struct {
		name   string
		path   string
		invoke func(*legacyInternalEventsHandler, http.ResponseWriter, *http.Request)
	}{
		{name: "list", path: legacyInternalEventsPath, invoke: func(handler *legacyInternalEventsHandler, writer http.ResponseWriter, request *http.Request) {
			handler.List(writer, request)
		}},
		{name: "diagnostics", path: legacyInternalEventsDiagnosticsPath, invoke: func(handler *legacyInternalEventsHandler, writer http.ResponseWriter, request *http.Request) {
			handler.Diagnostics(writer, request)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &legacyInternalEventsRepositoryStub{err: errors.New("repository unavailable")}
			handler := newLegacyInternalEventsHandler(repository)
			response := httptest.NewRecorder()
			test.invoke(handler, response, internalEventsAuthorizedRequest(http.MethodGet, test.path, authport.RoleAdmin))
			if response.Code != http.StatusServiceUnavailable || repository.calls != 1 {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
			}
			assertLegacyInternalEventsAuthErrorBody(t, response, http.StatusServiceUnavailable, "internal_event_observation_unavailable")
		})
	}
}

func TestLegacyInternalEventsAuthAndMethodHeaders(t *testing.T) {
	repository := &legacyInternalEventsRepositoryStub{}
	handler := newLegacyInternalEventsHandler(repository)
	response := httptest.NewRecorder()
	handler.List(response, httptest.NewRequest(http.MethodGet, legacyInternalEventsPath, nil))
	if response.Code != http.StatusUnauthorized || repository.calls != 0 || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("status/calls/headers=%d/%d/%v", response.Code, repository.calls, response.Header())
	}
	response = httptest.NewRecorder()
	writeLegacyInternalEventsMethodNotAllowed(response, httptest.NewRequest(http.MethodPost, legacyInternalEventsPath, nil))
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("method status/headers=%d/%v", response.Code, response.Header())
	}
}

func TestLegacyInternalEventsRouteMethodAndAuthMatrix(t *testing.T) {
	repository := &legacyInternalEventsRepositoryStub{}
	service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := NewHandler(service, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithAdminRead(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil, repository, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{legacyInternalEventsPath, legacyInternalEventsDiagnosticsPath} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("X-Content-Type-Options") != "nosniff" || repository.calls != 0 {
			t.Fatalf("path=%s status/allow/headers/calls=%d/%q/%q/%d", path, response.Code, response.Header().Get("Allow"), response.Header().Get("X-Content-Type-Options"), repository.calls)
		}
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("anonymous path=%s status/headers=%d/%v", path, response.Code, response.Header())
		}
		assertLegacyInternalEventsAuthErrorBody(t, response, http.StatusUnauthorized, "authentication_required")
		response = httptest.NewRecorder()
		bearerOnly := httptest.NewRequest(http.MethodGet, path, nil)
		bearerOnly.Header.Set("Authorization", "Bearer "+legacyToken(12))
		router.ServeHTTP(response, bearerOnly)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("bearer-only path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		assertLegacyInternalEventsAuthErrorBody(t, response, http.StatusUnauthorized, "authentication_required")
	}
	request := legacyRequest(http.MethodGet, legacyInternalEventsPath, legacyToken(11))
	request.Header.Set("X-Request-ID", "router-test-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repository.calls != 1 {
		t.Fatalf("authorized status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
	}
}

func TestLegacyInternalEventsAuthErrorBodiesForRoleScopeAndCapability(t *testing.T) {
	testCases := []struct {
		name           string
		service        authport.Service
		wantStatus     int
		wantCode       string
		wantRepository int
	}{
		{
			name:       "wrong role",
			service:    &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleOps}},
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name: "wrong scope",
			service: &dataHealthAuthStub{
				principal:     authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin},
				authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9},
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name: "wrong capability",
			service: &dataHealthAuthStub{
				principal:     authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin},
				authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "forbidden",
		},
		{
			name:       "authentication dependency unavailable",
			service:    &legacyAuthStub{authenticateErr: authport.ErrAuthenticationUnavailable},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "internal_event_observation_unavailable",
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			repository := &legacyInternalEventsRepositoryStub{}
			authHandler, err := authhttp.NewHandler(test.service)
			if err != nil {
				t.Fatal(err)
			}
			legacy, err := NewHandler(test.service, &legacyCustomerStub{})
			if err != nil {
				t.Fatal(err)
			}
			router, err := newAPIHandlerWithAdminRead(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy, nil, repository, nil)
			if err != nil {
				t.Fatal(err)
			}
			request := legacyRequest(http.MethodGet, legacyInternalEventsPath, legacyToken(13))
			request.Header.Set("X-Request-ID", "auth-matrix-"+test.name)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus || repository.calls != test.wantRepository {
				t.Fatalf("status/calls=%d/%d body=%s", response.Code, repository.calls, response.Body.String())
			}
			if test.wantStatus == http.StatusOK {
				return
			}
			assertLegacyInternalEventsAuthErrorBody(t, response, test.wantStatus, test.wantCode)
		})
	}
}

func assertLegacyInternalEventsAuthErrorBody(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth error status=%d body=%q: %v", status, response.Body.String(), err)
	}
	if len(payload) != 6 {
		t.Fatalf("auth error keys=%d, want exactly 6 payload=%v", len(payload), payload)
	}
	if got, ok := payload["ok"].(bool); !ok || got {
		t.Fatalf("auth error ok=%v, want false payload=%v", payload["ok"], payload)
	}
	if got, ok := payload["status_code"].(float64); !ok || got != float64(status) {
		t.Fatalf("auth error status_code=%v, want %d payload=%v", payload["status_code"], status, payload)
	}
	got, _ := payload["error_code"].(string)
	if got != code {
		t.Fatalf("auth error code=%q, want %q payload=%v", got, code, payload)
	}
	if message, ok := payload["message"].(string); !ok || message == "" {
		t.Fatalf("auth error message=%v, want non-empty payload=%v", payload["message"], payload)
	}
	if got, ok := payload["request_id"].(string); !ok || got == "" {
		t.Fatalf("auth error request_id=%v payload=%v", payload["request_id"], payload)
	}
	if got, ok := payload["real_external_call_executed"].(bool); !ok || got {
		t.Fatalf("auth error real_external_call_executed=%v payload=%v", payload["real_external_call_executed"], payload)
	}
}

func TestLegacyInternalEventsUnexpected500IsFixedAndRedacted(t *testing.T) {
	body := []byte(`{"code":"INTERNAL_ERROR","message":"internal","request_id":"fixed-500"}`)
	got := normalizeLegacyInternalEventsError(http.StatusInternalServerError, body, "")
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["error_code"] != "internal_event_observation_failed" || payload["message"] != "internal event observation failed" || payload["request_id"] != "fixed-500" || payload["real_external_call_executed"] != false {
		t.Fatalf("normalized 500=%v", payload)
	}
	if _, leaked := payload["code"]; leaked {
		t.Fatalf("normalized 500 retained platform code: %v", payload)
	}
}

func TestLegacyInternalEventSQLDoesNotReadSensitiveColumns(t *testing.T) {
	contents, err := os.ReadFile("../../internal/events/store/queries/admin_read.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"payload", "customer_id", "idempotency_key", "lease_owner", "lease_expires_at", "last_error_code", "river_job_id", "raw_error"} {
		if strings.Contains(string(contents), forbidden) {
			t.Fatalf("forbidden SQL token %q", forbidden)
		}
	}
}

func internalEventsAuthorizedRequest(method, target string, role authport.Role) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 9, Role: role}, authport.SessionRef("session"))
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal})
	return request.WithContext(ctx)
}

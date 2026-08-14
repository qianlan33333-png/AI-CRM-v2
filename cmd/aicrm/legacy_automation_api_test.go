package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestLegacyAutomationRouteReadsRealTriggerReceiptContract(t *testing.T) {
	service := &recordingAuth{}
	query := &automationRunQueryStub{result: automationstore.TriggerListResult{
		Items: []automationstore.TriggerReceipt{{
			ID: 71, EventID: 81, CustomerID: 91, TagID: 101,
			TriggeredEventID: 111,
			TriggeredAt:      time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
			CompletedAt:      time.Date(2026, 8, 14, 10, 0, 1, 0, time.UTC),
		}},
		Total: 1,
	}}
	router := legacyAutomationRouter(t, service, query)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet,
		"/api/admin/automation-conversion/agent-runs?page=2&page_size=25&run_id=automation-trigger%3A71&request_id=event%3A81&agent_code=tag-trigger-v1&run_status=completed&trigger_source=customer.tag_applied",
		legacyToken(21)))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.calls != 1 || query.input.Page != 2 || query.input.PageSize != 25 || query.input.ReceiptID == nil || *query.input.ReceiptID != 71 || query.input.EventID == nil || *query.input.EventID != 81 {
		t.Fatalf("query calls/input=%d/%+v", query.calls, query.input)
	}
	if got := service.capabilities(); len(got) != 1 || got[0] != authport.CapabilityConfigOverviewRead {
		t.Fatalf("authorized capabilities=%v", got)
	}
	var payload struct {
		Items []struct {
			RunID            string `json:"run_id"`
			RequestID        string `json:"request_id"`
			AgentCode        string `json:"agent_code"`
			RunStatus        string `json:"run_status"`
			TriggerSource    string `json:"trigger_source"`
			CustomerID       int64  `json:"customer_id"`
			TagID            int64  `json:"tag_id"`
			SourceEventID    int64  `json:"source_event_id"`
			TriggeredEventID int64  `json:"triggered_event_id"`
			HasError         bool   `json:"has_error"`
		} `json:"items"`
		Total      int64  `json:"total"`
		Page       int32  `json:"page"`
		PageSize   int32  `json:"page_size"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].RunID != "automation-trigger:71" || payload.Total != 1 ||
		payload.Page != 2 || payload.PageSize != 25 || payload.Visibility != "masked" || payload.Items[0].HasError {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestLegacyAutomationRouteFailsClosedForIdentityFiltersAndNonAdmin(t *testing.T) {
	t.Run("identity filter", func(t *testing.T) {
		query := &automationRunQueryStub{}
		response := httptest.NewRecorder()
		legacyAutomationRouter(t, &recordingAuth{}, query).ServeHTTP(response,
			legacyRequest(http.MethodGet, "/api/admin/automation-conversion/agent-runs?unionid=secret", legacyToken(22)))
		assertLegacyError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
		if query.calls != 0 {
			t.Fatalf("query calls=%d, want 0", query.calls)
		}
	})

	t.Run("sales role", func(t *testing.T) {
		staffID := int64(42)
		service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}}
		query := &automationRunQueryStub{}
		response := httptest.NewRecorder()
		legacyAutomationRouter(t, service, query).ServeHTTP(response,
			legacyRequest(http.MethodGet, "/api/admin/automation-conversion/agent-runs", legacyToken(23)))
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
		if query.calls != 0 {
			t.Fatalf("query calls=%d, want 0", query.calls)
		}
	})
}

func legacyAutomationRouter(t *testing.T, service authport.Service, query *automationRunQueryStub) http.Handler {
	t.Helper()
	legacy, err := NewHandler(service, &legacyCustomerStub{result: legacyCustomerResult()})
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &candidateHandler{Handler: authHandler, automationRuns: query}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, candidate, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type automationRunQueryStub struct {
	result automationstore.TriggerListResult
	err    error
	input  automationstore.TriggerListInput
	calls  int
}

func (stub *automationRunQueryStub) List(_ context.Context, input automationstore.TriggerListInput) (automationstore.TriggerListResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	operationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
)

type operationCycleAPIStub struct {
	reports []operationapp.ReportCommand
	events  []operationapp.ActionEventCommand
	starts  []operationapp.StartCommand
}

func (stub *operationCycleAPIStub) Report(_ context.Context, command operationapp.ReportCommand) (map[string]any, error) {
	stub.reports = append(stub.reports, command)
	return map[string]any{"accepted": true}, nil
}
func (*operationCycleAPIStub) ListStrategies(context.Context, int32, int32) (map[string]any, error) {
	return map[string]any{"items": []any{}}, nil
}
func (*operationCycleAPIStub) GetStrategy(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) ListRuns(context.Context, string, int32, int32) (map[string]any, error) {
	return map[string]any{"items": []any{}}, nil
}
func (*operationCycleAPIStub) GetRun(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (stub *operationCycleAPIStub) Start(_ context.Context, command operationapp.StartCommand) (map[string]any, error) {
	stub.starts = append(stub.starts, command)
	return map[string]any{"status": "queued"}, nil
}
func (*operationCycleAPIStub) CurrentAction(context.Context, string) (map[string]any, error) {
	return map[string]any{"current_action": nil}, nil
}
func (*operationCycleAPIStub) GetActionResult(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) Claim(context.Context, string, string) (map[string]any, error) {
	return map[string]any{"claimed": false}, nil
}
func (stub *operationCycleAPIStub) RecordActionEvent(_ context.Context, command operationapp.ActionEventCommand) (map[string]any, error) {
	stub.events = append(stub.events, command)
	return map[string]any{"status": command.EventType}, nil
}
func (*operationCycleAPIStub) Heartbeat(context.Context, operationapp.RunnerHeartbeatCommand) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) ContextIndex(context.Context, int32, int32) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) StrategyContext(context.Context, string, string, int32, int32, map[string]string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) CreateProposal(context.Context, operationapp.ProposalCommand) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*operationCycleAPIStub) ListProposals(context.Context, string, int32, int32) (map[string]any, error) {
	return map[string]any{"items": []any{}}, nil
}
func (*operationCycleAPIStub) DecideProposal(context.Context, string, string, string) (map[string]any, error) {
	return map[string]any{}, nil
}

var _ legacyOperationCycleApplication = (*operationCycleAPIStub)(nil)

type operationCycleAuthStub struct {
	principal operationServicePrincipal
	purposes  []string
	err       error
}

func (stub *operationCycleAuthStub) AuthenticateOperation(_ context.Context, _ *http.Request, purpose string) (operationServicePrincipal, error) {
	stub.purposes = append(stub.purposes, purpose)
	return stub.principal, stub.err
}

var _ operationServiceAuthenticator = (*operationCycleAuthStub)(nil)

func TestOperationCycleReportUsesOnlyServiceIdentity(t *testing.T) {
	service := &operationCycleAPIStub{}
	authenticator := &operationCycleAuthStub{principal: operationServicePrincipal{ClientID: "campaign-agent", PrincipalID: "runner-1"}}
	handler := &Handler{operationCycles: service, operationAuth: authenticator}
	request := httptest.NewRequest(http.MethodPost, "/api/operation-cycles/reports", strings.NewReader(`{"schema_version":"operation_cycle_snapshot.v1","strategy_key":"growth","run_key":"run-1","revision":1}`))
	request.Header.Set("Idempotency-Key", "report-key")
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(1)})
	response := httptest.NewRecorder()
	handler.ReportOperationCycle(response, request)
	if response.Code != http.StatusAccepted || len(service.reports) != 1 {
		t.Fatalf("status/reports=%d/%d body=%s", response.Code, len(service.reports), response.Body.String())
	}
	if got := service.reports[0]; got.ClientID != "campaign-agent" || got.ReporterID != "runner-1" || got.IdempotencyKey != "report-key" {
		t.Fatalf("report=%#v", got)
	}
	if len(authenticator.purposes) != 1 || authenticator.purposes[0] != "operation_cycle_report_write" {
		t.Fatalf("purposes=%v", authenticator.purposes)
	}
}

func TestOperationCycleServiceRoutesFailClosedWithoutVerifierAndRequireEventHeader(t *testing.T) {
	service := &operationCycleAPIStub{}
	t.Run("no verifier has no cookie fallback", func(t *testing.T) {
		response := httptest.NewRecorder()
		(&Handler{operationCycles: service}).ReportOperationCycle(response, httptest.NewRequest(http.MethodPost, "/api/operation-cycles/reports", strings.NewReader(`{}`)))
		if response.Code != http.StatusServiceUnavailable || len(service.reports) != 0 {
			t.Fatalf("status/reports=%d/%d", response.Code, len(service.reports))
		}
	})
	t.Run("event key is mandatory header", func(t *testing.T) {
		handler := &Handler{operationCycles: service, operationAuth: &operationCycleAuthStub{principal: operationServicePrincipal{ClientID: "runner", PrincipalID: "runner-1"}}}
		response := httptest.NewRecorder()
		handler.RecordOperationCycleActionEvent(response, httptest.NewRequest(http.MethodPost, "/api/operation-cycles/action-requests/x/events", strings.NewReader(`{"schema_version":"operation_cycle_action_event.v1","event_type":"completed","lease_token":"lease","result":{"outcome":"outcome_unknown"}}`)))
		if response.Code != http.StatusBadRequest || len(service.events) != 0 {
			t.Fatalf("status/events=%d/%d", response.Code, len(service.events))
		}
	})
}

func TestOperationCycleHumanStartRequiresRBACCSRFAndIdempotency(t *testing.T) {
	service := &operationCycleAPIStub{}
	handler := &Handler{auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}, operationCycles: service}
	endpoint := http.HandlerFunc(handler.StartOperationCycleAction)
	protected, err := handler.RequireCSRF(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	protected, err = handler.Authorize(authport.CapabilityOperationsManage, protected)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Post("/api/admin/operation-cycles/strategies/{strategy_key}/actions/{action_key}/start", handler.Authenticate(protected).ServeHTTP)
	request := httptest.NewRequest(http.MethodPost, "/api/admin/operation-cycles/strategies/growth/actions/refresh/start", strings.NewReader(`{"run_key":"run-1"}`))
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(2)})
	request.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: legacyToken(3)})
	request.Header.Set("X-CSRF-Token", legacyToken(3))
	request.Header.Set("Idempotency-Key", "start-key")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || len(service.starts) != 1 {
		t.Fatalf("status/starts=%d/%d body=%s", response.Code, len(service.starts), response.Body.String())
	}
	if got := service.starts[0]; got.StrategyKey != "growth" || got.ActionKey != "refresh" || got.RunKey != "run-1" || got.ActorID != "7" || got.IdempotencyKey != "start-key" {
		t.Fatalf("start=%#v", got)
	}

	bad := httptest.NewRecorder()
	router.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/admin/operation-cycles/strategies/growth/actions/refresh/start", strings.NewReader(`{"run_key":"run-1"}`)))
	if bad.Code != http.StatusUnauthorized && bad.Code != http.StatusForbidden {
		t.Fatalf("missing auth/csrf status=%d", bad.Code)
	}
}

func TestOperationCycleServiceAuthErrorDoesNotInvokeCommand(t *testing.T) {
	service := &operationCycleAPIStub{}
	handler := &Handler{operationCycles: service, operationAuth: &operationCycleAuthStub{err: errors.New("verifier unavailable")}}
	response := httptest.NewRecorder()
	handler.ReportOperationCycle(response, httptest.NewRequest(http.MethodPost, "/api/operation-cycles/reports", strings.NewReader(`{}`)))
	if response.Code != http.StatusServiceUnavailable || len(service.reports) != 0 {
		t.Fatalf("status/reports=%d/%d", response.Code, len(service.reports))
	}
}

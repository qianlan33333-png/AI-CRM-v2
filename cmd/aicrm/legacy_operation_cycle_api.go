package main

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	operationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const operationCycleBodyLimit = 256 << 10

func (handler *Handler) OperationCyclesPage(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	result, err := service.ListStrategies(request.Context(), operationapp.DefaultLimit, 0)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeOperationCyclePage(writer, "运营周期", result)
}

func (handler *Handler) OperationCycleStrategyPage(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	result, err := service.GetStrategy(request.Context(), chi.URLParam(request, "strategy_key"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeOperationCyclePage(writer, "运营周期策略", result)
}

func (handler *Handler) OperationCycleRunPage(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	result, err := service.GetRun(request.Context(), chi.URLParam(request, "run_key"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeOperationCyclePage(writer, "运营周期运行", result)
}

func (handler *Handler) GetOperationCycleActionResult(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil || rejectOperationCycleQuery(writer, request, nil) {
		return
	}
	result, err := service.GetActionResult(request.Context(), chi.URLParam(request, "request_id"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) GetOperationCycleRun(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil || rejectOperationCycleQuery(writer, request, nil) {
		return
	}
	result, err := service.GetRun(request.Context(), chi.URLParam(request, "run_key"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ListOperationCycleStrategies(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	limit, offset, err := operationCyclePageParams(writer, request, nil)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.ListStrategies(request.Context(), limit, offset)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) GetOperationCycleStrategy(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil || rejectOperationCycleQuery(writer, request, nil) {
		return
	}
	result, err := service.GetStrategy(request.Context(), chi.URLParam(request, "strategy_key"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

type operationCycleActionStartRequest struct {
	RunKey          string `json:"run_key"`
	ParentRequestID string `json:"parent_request_id"`
}

func (handler *Handler) StartOperationCycleAction(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	actor, err := operationCycleHumanActor(request)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	var body operationCycleActionStartRequest
	if err = decodeOperationCycleJSON(writer, request, &body); err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.Start(request.Context(), operationapp.StartCommand{
		StrategyKey: chi.URLParam(request, "strategy_key"), ActionKey: chi.URLParam(request, "action_key"),
		RunKey: body.RunKey, ParentRequest: body.ParentRequestID, IdempotencyKey: key, ActorID: actor,
	})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, result)
}

func (handler *Handler) GetOperationCycleCurrentAction(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil || rejectOperationCycleQuery(writer, request, nil) {
		return
	}
	result, err := service.CurrentAction(request.Context(), chi.URLParam(request, "strategy_key"))
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ListOperationCycleRuns(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	limit, offset, err := operationCyclePageParams(writer, request, nil)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.ListRuns(request.Context(), chi.URLParam(request, "strategy_key"), limit, offset)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ListOperationCycleProposals(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	limit, offset, err := operationCyclePageParams(writer, request, nil)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.ListProposals(request.Context(), chi.URLParam(request, "strategy_key"), limit, offset)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

type operationCycleDecisionRequest struct {
	Decision string `json:"decision"`
}

func (handler *Handler) DecideOperationCycleProposal(writer http.ResponseWriter, request *http.Request) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return
	}
	actor, err := operationCycleHumanActor(request)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	var body operationCycleDecisionRequest
	if err = decodeOperationCycleJSON(writer, request, &body); err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.DecideProposal(request.Context(), chi.URLParam(request, "proposal_id"), body.Decision, actor)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

// ClaimOperationCycleAction is service-authenticated. It has no browser
// fallback, no polling interval, and no queue retry control.
type operationCycleClaimRequest struct {
	SchemaVersion string `json:"schema_version"`
	RunnerID      string `json:"runner_id"`
	WaitSeconds   int32  `json:"wait_seconds"`
}

func (handler *Handler) ClaimOperationCycleAction(writer http.ResponseWriter, request *http.Request) {
	service, principal := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_action_claim")
	if service == nil {
		return
	}
	var body operationCycleClaimRequest
	if err := decodeOperationCycleJSON(writer, request, &body); err != nil || body.SchemaVersion != "operation_cycle_action_claim.v1" || body.WaitSeconds != 0 {
		writeOperationCycleError(writer, operationapp.ErrInvalid)
		return
	}
	result, err := service.Claim(request.Context(), body.RunnerID, principal.PrincipalID)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, result)
}

type operationCycleActionEventRequest struct {
	SchemaVersion string         `json:"schema_version"`
	EventType     string         `json:"event_type"`
	LeaseToken    string         `json:"lease_token"`
	ThreadID      string         `json:"thread_id"`
	TurnID        string         `json:"turn_id"`
	Result        map[string]any `json:"result"`
	FailureCode   string         `json:"failure_code"`
}

func (handler *Handler) RecordOperationCycleActionEvent(writer http.ResponseWriter, request *http.Request) {
	service, _ := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_action_event_write")
	if service == nil {
		return
	}
	eventID := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if eventID == "" {
		writeOperationCycleError(writer, operationapp.ErrInvalid)
		return
	}
	var body operationCycleActionEventRequest
	if err := decodeOperationCycleJSON(writer, request, &body); err != nil || body.SchemaVersion != "operation_cycle_action_event.v1" {
		writeOperationCycleError(writer, operationapp.ErrInvalid)
		return
	}
	result, err := service.RecordActionEvent(request.Context(), operationapp.ActionEventCommand{
		RequestID: chi.URLParam(request, "request_id"), EventID: eventID,
		EventType: body.EventType, LeaseToken: body.LeaseToken, ThreadID: body.ThreadID, TurnID: body.TurnID,
		Result: body.Result, FailureCode: body.FailureCode,
	})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) OperationCycleContextIndex(writer http.ResponseWriter, request *http.Request) {
	service, _ := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_context_read")
	if service == nil {
		return
	}
	limit, offset, err := operationCyclePageParams(writer, request, nil)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.ContextIndex(request.Context(), limit, offset)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ReportOperationCycle(writer http.ResponseWriter, request *http.Request) {
	service, principal := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_report_write")
	if service == nil {
		return
	}
	snapshot, err := decodeOperationCycleMap(writer, request)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.Report(request.Context(), operationapp.ReportCommand{
		Snapshot: snapshot, IdempotencyKey: strings.TrimSpace(request.Header.Get("Idempotency-Key")), ReporterID: principal.PrincipalID, ClientID: principal.ClientID,
	})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, result)
}

type operationCycleHeartbeatRequest struct {
	SchemaVersion       string   `json:"schema_version"`
	RunnerID            string   `json:"runner_id"`
	ConnectorVersion    string   `json:"connector_version"`
	CodexVersion        string   `json:"codex_version"`
	AppServerProtocol   string   `json:"app_server_protocol"`
	CompatibilityStatus string   `json:"compatibility_status"`
	BindingKeys         []string `json:"binding_keys"`
}

func (handler *Handler) HeartbeatOperationCycleRunner(writer http.ResponseWriter, request *http.Request) {
	service, principal := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_runner_heartbeat")
	if service == nil {
		return
	}
	var body operationCycleHeartbeatRequest
	if err := decodeOperationCycleJSON(writer, request, &body); err != nil || body.SchemaVersion != "operation_cycle_runner_heartbeat.v1" {
		writeOperationCycleError(writer, operationapp.ErrInvalid)
		return
	}
	result, err := service.Heartbeat(request.Context(), operationapp.RunnerHeartbeatCommand{
		RunnerID: body.RunnerID, ConnectorVersion: body.ConnectorVersion, CodexVersion: body.CodexVersion,
		AppServerProtocol: body.AppServerProtocol, CompatibilityStatus: body.CompatibilityStatus, BindingKeys: body.BindingKeys, PrincipalID: principal.PrincipalID,
	})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) OperationCycleStrategyContext(writer http.ResponseWriter, request *http.Request) {
	service, _ := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_context_read")
	if service == nil {
		return
	}
	limit, offset, err := operationCyclePageParams(writer, request, map[string]bool{"mode": true})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	mode := request.URL.Query().Get("mode")
	if mode == "" {
		mode = "execution"
	}
	result, err := service.StrategyContext(request.Context(), chi.URLParam(request, "strategy_key"), mode, limit, offset, nil)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) CreateOperationCycleProposal(writer http.ResponseWriter, request *http.Request) {
	service, principal := handler.operationCyclesAndServicePrincipal(writer, request, "operation_cycle_strategy_propose")
	if service == nil {
		return
	}
	payload, err := decodeOperationCycleMap(writer, request)
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	result, err := service.CreateProposal(request.Context(), operationapp.ProposalCommand{Payload: payload, IdempotencyKey: strings.TrimSpace(request.Header.Get("Idempotency-Key")), ActorID: principal.PrincipalID})
	if err != nil {
		writeOperationCycleError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, result)
}

func (handler *Handler) operationCyclesOrFail(writer http.ResponseWriter) legacyOperationCycleApplication {
	if handler == nil || handler.operationCycles == nil {
		writeOperationCycleError(writer, operationapp.ErrUnavailable)
		return nil
	}
	return handler.operationCycles
}

func (handler *Handler) operationCyclesAndServicePrincipal(writer http.ResponseWriter, request *http.Request, purpose string) (legacyOperationCycleApplication, operationServicePrincipal) {
	service := handler.operationCyclesOrFail(writer)
	if service == nil {
		return nil, operationServicePrincipal{}
	}
	if handler.operationAuth == nil {
		writeOperationCycleError(writer, operationapp.ErrUnavailable)
		return nil, operationServicePrincipal{}
	}
	principal, err := handler.operationAuth.AuthenticateOperation(request.Context(), request, purpose)
	if err != nil || strings.TrimSpace(principal.ClientID) == "" || strings.TrimSpace(principal.PrincipalID) == "" {
		if err == nil {
			err = authport.ErrUnauthorized
		}
		writeOperationCycleError(writer, err)
		return nil, operationServicePrincipal{}
	}
	return service, principal
}

func operationCycleHumanActor(request *http.Request) (string, error) {
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		return "", authport.ErrUnauthorized
	}
	return strconv.FormatInt(principal.AdminUserID, 10), nil
}

func operationCyclePageParams(writer http.ResponseWriter, request *http.Request, extra map[string]bool) (int32, int32, error) {
	if rejectOperationCycleQuery(writer, request, extra) {
		return 0, 0, operationapp.ErrInvalid
	}
	limit, offset := int64(operationapp.DefaultLimit), int64(0)
	var err error
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 32)
	}
	if err == nil {
		if raw := strings.TrimSpace(request.URL.Query().Get("offset")); raw != "" {
			offset, err = strconv.ParseInt(raw, 10, 32)
		}
	}
	if err != nil || limit < 1 || limit > int64(operationapp.MaximumLimit) || offset < 0 || offset > int64(operationapp.MaximumOffset) {
		return 0, 0, operationapp.ErrInvalid
	}
	return int32(limit), int32(offset), nil
}

func rejectOperationCycleQuery(writer http.ResponseWriter, request *http.Request, extra map[string]bool) bool {
	allowed := map[string]bool{"limit": true, "offset": true}
	for key := range extra {
		allowed[key] = true
	}
	for key, values := range request.URL.Query() {
		if !allowed[key] || len(values) != 1 {
			writeOperationCycleError(writer, operationapp.ErrInvalid)
			return true
		}
	}
	return false
}

func decodeOperationCycleJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, operationCycleBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return operationapp.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return operationapp.ErrInvalid
	}
	return nil
}

func decodeOperationCycleMap(writer http.ResponseWriter, request *http.Request) (map[string]any, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, operationCycleBodyLimit))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		return nil, operationapp.ErrInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, operationapp.ErrInvalid
	}
	return payload, nil
}

func writeOperationCyclePage(writer http.ResponseWriter, title string, value map[string]any) {
	payload, err := json.Marshal(value)
	if err != nil {
		writeOperationCycleError(writer, operationapp.ErrUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, "<!doctype html><title>"+html.EscapeString(title)+"</title><main><h1>"+html.EscapeString(title)+"</h1><pre>"+html.EscapeString(string(payload))+"</pre></main>")
}

func writeOperationCycleError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, operationapp.ErrInvalid):
		status, code = http.StatusBadRequest, platformhttp.CodeMalformedRequest
	case errors.Is(err, operationapp.ErrNotFound):
		status, code = http.StatusNotFound, platformhttp.CodeNotFound
	case errors.Is(err, operationapp.ErrConflict), errors.Is(err, operationapp.ErrLeaseInvalid), errors.Is(err, operationapp.ErrActionUnavailable):
		status, code = http.StatusConflict, platformhttp.CodeConflict
	case errors.Is(err, authport.ErrUnauthenticated):
		status, code = http.StatusUnauthorized, platformhttp.CodeUnauthenticated
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, platformhttp.CodeUnauthorized
	}
	platformhttp.MarkCompatibilityError(writer, code)
	writeJSON(writer, status, map[string]any{"ok": false, "detail": err.Error()})
}

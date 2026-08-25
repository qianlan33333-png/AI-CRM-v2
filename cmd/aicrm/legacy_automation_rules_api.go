package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

func (handler *Handler) ListAutomationRules(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.automationRules == nil {
		writeAutomationRuleError(writer, http.StatusServiceUnavailable, "automation_unavailable")
		return
	}
	items, err := handler.automationRules.ListRules(request.Context())
	if err != nil {
		writeAutomationRuleError(writer, http.StatusServiceUnavailable, "automation_unavailable")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (handler *Handler) CreateAutomationRule(writer http.ResponseWriter, request *http.Request) {
	actor, ok := automationRuleActor(request)
	if handler == nil || handler.automationRules == nil || !ok {
		writeAutomationRuleError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	var input struct {
		Code      string                             `json:"code"`
		Name      string                             `json:"name"`
		Status    automationport.RuleStatus          `json:"status"`
		Condition automationport.TagAppliedCondition `json:"condition"`
		Action    automationport.Action              `json:"action"`
	}
	if !decodeAutomationRuleJSON(request, &input) {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_rule")
		return
	}
	rule, err := handler.automationRules.CreateRule(request.Context(), automationport.CreateRuleCommand{Code: input.Code, Name: input.Name, Status: input.Status, Condition: input.Condition, Action: input.Action, Actor: actor, IdempotencyKey: request.Header.Get("Idempotency-Key")})
	if err != nil {
		writeAutomationRuleError(writer, automationRuleStatus(err), "automation_rule_command_failed")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, rule)
}

func (handler *Handler) GetAutomationRule(writer http.ResponseWriter, request *http.Request) {
	id, ok := automationRuleID(request)
	if handler == nil || handler.automationRules == nil || !ok {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_rule")
		return
	}
	rule, err := handler.automationRules.GetRule(request.Context(), id)
	if err != nil {
		writeAutomationRuleError(writer, http.StatusNotFound, "automation_rule_not_found")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, rule)
}

func (handler *Handler) UpdateAutomationRule(writer http.ResponseWriter, request *http.Request) {
	id, validID := automationRuleID(request)
	actor, validActor := automationRuleActor(request)
	if handler == nil || handler.automationRules == nil || !validID || !validActor {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_rule")
		return
	}
	var input struct {
		Name      string                             `json:"name"`
		Status    automationport.RuleStatus          `json:"status"`
		Condition automationport.TagAppliedCondition `json:"condition"`
		Action    automationport.Action              `json:"action"`
	}
	if !decodeAutomationRuleJSON(request, &input) {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_rule")
		return
	}
	rule, err := handler.automationRules.UpdateRule(request.Context(), automationport.UpdateRuleCommand{ID: id, Name: input.Name, Status: input.Status, Condition: input.Condition, Action: input.Action, Actor: actor, IdempotencyKey: request.Header.Get("Idempotency-Key")})
	if err != nil {
		writeAutomationRuleError(writer, automationRuleStatus(err), "automation_rule_command_failed")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, rule)
}

func (handler *Handler) SetAutomationRuleStatus(writer http.ResponseWriter, request *http.Request) {
	id, validID := automationRuleID(request)
	actor, validActor := automationRuleActor(request)
	status := automationport.RuleStatus(chi.URLParam(request, "status"))
	if handler == nil || handler.automationRules == nil || !validID || !validActor {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_rule")
		return
	}
	rule, err := handler.automationRules.SetRuleStatus(request.Context(), id, status, actor, request.Header.Get("Idempotency-Key"))
	if err != nil {
		writeAutomationRuleError(writer, automationRuleStatus(err), "automation_rule_command_failed")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, rule)
}

func (handler *Handler) ListAutomationRuleRuns(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.automationRuleRuns == nil {
		writeAutomationRuleError(writer, http.StatusServiceUnavailable, "automation_unavailable")
		return
	}
	items, err := handler.automationRuleRuns.ListRuleExecutions(request.Context(), 0, 100)
	if err != nil {
		writeAutomationRuleError(writer, http.StatusServiceUnavailable, "automation_unavailable")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (handler *Handler) ReconcileAutomationRuleRun(writer http.ResponseWriter, request *http.Request) {
	actor, actorOK := automationRuleActor(request)
	actionID, actionOK := automationActionID(request)
	if handler == nil || handler.automationRuleReconcile == nil || !actorOK || !actionOK {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_reconciliation")
		return
	}
	var input struct {
		Generation     int64  `json:"generation"`
		Fence          int64  `json:"fence"`
		LeaseExpiresAt string `json:"lease_expires_at"`
		EvidenceDigest string `json:"evidence_digest"`
	}
	if !decodeAutomationRuleJSON(request, &input) {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_reconciliation")
		return
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, input.LeaseExpiresAt)
	if err != nil {
		writeAutomationRuleError(writer, http.StatusBadRequest, "invalid_automation_reconciliation")
		return
	}
	result, err := handler.automationRuleReconcile.ReconcileOutboundMessage(request.Context(), automationport.ReconcileOutboundMessageCommand{
		ActionID: actionID, Actor: actor, IdempotencyKey: request.Header.Get("Idempotency-Key"), Generation: input.Generation,
		Fence: input.Fence, LeaseExpiresAt: expiresAt, EvidenceDigest: eer.Digest(input.EvidenceDigest),
	})
	if err != nil {
		writeAutomationRuleError(writer, automationRuleStatus(err), "automation_reconciliation_failed")
		return
	}
	writeAutomationRuleJSON(writer, http.StatusOK, result)
}

func automationRuleActor(request *http.Request) (int64, bool) {
	principal, ok := authport.PrincipalFromContext(request.Context())
	return principal.AdminUserID, ok && principal.AdminUserID > 0
}
func automationRuleID(request *http.Request) (automationport.RuleID, bool) {
	value, err := strconv.ParseInt(chi.URLParam(request, "rule_id"), 10, 64)
	return automationport.RuleID(value), err == nil && value > 0
}
func automationActionID(request *http.Request) (int64, bool) {
	value, err := strconv.ParseInt(chi.URLParam(request, "action_id"), 10, 64)
	return value, err == nil && value > 0
}
func decodeAutomationRuleJSON(request *http.Request, target any) bool {
	if request == nil || request.Body == nil || request.Header.Get("Content-Type") != "application/json" || strings.TrimSpace(request.Header.Get("Idempotency-Key")) == "" {
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(&struct{}{}) == io.EOF
}
func automationRuleStatus(error) int { return http.StatusConflict }
func writeAutomationRuleJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func writeAutomationRuleError(writer http.ResponseWriter, status int, code string) {
	writeAutomationRuleJSON(writer, status, map[string]string{"error": code})
}

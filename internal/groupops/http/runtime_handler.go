package groupopshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

const (
	PlanEnablePath         = PlanPath + "/enable"
	PlanDisablePath        = PlanPath + "/disable"
	PlanGroupsPath         = PlanPath + "/groups"
	PlanGroupPath          = PlanGroupsPath + "/{chat_id}"
	PlanWebhookPath        = PlanPath + "/webhook"
	RunDuePath             = PlanPath + "/run-due"
	RunDuePreviewPath      = RunDuePath + "/preview"
	ExecutionsPath         = PlanPath + "/executions"
	ExecutionReconcilePath = PlansPath + "/executions/{execution_id}/reconcile"
	GroupsPath             = "/api/admin/automation-conversion/group-ops/groups"
	GroupsSyncPath         = GroupsPath + "/sync"
	GroupPickerPath        = "/api/admin/automation-conversion/group-ops/group-picker"
	GroupPickerSyncPath    = GroupPickerPath + "/sync"
	OperationMembersPath   = "/api/admin/common/operation-members"
	OperationMembersSync   = OperationMembersPath + "/sync"
	BroadcastPath          = "/api/automation/group-ops/broadcast"
	WebhookPath            = "/api/automation/group-ops/webhooks/{webhook_key}"
)

var ErrProtocolAuthentication = errors.New("group ops protocol authentication failed")

type RuntimeApplication interface {
	PreviewRunDue(context.Context, int64) (groupopsport.RunDuePreview, error)
	RunDue(context.Context, groupopsport.RunDueCommand) (groupopsport.RunSummary, error)
	AcceptPlan(context.Context, groupopsport.AcceptPlanCommand) (groupopsport.RunSummary, error)
	AcceptWebhook(context.Context, string, string) (groupopsport.RunSummary, error)
	ListExecutions(context.Context, int64, int32, int32) (groupopsport.ExecutionPage, error)
	ManualReconcile(context.Context, groupopsport.ManualReconcileCommand) (groupopsport.Execution, error)
	ListOperationMembers(context.Context, int32) (groupopsport.OperationMemberPage, error)
	RefreshOperationMembers(context.Context, groupopsport.OperationMemberRefreshCommand) (groupopsport.OperationMemberPage, error)
	ListGroups(context.Context, int64, int32, int32) (groupopsport.GroupDirectoryPage, error)
	RefreshGroups(context.Context, groupopsport.GroupRefreshCommand) (groupopsport.GroupDirectoryPage, error)
}

var _ RuntimeApplication = (*groupopsapp.RuntimeService)(nil)

type ProtocolPrincipal struct{ ID string }

// ProtocolAuthenticator is intentionally an injected boundary. The Group Ops
// package neither invents API-client JWT nor webhook-HMAC credential policy,
// and a nil implementation fails closed in production.
type ProtocolAuthenticator interface {
	Authenticate(context.Context, *http.Request, string, string, []byte) (ProtocolPrincipal, error)
}

func NewWithRuntime(application Application, runtime RuntimeApplication, protocols ProtocolAuthenticator) *Handler {
	return &Handler{Application: application, Runtime: runtime, Protocols: protocols}
}

func (h *Handler) PreviewRunDue(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodPost) || !authorized(r, "operations.manage") {
		authorization(w, r)
		return
	}
	id, ok := planID(r, RunDuePreviewPath)
	if !ok || !emptyBody(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.PreviewRunDue(r.Context(), id)
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) RunDue(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	id, idOK := planID(r, RunDuePath)
	idempotencyKey, keyOK := key(r)
	if !idOK || !keyOK || !emptyBody(r) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.RunDue(r.Context(), groupopsport.RunDueCommand{PlanID: id, ActorID: actor, IdempotencyKey: idempotencyKey})
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, "admin.read") {
		authorization(w, r)
		return
	}
	id, ok := planID(r, ExecutionsPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_plan_id")
		return
	}
	limit, offset, ok := pageQuery(r)
	if !ok || !runtimeAvailable(h) {
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_page")
		} else {
			unavailable(w)
		}
		return
	}
	result, err := h.Runtime.ListExecutions(r.Context(), id, limit, offset)
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ReconcileExecution(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	executionID, ok := templateID(r, ExecutionReconcilePath, "{execution_id}")
	idempotencyKey, keyOK := key(r)
	var body struct {
		Generation     int64     `json:"generation"`
		Fence          int64     `json:"fence"`
		LeaseExpiresAt time.Time `json:"lease_expires_at"`
		EvidenceDigest string    `json:"evidence_digest"`
		DeliveryProven bool      `json:"delivery_proven"`
	}
	if !ok || !keyOK || !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.ManualReconcile(r.Context(), groupopsport.ManualReconcileCommand{ExecutionID: executionID, ActorID: actor, IdempotencyKey: idempotencyKey, Generation: body.Generation, Fence: body.Fence, LeaseExpiresAt: body.LeaseExpiresAt, EvidenceDigest: body.EvidenceDigest, DeliveryProven: body.DeliveryProven})
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	h.listDirectoryGroups(w, r, true)
}

func (h *Handler) ListGroupPicker(w http.ResponseWriter, r *http.Request) {
	h.listDirectoryGroups(w, r, false)
}

func (h *Handler) listDirectoryGroups(w http.ResponseWriter, r *http.Request, ownerRequired bool) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, "admin.read") {
		authorization(w, r)
		return
	}
	owner, limit, offset, ok := directoryQuery(r.URL.Query(), ownerRequired)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.ListGroups(r.Context(), owner, limit, offset)
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) SyncGroups(w http.ResponseWriter, r *http.Request)      { h.syncGroups(w, r) }
func (h *Handler) SyncGroupPicker(w http.ResponseWriter, r *http.Request) { h.syncGroups(w, r) }

func (h *Handler) ListOperationMembers(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodGet) || !authorized(r, "admin.read") {
		authorization(w, r)
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	if r.URL.RawQuery != "" {
		writeError(w, http.StatusBadRequest, "invalid_page")
		return
	}
	result, err := h.Runtime.ListOperationMembers(r.Context(), 100)
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) SyncOperationMembers(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	idempotencyKey, keyOK := key(r)
	var body struct {
		PageSize int32 `json:"page_size"`
	}
	if !keyOK || !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.RefreshOperationMembers(r.Context(), groupopsport.OperationMemberRefreshCommand{ActorID: actor, PageSize: body.PageSize, IdempotencyKey: idempotencyKey})
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) syncGroups(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	actor, ok := writeAuthorized(r)
	if !method(w, r, http.MethodPost) || !ok {
		authorization(w, r)
		return
	}
	idempotencyKey, keyOK := key(r)
	var body struct {
		OwnerStaffID int64 `json:"owner_staff_id"`
		Limit        int32 `json:"limit"`
	}
	if !keyOK || !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !runtimeAvailable(h) {
		unavailable(w)
		return
	}
	result, err := h.Runtime.RefreshGroups(r.Context(), groupopsport.GroupRefreshCommand{OwnerStaffID: body.OwnerStaffID, ActorID: actor, Limit: body.Limit, IdempotencyKey: idempotencyKey})
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) Broadcast(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodPost) {
		return
	}
	idempotencyKey, keyOK := key(r)
	var body struct {
		PlanID int64 `json:"plan_id"`
	}
	if !keyOK || !decodeBody(r, &body) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	principal, ok := h.protocolPrincipal(w, r, "group_ops_broadcast", "service", nil)
	if !ok {
		return
	}
	result, err := h.Runtime.AcceptPlan(r.Context(), groupopsport.AcceptPlanCommand{PlanID: body.PlanID, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:" + principal.ID, IdempotencyKey: idempotencyKey})
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if !method(w, r, http.MethodPost) {
		return
	}
	reference, ok := templateOpaque(r, WebhookPath, "{webhook_key}")
	idempotencyKey, keyOK := key(r)
	body, bodyOK := jsonObjectBody(r)
	if !ok || !keyOK || !bodyOK {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, ok = h.protocolPrincipal(w, r, "group_ops_webhook", reference, body); !ok {
		return
	}
	result, err := h.Runtime.AcceptWebhook(r.Context(), reference, idempotencyKey)
	if err != nil {
		runtimeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (h *Handler) protocolPrincipal(w http.ResponseWriter, r *http.Request, purpose, resource string, body []byte) (ProtocolPrincipal, bool) {
	if !runtimeAvailable(h) || h.Protocols == nil {
		writeError(w, http.StatusServiceUnavailable, "protocol_auth_unavailable")
		return ProtocolPrincipal{}, false
	}
	principal, err := h.Protocols.Authenticate(r.Context(), r, purpose, resource, body)
	if err != nil || !opaqueHTTP(principal.ID) {
		writeError(w, http.StatusUnauthorized, "protocol_authentication_failed")
		return ProtocolPrincipal{}, false
	}
	return principal, true
}

func runtimeAvailable(h *Handler) bool { return h != nil && h.Runtime != nil }

func runtimeError(w http.ResponseWriter, err error) {
	if errors.Is(err, groupopsapp.ErrProviderDisabled) {
		writeError(w, http.StatusServiceUnavailable, "provider_disabled")
		return
	}
	applicationError(w, err)
}

func directoryQuery(values url.Values, ownerRequired bool) (int64, int32, int32, bool) {
	for name, entries := range values {
		if name != "owner_userid" && name != "limit" && name != "offset" || len(entries) != 1 || entries[0] == "" {
			return 0, 0, 0, false
		}
	}
	owner := int64(0)
	if raw := values.Get("owner_userid"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			return 0, 0, 0, false
		}
		owner = parsed
	} else if ownerRequired {
		return 0, 0, 0, false
	}
	limit, offset := int32(200), int32(0)
	for name, target := range map[string]*int32{"limit": &limit, "offset": &offset} {
		if raw := values.Get(name); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 32)
			if err != nil {
				return 0, 0, 0, false
			}
			*target = int32(parsed)
		}
	}
	return owner, limit, offset, limit >= 1 && limit <= 200 && offset >= 0 && offset <= groupopsapp.MaximumOffset
}

func templateID(r *http.Request, pattern, placeholder string) (int64, bool) {
	value, ok := templateValue(r, pattern, placeholder)
	if !ok || !canonicalID.MatchString(value) {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}

func templateOpaque(r *http.Request, pattern, placeholder string) (string, bool) {
	value, ok := templateValue(r, pattern, placeholder)
	return value, ok && opaqueHTTP(value)
}

func templateValue(r *http.Request, pattern, placeholder string) (string, bool) {
	if r == nil || r.URL == nil {
		return "", false
	}
	parts := strings.Split(pattern, placeholder)
	if len(parts) != 2 || !strings.HasPrefix(r.URL.Path, parts[0]) || !strings.HasSuffix(r.URL.Path, parts[1]) {
		return "", false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, parts[0]), parts[1])
	return value, value != "" && !strings.ContainsAny(value, "/\\\x00\r\n")
}

func opaqueHTTP(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("._:-", character) {
			continue
		}
		return false
	}
	return true
}

func jsonObjectBody(r *http.Request) ([]byte, bool) {
	if r == nil || r.Body == nil || r.Header.Get("Content-Type") != "application/json" {
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maximumBodyBytes))
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return nil, false
	}
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if decoder.Decode(&object) != nil || object == nil {
		return nil, false
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return nil, false
	}
	return body, true
}

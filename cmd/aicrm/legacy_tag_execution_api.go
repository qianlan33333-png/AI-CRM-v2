package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	wecomtag "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/tag"
)

// LegacyWecomTagsPage deliberately delegates rendering to the existing admin
// shell. The immutable mapping proves TemplateResponse/shell_context but not a
// tag-page body; redirecting to the generic shell preserves the route and
// session boundary without inventing a second tag-management UI.
func (*Handler) LegacyWecomTagsPage(writer http.ResponseWriter, request *http.Request) {
	if !legacyWecomTagsGlobalReadAuthorized(request) {
		writeLegacyTagError(writer, authport.ErrUnauthorized)
		return
	}
	http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape("/admin/wecom-tags"), http.StatusFound)
}

func (handler *Handler) ListLegacyTagGroups(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.legacyTags) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagUnavailable)
		return
	}
	catalog, err := handler.legacyTags.List(request.Context())
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "groups": catalog.Groups, "items": catalog.Groups, "count": len(catalog.Groups), "source_status": "local_catalog", "real_external_call_executed": false, "sync_executed": false})
}

func (handler *Handler) GetLegacyTagGroup(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.legacyTags) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagUnavailable)
		return
	}
	groupID, err := parseLegacyTagID(chi.URLParam(request, "group_id"))
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	group, err := handler.legacyTags.GetGroup(request.Context(), groupID)
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "group": group, "source_status": "local_catalog", "real_external_call_executed": false, "sync_executed": false})
}

func (handler *Handler) GetLegacyTag(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.legacyTags) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagUnavailable)
		return
	}
	tagID, err := parseLegacyTagID(chi.URLParam(request, "tag_id"))
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	tag, err := handler.legacyTags.GetTag(request.Context(), tagID)
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "tag": tag, "source_status": "local_catalog", "real_external_call_executed": false, "sync_executed": false})
}

func (handler *Handler) GetLegacyTagExecutionStatus(writer http.ResponseWriter, request *http.Request) {
	if !legacyWecomTagsGlobalReadAuthorized(request) {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	if handler == nil || nilLegacyDependency(handler.legacyTagStatus) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagExecutionUnavailable)
		return
	}
	status, err := handler.legacyTagStatus.Get(request.Context())
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, legacyTagExecutionGateResponse{
		ProviderExecutionEligible:       status.ProviderExecutionEligible,
		LocalCommandAcceptanceAvailable: status.LocalCommandAcceptanceAvailable,
		LocalQueueAvailable:             status.LocalQueueAvailable,
		SyncExecuted:                    status.SyncExecuted,
		ObservedAt:                      status.ObservedAt.UTC().Format(time.RFC3339Nano),
		RealExternalCallExecuted:        status.RealExternalCallExecuted,
	})
}

type legacyTagExecutionGateResponse struct {
	ProviderExecutionEligible       bool   `json:"provider_execution_eligible"`
	LocalCommandAcceptanceAvailable bool   `json:"local_command_acceptance_available"`
	LocalQueueAvailable             bool   `json:"local_queue_available"`
	SyncExecuted                    bool   `json:"sync_executed"`
	ObservedAt                      string `json:"observed_at"`
	RealExternalCallExecuted        bool   `json:"real_external_call_executed"`
}

func (handler *Handler) SyncLegacyTags(writer http.ResponseWriter, request *http.Request) {
	handler.requestLegacyTagSync(writer, request, contactapp.LegacyTagSyncManual)
}

func (handler *Handler) SyncLegacyTagsDue(writer http.ResponseWriter, request *http.Request) {
	handler.requestLegacyTagSync(writer, request, contactapp.LegacyTagSyncDue)
}

func (handler *Handler) requestLegacyTagSync(writer http.ResponseWriter, request *http.Request, kind contactapp.LegacyTagSyncKind) {
	if handler == nil || nilLegacyDependency(handler.legacyTagSync) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagSyncFailed)
		return
	}
	metadata, payload, ok := legacyTagExecutionBody(writer, request, false)
	if !ok {
		return
	}
	if !legacyTagSyncPayload(payload) {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagSync)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	acceptance, effect, err := handler.legacyTagSync.Request(request.Context(), contactapp.LegacyTagSyncCommand{
		Actor: principal.AdminUserID, IdempotencyKey: metadata.IdempotencyKey, TraceID: metadata.TraceID, Kind: kind,
	})
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeLegacyTagAcceptance(writer, acceptance.ReceiptID, int64(acceptance.EventID), acceptance.RiverJobID, string(acceptance.State), effect)
}

func (handler *Handler) MarkLegacyTagLive(writer http.ResponseWriter, request *http.Request) {
	handler.requestLegacyTagLiveMutation(writer, request, contactapp.LegacyTagLiveMutationMark)
}

func (handler *Handler) UnmarkLegacyTagLive(writer http.ResponseWriter, request *http.Request) {
	handler.requestLegacyTagLiveMutation(writer, request, contactapp.LegacyTagLiveMutationUnmark)
}

func (handler *Handler) requestLegacyTagLiveMutation(writer http.ResponseWriter, request *http.Request, operation contactapp.LegacyTagLiveMutationOperation) {
	if handler == nil || nilLegacyDependency(handler.legacyTagLive) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagLiveMutationFailed)
		return
	}
	metadata, payload, ok := legacyTagExecutionBody(writer, request, true)
	if !ok {
		return
	}
	externalUserID, providerTagIDs, ok := legacyTagLivePayload(payload)
	if !ok {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	acceptance, effect, err := handler.legacyTagLive.Request(request.Context(), contactapp.LegacyTagLiveMutationCommand{
		Actor: principal.AdminUserID, IdempotencyKey: metadata.IdempotencyKey, TraceID: metadata.TraceID, Operation: operation, Payload: payload,
	}, externalUserID, providerTagIDs)
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeLegacyTagAcceptance(writer, acceptance.ReceiptID, int64(acceptance.EventID), acceptance.RiverJobID, string(acceptance.State), effect)
}

func legacyTagSyncPayload(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return false
	}
	for key := range object {
		if key != "idempotency_key" && key != "trace_id" {
			return false
		}
	}
	return true
}

func legacyTagLivePayload(raw json.RawMessage) (string, []string, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return "", nil, false
	}
	for key := range object {
		switch key {
		case "idempotency_key", "trace_id", "external_userid", "tag_id", "tag_ids":
		default:
			return "", nil, false
		}
	}
	var externalUserID string
	if value, exists := object["external_userid"]; !exists || json.Unmarshal(value, &externalUserID) != nil || strings.TrimSpace(externalUserID) != externalUserID || externalUserID == "" || len(externalUserID) > 1024 {
		return "", nil, false
	}
	_, hasTagID := object["tag_id"]
	_, hasTagIDs := object["tag_ids"]
	if hasTagID == hasTagIDs {
		return "", nil, false
	}
	if hasTagIDs {
		var values []string
		if json.Unmarshal(object["tag_ids"], &values) != nil || len(values) == 0 || len(values) > 100 {
			return "", nil, false
		}
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
				return "", nil, false
			}
			if _, duplicate := seen[value]; duplicate {
				return "", nil, false
			}
			seen[value] = struct{}{}
		}
		return externalUserID, values, true
	}
	var tagID string
	if json.Unmarshal(object["tag_id"], &tagID) != nil {
		var number json.Number
		if json.Unmarshal(object["tag_id"], &number) != nil {
			return "", nil, false
		}
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil || parsed < 1 {
			return "", nil, false
		}
		tagID = strconv.FormatInt(parsed, 10)
	}
	if tagID == "" || len(tagID) > 128 || strings.TrimSpace(tagID) != tagID {
		return "", nil, false
	}
	return externalUserID, []string{tagID}, true
}

type legacyTagExecutionMetadata struct {
	IdempotencyKey string `json:"idempotency_key"`
	TraceID        string `json:"trace_id"`
}

func legacyTagExecutionBody(writer http.ResponseWriter, request *http.Request, requireBody bool) (legacyTagExecutionMetadata, json.RawMessage, bool) {
	if request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.UseNumber()
	var raw json.RawMessage
	err := decoder.Decode(&raw)
	if errors.Is(err, io.EOF) && !requireBody {
		raw = json.RawMessage("{}")
	} else if err != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	metadata := legacyTagExecutionMetadata{}
	if value, exists := object["idempotency_key"]; exists && (strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, &metadata.IdempotencyKey) != nil) {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	if value, exists := object["trace_id"]; exists && (strings.TrimSpace(string(value)) == "null" || json.Unmarshal(value, &metadata.TraceID) != nil) {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	keys := request.Header.Values("Idempotency-Key")
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	metadata.IdempotencyKey = keys[0]
	metadata.TraceID = strings.TrimSpace(metadata.TraceID)
	return metadata, append(json.RawMessage(nil), raw...), true
}

func writeLegacyTagAcceptance(writer http.ResponseWriter, receiptID, eventID, riverJobID int64, state string, effect wecomtag.Acceptance) {
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"ok": true, "accepted": true, "receipt_id": receiptID, "event_id": eventID, "river_job_id": riverJobID, "state": state,
		"effect_id": effect.EffectID, "effect_state": effect.State, "effect_river_job_id": effect.RiverJobID,
		"accept_receipt_id": effect.AcceptReceiptID, "queue_receipt_id": effect.QueueReceiptID,
		"real_external_call_executed": false, "sync_executed": false,
	})
}

type wecomTagEffectReconcileRequest struct {
	Generation     int64                        `json:"generation"`
	Fence          int64                        `json:"fence"`
	LeaseExpiresAt string                       `json:"lease_expires_at"`
	EvidenceDigest eer.Digest                   `json:"evidence_digest"`
	Resolution     wecomtag.ReconcileResolution `json:"resolution"`
}

func (handler *Handler) ReconcileWeComTagEffect(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || nilLegacyDependency(handler.wecomTagEffects) || request == nil {
		writeLegacyTagExecutionError(writer, wecomtag.ErrEffectUnavailable)
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	keys := request.Header.Values("Idempotency-Key")
	if !ok || principal.AdminUserID < 1 {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	if len(keys) != 1 || len(keys[0]) < 16 || len(keys[0]) > 128 || strings.TrimSpace(keys[0]) != keys[0] {
		writeLegacyTagExecutionError(writer, wecomtag.ErrInvalidCommand)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	var body wecomTagEffectReconcileRequest
	if decoder.Decode(&body) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		writeLegacyTagExecutionError(writer, wecomtag.ErrInvalidCommand)
		return
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, body.LeaseExpiresAt)
	if err != nil {
		writeLegacyTagExecutionError(writer, wecomtag.ErrInvalidCommand)
		return
	}
	result, err := handler.wecomTagEffects.Reconcile(request.Context(), wecomtag.ReconcileCommand{
		EffectID: chi.URLParam(request, "effect_id"), Actor: principal.AdminUserID, IdempotencyKey: keys[0],
		Generation: body.Generation, Fence: body.Fence, LeaseExpiresAt: expiresAt,
		EvidenceDigest: body.EvidenceDigest, Resolution: body.Resolution,
	})
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func writeLegacyTagExecutionError(writer http.ResponseWriter, err error) {
	status, code := http.StatusServiceUnavailable, "production_unavailable"
	switch {
	case errors.Is(err, authport.ErrUnauthorized):
		status, code = http.StatusForbidden, "unauthorized"
	case errors.Is(err, contactapp.ErrLegacyTagNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, contactapp.ErrInvalidLegacyTag), errors.Is(err, contactapp.ErrInvalidLegacyTagSync), errors.Is(err, contactapp.ErrInvalidLegacyTagLiveMutation), errors.Is(err, contactapp.ErrInvalidLegacyTagExecutionStatus):
		status, code = http.StatusBadRequest, "input_error"
	case errors.Is(err, contactapp.ErrLegacyTagSyncConflict), errors.Is(err, contactapp.ErrLegacyTagLiveMutationConflict):
		status, code = http.StatusConflict, "idempotency_conflict"
	case errors.Is(err, wecomtag.ErrInvalidCommand):
		status, code = http.StatusBadRequest, "input_error"
	case errors.Is(err, wecomtag.ErrEffectConflict), errors.Is(err, wecomtag.ErrReconcileRequired):
		status, code = http.StatusConflict, "effect_conflict"
	}
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, status, map[string]any{"ok": false, "error_code": code, "detail": err.Error(), "real_external_call_executed": false, "sync_executed": false})
}

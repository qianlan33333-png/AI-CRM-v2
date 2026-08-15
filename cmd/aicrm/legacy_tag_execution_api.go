package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

// LegacyWecomTagsPage deliberately delegates rendering to the existing admin
// shell. The immutable mapping proves TemplateResponse/shell_context but not a
// tag-page body; redirecting to the generic shell preserves the route and
// session boundary without inventing a second tag-management UI.
func (*Handler) LegacyWecomTagsPage(writer http.ResponseWriter, request *http.Request) {
	if request == nil {
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
	if handler == nil || nilLegacyDependency(handler.legacyTagStatus) || request == nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagExecutionUnavailable)
		return
	}
	status, err := handler.legacyTagStatus.Get(request.Context())
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(status.Payload)
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
	metadata, _, ok := legacyTagExecutionBody(writer, request, false)
	if !ok {
		return
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	acceptance, err := handler.legacyTagSync.Request(request.Context(), contactapp.LegacyTagSyncCommand{
		Actor: principal.AdminUserID, IdempotencyKey: metadata.IdempotencyKey, TraceID: metadata.TraceID, Kind: kind,
	})
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeLegacyTagAcceptance(writer, acceptance.ReceiptID, int64(acceptance.EventID), acceptance.RiverJobID, string(acceptance.State))
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
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok || principal.AdminUserID < 1 {
		writeLegacyTagExecutionError(writer, authport.ErrUnauthorized)
		return
	}
	acceptance, err := handler.legacyTagLive.Request(request.Context(), contactapp.LegacyTagLiveMutationCommand{
		Actor: principal.AdminUserID, IdempotencyKey: metadata.IdempotencyKey, TraceID: metadata.TraceID, Operation: operation, Payload: payload,
	})
	if err != nil {
		writeLegacyTagExecutionError(writer, err)
		return
	}
	writeLegacyTagAcceptance(writer, acceptance.ReceiptID, int64(acceptance.EventID), acceptance.RiverJobID, string(acceptance.State))
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
	if value, exists := object["idempotency_key"]; exists && json.Unmarshal(value, &metadata.IdempotencyKey) != nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	if value, exists := object["trace_id"]; exists && json.Unmarshal(value, &metadata.TraceID) != nil {
		writeLegacyTagExecutionError(writer, contactapp.ErrInvalidLegacyTagLiveMutation)
		return legacyTagExecutionMetadata{}, nil, false
	}
	if headerKey := strings.TrimSpace(request.Header.Get("Idempotency-Key")); headerKey != "" {
		metadata.IdempotencyKey = headerKey
	}
	if metadata.IdempotencyKey == "" {
		key, err := legacyTagRandomIdempotencyKey()
		if err != nil {
			writeLegacyTagExecutionError(writer, contactapp.ErrLegacyTagExecutionUnavailable)
			return legacyTagExecutionMetadata{}, nil, false
		}
		metadata.IdempotencyKey = key
	}
	metadata.IdempotencyKey = strings.TrimSpace(metadata.IdempotencyKey)
	metadata.TraceID = strings.TrimSpace(metadata.TraceID)
	return metadata, append(json.RawMessage(nil), raw...), true
}

func legacyTagRandomIdempotencyKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "legacy-tag-execution:" + hex.EncodeToString(value[:]), nil
}

func writeLegacyTagAcceptance(writer http.ResponseWriter, receiptID, eventID, riverJobID int64, state string) {
	writeJSON(writer, http.StatusAccepted, map[string]any{
		"ok": true, "accepted": true, "receipt_id": receiptID, "event_id": eventID, "river_job_id": riverJobID, "state": state,
		"real_external_call_executed": false, "sync_executed": false,
	})
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
	}
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, status, map[string]any{"ok": false, "error_code": code, "detail": err.Error(), "real_external_call_executed": false, "sync_executed": false})
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type IngestApplication interface {
	Ingest(context.Context, identityport.IngestCommand) (identityport.IngestResult, error)
}

type IngestHandler struct{ application IngestApplication }

func NewIngestHandler(application IngestApplication) (*IngestHandler, error) {
	if application == nil {
		return nil, identityapp.ErrIdentityIngestFailed
	}
	return &IngestHandler{application: application}, nil
}

func (handler *IngestHandler) IngestIdentityEvent(writer http.ResponseWriter, request *http.Request, params generated.IngestIdentityEventParams) {
	if handler == nil || handler.application == nil || request == nil {
		writeIngestError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, identityapp.ErrIdentityIngestFailed))
		return
	}
	if !ingestAuthorized(request) {
		writeIngestError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if request.Method != http.MethodPost || !validIngestKey(string(params.IdempotencyKey)) {
		writeIngestError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrInvalidIdentity))
		return
	}
	var body generated.IngestIdentityEventRequest
	if err := decodeIngestBody(writer, request, &body); err != nil {
		writeIngestError(writer, request, err)
		return
	}
	refs := make([]identityport.IDRef, 0, len(body.Refs))
	for _, ref := range body.Refs {
		kind, ok := ingestIdentityKind(ref.Type)
		if !ok || strings.TrimSpace(ref.Scope) != ref.Scope || strings.TrimSpace(ref.Value) != ref.Value || ref.Scope == "" || ref.Value == "" {
			writeIngestError(writer, request, platformhttp.NewError(platformhttp.CodeValidationFailed, identityapp.ErrInvalidIdentity))
			return
		}
		refs = append(refs, identityport.IDRef{Kind: kind, Scope: ref.Scope, Value: ref.Value, Assurance: identityport.AssuranceDeclared, Source: "admin"})
	}
	payload, err := json.Marshal(body.Payload)
	if err != nil {
		writeIngestError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, err))
		return
	}
	result, err := handler.application.Ingest(request.Context(), identityport.IngestCommand{
		Refs: refs, EventType: body.EventType, Payload: payload, Source: "admin",
		OccurredAt: body.OccurredAt, IdempotencyKey: string(params.IdempotencyKey),
	})
	if err != nil {
		writeIngestError(writer, request, err)
		return
	}
	response := generated.IngestIdentityEventResponse{}
	switch result.Status {
	case identityport.IngestAttributed:
		err = response.FromIngestIdentityEventAttributed(generated.IngestIdentityEventAttributed{CustomerId: int64(result.CustomerID), EventId: int64(result.EventID)})
	case identityport.IngestPending:
		err = response.FromIngestIdentityEventPending(generated.IngestIdentityEventPending{PendingEventId: result.PendingEventID})
	case identityport.IngestConflict:
		err = response.FromIngestIdentityEventConflict(generated.IngestIdentityEventConflict{PendingEventId: result.PendingEventID})
	default:
		err = identityapp.ErrIdentityIngestFailed
	}
	if err != nil {
		writeIngestError(writer, request, err)
		return
	}
	writeIngestJSON(writer, http.StatusOK, response)
}

func ingestAuthorized(request *http.Request) bool {
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != authport.CapabilityIdentityIngest || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return false
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	return ok && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps)
}

func ingestIdentityKind(value generated.IdentityRefType) (identityport.IDKind, bool) {
	kind, ok := map[generated.IdentityRefType]identityport.IDKind{
		generated.IdentityRefTypeWecomExternalUserid: identityport.KindWeComExternalUserID,
		generated.IdentityRefTypeUnionid:             identityport.KindUnionID,
		generated.IdentityRefTypeMpOpenid:            identityport.KindMPOpenID,
		generated.IdentityRefTypeOaOpenid:            identityport.KindOAOpenID,
		generated.IdentityRefTypeAlipayUserId:        identityport.KindAlipayUserID,
		generated.IdentityRefTypePhone:               identityport.KindPhone,
		generated.IdentityRefTypeExt:                 identityport.KindExtension,
	}[value]
	return kind, ok
}

func decodeIngestBody(writer http.ResponseWriter, request *http.Request, target any) error {
	if request.Body == nil || !strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "application/json") {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, identityapp.ErrInvalidIdentity)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return platformhttp.NewError(platformhttp.CodeMalformedRequest, err)
	}
	return nil
}

func validIngestKey(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && strings.TrimSpace(value) == value
}

func writeIngestError(writer http.ResponseWriter, request *http.Request, err error) {
	if request == nil {
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, identityapp.ErrInvalidIdentity):
		code = platformhttp.CodeValidationFailed
	case errors.Is(err, identityapp.ErrIdentityIngestIdempotencyConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func writeIngestJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

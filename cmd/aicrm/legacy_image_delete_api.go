package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type legacyImageDeleteApplication interface {
	DeleteImage(context.Context, mediaapp.ImageDeleteCommand) (mediaapp.ImageDeleteResult, error)
}

type legacyImageDeleteSuccess struct {
	OK                       bool                         `json:"ok"`
	Deleted                  bool                         `json:"deleted"`
	HardDeleted              bool                         `json:"hard_deleted"`
	ID                       int64                        `json:"id"`
	ReferencesCleared        legacyImageReferencesCleared `json:"references_cleared"`
	SourceStatus             string                       `json:"source_status"`
	RouteOwner               string                       `json:"route_owner"`
	FallbackUsed             bool                         `json:"fallback_used"`
	RealExternalCallExecuted bool                         `json:"real_external_call_executed"`
	StorageAdapterMode       string                       `json:"storage_adapter_mode"`
	AdapterMode              string                       `json:"adapter_mode"`
}

type legacyImageReferencesCleared struct {
	MiniprogramsCleared  int `json:"miniprograms_cleared"`
	CampaignStepsCleared int `json:"campaign_steps_cleared"`
}

type legacyImageDeleteConflict struct {
	OK                       bool                        `json:"ok"`
	Error                    string                      `json:"error"`
	References               legacyImageDeleteReferences `json:"references"`
	SourceStatus             string                      `json:"source_status"`
	RouteOwner               string                      `json:"route_owner"`
	FallbackUsed             bool                        `json:"fallback_used"`
	RealExternalCallExecuted bool                        `json:"real_external_call_executed"`
	StorageAdapterMode       string                      `json:"storage_adapter_mode"`
	AdapterMode              string                      `json:"adapter_mode"`
}

type legacyImageDeleteReferences struct {
	Miniprograms     []legacyImageReferenceID `json:"miniprograms"`
	CampaignSteps    []legacyImageReferenceID `json:"campaign_steps"`
	GroupInvites     []legacyImageReferenceID `json:"group_invites"`
	AutomationAgents []legacyImageReferenceID `json:"automation_agents"`
	Channels         []legacyImageReferenceID `json:"channels"`
	ImportPreflights []legacyImageReferenceID `json:"import_preflights"`
}

type legacyImageReferenceID struct {
	ID int64 `json:"id"`
}

func (handler *Handler) DeleteImage(writer http.ResponseWriter, request *http.Request) {
	if request == nil || request.URL == nil || request.ContentLength != 0 {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	imageID, err := parseLegacyImageDetailID(chi.URLParam(request, "image_id"))
	if err != nil {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	force, err := parseLegacyImageDeleteQuery(request.URL.RawQuery)
	if err != nil {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	key, err := legacyImageDeleteIdempotencyKey(request)
	if err != nil {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeMalformedRequest)
		return
	}
	principal, authorized := legacyImageDeletePrincipal(request)
	if !authorized {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeUnauthorized)
		return
	}
	if handler == nil || nilLegacyDependency(handler.imageDeletes) {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	application, ok := handler.imageDeletes.(legacyImageDeleteApplication)
	if !ok || nilLegacyDependency(application) {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	result, err := application.DeleteImage(request.Context(), mediaapp.ImageDeleteCommand{ImageID: imageID, Actor: principal.AdminUserID, IdempotencyKey: key, Force: force})
	if err != nil {
		switch {
		case errors.Is(err, mediaapp.ErrImageHasReferences):
			payload, marshalErr := json.Marshal(legacyImageDeleteConflict{OK: false, Error: "image_has_references", References: legacyImageDeleteReferencesFrom(result.References),
				SourceStatus: "local_delete", RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false, StorageAdapterMode: "postgresql", AdapterMode: "postgresql"})
			if marshalErr != nil {
				writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
				return
			}
			platformhttp.MarkCompatibilityError(writer, platformhttp.CodeConflict)
			writeLegacyImageDeleteJSON(writer, http.StatusConflict, payload)
			return
		case errors.Is(err, mediaapp.ErrImageDeleteNotFound):
			writeLegacyImageDeleteError(writer, request, platformhttp.CodeNotFound)
		case errors.Is(err, mediaapp.ErrImageDeleteConflict):
			writeLegacyImageDeleteError(writer, request, platformhttp.CodeConflict)
		default:
			writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
		}
		return
	}
	if !result.Deleted || !result.HardDeleted || result.ID != imageID || result.References.Any() {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	payload, err := json.Marshal(legacyImageDeleteSuccess{OK: true, Deleted: true, HardDeleted: true, ID: result.ID,
		ReferencesCleared: legacyImageReferencesCleared{MiniprogramsCleared: 0, CampaignStepsCleared: 0}, SourceStatus: "local_delete",
		RouteOwner: "ai_crm_next", FallbackUsed: false, RealExternalCallExecuted: false, StorageAdapterMode: "postgresql", AdapterMode: "postgresql"})
	if err != nil {
		writeLegacyImageDeleteError(writer, request, platformhttp.CodeDependencyUnavailable)
		return
	}
	writeLegacyImageDeleteJSON(writer, http.StatusOK, payload)
}

func parseLegacyImageDeleteQuery(rawQuery string) (bool, error) {
	if !utf8.ValidString(rawQuery) {
		return false, errInvalidImageDetailRequest
	}
	switch rawQuery {
	case "":
		return false, nil
	case "force=true":
		return true, nil
	case "force=false":
		return false, nil
	default:
		return false, errInvalidImageDetailRequest
	}
}

func legacyImageDeleteIdempotencyKey(request *http.Request) (string, error) {
	if request == nil {
		return "", errInvalidImageDetailRequest
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) == 0 {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return "", err
		}
		return hex.EncodeToString(bytes[:]), nil
	}
	if len(values) != 1 || !utf8.ValidString(values[0]) || values[0] != strings.TrimSpace(values[0]) || len(values[0]) < 16 || len(values[0]) > 128 {
		return "", errInvalidImageDetailRequest
	}
	return values[0], nil
}

func legacyImageDeletePrincipal(request *http.Request) (authport.Principal, bool) {
	if request == nil {
		return authport.Principal{}, false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principal, principalOK && principal.AdminUserID > 0 && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) &&
		authorizationOK && authorization.Capability == authport.CapabilityMediaLibraryWrite && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func legacyImageDeleteReferencesFrom(references mediaapp.ImageDeleteReferences) legacyImageDeleteReferences {
	return legacyImageDeleteReferences{
		Miniprograms: imageDeleteReferenceIDs(references.Miniprograms), CampaignSteps: imageDeleteReferenceIDs(references.CampaignSteps),
		GroupInvites: imageDeleteReferenceIDs(references.GroupInvites), AutomationAgents: imageDeleteReferenceIDs(references.AutomationAgents),
		Channels: imageDeleteReferenceIDs(references.Channels), ImportPreflights: imageDeleteReferenceIDs(references.ImportPreflights),
	}
}

func imageDeleteReferenceIDs(ids []int64) []legacyImageReferenceID {
	values := make([]legacyImageReferenceID, len(ids))
	for index, id := range ids {
		values[index] = legacyImageReferenceID{ID: id}
	}
	return values
}

func writeLegacyImageDeleteJSON(writer http.ResponseWriter, status int, payload []byte) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func writeLegacyImageDeleteError(writer http.ResponseWriter, request *http.Request, code platformhttp.ErrorCode) {
	status, message := http.StatusServiceUnavailable, "A required dependency is unavailable."
	switch code {
	case platformhttp.CodeMalformedRequest:
		status, message = http.StatusBadRequest, "The request is malformed."
	case platformhttp.CodeUnauthorized:
		status, message = http.StatusForbidden, "Permission is denied."
	case platformhttp.CodeNotFound:
		status, message = http.StatusNotFound, "The resource was not found."
	case platformhttp.CodeConflict:
		status, message = http.StatusConflict, "The request conflicts with the current state."
	case platformhttp.CodeDependencyUnavailable:
	default:
		code = platformhttp.CodeDependencyUnavailable
	}
	platformhttp.MarkCompatibilityError(writer, code)
	requestID := "unknown"
	if request != nil && platformhttp.RequestID(request.Context()) != "" {
		requestID = platformhttp.RequestID(request.Context())
	}
	payload, err := json.Marshal(struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}{Code: code, Message: message, RequestID: requestID})
	if err == nil {
		writeLegacyImageDeleteJSON(writer, status, payload)
	}
}

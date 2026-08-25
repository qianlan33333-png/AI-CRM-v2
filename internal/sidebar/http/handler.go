package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
	sidebarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/sidebar/app"
)

const bodyLimit = 32 << 10

type Handler struct{ service *sidebarapp.Service }

func NewHandler(service *sidebarapp.Service) (*Handler, error) {
	if nilValue(service) {
		return nil, sidebarapp.ErrUnavailable
	}
	return &Handler{service: service}, nil
}

func (handler *Handler) MintContext(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		ExternalUserID string `json:"external_userid"`
	}
	if err := decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	principal, authenticated := authport.PrincipalFromContext(request.Context())
	session, _ := authport.SessionFromContext(request.Context())
	result, err := handler.service.MintContext(request.Context(), principal, session, authenticated, body.ExternalUserID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Workbench(writer http.ResponseWriter, request *http.Request, token string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Workbench(request.Context(), scope)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) UpdateProfile(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersWrite)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var body struct {
		ExpectedUpdatedAt time.Time                       `json:"expected_updated_at"`
		Patch             contactport.SidebarProfilePatch `json:"patch"`
	}
	if err = decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.UpdateProfile(request.Context(), scope, body.ExpectedUpdatedAt, body.Patch, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Questionnaires(writer http.ResponseWriter, request *http.Request, token string, limit int32) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Questionnaires(request.Context(), scope, limit)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Orders(writer http.ResponseWriter, request *http.Request, token string, limit, offset int32) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.Orders(request.Context(), scope, limit, offset)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) PeriodicOrders(writer http.ResponseWriter, request *http.Request, token string, limit, offset int) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersRead)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.PeriodicOrders(request.Context(), scope, limit, offset)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) UpdatePeriodicRemark(writer http.ResponseWriter, request *http.Request, token, idempotencyKey string, serviceProductID int64, memberRef string) {
	scope, err := handler.scope(request, token, authport.CapabilityCustomersWrite)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	var body struct {
		ExpectedVersion int64  `json:"expected_version"`
		Remark          string `json:"remark"`
	}
	if err = decodeBody(writer, request, &body); err != nil {
		writeError(writer, request, err)
		return
	}
	result, err := handler.service.UpdatePeriodicRemark(request.Context(), scope, serviceProductID, memberRef, body.ExpectedVersion, &body.Remark, idempotencyKey)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) Materials(writer http.ResponseWriter, request *http.Request, token string, query mediaport.ImageListQuery) {
	if _, err := handler.scope(request, token, authport.CapabilityCustomersRead); err != nil {
		writeError(writer, request, err)
		return
	}
	query.EnabledOnly = true
	result, err := handler.service.Materials(request.Context(), query)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (handler *Handler) ThumbnailStatus(writer http.ResponseWriter, request *http.Request, token string, imageID int64) {
	if _, err := handler.scope(request, token, authport.CapabilityCustomersRead); err != nil {
		writeError(writer, request, err)
		return
	}
	status, err := handler.service.ThumbnailStatus(request.Context(), imageID)
	if err != nil {
		writeError(writer, request, err)
		return
	}
	writer.Header().Set("X-Thumbnail-Status", status)
	writeJSON(writer, http.StatusAccepted, struct {
		Status string            `json:"status"`
		Safety sidebarapp.Safety `json:"safety"`
	}{status, sidebarapp.Safety{LocalOnly: true}})
}

func (handler *Handler) scope(request *http.Request, token string, capability authport.Capability) (sidebarapp.Scope, error) {
	if handler == nil || nilValue(handler.service) || request == nil {
		return sidebarapp.Scope{}, sidebarapp.ErrUnavailable
	}
	principal, ok := authport.PrincipalFromContext(request.Context())
	if !ok {
		return sidebarapp.Scope{}, sidebarapp.ErrViewerSession
	}
	authorization, ok := authport.AuthorizationFromContext(request.Context())
	if !ok || authorization.Capability != capability {
		return sidebarapp.Scope{}, sidebarapp.ErrForbidden
	}
	session, ok := authport.SessionFromContext(request.Context())
	if !ok {
		return sidebarapp.Scope{}, sidebarapp.ErrViewerSession
	}
	scope, err := handler.service.VerifyContext(request.Context(), principal, session, token)
	if err != nil {
		return sidebarapp.Scope{}, err
	}
	if !authorization.AllowsOwner(scope.OwnerStaffID) {
		return sidebarapp.Scope{}, sidebarapp.ErrForbidden
	}
	return scope, nil
}

func decodeBody(writer http.ResponseWriter, request *http.Request, target any) error {
	if request == nil || request.Body == nil || target == nil {
		return sidebarapp.ErrInvalidInput
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return sidebarapp.ErrInvalidInput
	}
	request.Body = http.MaxBytesReader(writer, request.Body, bodyLimit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return sidebarapp.ErrInvalidInput
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sidebarapp.ErrInvalidInput
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, request *http.Request, err error) {
	code := platformhttp.CodeDependencyUnavailable
	switch {
	case errors.Is(err, sidebarapp.ErrInvalidInput):
		code = platformhttp.CodeMalformedRequest
	case errors.Is(err, sidebarapp.ErrViewerSession), errors.Is(err, sidebarapp.ErrTokenInvalid), errors.Is(err, sidebarapp.ErrTokenExpired):
		code = platformhttp.CodeUnauthenticated
	case errors.Is(err, sidebarapp.ErrForbidden):
		code = platformhttp.CodeUnauthorized
	case errors.Is(err, sidebarapp.ErrNotFound), errors.Is(err, sidebarapp.ErrCustomerNotBound):
		code = platformhttp.CodeNotFound
	case errors.Is(err, sidebarapp.ErrConflict):
		code = platformhttp.CodeConflict
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

func nilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface) && reflected.IsNil()
}

func ValidContextToken(value string) bool {
	return len(value) >= 64 && len(value) <= 4096 && strings.TrimSpace(value) == value
}

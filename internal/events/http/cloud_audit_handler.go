package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type CloudAuditApplication interface {
	List(context.Context, eventport.CloudAuditFilter) (eventapp.CloudAuditResult, error)
}

type CloudAuditHandler struct{ application CloudAuditApplication }

func NewCloudAuditHandler(application CloudAuditApplication) (*CloudAuditHandler, error) {
	if application == nil {
		return nil, eventapp.ErrCloudAuditUnavailable
	}
	return &CloudAuditHandler{application: application}, nil
}

// List is a leaf-only cloud-orchestrator audit endpoint. Route registration is
// owned by the central API integration lane.
func (handler *CloudAuditHandler) List(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if handler == nil || handler.application == nil || request == nil || request.URL == nil {
		writeCloudAuditError(writer, request, eventapp.ErrCloudAuditUnavailable)
		return
	}
	if request.Method != stdhttp.MethodGet || request.Body != nil && request.Body != stdhttp.NoBody {
		writeCloudAuditError(writer, request, eventapp.ErrCloudAuditUnavailable)
		return
	}
	if err := authorizeCloudAudit(request.Context()); err != nil {
		writeCloudAuditError(writer, request, err)
		return
	}
	filter, err := parseCloudAuditFilter(request.URL.RawQuery)
	if err != nil {
		writeCloudAuditError(writer, request, err)
		return
	}
	result, err := handler.application.List(request.Context(), filter)
	if err != nil {
		writeCloudAuditError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(writer).Encode(result)
}

func parseCloudAuditFilter(raw string) (eventport.CloudAuditFilter, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return eventport.CloudAuditFilter{}, eventapp.ErrCloudAuditInvalid
	}
	for key, entries := range values {
		if (key != "trace_id" && key != "session_id" && key != "limit") || len(entries) != 1 || entries[0] == "" || !utf8.ValidString(entries[0]) || strings.TrimSpace(entries[0]) != entries[0] {
			return eventport.CloudAuditFilter{}, eventapp.ErrCloudAuditInvalid
		}
	}
	filter := eventport.CloudAuditFilter{TraceID: values.Get("trace_id"), SessionID: values.Get("session_id"), Limit: 50}
	if value := values.Get("limit"); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 32)
		if parseErr != nil || parsed < 1 || parsed > 100 {
			return eventport.CloudAuditFilter{}, eventapp.ErrCloudAuditInvalid
		}
		filter.Limit = int32(parsed)
	}
	if filter.TraceID == "" && filter.SessionID == "" {
		return eventport.CloudAuditFilter{}, eventapp.ErrCloudAuditInvalid
	}
	return filter, nil
}

func authorizeCloudAudit(ctx context.Context) error {
	principal, principalOK := authport.PrincipalFromContext(ctx)
	authorization, authorizationOK := authport.AuthorizationFromContext(ctx)
	if !principalOK || principal.AdminUserID < 1 {
		return platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated)
	}
	if !authorizationOK || principal.Role != authport.RoleAdmin || authorization.Capability != authport.CapabilityAdminRead || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		return platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized)
	}
	return nil
}

func writeCloudAuditError(writer stdhttp.ResponseWriter, request *stdhttp.Request, err error) {
	var platformError *platformhttp.HTTPError
	if errors.As(err, &platformError) {
		platformhttp.WriteError(writer, request, platformError)
		return
	}
	code := platformhttp.CodeDependencyUnavailable
	if errors.Is(err, eventapp.ErrCloudAuditInvalid) {
		code = platformhttp.CodeMalformedRequest
	}
	platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
}

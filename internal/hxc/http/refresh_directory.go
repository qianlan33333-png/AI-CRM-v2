package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
)

const RefreshDirectoryPath = "/api/admin/hxc-dashboard/refresh-directory"

type DirectoryRefreshApplication interface {
	Refresh(context.Context, hxcapp.RefreshDirectoryCommand) (hxcapp.RefreshDirectoryResult, error)
}

type DirectoryRefreshHandler struct{ Application DirectoryRefreshApplication }

func (handler DirectoryRefreshHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	setHeaders(writer)
	if request == nil || request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 || (principal.Role != authport.RoleAdmin && principal.Role != authport.RoleOps) || !authorizationOK || authorization.Capability != authport.CapabilityOperationsManage || authorization.Scope != authport.ScopeGlobal || authorization.OwnerStaffID != 0 {
		writeError(writer, http.StatusForbidden, "permission_denied")
		return
	}
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !validKey(values[0]) || !emptyBody(writer, request) {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if handler.Application == nil {
		writeError(writer, http.StatusServiceUnavailable, "hxc_directory_refresh_unavailable")
		return
	}
	result, err := handler.Application.Refresh(request.Context(), hxcapp.RefreshDirectoryCommand{ActorID: principal.AdminUserID, PageSize: 100, IdempotencyKey: values[0]})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "hxc_directory_refresh_unavailable")
		return
	}
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(result)
}

func validKey(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) >= 16 && utf8.RuneCountInString(value) <= 128
}

func emptyBody(writer http.ResponseWriter, request *http.Request) bool {
	if request.Body == nil {
		return true
	}
	raw, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 1024))
	if err != nil {
		return false
	}
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "{}"
}

func setHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"ok": false, "error": map[string]string{"code": code}, "provider_read_executed": false})
}

package authhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	authapp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/app"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

const AdminAccessPath = "/api/admin/admin-access"

type adminAccessApplication interface {
	List(context.Context) ([]authapp.AdminAccessMember, error)
	Save(context.Context, []authapp.AdminAccessSaveInput) ([]authapp.AdminAccessMember, error)
}

type AdminAccessHandler struct{ application adminAccessApplication }

func NewAdminAccessHandler(application adminAccessApplication) (*AdminAccessHandler, error) {
	if application == nil {
		return nil, authapp.ErrAdminAccessUnavailable
	}
	return &AdminAccessHandler{application: application}, nil
}

func (handler *AdminAccessHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	adminAccessHeaders(writer)
	if handler == nil || handler.application == nil || request == nil || request.URL == nil {
		adminAccessError(writer, http.StatusServiceUnavailable, "admin_access_unavailable")
		return
	}
	if request.URL.EscapedPath() != request.URL.Path || strings.Contains(request.URL.Path, `\`) || request.URL.Path != AdminAccessPath {
		adminAccessError(writer, http.StatusNotFound, "not_found")
		return
	}
	switch request.Method {
	case http.MethodGet:
		handler.list(writer, request)
	case http.MethodPut:
		handler.save(writer, request)
	default:
		writer.Header().Set("Allow", "GET, PUT")
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (handler *AdminAccessHandler) list(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" || !adminAccessEmptyBody(writer, request) {
		adminAccessError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if !adminAccessAuthorized(request) {
		adminAccessError(writer, adminAccessAuthorizationStatus(request), "permission_denied")
		return
	}
	members, err := handler.application.List(request.Context())
	if err != nil {
		adminAccessApplicationError(writer, err)
		return
	}
	adminAccessJSON(writer, http.StatusOK, map[string]any{"ok": true, "members": adminAccessMembers(members), "local_only": true, "external": false})
}

func (handler *AdminAccessHandler) save(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" || !adminAccessAuthorized(request) {
		status := http.StatusBadRequest
		code := "invalid_request"
		if request.URL.RawQuery == "" {
			status, code = adminAccessAuthorizationStatus(request), "permission_denied"
		}
		adminAccessError(writer, status, code)
		return
	}
	key, validKey := adminAccessIdempotencyKey(request)
	input, validInput := adminAccessInput(writer, request)
	if !validKey || !validInput || !adminAccessValidMembers(input.Members) {
		adminAccessError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := handler.application.Save(request.Context(), input.Members); err != nil {
		adminAccessApplicationError(writer, err)
		return
	}
	members, err := handler.application.List(request.Context())
	if err != nil {
		adminAccessApplicationError(writer, err)
		return
	}
	adminAccessJSON(writer, http.StatusOK, map[string]any{"ok": true, "members": adminAccessMembers(members), "idempotency_key": key, "local_only": true, "external": false})
}

func adminAccessValidMembers(members []authapp.AdminAccessSaveInput) bool {
	if len(members) < 1 || len(members) > 200 {
		return false
	}
	seen := make(map[int64]struct{}, len(members))
	for _, member := range members {
		if member.AdminUserID < 1 {
			return false
		}
		if _, duplicate := seen[member.AdminUserID]; duplicate {
			return false
		}
		seen[member.AdminUserID] = struct{}{}
	}
	return true
}

type adminAccessMemberResponse struct {
	AdminUserID      int64  `json:"admin_user_id"`
	DisplayName      string `json:"display_name"`
	Role             string `json:"role"`
	StaffID          *int64 `json:"staff_id"`
	StaffWeComUserID string `json:"staff_wecom_userid"`
	StaffName        string `json:"staff_name"`
	IsActive         bool   `json:"is_active"`
	LoginEnabled     bool   `json:"login_enabled"`
}

func adminAccessMembers(members []authapp.AdminAccessMember) []adminAccessMemberResponse {
	result := make([]adminAccessMemberResponse, len(members))
	for index, member := range members {
		result[index] = adminAccessMemberResponse{
			AdminUserID: member.AdminUserID, DisplayName: member.DisplayName, Role: string(member.Role), StaffID: member.StaffID,
			StaffWeComUserID: member.StaffWeComUserID, StaffName: member.StaffName, IsActive: member.IsActive, LoginEnabled: member.LoginEnabled,
		}
	}
	return result
}

type adminAccessSaveRequest struct {
	Members []authapp.AdminAccessSaveInput `json:"members"`
}

func adminAccessInput(writer http.ResponseWriter, request *http.Request) (adminAccessSaveRequest, bool) {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		return adminAccessSaveRequest{}, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var input adminAccessSaveRequest
	if decoder.Decode(&input) != nil {
		return adminAccessSaveRequest{}, false
	}
	var extra any
	return input, errors.Is(decoder.Decode(&extra), io.EOF)
}

func adminAccessIdempotencyKey(request *http.Request) (string, bool) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 || values[0] == "" || values[0] != strings.TrimSpace(values[0]) || len(values[0]) > 200 {
		return "", false
	}
	return values[0], true
}

func adminAccessAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK &&
		authorization.Capability == authport.CapabilityConfigSettingsManage && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func adminAccessAuthorizationStatus(request *http.Request) int {
	if request == nil {
		return http.StatusUnauthorized
	}
	if _, present := authport.PrincipalFromContext(request.Context()); !present {
		return http.StatusUnauthorized
	}
	return http.StatusForbidden
}

func adminAccessApplicationError(writer http.ResponseWriter, err error) {
	if errors.Is(err, authapp.ErrInvalidAdminAccessInput) || errors.Is(err, authapp.ErrAdminAccessMemberMissing) {
		adminAccessError(writer, http.StatusBadRequest, "invalid_member")
		return
	}
	adminAccessError(writer, http.StatusServiceUnavailable, "admin_access_unavailable")
}

func adminAccessHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}

func adminAccessEmptyBody(writer http.ResponseWriter, request *http.Request) bool {
	if request.Body == nil {
		return true
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 64<<10))
	return err == nil && len(body) == 0
}

func adminAccessError(writer http.ResponseWriter, status int, code string) {
	adminAccessJSON(writer, status, map[string]any{"ok": false, "error": code})
}

func adminAccessJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

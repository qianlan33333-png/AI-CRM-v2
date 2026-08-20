package main

import (
	"context"
	"net/http"
	"net/url"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	legacyHXCSenderPagePath    = "/admin/hxc-send-config"
	legacyHXCSenderReadPath    = "/api/admin/hxc-dashboard/send-config"
	legacyHXCSenderItemPath    = "/api/admin/hxc-dashboard/send-config/{sender_userid}"
	legacyHXCSenderReorderPath = "/api/admin/hxc-dashboard/send-config/reorder"
	hxcSenderWarning           = "HXC senders use the local staff projection; no WeCom directory call was executed."
)

type hxcSenderRead interface {
	Read(context.Context) (hxcapp.Projection, error)
}
type hxcSenderManage interface {
	Save(context.Context, hxcapp.ManageCommand) (hxcport.SenderConfig, error)
	Archive(context.Context, hxcapp.ManageCommand) error
	Reorder(context.Context, string, string, []string) ([]hxcport.SenderConfig, error)
}
type hxcSenderHandler struct {
	reader  hxcSenderRead
	manager hxcSenderManage
}

type hxcSenderConfigResponse struct {
	ID           string `json:"id"`
	SenderUserID string `json:"sender_userid"`
	DisplayName  string `json:"display_name"`
	Priority     int    `json:"priority"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}
type hxcDirectoryCandidateResponse struct {
	WeComUserID string `json:"wecom_userid"`
	DisplayName string `json:"display_name"`
	Position    string `json:"position"`
	WeComStatus int    `json:"wecom_status"`
	IsSender    bool   `json:"is_sender"`
	Priority    int    `json:"priority"`
	IsActive    bool   `json:"is_active"`
}
type hxcSenderReadResponse struct {
	OK                       bool                            `json:"ok"`
	SourceStatus             string                          `json:"source_status"`
	RouteOwner               string                          `json:"route_owner"`
	FallbackUsed             bool                            `json:"fallback_used"`
	RealExternalCallExecuted bool                            `json:"real_external_call_executed"`
	SendConfigs              []hxcSenderConfigResponse       `json:"send_configs"`
	DirectoryCandidates      []hxcDirectoryCandidateResponse `json:"directory_candidates"`
	Members                  []hxcDirectoryCandidateResponse `json:"members"`
	DirectoryCount           int                             `json:"directory_count"`
	SenderCount              int                             `json:"sender_count"`
	ActiveSenderCount        int                             `json:"active_sender_count"`
	LastSyncedAt             string                          `json:"last_synced_at"`
	Warnings                 []string                        `json:"warnings"`
	Degraded                 bool                            `json:"degraded"`
	EmptyState               bool                            `json:"empty_state"`
}

func (handler *hxcSenderHandler) Page(writer http.ResponseWriter, request *http.Request) {
	if request != nil {
		http.Redirect(writer, request, "/?legacy_admin_path="+url.QueryEscape(legacyHXCSenderPagePath), http.StatusFound)
	}
}
func (handler *hxcSenderHandler) Read(writer http.ResponseWriter, request *http.Request) {
	if !legacyHXCSenderAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if handler == nil || handler.reader == nil {
		writeHXCSenderUnavailable(writer)
		return
	}
	projection, err := handler.reader.Read(request.Context())
	if err != nil {
		writeHXCSenderUnavailable(writer)
		return
	}
	configs := make([]hxcSenderConfigResponse, 0, len(projection.SendConfigs))
	for _, config := range projection.SendConfigs {
		configs = append(configs, projectHXCSenderConfig(config))
	}
	candidates := make([]hxcDirectoryCandidateResponse, 0, len(projection.Directory))
	for _, candidate := range projection.Directory {
		candidates = append(candidates, hxcDirectoryCandidateResponse{WeComUserID: candidate.WeComUserID, DisplayName: candidate.DisplayName, Position: "", WeComStatus: 0, IsSender: candidate.IsSender, Priority: candidate.Priority, IsActive: candidate.IsActive})
	}
	lastSyncedAt := ""
	if !projection.LastSyncedAt.IsZero() {
		lastSyncedAt = projection.LastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	writeJSON(writer, http.StatusOK, hxcSenderReadResponse{OK: true, SourceStatus: "v2_local_staff", RouteOwner: "aicrm_v2", FallbackUsed: false, RealExternalCallExecuted: false, SendConfigs: configs, DirectoryCandidates: candidates, Members: candidates, DirectoryCount: len(candidates), SenderCount: len(configs), ActiveSenderCount: projection.ActiveSenderCount, LastSyncedAt: lastSyncedAt, Warnings: []string{hxcSenderWarning}, Degraded: false, EmptyState: len(candidates) == 0})
}
func projectHXCSenderConfig(config hxcport.SenderConfig) hxcSenderConfigResponse {
	return hxcSenderConfigResponse{ID: config.ID, SenderUserID: config.SenderUserID, DisplayName: config.DisplayName, Priority: config.Priority, IsActive: config.IsActive, CreatedAt: config.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: config.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}
func writeHXCSenderUnavailable(writer http.ResponseWriter) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "status_code": http.StatusServiceUnavailable, "error_code": "hxc_send_config_unavailable", "real_external_call_executed": false})
}
func legacyHXCSenderAuthorized(request *http.Request) bool {
	if request == nil {
		return false
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK && authorization.Capability == authport.CapabilityAdminRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

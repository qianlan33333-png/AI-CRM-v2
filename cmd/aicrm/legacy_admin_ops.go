// This adapter preserves the frozen Admin Config and Jobs contracts over a
// local control plane. It never invokes a provider or a foreign-domain worker.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	adminopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/store"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

// legacyAdminOps is local to the composition root: no cross-domain public port
// is created merely to serve legacy compatibility routes.
type legacyAdminOps interface {
	CreateCredential(context.Context, adminopsapp.CredentialCommand) (adminopsport.Credential, error)
	RotateCredential(context.Context, adminopsport.CredentialKind, string, string, string) (adminopsport.Credential, error)
	SetCredentialEnabled(context.Context, adminopsport.CredentialKind, string, bool, string, string, string) (adminopsport.Credential, error)
	UpdateCredential(context.Context, adminopsapp.CredentialCommand) (adminopsport.Credential, error)
	ListCredentials(context.Context) ([]adminopsport.Credential, error)
	GetCredential(context.Context, adminopsport.CredentialKind, string) (adminopsport.Credential, error)
	SetCategory(context.Context, string, bool, map[string]any, string, string) (adminopsport.Category, error)
	ListCategories(context.Context) ([]adminopsport.Category, error)
	GetCategory(context.Context, string) (adminopsport.Category, error)
	CreateRelease(context.Context, adminopsapp.ReleaseCommand) (adminopsport.Release, error)
	ValidateRelease(context.Context, int64, string, string) (adminopsport.Release, error)
	PublishRelease(context.Context, int64, string, string, string) (adminopsport.Release, error)
	RollbackRelease(context.Context, int64, string, string) (adminopsport.Release, error)
	GetRelease(context.Context, int64) (adminopsport.Release, error)
	ListReleases(context.Context, int32) ([]adminopsport.Release, error)
	EnqueueJob(context.Context, adminopsapp.JobCommand) (adminopsport.Job, error)
	GetJob(context.Context, string) (adminopsport.Job, error)
	ListJobs(context.Context, string, string, int32) ([]adminopsport.Job, error)
	CancelJob(context.Context, string, int64, string, string) (adminopsport.Job, error)
	SaveFeishuNotification(context.Context, bool, string, string, string) (adminopsapp.NotificationSetting, error)
	GetFeishuNotification(context.Context) (adminopsapp.NotificationSetting, error)
}

var adminOpsPageTemplate = template.Must(template.New("admin-ops").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>{{.Title}}</title><main><h1>{{.Title}}</h1><p>{{.Summary}}</p><pre>{{.Payload}}</pre></main></html>`))

var adminOpsAPIClientListTemplate = template.Must(template.New("admin-ops-api-clients").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>API 接入与 Token</title><main><h1>API 接入与 Token</h1><p>创建、轮换和停用外部 API 客户端；Secret 只在创建或轮换时显示一次。</p><nav><a href="/admin/config/api-clients/new">新建客户端</a></nav><form method="get" action="/admin/config/api-clients"><label>搜索 <input name="q" value="{{.Query}}"></label><label>状态 <select name="status"><option value=""{{if eq .Status ""}} selected{{end}}>全部</option><option value="enabled"{{if eq .Status "enabled"}} selected{{end}}>已启用</option><option value="disabled"{{if eq .Status "disabled"}} selected{{end}}>已停用</option><option value="pending_activation"{{if eq .Status "pending_activation"}} selected{{end}}>待激活</option></select></label><button type="submit">应用筛选</button><a href="/admin/config/api-clients">重置</a></form><p>共 {{.ConfiguredCount}} 个；已启用 {{.EnabledCount}} 个；已停用 {{.DisabledCount}} 个；待激活 {{.PendingActivationCount}} 个。</p><table><thead><tr><th>客户端</th><th>状态</th><th>版本</th><th>最近更新</th><th>操作</th></tr></thead><tbody>{{range .Clients}}<tr><td><strong>{{.DisplayName}}</strong><br><code>{{.ClientID}}</code></td><td>{{.StatusLabel}}</td><td>v{{.Version}}</td><td>{{.UpdatedAt}}</td><td><a href="/admin/config/api-clients/{{.EscapedClientID}}">管理 Secret</a></td></tr>{{else}}<tr><td colspan="5">没有符合条件的 API 客户端。</td></tr>{{end}}</tbody></table></main></html>`))

type adminOpsPageData struct{ Title, Summary, Payload string }

type adminOpsAPIClientListPageData struct {
	Query, Status                                                        string
	ConfiguredCount, EnabledCount, DisabledCount, PendingActivationCount int
	Clients                                                              []adminOpsAPIClientListItem
}

type adminOpsAPIClientListItem struct {
	ClientID, EscapedClientID, DisplayName, StatusLabel, UpdatedAt string
	Version                                                        int64
}

func adminOpsActionToken(session authport.SessionRef, method, pattern string) string {
	mac := hmac.New(sha256.New, []byte(session))
	_, _ = mac.Write([]byte("v1\n" + method + "\n" + pattern))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (handler *Handler) AdminOps(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || handler.adminOps == nil || request == nil {
		writeAdminOpsError(writer, http.StatusServiceUnavailable, "admin_ops_unavailable")
		return
	}
	if strings.HasPrefix(request.URL.Path, "/api/") {
		handler.adminOpsAPI(writer, request)
		return
	}
	if request.Method == http.MethodPost {
		handler.adminOpsForm(writer, request)
		return
	}
	handler.adminOpsPage(writer, request)
}

func (handler *Handler) adminOpsPage(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/admin/config/wecom-tags" {
		http.Redirect(writer, request, "/admin/wecom-tags", http.StatusFound)
		return
	}
	if request.URL.Path == "/admin/config/api-clients" {
		handler.adminOpsAPIClientListPage(writer, request)
		return
	}
	title, summary := "配置控制面", "本地读取模型；密钥只展示引用和掩码，任务不会在 HTTP 请求中执行。"
	payload := map[string]any{"route_owner": "adminops", "provider_execution": false, "worker_isolated": true, "secret_boundary": "reference_and_mask_only"}
	if strings.Contains(request.URL.Path, "release") {
		title = "配置发布"
		releases, err := handler.adminOps.ListReleases(request.Context(), 50)
		if err == nil {
			payload["releases"] = releases
		}
	}
	if request.URL.Path == "/admin/config" {
		categories, err := handler.adminOps.ListCategories(request.Context())
		if err == nil {
			payload["categories"] = categories
		}
	}
	if request.URL.Path == "/admin/runtime-config" {
		title, summary = "运行配置快照", "只显示非 secret 的本地运行治理状态。"
	}
	raw, _ := json.Marshal(payload)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminOpsPageTemplate.Execute(writer, adminOpsPageData{Title: title, Summary: summary, Payload: string(raw)})
}

func (handler *Handler) adminOpsAPIClientListPage(writer http.ResponseWriter, request *http.Request) {
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	status := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("status")))
	if status != "" && status != "enabled" && status != "disabled" && status != "pending_activation" {
		writeAdminOpsError(writer, http.StatusBadRequest, "invalid_status_filter")
		return
	}
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) > 200 {
		writeAdminOpsError(writer, http.StatusBadRequest, "invalid_query_filter")
		return
	}
	credentials, err := handler.adminOps.ListCredentials(request.Context())
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	data := adminOpsAPIClientListPageData{Query: query, Status: status}
	normalizedQuery := strings.ToLower(query)
	for _, credential := range credentials {
		if credential.Kind != adminopsport.CredentialAPIClient {
			continue
		}
		if !validAdminOpsPageClientID(credential.ClientID) || credential.DisplayName == "" ||
			credential.DisplayName != strings.TrimSpace(credential.DisplayName) || !utf8.ValidString(credential.DisplayName) ||
			utf8.RuneCountInString(credential.DisplayName) > 200 || (credential.State != "active" && credential.State != "disabled" && credential.State != "pending_activation") ||
			credential.Version < 1 || credential.UpdatedAt.IsZero() {
			writeAdminOpsError(writer, http.StatusServiceUnavailable, "admin_ops_unavailable")
			return
		}
		data.ConfiguredCount++
		switch credential.State {
		case "active":
			data.EnabledCount++
		case "disabled":
			data.DisabledCount++
		case "pending_activation":
			data.PendingActivationCount++
		}
		requestedState := status
		if requestedState == "enabled" {
			requestedState = "active"
		}
		if requestedState != "" && credential.State != requestedState {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{credential.ClientID, credential.DisplayName, string(credential.Kind)}, " "))
		if normalizedQuery != "" && !strings.Contains(haystack, normalizedQuery) {
			continue
		}
		statusLabel := "待激活"
		if credential.State == "disabled" {
			statusLabel = "已停用"
		} else if credential.State == "active" {
			statusLabel = "已启用"
		}
		data.Clients = append(data.Clients, adminOpsAPIClientListItem{
			ClientID: credential.ClientID, EscapedClientID: url.PathEscape(credential.ClientID),
			DisplayName: credential.DisplayName, StatusLabel: statusLabel,
			Version: credential.Version, UpdatedAt: credential.UpdatedAt.UTC().Format("2006-01-02 15:04:05Z"),
		})
	}
	sort.Slice(data.Clients, func(i, j int) bool { return data.Clients[i].ClientID < data.Clients[j].ClientID })
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if err := adminOpsAPIClientListTemplate.Execute(writer, data); err != nil {
		return
	}
}

func validAdminOpsPageClientID(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 120 || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, item := range value {
		if !(item == '-' || item == '_' || item == '.' || item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z' || item >= '0' && item <= '9') {
			return false
		}
	}
	return true
}

func (handler *Handler) adminOpsForm(writer http.ResponseWriter, request *http.Request) {
	switch {
	case request.URL.Path == "/admin/config/releases":
		handler.adminOpsCreateReleaseForm(writer, request)
	case strings.HasPrefix(request.URL.Path, "/admin/config/releases/") && strings.HasSuffix(request.URL.Path, "/validate"):
		handler.adminOpsReleaseActionForm(writer, request, "validate")
	case strings.HasPrefix(request.URL.Path, "/admin/config/releases/") && strings.HasSuffix(request.URL.Path, "/publish"):
		handler.adminOpsReleaseActionForm(writer, request, "publish")
	case strings.HasPrefix(request.URL.Path, "/admin/config/releases/") && strings.HasSuffix(request.URL.Path, "/rollback"):
		handler.adminOpsReleaseActionForm(writer, request, "rollback")
	case request.URL.Path == "/setup/wizard/save":
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "saved": false, "reason": "generic_setup_wizard_not_owned", "real_external_call_executed": false})
	default:
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
	}
}

func (handler *Handler) adminOpsCreateReleaseForm(writer http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := adminOpsFormIdentity(writer, request)
	if !ok || request.PostForm.Get("confirm") != "1" {
		redirectAdminOpsRelease(writer, request, "/admin/config/releases/new", "invalid_action")
		return
	}
	changes, err := releaseChangesFromForm(request)
	if err != nil {
		redirectAdminOpsRelease(writer, request, "/admin/config/releases/new", "invalid_release_changes")
		return
	}
	release, err := handler.adminOps.CreateRelease(request.Context(), adminopsapp.ReleaseCommand{Changes: changes, Actor: actor, RequestID: requestID})
	if err != nil {
		redirectAdminOpsRelease(writer, request, "/admin/config/releases/new", "release_create_failed")
		return
	}
	http.Redirect(writer, request, "/admin/config/releases/"+strconv.FormatInt(release.ID, 10)+"?created=1", http.StatusFound)
}

func (handler *Handler) adminOpsReleaseActionForm(writer http.ResponseWriter, request *http.Request, action string) {
	actor, requestID, ok := adminOpsFormIdentity(writer, request)
	id, err := strconv.ParseInt(chi.URLParam(request, "release_id"), 10, 64)
	path := "/admin/config/releases/" + strconv.FormatInt(id, 10)
	if !ok || err != nil || id < 1 {
		redirectAdminOpsRelease(writer, request, path, "invalid_action")
		return
	}
	var itemErr error
	switch action {
	case "validate":
		_, itemErr = handler.adminOps.ValidateRelease(request.Context(), id, actor, requestID)
	case "publish":
		if request.PostForm.Get("confirm") != "1" {
			redirectAdminOpsRelease(writer, request, path, "confirmation_required")
			return
		}
		_, itemErr = handler.adminOps.PublishRelease(request.Context(), id, strings.TrimSpace(request.PostForm.Get("checksum")), actor, requestID)
	case "rollback":
		if request.PostForm.Get("confirm") != "1" {
			redirectAdminOpsRelease(writer, request, path, "confirmation_required")
			return
		}
		_, itemErr = handler.adminOps.RollbackRelease(request.Context(), id, actor, requestID)
	}
	if itemErr != nil {
		redirectAdminOpsRelease(writer, request, path, "release_action_failed")
		return
	}
	if action == "validate" {
		http.Redirect(writer, request, path+"?validated=1", http.StatusFound)
	} else {
		http.Redirect(writer, request, "/admin/config/releases?"+action+"d=1", http.StatusFound)
	}
}

func adminOpsFormIdentity(writer http.ResponseWriter, request *http.Request) (string, string, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	if request.ParseForm() != nil {
		return "", "", false
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	token := request.PostForm["admin_action_token"]
	if !sessionOK || !principalOK || principal.AdminUserID < 1 || len(token) != 1 {
		return "", "", false
	}
	expected := adminOpsActionToken(session, request.Method, request.URL.Path)
	if len(token[0]) != len(expected) || !hmac.Equal([]byte(token[0]), []byte(expected)) {
		return "", "", false
	}
	requestID := platformhttp.RequestID(request.Context())
	if requestID == "" {
		return "", "", false
	}
	return "admin:" + strconv.FormatInt(principal.AdminUserID, 10), requestID, true
}

func releaseChangesFromForm(request *http.Request) (map[string]any, error) {
	indexes := make([]int, 0)
	for name, values := range request.PostForm {
		if !strings.HasPrefix(name, "key__") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(name, "key__"))
		if err != nil || index < 0 || index > 31 || len(values) != 1 {
			return nil, adminopsapp.ErrInvalidCommand
		}
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	changes := make(map[string]any, len(indexes))
	for _, index := range indexes {
		key := strings.TrimSpace(request.PostForm.Get("key__" + strconv.Itoa(index)))
		if key == "" {
			continue
		}
		if !validReleaseKey(key) {
			return nil, adminopsapp.ErrInvalidCommand
		}
		if _, ok := changes[key]; ok {
			return nil, adminopsapp.ErrConflict
		}
		remove := request.PostForm.Get("remove__" + strconv.Itoa(index))
		if remove == "1" {
			changes[key] = nil
			continue
		}
		values := request.PostForm["value__"+strconv.Itoa(index)]
		if len(values) != 1 || len(values[0]) > 4096 {
			return nil, adminopsapp.ErrInvalidCommand
		}
		value := strings.TrimSpace(values[0])
		if releaseKeyRequiresSecretReference(key) && !validSecretReference(value) {
			return nil, adminopsapp.ErrSecretMaterial
		}
		changes[key] = value
	}
	if len(changes) == 0 {
		return nil, adminopsapp.ErrInvalidCommand
	}
	return changes, nil
}

func validReleaseKey(value string) bool {
	if value == "" || len(value) > 160 || strings.ContainsAny(value, "\n\r ") {
		return false
	}
	for _, character := range value {
		if !(character == '.' || character == '_' || character == '-' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func releaseKeyRequiresSecretReference(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "webhook") || lower == "token" || strings.HasSuffix(lower, ".token")
}

func validSecretReference(value string) bool {
	return (strings.HasPrefix(value, "secret://") || strings.HasPrefix(value, "secretref:")) && len(value) <= 250 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "?&# ")
}

func redirectAdminOpsRelease(writer http.ResponseWriter, request *http.Request, path, code string) {
	http.Redirect(writer, request, path+"?error="+code, http.StatusFound)
}

func (handler *Handler) adminOpsAPI(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	switch {
	case strings.HasPrefix(path, "/api/admin/config/api-key"):
		handler.adminOpsDirectKey(writer, request)
	case strings.HasPrefix(path, "/api/admin/config/api-clients"):
		handler.adminOpsClients(writer, request)
	case strings.HasPrefix(path, "/api/admin/config/categories"):
		handler.adminOpsCategories(writer, request)
	case strings.HasPrefix(path, "/api/admin/config/releases"):
		handler.adminOpsReleases(writer, request)
	case strings.HasPrefix(path, "/api/admin/config/push-capabilities"):
		handler.adminOpsPush(writer, request)
	case path == "/api/admin/config/definitions" || path == "/api/admin/config/deployment-profile":
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "definitions": []any{}, "deployment_profile": "local_control_plane", "secret_values_exposed": false})
	case strings.HasPrefix(path, "/api/admin/config/routing") || path == "/api/admin/config/signup-tags" || path == "/api/admin/config/class-term-tags":
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_retired")
	case strings.HasPrefix(path, "/api/admin/jobs/") || path == "/api/admin/broadcast-jobs" || strings.HasPrefix(path, "/api/admin/broadcast-jobs/"):
		handler.adminOpsJobs(writer, request)
	default:
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
	}
}

func (handler *Handler) adminOpsDirectKey(writer http.ResponseWriter, request *http.Request) {
	const clientID = "direct-default"
	path := request.URL.Path
	if request.Method == http.MethodGet {
		item, err := handler.adminOps.GetCredential(request.Context(), adminopsport.CredentialDirectAPIKey, clientID)
		if errors.Is(err, adminopsstore.ErrNotFound) {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "configured": false, "secret_values_exposed": false})
			return
		}
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "configured": true, "api_key": publicCredential(item)})
		return
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, path)
	if !ok {
		return
	}
	if payload["confirm"] != true {
		writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
		return
	}
	var item adminopsport.Credential
	var err error
	switch path {
	case "/api/admin/config/api-key/generate":
		item, err = handler.adminOps.CreateCredential(request.Context(), adminopsapp.CredentialCommand{Kind: adminopsport.CredentialDirectAPIKey, ClientID: clientID, DisplayName: "Legacy direct API key", Metadata: map[string]any{}, Actor: actor, RequestID: requestID})
	case "/api/admin/config/api-key/rotate":
		item, err = handler.adminOps.RotateCredential(request.Context(), adminopsport.CredentialDirectAPIKey, clientID, actor, requestID)
	case "/api/admin/config/api-key/enabled":
		enabled, valid := payload["enabled"].(bool)
		if !valid || enabled {
			writeAdminOpsError(writer, http.StatusConflict, "direct_api_key_reactivation_requires_rotation")
			return
		}
		item, err = handler.adminOps.SetCredentialEnabled(request.Context(), adminopsport.CredentialDirectAPIKey, clientID, false, "", actor, requestID)
	default:
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
		return
	}
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	status := http.StatusOK
	if path == "/api/admin/config/api-key/generate" {
		status = http.StatusCreated
	}
	writeJSON(writer, status, map[string]any{"ok": true, "api_key": publicCredential(item), "real_external_call_executed": false})
}

func (handler *Handler) adminOpsClients(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if path == "/api/admin/config/api-clients" && request.Method == http.MethodGet {
		items, err := handler.adminOps.ListCredentials(request.Context())
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		clients := make([]map[string]any, 0)
		for _, item := range items {
			if item.Kind == adminopsport.CredentialAPIClient {
				clients = append(clients, publicCredential(item))
			}
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "clients": clients})
		return
	}
	clientID := chi.URLParam(request, "client_id")
	if request.Method == http.MethodGet {
		item, err := handler.adminOps.GetCredential(request.Context(), adminopsport.CredentialAPIClient, clientID)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "client": publicCredential(item)})
		return
	}
	pattern := "/api/admin/config/api-clients"
	if clientID != "" {
		pattern += "/{client_id}"
		if strings.HasSuffix(path, "/activate") {
			pattern += "/activate"
		}
		if strings.HasSuffix(path, "/rotate-secret") {
			pattern += "/rotate-secret"
		}
		if strings.HasSuffix(path, "/enabled") {
			pattern += "/enabled"
		}
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, pattern)
	if !ok {
		return
	}
	if payload["confirm"] != true {
		writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
		return
	}
	var item adminopsport.Credential
	var err error
	switch {
	case path == "/api/admin/config/api-clients" && request.Method == http.MethodPost:
		clientID = textValue(payload, "client_id")
		metadata := objectValue(payload, "metadata")
		if len(metadata) == 0 {
			metadata = map[string]any{"client_type": textValue(payload, "client_type"), "token_ttl_minutes": payload["token_ttl_minutes"], "allowed_cidrs": payload["allowed_cidrs"]}
		}
		item, err = handler.adminOps.CreateCredential(request.Context(), adminopsapp.CredentialCommand{Kind: adminopsport.CredentialAPIClient, ClientID: clientID, DisplayName: textValue(payload, "display_name"), Metadata: metadata, Actor: actor, RequestID: requestID})
	case strings.HasSuffix(path, "/rotate-secret"):
		item, err = handler.adminOps.RotateCredential(request.Context(), adminopsport.CredentialAPIClient, clientID, actor, requestID)
	case strings.HasSuffix(path, "/activate"):
		if payload["copied_confirmed"] != true {
			writeAdminOpsError(writer, http.StatusBadRequest, "secret_reference_confirmation_required")
			return
		}
		item, err = handler.adminOps.SetCredentialEnabled(request.Context(), adminopsport.CredentialAPIClient, clientID, true, textValue(payload, "secret_ref"), actor, requestID)
	case strings.HasSuffix(path, "/enabled"):
		enabled, valid := payload["enabled"].(bool)
		if !valid || enabled {
			writeAdminOpsError(writer, http.StatusConflict, "activation_requires_secret_reference_self_check")
			return
		}
		item, err = handler.adminOps.SetCredentialEnabled(request.Context(), adminopsport.CredentialAPIClient, clientID, false, "", actor, requestID)
	case request.Method == http.MethodPut:
		current, getErr := handler.adminOps.GetCredential(request.Context(), adminopsport.CredentialAPIClient, clientID)
		if getErr != nil {
			writeAdminOpsServiceError(writer, getErr)
			return
		}
		if current.State == "active" {
			writeAdminOpsError(writer, http.StatusConflict, "active_client_requires_disable_before_update")
			return
		}
		metadata := objectValue(payload, "metadata")
		if len(metadata) == 0 {
			metadata = map[string]any{"token_ttl_minutes": payload["token_ttl_minutes"], "allowed_cidrs": payload["allowed_cidrs"]}
		}
		displayName := textValue(payload, "display_name")
		if displayName == "" {
			displayName = current.DisplayName
		}
		item, err = handler.adminOps.UpdateCredential(request.Context(), adminopsapp.CredentialCommand{Kind: adminopsport.CredentialAPIClient, ClientID: clientID, DisplayName: displayName, Metadata: metadata, Actor: actor, RequestID: requestID})
	default:
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
		return
	}
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	status := http.StatusOK
	if path == "/api/admin/config/api-clients" && request.Method == http.MethodPost {
		status = http.StatusCreated
		writer.Header().Set("Cache-Control", "no-store")
	}
	writeJSON(writer, status, map[string]any{"ok": true, "client": publicCredential(item), "real_external_call_executed": false})
}

func (handler *Handler) adminOpsCategories(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if path == "/api/admin/config/categories" {
		if request.Method != http.MethodGet {
			writeAdminOpsError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		items, err := handler.adminOps.ListCategories(request.Context())
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "categories": items})
		return
	}
	key := chi.URLParam(request, "category_key")
	if request.Method == http.MethodGet {
		item, err := handler.adminOps.GetCategory(request.Context(), key)
		if errors.Is(err, adminopsstore.ErrNotFound) {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "category": map[string]any{"key": key, "enabled": false, "settings": map[string]any{}}})
			return
		}
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "category": item})
		return
	}
	pattern := "/api/admin/config/categories/{category_key}"
	if strings.HasSuffix(path, "/enabled") {
		pattern += "/enabled"
	}
	if strings.HasSuffix(path, "/settings") {
		pattern += "/settings"
	}
	if strings.HasSuffix(path, "/check") {
		pattern += "/check"
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, pattern)
	if !ok {
		return
	}
	if strings.HasSuffix(path, "/check") {
		item, err := handler.adminOps.GetCategory(request.Context(), key)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "summary": map[string]any{"category": key, "failed": 0, "external_calls": false}, "config": item, "real_external_call_executed": false})
		return
	}
	current, getErr := handler.adminOps.GetCategory(request.Context(), key)
	enabled, settings := true, map[string]any{}
	if getErr == nil {
		enabled, settings = current.Enabled, decodeObject(current.Settings)
	} else if !errors.Is(getErr, adminopsstore.ErrNotFound) {
		writeAdminOpsServiceError(writer, getErr)
		return
	}
	if strings.HasSuffix(path, "/enabled") {
		value, valid := payload["enabled"].(bool)
		if !valid {
			writeAdminOpsError(writer, http.StatusBadRequest, "enabled_required")
			return
		}
		enabled = value
	}
	if strings.HasSuffix(path, "/settings") {
		settings, _ = payload["settings"].(map[string]any)
		if settings == nil {
			writeAdminOpsError(writer, http.StatusBadRequest, "settings_must_be_object")
			return
		}
	}
	item, err := handler.adminOps.SetCategory(request.Context(), key, enabled, settings, actor, requestID)
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "changed": true, "config": item, "real_external_call_executed": false})
}

func (handler *Handler) adminOpsReleases(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if path == "/api/admin/config/releases" && request.Method == http.MethodGet {
		items, err := handler.adminOps.ListReleases(request.Context(), 50)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "releases": items})
		return
	}
	if path == "/api/admin/config/releases" && request.Method == http.MethodPost {
		payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, "/api/admin/config/releases")
		if !ok {
			return
		}
		if payload["confirm"] != true {
			writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
			return
		}
		item, err := handler.adminOps.CreateRelease(request.Context(), adminopsapp.ReleaseCommand{Changes: objectValue(payload, "changes"), Actor: actor, RequestID: requestID})
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "release": item, "real_external_call_executed": false})
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(request, "release_id"), 10, 64)
	if err != nil || id < 1 {
		writeAdminOpsError(writer, http.StatusBadRequest, "invalid_release_id")
		return
	}
	if request.Method == http.MethodGet {
		item, itemErr := handler.adminOps.GetRelease(request.Context(), id)
		if itemErr != nil {
			writeAdminOpsServiceError(writer, itemErr)
			return
		}
		if strings.HasSuffix(path, "/shadow-compare") {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": item.State == "validated" || item.State == "published", "comparison": map[string]any{"release_id": id, "external_calls": false}})
		} else {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "release": item})
		}
		return
	}
	pattern := "/api/admin/config/releases/{release_id}"
	if strings.HasSuffix(path, "/validate") {
		pattern += "/validate"
	}
	if strings.HasSuffix(path, "/publish") {
		pattern += "/publish"
	}
	if strings.HasSuffix(path, "/rollback") {
		pattern += "/rollback"
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, pattern)
	if !ok {
		return
	}
	var item adminopsport.Release
	switch {
	case strings.HasSuffix(path, "/validate"):
		item, err = handler.adminOps.ValidateRelease(request.Context(), id, actor, requestID)
	case strings.HasSuffix(path, "/publish"):
		if payload["confirm"] != true {
			writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
			return
		}
		item, err = handler.adminOps.PublishRelease(request.Context(), id, textValue(payload, "checksum"), actor, requestID)
	case strings.HasSuffix(path, "/rollback"):
		if payload["confirm"] != true {
			writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
			return
		}
		item, err = handler.adminOps.RollbackRelease(request.Context(), id, actor, requestID)
	default:
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
		return
	}
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "release": item, "real_external_call_executed": false})
}

func (handler *Handler) adminOpsPush(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet {
		item, err := handler.adminOps.GetCategory(request.Context(), "push_capabilities")
		if errors.Is(err, adminopsstore.ErrNotFound) {
			writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "capabilities": []any{}, "scheduler": map[string]any{"enabled": false}})
			return
		}
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "capabilities": decodeObject(item.Settings), "scheduler": map[string]any{"enabled": item.Enabled}})
		return
	}
	pattern := "/api/admin/config/push-capabilities/{capability_key}"
	if strings.HasSuffix(request.URL.Path, "/scheduler") {
		pattern = "/api/admin/config/push-capabilities/scheduler"
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, pattern)
	if !ok {
		return
	}
	enabled, valid := payload["enabled"].(bool)
	if !valid {
		writeAdminOpsError(writer, http.StatusBadRequest, "enabled_required")
		return
	}
	key := "push_capabilities"
	if strings.HasSuffix(request.URL.Path, "/scheduler") {
		key = "push_scheduler"
	} else if value := chi.URLParam(request, "capability_key"); value != "" {
		key = "push_" + value
	}
	item, err := handler.adminOps.SetCategory(request.Context(), key, enabled, map[string]any{"enabled": enabled}, actor, requestID)
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "capability": item, "derived_gates": map[string]any{"api_worker_isolated": true}, "real_external_call_executed": false})
}

func (handler *Handler) adminOpsJobs(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if path == "/api/admin/jobs/order-identity-repair/run" {
		writeJSON(writer, http.StatusGone, map[string]any{"ok": false, "retired": true, "replacement": "current_order_customer_identity_projection", "real_external_call_executed": false})
		return
	}
	if path == "/api/admin/jobs/summary" && request.Method == http.MethodGet {
		items, err := handler.adminOps.ListJobs(request.Context(), "", "", 100)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		counts := map[string]int{}
		for _, item := range items {
			counts[item.State]++
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "summary": map[string]any{"counts": counts, "worker": "separate_from_http", "outcome_unknown_auto_retry": false}})
		return
	}
	if path == "/api/admin/broadcast-jobs/notification-settings/feishu" || path == "/api/admin/broadcast-jobs/notification-settings/feishu/validate" || path == "/api/admin/broadcast-jobs/feishu-hourly-report/run" {
		handler.adminOpsNotification(writer, request)
		return
	}
	if request.Method == http.MethodGet {
		kind := ""
		if strings.Contains(path, "archive-sync") {
			kind = "archive_sync"
		}
		if strings.Contains(path, "message-batches") {
			kind = "message_batch_ack"
		}
		if path == "/api/admin/broadcast-jobs" {
			kind = "feishu_hourly_report"
		}
		items, err := handler.adminOps.ListJobs(request.Context(), kind, "", 100)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "jobs": publicJobs(items), "real_external_call_executed": false})
		return
	}
	pattern := "/api/admin/jobs"
	if path == "/api/admin/jobs/archive-sync/run" {
		pattern = "/api/admin/jobs/archive-sync/run"
	}
	if strings.HasSuffix(path, "/ack") {
		pattern = "/api/admin/jobs/message-batches/{batch_id}/ack"
	}
	if strings.HasSuffix(path, "/cancel") {
		pattern = "/api/admin/broadcast-jobs/{job_id}/cancel"
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, pattern)
	if !ok {
		return
	}
	if payload["confirm"] != true {
		writeAdminOpsError(writer, http.StatusBadRequest, "operation_confirmation_required")
		return
	}
	kind, target := "", ""
	switch {
	case path == "/api/admin/jobs/archive-sync/run":
		kind, target = "archive_sync", "message_archive:sync"
	case strings.HasSuffix(path, "/ack"):
		batchID := chi.URLParam(request, "batch_id")
		if batchID == "" || textValue(payload, "ack_note") == "" {
			writeAdminOpsError(writer, http.StatusBadRequest, "manual_action_reason_required")
			return
		}
		kind, target = "message_batch_ack", "message_batch:"+batchID
	case strings.HasSuffix(path, "/cancel"):
		key := chi.URLParam(request, "job_id")
		version, err := numberValue(payload, "version")
		if err != nil {
			writeAdminOpsError(writer, http.StatusBadRequest, "version_required")
			return
		}
		item, err := handler.adminOps.CancelJob(request.Context(), key, version, actor, requestID)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "job": publicJob(item)})
		return
	default:
		writeAdminOpsError(writer, http.StatusConflict, "outbound_owner_required")
		return
	}
	item, err := handler.adminOps.EnqueueJob(request.Context(), adminopsapp.JobCommand{Kind: kind, TargetRef: target, Actor: actor, RequestID: requestID, Summary: map[string]any{"request_path": path, "provider_call": false}})
	if err != nil {
		writeAdminOpsServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]any{"ok": true, "accepted": true, "job": publicJob(item), "real_external_call_executed": false})
}

func (handler *Handler) adminOpsNotification(writer http.ResponseWriter, request *http.Request) {
	path := request.URL.Path
	if path == "/api/admin/broadcast-jobs/notification-settings/feishu" && request.Method == http.MethodGet {
		setting, err := handler.adminOps.GetFeishuNotification(request.Context())
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "enabled": setting.Enabled, "channel": setting.Channel, "webhookMasked": setting.SecretMask, "validationStatus": setting.ValidationState})
		return
	}
	payload, actor, requestID, ok := handler.adminOpsCommand(writer, request, path)
	if !ok {
		return
	}
	if path == "/api/admin/broadcast-jobs/notification-settings/feishu" {
		enabled, valid := payload["enabled"].(bool)
		if !valid {
			writeAdminOpsError(writer, http.StatusBadRequest, "enabled_required")
			return
		}
		setting, err := handler.adminOps.SaveFeishuNotification(request.Context(), enabled, textValue(payload, "secret_ref"), actor, requestID)
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"ok": true, "webhookMasked": setting.SecretMask, "validationStatus": setting.ValidationState, "real_external_call_executed": false})
		return
	}
	if path == "/api/admin/broadcast-jobs/notification-settings/feishu/validate" {
		setting, err := handler.adminOps.GetFeishuNotification(request.Context())
		if err != nil || setting.ValidationState == "unconfigured" {
			writeAdminOpsError(writer, http.StatusBadRequest, "notification_secret_reference_required")
			return
		}
		item, err := handler.adminOps.EnqueueJob(request.Context(), adminopsapp.JobCommand{Kind: "feishu_webhook_validate", TargetRef: setting.SecretRef, Actor: actor, RequestID: requestID, Summary: map[string]any{"channel": "feishu", "secret_ref": setting.SecretRef}})
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusAccepted, map[string]any{"ok": true, "accepted": true, "validationStatus": "queued", "webhookMasked": setting.SecretMask, "job": publicJob(item), "real_external_call_executed": false})
		return
	}
	if path == "/api/admin/broadcast-jobs/feishu-hourly-report/run" {
		item, err := handler.adminOps.EnqueueJob(request.Context(), adminopsapp.JobCommand{Kind: "feishu_hourly_report", TargetRef: "notification:feishu", Actor: actor, RequestID: requestID, Summary: map[string]any{"report": "hourly", "provider_call": false}})
		if err != nil {
			writeAdminOpsServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusAccepted, map[string]any{"ok": true, "status": "queued", "job": publicJob(item), "real_external_call_executed": false})
		return
	}
	writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_route_not_found")
}

func (handler *Handler) adminOpsCommand(writer http.ResponseWriter, request *http.Request, pattern string) (map[string]any, string, string, bool) {
	payload, ok := decodeAdminOpsPayload(writer, request)
	if !ok {
		return nil, "", "", false
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !sessionOK || !principalOK || principal.AdminUserID < 1 {
		writeAdminOpsError(writer, http.StatusUnauthorized, "authentication_required")
		return nil, "", "", false
	}
	token := strings.TrimSpace(request.Header.Get("X-Admin-Action-Token"))
	if token == "" {
		token = textValue(payload, "admin_action_token")
	}
	expected := adminOpsActionToken(session, request.Method, pattern)
	if len(token) != len(expected) || !hmac.Equal([]byte(token), []byte(expected)) {
		writeAdminOpsError(writer, http.StatusUnauthorized, "invalid_action_token")
		return nil, "", "", false
	}
	requestID := platformhttp.RequestID(request.Context())
	if requestID == "" {
		requestID = "legacy:" + request.Method + ":" + pattern
	}
	return payload, "admin:" + strconv.FormatInt(principal.AdminUserID, 10), requestID, true
}

func decodeAdminOpsPayload(writer http.ResponseWriter, request *http.Request) (map[string]any, bool) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil || payload == nil {
		writeAdminOpsError(writer, http.StatusBadRequest, "payload_must_be_object")
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeAdminOpsError(writer, http.StatusBadRequest, "payload_must_be_single_object")
		return nil, false
	}
	for key := range payload {
		lower := strings.ToLower(key)
		if lower == "client_secret" || lower == "api_key" || lower == "webhook_url" || lower == "password" || lower == "secret" {
			writeAdminOpsError(writer, http.StatusBadRequest, "secret_material_forbidden")
			return nil, false
		}
	}
	return payload, true
}

func publicCredential(item adminopsport.Credential) map[string]any {
	return map[string]any{"id": item.ID, "kind": item.Kind, "client_id": item.ClientID, "display_name": item.DisplayName, "state": item.State, "secret_ref": item.SecretRef, "secret_mask": item.SecretMask, "version": item.Version, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt}
}
func publicJob(item adminopsport.Job) map[string]any {
	return map[string]any{"job_id": item.Key, "kind": item.Kind, "status": item.State, "target_ref": item.TargetRef, "version": item.Version, "failure_code": item.FailureCode, "created_at": item.CreatedAt, "completed_at": item.CompletedAt, "real_external_call_executed": false}
}
func publicJobs(items []adminopsport.Job) []map[string]any {
	result := make([]map[string]any, len(items))
	for i, item := range items {
		result[i] = publicJob(item)
	}
	return result
}
func decodeObject(raw []byte) map[string]any {
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	if result == nil {
		result = map[string]any{}
	}
	return result
}
func textValue(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return strings.TrimSpace(value)
}
func objectValue(item map[string]any, key string) map[string]any {
	value, _ := item[key].(map[string]any)
	if value == nil {
		return map[string]any{}
	}
	return value
}
func numberValue(item map[string]any, key string) (int64, error) {
	value, ok := item[key].(json.Number)
	if !ok {
		return 0, adminopsapp.ErrInvalidCommand
	}
	number, err := value.Int64()
	if err != nil || number < 1 {
		return 0, adminopsapp.ErrInvalidCommand
	}
	return number, nil
}
func writeAdminOpsError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]any{"ok": false, "error": code})
}
func writeAdminOpsServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, adminopsstore.ErrNotFound):
		writeAdminOpsError(writer, http.StatusNotFound, "admin_ops_not_found")
	case errors.Is(err, adminopsapp.ErrInvalidCommand), errors.Is(err, adminopsapp.ErrSecretMaterial):
		writeAdminOpsError(writer, http.StatusBadRequest, "invalid_admin_ops_request")
	case errors.Is(err, adminopsapp.ErrConflict), errors.Is(err, adminopsapp.ErrVersionConflict), errors.Is(err, adminopsapp.ErrInvalidTransition):
		writeAdminOpsError(writer, http.StatusConflict, "admin_ops_conflict")
	default:
		writeAdminOpsError(writer, http.StatusServiceUnavailable, "admin_ops_unavailable")
	}
}

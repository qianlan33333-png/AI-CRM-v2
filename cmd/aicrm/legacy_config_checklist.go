package main

import (
	"html/template"
	"net/http"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const legacyConfigChecklistPath = "/admin/config/checklist"

var configChecklistTemplate = template.Must(template.New("config-checklist").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>配置检查清单</title><main><h1>配置检查清单</h1><p>仅显示 V2 本地配置登记状态，不验证外部服务</p><nav><a href="/admin/config/app-settings">管理 V2 应用设置</a><a href="/admin/runtime-config">查看本地运行声明</a></nav><section><h2>可直接编辑</h2><table><thead><tr><th>配置项</th><th>状态</th></tr></thead><tbody>{{range .Editable}}<tr><td>{{.Key}}</td><td>{{.Status}}</td></tr>{{end}}</tbody></table></section><section><h2>敏感信息</h2><table><thead><tr><th>配置项</th><th>状态</th></tr></thead><tbody>{{range .Masked}}<tr><td>{{.Key}}</td><td>{{.Status}}</td></tr>{{end}}</tbody></table></section></main></html>`))

var configChecklistEditableKeys = []configport.Key{
	configport.WeComCorpID,
	configport.WeComAgentID,
	configport.OutboundRatePerSecond,
	configport.OutboundMaxAttempts,
}

var configChecklistMaskedKeys = []configport.Key{
	configport.DatabaseURL,
	configport.WeComSecret,
	configport.WeComCallbackToken,
	configport.WeComCallbackAESKey,
	configport.AIAPIKey,
	configport.AuthJWTSecret,
	configport.ExtensionAPIKeyPepper,
	configport.WebhookMasterKey,
}

type configChecklistItem struct{ Key, Status string }

type configChecklistPageData struct {
	Editable []configChecklistItem
	Masked   []configChecklistItem
}

func (handler *Handler) ConfigChecklist(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil {
		writeConfigChecklistUnavailable(writer)
		return
	}
	if !configChecklistAuthorized(request) {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthorized, authport.ErrUnauthorized))
		return
	}
	if nilLegacyDependency(handler.settings) {
		writeConfigChecklistUnavailable(writer)
		return
	}
	projection, err := handler.settings.List(request.Context(), configapp.SettingsListInput{})
	if err != nil {
		writeConfigChecklistUnavailable(writer)
		return
	}
	data, ok := configChecklistPageDataFromProjection(projection)
	if !ok {
		writeConfigChecklistUnavailable(writer)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_ = configChecklistTemplate.Execute(writer, data)
}

func configChecklistPageDataFromProjection(projection configapp.SettingsProjection) (configChecklistPageData, bool) {
	if len(projection.Rows) != len(configChecklistEditableKeys)+len(configChecklistMaskedKeys) {
		return configChecklistPageData{}, false
	}
	data := configChecklistPageData{
		Editable: make([]configChecklistItem, 0, len(configChecklistEditableKeys)),
		Masked:   make([]configChecklistItem, 0, len(configChecklistMaskedKeys)),
	}
	for index, key := range configChecklistEditableKeys {
		row, ok := projection.Rows[index].(configapp.EditableSettingRow)
		if !ok || row.Key != key || row.Mode != "editable" {
			return configChecklistPageData{}, false
		}
		data.Editable = append(data.Editable, configChecklistItem{Key: string(row.Key), Status: configuredStatus(row.Configured)})
	}
	for index, key := range configChecklistMaskedKeys {
		row, ok := projection.Rows[len(configChecklistEditableKeys)+index].(configapp.MaskedSettingRow)
		if !ok || row.Key != key || row.Mode != "masked" || !row.Masked {
			return configChecklistPageData{}, false
		}
		data.Masked = append(data.Masked, configChecklistItem{Key: string(row.Key), Status: configuredStatus(row.Configured)})
	}
	return data, true
}

func configuredStatus(configured bool) string {
	if configured {
		return "configured"
	}
	return "missing"
}

func configChecklistAuthorized(request *http.Request) bool {
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	return principalOK && principal.AdminUserID > 0 && principal.Role == authport.RoleAdmin && authorizationOK &&
		authorization.Capability == authport.CapabilityConfigOverviewRead && authorization.Scope == authport.ScopeGlobal && authorization.OwnerStaffID == 0
}

func writeConfigChecklistUnavailable(writer http.ResponseWriter) {
	platformhttp.MarkCompatibilityError(writer, platformhttp.CodeDependencyUnavailable)
	writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "config_checklist_unavailable"})
}

func writeLegacyConfigChecklistMethodNotAllowed(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Allow", http.MethodGet)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusMethodNotAllowed)
}

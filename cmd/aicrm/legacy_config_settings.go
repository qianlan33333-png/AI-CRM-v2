package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

const (
	appSettingsPath          = "/admin/config/app-settings"
	appSettingsSavePath      = "/admin/config/app-settings/save"
	appSettingsActionMessage = "v1\nPOST\n/admin/config/app-settings/save"
)

const appSettingsTemplateSource = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>系统设置</title></head>
<body><main><h1>系统设置</h1><p>集中查看和修改系统级参数；敏感信息仅显示掩码。</p>
{{if .Saved}}<p role="status">保存成功</p>{{end}}{{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
<form method="get" action="/admin/config/app-settings"><label>筛选 <input type="text" name="q" value="{{.Search}}"></label>
<label>查看范围 <xselect name="scope"><option value="">全部</option><option value="editable"{{if eq .Scope "editable"}} selected{{end}}>可直接编辑</option><option value="masked"{{if eq .Scope "masked"}} selected{{end}}>敏感信息</option></xselect></label><button type="submit">应用筛选</button></form>
<section><h2>设置快照</h2><table><thead><tr><th>设置项</th><th>名称</th><th>类型</th><th>当前值</th><th>来源</th><th>最近修改人</th></tr></thead><tbody>{{range .Projection.Rows}}<tr><td><code>{{.Key}}</code></td><td>{{.Label}}</td><td>{{.Mode}}</td><td>{{if eq .Mode "masked"}}已掩码{{else}}{{.DisplayValue}}{{end}}</td><td>{{if eq .Mode "editable"}}{{.Source}}{{end}}</td><td>{{if eq .Mode "editable"}}{{.LastModifiedBy}}{{end}}</td></tr>{{end}}</tbody></table></section>
<section><h2>编辑系统设置</h2><form method="post" action="/admin/config/app-settings/save"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><input type="hidden" name="admin_action_token" value="{{.ActionToken}}">
{{range .Projection.Rows}}{{if eq .Mode "editable"}}<label>{{.Label}} <code>{{.Key}}</code><input type="{{.InputType}}" name="setting__{{.Key}}" value="{{.Value}}"></label>{{else}}<label>{{.Label}} <code>{{.Key}}</code><input type="password" name="setting__{{.Key}}" value="" placeholder="留空保持原值"></label>{{end}}{{end}}
<input type="hidden" name="operator" value=""><label><input type="checkbox" name="confirm" value="1">我确认保存本次系统设置修改</label><button type="submit">保存系统设置</button></form></section>
<section><h2>最近审计</h2>{{range .Projection.AuditEntries}}<p><strong>{{.TargetID}}</strong> {{.ActionType}} · {{.Operator}} · {{.CreatedAt}}</p>{{end}}</section></main></body></html>`

var appSettingsTemplate = template.Must(template.New("app-settings").Parse(strings.ReplaceAll(appSettingsTemplateSource, "xselect", "select")))

type appSettingsPageData struct {
	Projection                                   configapp.SettingsProjection
	Search, Scope, Error, ActionToken, CSRFToken string
	Saved                                        bool
}

func settingsActionToken(session authport.SessionRef) string {
	mac := hmac.New(sha256.New, []byte(session))
	_, _ = mac.Write([]byte(appSettingsActionMessage))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validSettingsActionToken(session authport.SessionRef, token string) bool {
	want := settingsActionToken(session)
	return len(token) == len(want) && hmac.Equal([]byte(token), []byte(want))
}

func (handler *Handler) AppSettingsPage(writer http.ResponseWriter, request *http.Request) {
	projection, ok := handler.appSettingsProjection(writer, request)
	if !ok {
		return
	}
	session, present := authport.SessionFromContext(request.Context())
	if !present {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeUnauthenticated, authport.ErrUnauthenticated))
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	data := appSettingsPageData{Projection: projection, Search: request.URL.Query().Get("q"), Scope: request.URL.Query().Get("scope"), Error: request.URL.Query().Get("error"), Saved: request.URL.Query().Get("saved") == "1", ActionToken: settingsActionToken(session), CSRFToken: settingsPageCSRF(request)}
	if err := appSettingsTemplate.Execute(writer, data); err != nil {
		return
	}
}

func settingsPageCSRF(request *http.Request) string {
	for _, name := range []string{"aicrm_csrf", LegacyCSRFCookieName} {
		cookies := namedCookies(request, name)
		if len(cookies) == 1 && validToken(cookies[0].Value) {
			return cookies[0].Value
		}
	}
	return ""
}

func (handler *Handler) AppSettingsResource(writer http.ResponseWriter, request *http.Request) {
	projection, ok := handler.appSettingsProjection(writer, request)
	if !ok {
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	response := map[string]any{"ok": true, "config": projection, "source_status": "next_read_model", "fallback_used": false}
	if session, present := authport.SessionFromContext(request.Context()); present {
		response["admin_action_token"] = appSettingsResourceActionToken(session)
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) appSettingsProjection(writer http.ResponseWriter, request *http.Request) (configapp.SettingsProjection, bool) {
	if handler == nil || nilLegacyDependency(handler.settings) || request == nil || request.URL == nil {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeDependencyUnavailable, authport.ErrAuthenticationUnavailable))
		return configapp.SettingsProjection{}, false
	}
	query := request.URL.Query()
	if len(query["q"]) > 1 || len(query["scope"]) > 1 {
		platformhttp.WriteError(writer, request, platformhttp.NewError(platformhttp.CodeMalformedRequest, configapp.ErrInvalidAppSettingsRequest))
		return configapp.SettingsProjection{}, false
	}
	projection, err := handler.settings.List(request.Context(), configapp.SettingsListInput{Search: query.Get("q"), Scope: query.Get("scope")})
	if err != nil {
		code := platformhttp.CodeDependencyUnavailable
		if errors.Is(err, configapp.ErrInvalidAppSettingsRequest) {
			code = platformhttp.CodeMalformedRequest
		}
		platformhttp.WriteError(writer, request, platformhttp.NewError(code, err))
		return configapp.SettingsProjection{}, false
	}
	return projection, true
}

func (handler *Handler) SaveAppSettings(writer http.ResponseWriter, request *http.Request) {
	code := "save_failed"
	if handler == nil || nilLegacyDependency(handler.settings) || request == nil {
		redirectSettingsError(writer, request, code)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	if err := request.ParseForm(); err != nil {
		redirectSettingsError(writer, request, "invalid_request")
		return
	}
	session, sessionOK := authport.SessionFromContext(request.Context())
	csrfTokens := request.PostForm["csrf_token"]
	if !sessionOK || len(csrfTokens) != 1 || handler.auth == nil || handler.auth.ValidateCSRF(request.Context(), session, authport.CSRFToken(csrfTokens[0])) != nil {
		redirectSettingsError(writer, request, "invalid_csrf_token")
		return
	}
	if request.PostForm.Get("confirm") != "1" || len(request.PostForm["confirm"]) != 1 {
		redirectSettingsError(writer, request, "confirmation_required")
		return
	}
	tokens := request.PostForm["admin_action_token"]
	if !sessionOK || len(tokens) != 1 || !validSettingsActionToken(session, tokens[0]) {
		redirectSettingsError(writer, request, "invalid_action_token")
		return
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 {
		redirectSettingsError(writer, request, code)
		return
	}
	values := make(map[string][]string)
	for key, item := range request.PostForm {
		switch key {
		case "csrf_token", "confirm", "admin_action_token", "operator":
			continue
		}
		if !strings.HasPrefix(key, "setting__") || len(key) == len("setting__") {
			redirectSettingsError(writer, request, "invalid_setting")
			return
		}
		values[strings.TrimPrefix(key, "setting__")] = item
	}
	err := handler.settings.Save(request.Context(), configapp.SaveSettingsInput{Values: values, Actor: "admin:" + strconvFormatInt(principal.AdminUserID), RequestID: platformhttp.RequestID(request.Context())})
	if err != nil {
		switch {
		case errors.Is(err, configport.ErrSecretSetting):
			code = "secret_input_forbidden"
		case errors.Is(err, configapp.ErrInvalidAppSettingsRequest), errors.Is(err, configport.ErrUnknownSetting), errors.Is(err, configport.ErrInvalidSetting):
			code = "invalid_setting"
		}
		redirectSettingsError(writer, request, code)
		return
	}
	http.Redirect(writer, request, appSettingsPath+"?saved=1", http.StatusFound)
}

func redirectSettingsError(writer http.ResponseWriter, request *http.Request, code string) {
	http.Redirect(writer, request, appSettingsPath+"?error="+url.QueryEscape(code), http.StatusFound)
}

func strconvFormatInt(value int64) string {
	if value < 1 {
		return ""
	}
	const digits = "0123456789"
	var buf [20]byte
	pos := len(buf)
	for value > 0 {
		pos--
		buf[pos] = digits[value%10]
		value /= 10
	}
	return string(buf[pos:])
}

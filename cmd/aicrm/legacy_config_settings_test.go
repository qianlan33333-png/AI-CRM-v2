package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

type legacySettingsStub struct {
	projection configapp.SettingsProjection
	saves      []configapp.SaveSettingsInput
	err        error
}

func (stub *legacySettingsStub) List(context.Context, configapp.SettingsListInput) (configapp.SettingsProjection, error) {
	return stub.projection, stub.err
}
func (stub *legacySettingsStub) Save(_ context.Context, input configapp.SaveSettingsInput) error {
	stub.saves = append(stub.saves, input)
	return stub.err
}

func settingsRequestContext(request *http.Request, adminID int64, session authport.SessionRef) *http.Request {
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: adminID, Role: authport.RoleAdmin}, session)
	return request.WithContext(ctx)
}

func TestSettingsActionTokenIsExactAndRouteBound(t *testing.T) {
	session := authport.SessionRef("session-material-never-log")
	token := settingsActionToken(session)
	if token != "kXVuVFrxkjdyJpxeXnvXxJwAL4xdkRyLXGWu-ETa52M" {
		t.Fatalf("token=%q", token)
	}
	if !validSettingsActionToken(session, token) || validSettingsActionToken("different", token) || validSettingsActionToken(session, token+"x") {
		t.Fatal("action token validation drifted")
	}
}

func TestAppSettingsPageAndResourceUseFrozenTransports(t *testing.T) {
	stub := &legacySettingsStub{projection: configapp.SettingsProjection{
		Rows:        []any{},
		MetadataMap: map[configport.Key]configapp.SettingMetadata{},
		SummaryCards: []configapp.SummaryCard{
			{Label: "可直接编辑", Value: 0, Description: "可以直接修改的设置项"},
			{Label: "敏感信息", Value: 0, Description: "只显示掩码的设置项"},
			{Label: "已配置", Value: 0, Description: "当前已经配置完成的设置项"},
		},
		AuditEntries: []configapp.AuditEntry{},
	}}
	handler := &Handler{settings: stub, auth: &legacyAuthStub{}}
	page := httptest.NewRecorder()
	session := authport.SessionRef(legacyToken(8))
	request := settingsRequestContext(httptest.NewRequest(http.MethodGet, appSettingsPath+"?saved=1", nil), 7, session)
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: string(session)})
	request.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: legacyToken(9)})
	handler.AppSettingsPage(page, request)
	if page.Code != http.StatusOK || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") || !strings.Contains(page.Body.String(), `name="csrf_token"`) || !strings.Contains(page.Body.String(), `name="admin_action_token"`) || !strings.Contains(page.Body.String(), "保存成功") {
		t.Fatalf("page=%d %s", page.Code, page.Body.String())
	}
	resource := httptest.NewRecorder()
	handler.AppSettingsResource(resource, httptest.NewRequest(http.MethodGet, "/api/admin/config/app-settings", nil))
	if resource.Header().Get("Cache-Control") != "no-store" || resource.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("resource headers=%v", resource.Header())
	}
	for _, literal := range []string{`"ok":true`, `"source_status":"next_read_model"`, `"fallback_used":false`, `"rows":[]`, `"metadata_map":{}`, `"audit_entries":[]`} {
		if !strings.Contains(resource.Body.String(), literal) {
			t.Fatalf("resource missing %s: %s", literal, resource.Body.String())
		}
	}
}

func TestSaveAppSettingsRequiresActionTokenAndIgnoresOperator(t *testing.T) {
	stub := &legacySettingsStub{}
	handler := &Handler{settings: stub, auth: &legacyAuthStub{}}
	session := authport.SessionRef("session-material-never-log")
	form := url.Values{"csrf_token": {legacyToken(2)}, "admin_action_token": {settingsActionToken(session)}, "confirm": {"1"}, "operator": {"attacker"}, "setting__wecom.corp_id": {"corp"}, "setting__database.url": {""}}
	request := httptest.NewRequest(http.MethodPost, appSettingsSavePath, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.SaveAppSettings(response, settingsRequestContext(request, 7, session))
	if response.Code != http.StatusFound || response.Header().Get("Location") != appSettingsPath+"?saved=1" || len(stub.saves) != 1 {
		t.Fatalf("response=%d location=%q saves=%#v", response.Code, response.Header().Get("Location"), stub.saves)
	}
	if stub.saves[0].Actor != "admin:7" {
		t.Fatalf("actor accepted form operator: %#v", stub.saves[0])
	}
	bad := url.Values{"csrf_token": {legacyToken(2)}, "admin_action_token": {settingsActionToken("other")}, "confirm": {"1"}, "setting__wecom.corp_id": {"corp"}}
	request = httptest.NewRequest(http.MethodPost, appSettingsSavePath, strings.NewReader(bad.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response = httptest.NewRecorder()
	handler.SaveAppSettings(response, settingsRequestContext(request, 7, session))
	if response.Code != http.StatusFound || response.Header().Get("Location") != appSettingsPath+"?error=invalid_action_token" || len(stub.saves) != 1 {
		t.Fatalf("invalid token response=%d location=%q saves=%d", response.Code, response.Header().Get("Location"), len(stub.saves))
	}
}

func TestSaveAppSettingsFiniteRedirectErrors(t *testing.T) {
	handler := &Handler{settings: &legacySettingsStub{}, auth: &legacyAuthStub{}}
	session := authport.SessionRef("session-material-never-log")
	tests := []struct {
		name string
		form url.Values
		want string
	}{
		{"csrf", url.Values{"admin_action_token": {settingsActionToken(session)}, "confirm": {"1"}}, "invalid_csrf_token"},
		{"confirm", url.Values{"csrf_token": {legacyToken(2)}, "admin_action_token": {settingsActionToken(session)}}, "confirmation_required"},
		{"unknown form field", url.Values{"csrf_token": {legacyToken(2)}, "admin_action_token": {settingsActionToken(session)}, "confirm": {"1"}, "value": {"leak-me"}}, "invalid_setting"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, appSettingsSavePath, strings.NewReader(test.form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			handler.SaveAppSettings(response, settingsRequestContext(request, 7, session))
			if response.Code != http.StatusFound || response.Header().Get("Location") != appSettingsPath+"?error="+test.want || strings.Contains(response.Header().Get("Location"), "leak-me") {
				t.Fatalf("response=%d location=%q", response.Code, response.Header().Get("Location"))
			}
		})
	}
}

func TestAppSettingsRouteRequiresAdminCapabilityAndBothCSRFGuards(t *testing.T) {
	session, csrf := legacyToken(4), legacyToken(5)
	form := url.Values{"csrf_token": {csrf}, "admin_action_token": {settingsActionToken(authport.SessionRef(session))}, "confirm": {"1"}, "setting__wecom.corp_id": {"corp"}}
	build := func(role authport.Role) (*legacySettingsStub, http.Handler) {
		stub := &legacySettingsStub{}
		handler := &Handler{settings: stub, auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: role}}}
		tail, err := handler.Authorize(authport.CapabilityConfigSettingsManage, http.HandlerFunc(handler.SaveAppSettings))
		if err != nil {
			t.Fatal(err)
		}
		tail, err = handler.RequireCSRF(tail)
		if err != nil {
			t.Fatal(err)
		}
		return stub, handler.Authenticate(tail)
	}
	request := func(values url.Values) *http.Request {
		r := httptest.NewRequest(http.MethodPost, appSettingsSavePath, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: session})
		r.AddCookie(&http.Cookie{Name: LegacyCSRFCookieName, Value: csrf})
		return r
	}
	stub, route := build(authport.RoleAdmin)
	response := httptest.NewRecorder()
	route.ServeHTTP(response, request(form))
	if response.Code != http.StatusFound || response.Header().Get("Location") != appSettingsPath+"?saved=1" || len(stub.saves) != 1 {
		t.Fatalf("admin=%d %q saves=%d", response.Code, response.Header().Get("Location"), len(stub.saves))
	}
	withoutFormCSRF := url.Values{"admin_action_token": {settingsActionToken(authport.SessionRef(session))}, "confirm": {"1"}}
	response = httptest.NewRecorder()
	route.ServeHTTP(response, request(withoutFormCSRF))
	if response.Code != http.StatusFound || response.Header().Get("Location") != appSettingsPath+"?error=invalid_csrf_token" || len(stub.saves) != 1 {
		t.Fatalf("form csrf=%d %q saves=%d", response.Code, response.Header().Get("Location"), len(stub.saves))
	}
	deniedStub, denied := build(authport.RoleOps)
	response = httptest.NewRecorder()
	denied.ServeHTTP(response, request(form))
	if response.Code != http.StatusForbidden || len(deniedStub.saves) != 0 {
		t.Fatalf("ops=%d saves=%d body=%s", response.Code, len(deniedStub.saves), response.Body.String())
	}
}

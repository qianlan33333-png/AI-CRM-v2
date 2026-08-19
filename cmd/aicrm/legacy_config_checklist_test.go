package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type configChecklistAuthSpy struct {
	principal           authport.Principal
	authenticationError error
	authorization       authport.Authorization
	authorizationError  error
	authenticateCalls   int
	authorizeCalls      int
	csrfCalls           int
}

func (spy *configChecklistAuthSpy) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	spy.authenticateCalls++
	if spy.authenticationError != nil {
		return authport.Principal{}, spy.authenticationError
	}
	return spy.principal, nil
}

func (spy *configChecklistAuthSpy) Authorize(_ context.Context, _ authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	spy.authorizeCalls++
	if spy.authorizationError != nil {
		return authport.Authorization{}, spy.authorizationError
	}
	if spy.authorization.Capability == "" {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	return spy.authorization, nil
}

func (spy *configChecklistAuthSpy) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	spy.csrfCalls++
	return nil
}

func (*configChecklistAuthSpy) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type configChecklistSettingsSpy struct {
	projection configapp.SettingsProjection
	err        error
	listInputs []configapp.SettingsListInput
	saveCalls  int
}

func (spy *configChecklistSettingsSpy) List(_ context.Context, input configapp.SettingsListInput) (configapp.SettingsProjection, error) {
	spy.listInputs = append(spy.listInputs, input)
	return spy.projection, spy.err
}

func (spy *configChecklistSettingsSpy) Save(context.Context, configapp.SaveSettingsInput) error {
	spy.saveCalls++
	return errors.New("save must not be called")
}

func configChecklistRouter(t *testing.T, service authport.Service, settings legacySettingsApplication) http.Handler {
	t.Helper()
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, &Handler{auth: service, settings: settings},
	)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func configChecklistRequest(method string, session bool) *http.Request {
	request := httptest.NewRequest(method, legacyConfigChecklistPath, nil)
	if session {
		request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(61)})
	}
	return request
}

func validConfigChecklistProjection(configured map[configport.Key]bool) configapp.SettingsProjection {
	rows := make([]any, 0, len(configChecklistEditableKeys)+len(configChecklistMaskedKeys))
	for _, key := range configChecklistEditableKeys {
		rows = append(rows, configapp.EditableSettingRow{
			SettingMetadata: configapp.SettingMetadata{Key: key, Label: "editable label", Mode: "editable", InputType: "text", Description: "editable description"},
			Value:           "raw-editable-value-sentinel", DisplayValue: "display-value-sentinel", Configured: configured[key], Source: "source-sentinel", Version: "version-sentinel",
			UpdatedAt: "updated-sentinel", LastModifiedAt: "audit-time-sentinel", LastModifiedBy: "audit-operator-sentinel", LastActionType: "audit-action-sentinel",
		})
	}
	for _, key := range configChecklistMaskedKeys {
		rows = append(rows, configapp.MaskedSettingRow{
			SettingMetadata: configapp.SettingMetadata{Key: key, Label: "masked label", Mode: "masked", InputType: "password", Description: "masked description"},
			Configured:      configured[key], Masked: true,
		})
	}
	return configapp.SettingsProjection{
		Rows:         rows,
		SummaryCards: []configapp.SummaryCard{{Label: "summary-sentinel", Value: 99, Description: "summary-description-sentinel"}},
		AuditEntries: []configapp.AuditEntry{{Operator: "audit-operator-sentinel", ActionType: "audit-action-sentinel", CreatedAt: "audit-time-sentinel"}},
	}
}

func TestConfigChecklistRendersOnlyFrozenRowsAndStatus(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		configured map[configport.Key]bool
	}{
		{name: "all missing", configured: map[configport.Key]bool{}},
		{name: "mixed", configured: map[configport.Key]bool{configport.WeComCorpID: true, configport.OutboundMaxAttempts: true, configport.DatabaseURL: true, configport.AuthJWTSecret: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			settings := &configChecklistSettingsSpy{projection: validConfigChecklistProjection(testCase.configured)}
			admin := authport.Principal{AdminUserID: 61, Role: authport.RoleAdmin}
			service := &configChecklistAuthSpy{principal: admin}
			response := httptest.NewRecorder()
			configChecklistRouter(t, service, settings).ServeHTTP(response, configChecklistRequest(http.MethodGet, true))
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("status/headers=%d/%v", response.Code, response.Header())
			}
			if len(settings.listInputs) != 1 || settings.listInputs[0] != (configapp.SettingsListInput{}) || settings.saveCalls != 0 || service.csrfCalls != 0 {
				t.Fatalf("list/save/csrf=%#v/%d/%d", settings.listInputs, settings.saveCalls, service.csrfCalls)
			}
			body := response.Body.String()
			if strings.Count(body, "<tr>") != 14 || strings.Count(body, "<a href=") != 2 {
				t.Fatalf("row/link count body=%s", body)
			}
			for _, required := range []string{
				"<title>配置检查清单</title>", "仅显示 V2 本地配置登记状态，不验证外部服务", "<h2>可直接编辑</h2>", "<h2>敏感信息</h2>",
				"/admin/config/app-settings", "管理 V2 应用设置", "/admin/runtime-config", "查看本地运行声明",
			} {
				if !strings.Contains(body, required) {
					t.Fatalf("missing %q in %s", required, body)
				}
			}
			last := -1
			for _, key := range append(append([]configport.Key(nil), configChecklistEditableKeys...), configChecklistMaskedKeys...) {
				status := "missing"
				if testCase.configured[key] {
					status = "configured"
				}
				value := string(key) + "</td><td>" + status
				index := strings.Index(body, value)
				if index <= last {
					t.Fatalf("missing or unordered row %q in %s", value, body)
				}
				last = index
			}
			for _, forbidden := range []string{
				"raw-editable-value-sentinel", "display-value-sentinel", "source-sentinel", "version-sentinel", "updated-sentinel",
				"audit-time-sentinel", "audit-operator-sentinel", "audit-action-sentinel", "summary-sentinel", "summary-description-sentinel",
			} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("forbidden projection material %q in %s", forbidden, body)
				}
			}
		})
	}
}

func TestConfigChecklistFailsClosedForUnavailableOrInvalidProjection(t *testing.T) {
	valid := validConfigChecklistProjection(map[configport.Key]bool{configport.WeComCorpID: true})
	short := valid
	short.Rows = append([]any(nil), valid.Rows[:len(valid.Rows)-1]...)
	long := valid
	long.Rows = append(append([]any(nil), valid.Rows...), struct{}{})
	outOfOrder := valid
	outOfOrder.Rows = append([]any(nil), valid.Rows...)
	outOfOrder.Rows[0], outOfOrder.Rows[1] = outOfOrder.Rows[1], outOfOrder.Rows[0]
	duplicate := valid
	duplicate.Rows = append([]any(nil), valid.Rows...)
	duplicate.Rows[1] = duplicate.Rows[0]
	wrongType := valid
	wrongType.Rows = append([]any(nil), valid.Rows...)
	wrongType.Rows[0] = configapp.MaskedSettingRow{SettingMetadata: configapp.SettingMetadata{Key: configChecklistEditableKeys[0], Mode: "masked"}, Masked: true}
	wrongMode := valid
	wrongMode.Rows = append([]any(nil), valid.Rows...)
	wrongMode.Rows[0] = configapp.EditableSettingRow{SettingMetadata: configapp.SettingMetadata{Key: configChecklistEditableKeys[0], Mode: "masked"}}
	maskedFalse := valid
	maskedFalse.Rows = append([]any(nil), valid.Rows...)
	maskedFalse.Rows[len(configChecklistEditableKeys)] = configapp.MaskedSettingRow{SettingMetadata: configapp.SettingMetadata{Key: configChecklistMaskedKeys[0], Mode: "masked"}, Masked: false}

	for _, testCase := range []struct {
		name     string
		settings legacySettingsApplication
	}{
		{name: "nil source"},
		{name: "list error", settings: &configChecklistSettingsSpy{err: errors.New("list-error-sentinel")}},
		{name: "short", settings: &configChecklistSettingsSpy{projection: short}},
		{name: "long", settings: &configChecklistSettingsSpy{projection: long}},
		{name: "out of order", settings: &configChecklistSettingsSpy{projection: outOfOrder}},
		{name: "duplicate", settings: &configChecklistSettingsSpy{projection: duplicate}},
		{name: "wrong type", settings: &configChecklistSettingsSpy{projection: wrongType}},
		{name: "wrong mode", settings: &configChecklistSettingsSpy{projection: wrongMode}},
		{name: "masked false", settings: &configChecklistSettingsSpy{projection: maskedFalse}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			admin := authport.Principal{AdminUserID: 61, Role: authport.RoleAdmin}
			response := httptest.NewRecorder()
			configChecklistRouter(t, &configChecklistAuthSpy{principal: admin}, testCase.settings).ServeHTTP(response, configChecklistRequest(http.MethodGet, true))
			if response.Code != http.StatusServiceUnavailable || response.Body.String() != "{\"error\":\"config_checklist_unavailable\",\"ok\":false}\n" || strings.Contains(response.Body.String(), "<html") || strings.Contains(response.Body.String(), "list-error-sentinel") {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
			if spy, ok := testCase.settings.(*configChecklistSettingsSpy); ok && (len(spy.listInputs) != 1 || spy.saveCalls != 0) {
				t.Fatalf("list/save=%#v/%d", spy.listInputs, spy.saveCalls)
			}
		})
	}
}

func TestConfigChecklistRouteEnforcesFrozenHTTPAndAuthContract(t *testing.T) {
	admin := authport.Principal{AdminUserID: 61, Role: authport.RoleAdmin}
	validSettings := func() *configChecklistSettingsSpy {
		return &configChecklistSettingsSpy{projection: validConfigChecklistProjection(map[configport.Key]bool{})}
	}

	t.Run("global admin GET without CSRF", func(t *testing.T) {
		service, settings := &configChecklistAuthSpy{principal: admin}, validSettings()
		response := httptest.NewRecorder()
		configChecklistRouter(t, service, settings).ServeHTTP(response, configChecklistRequest(http.MethodGet, true))
		if response.Code != http.StatusOK || service.authenticateCalls != 1 || service.authorizeCalls != 1 || service.csrfCalls != 0 || len(settings.listInputs) != 1 {
			t.Fatalf("status/calls=%d/%d/%d/%d/%d", response.Code, service.authenticateCalls, service.authorizeCalls, service.csrfCalls, len(settings.listInputs))
		}
	})

	t.Run("anonymous is unauthorized", func(t *testing.T) {
		service, settings := &configChecklistAuthSpy{principal: admin}, validSettings()
		response := httptest.NewRecorder()
		configChecklistRouter(t, service, settings).ServeHTTP(response, configChecklistRequest(http.MethodGet, false))
		assertLegacyError(t, response, http.StatusUnauthorized, platformhttp.CodeUnauthenticated)
		if service.authenticateCalls != 0 || service.authorizeCalls != 0 || len(settings.listInputs) != 0 {
			t.Fatalf("auth/list=%d/%d/%d", service.authenticateCalls, service.authorizeCalls, len(settings.listInputs))
		}
	})

	for _, testCase := range []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		authorizeErr  error
	}{
		{name: "wrong capability", principal: admin, authorization: authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}},
		{name: "wrong scope", principal: admin, authorization: authport.Authorization{Capability: authport.CapabilityConfigOverviewRead, Scope: authport.ScopeSelf}},
		{name: "sales", principal: authport.Principal{AdminUserID: 62, Role: authport.RoleSales}, authorizeErr: authport.ErrUnauthorized},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service, settings := &configChecklistAuthSpy{principal: testCase.principal, authorization: testCase.authorization, authorizationError: testCase.authorizeErr}, validSettings()
			response := httptest.NewRecorder()
			configChecklistRouter(t, service, settings).ServeHTTP(response, configChecklistRequest(http.MethodGet, true))
			assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
			if len(settings.listInputs) != 0 || settings.saveCalls != 0 {
				t.Fatalf("list/save=%d/%d", len(settings.listInputs), settings.saveCalls)
			}
		})
	}

	t.Run("non GET is rejected before auth", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions} {
			service, settings := &configChecklistAuthSpy{principal: admin}, validSettings()
			response := httptest.NewRecorder()
			configChecklistRouter(t, service, settings).ServeHTTP(response, configChecklistRequest(method, false))
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || service.authenticateCalls != 0 || service.authorizeCalls != 0 || service.csrfCalls != 0 || len(settings.listInputs) != 0 || settings.saveCalls != 0 {
				t.Fatalf("method=%s status/headers/auth/list/save=%d/%q/%q/%q/%d/%d/%d/%d/%d", method, response.Code, response.Header().Get("Allow"), response.Header().Get("Cache-Control"), response.Header().Get("X-Content-Type-Options"), service.authenticateCalls, service.authorizeCalls, service.csrfCalls, len(settings.listInputs), settings.saveCalls)
			}
		}
	})
}

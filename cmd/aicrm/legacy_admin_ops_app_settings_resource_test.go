package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

func TestAppSettingsResourceSaveRequiresSessionRBACCSRFRouteTokenAndSecretBoundary(t *testing.T) {
	session, csrf := authport.SessionRef(legacyToken(10)), legacyToken(11)
	projection := configapp.SettingsProjection{MetadataMap: map[configport.Key]configapp.SettingMetadata{
		configport.WeComCorpID: {Key: configport.WeComCorpID, Mode: "editable"},
		configport.DatabaseURL: {Key: configport.DatabaseURL, Mode: "masked"},
	}}
	build := func(role authport.Role) (*legacySettingsStub, http.Handler) {
		stub := &legacySettingsStub{projection: projection}
		handler := &Handler{settings: stub, auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: role}}}
		tail, err := handler.Authorize(authport.CapabilityConfigSettingsManage, http.HandlerFunc(handler.SaveAppSettingsResource))
		if err != nil {
			t.Fatal(err)
		}
		tail, err = handler.RequireCSRF(tail)
		if err != nil {
			t.Fatal(err)
		}
		return stub, handler.Authenticate(tail)
	}
	request := func(token, body string) *http.Request {
		r := httptest.NewRequest(http.MethodPut, appSettingsResourcePath, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("X-Admin-Action-Token", token)
		r.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: string(session)})
		return r
	}
	validToken := appSettingsResourceActionToken(session)

	stub, route := build(authport.RoleAdmin)
	response := httptest.NewRecorder()
	route.ServeHTTP(response, request(validToken, `{"settings":{"wecom.corp_id":"corp"},"confirm":true,"operator":"attacker"}`))
	if response.Code != http.StatusOK || len(stub.saves) != 1 || stub.saves[0].Actor != "admin:7" || stub.saves[0].Values["wecom.corp_id"][0] != "corp" || !strings.Contains(response.Body.String(), `"changed_count":1`) {
		t.Fatalf("success status=%d saves=%#v body=%s", response.Code, stub.saves, response.Body.String())
	}
	missingCSRF := request(validToken, `{"settings":{"wecom.corp_id":"blocked"},"confirm":true}`)
	missingCSRF.Header.Del("X-CSRF-Token")
	response = httptest.NewRecorder()
	route.ServeHTTP(response, missingCSRF)
	if response.Code != http.StatusForbidden || len(stub.saves) != 1 {
		t.Fatalf("csrf status=%d saves=%d body=%s", response.Code, len(stub.saves), response.Body.String())
	}
	missingSession := request(validToken, `{"settings":{"wecom.corp_id":"blocked"},"confirm":true}`)
	missingSession.Header.Del("Cookie")
	response = httptest.NewRecorder()
	route.ServeHTTP(response, missingSession)
	if response.Code != http.StatusUnauthorized || len(stub.saves) != 1 {
		t.Fatalf("session status=%d saves=%d body=%s", response.Code, len(stub.saves), response.Body.String())
	}

	response = httptest.NewRecorder()
	route.ServeHTTP(response, request("wrong", `{"settings":{"wecom.corp_id":"other"},"confirm":true}`))
	if response.Code != http.StatusBadRequest || len(stub.saves) != 1 || !strings.Contains(response.Body.String(), `"invalid_action_token"`) {
		t.Fatalf("token status=%d saves=%d body=%s", response.Code, len(stub.saves), response.Body.String())
	}

	response = httptest.NewRecorder()
	route.ServeHTTP(response, request(validToken, `{"settings":{"database.url":"raw-secret-must-not-reach-service"},"confirm":true}`))
	if response.Code != http.StatusBadRequest || len(stub.saves) != 1 || strings.Contains(response.Body.String(), "raw-secret-must-not-reach-service") || !strings.Contains(response.Body.String(), `"secret_input_forbidden"`) {
		t.Fatalf("secret status=%d saves=%d body=%s", response.Code, len(stub.saves), response.Body.String())
	}

	deniedStub, denied := build(authport.RoleOps)
	response = httptest.NewRecorder()
	denied.ServeHTTP(response, request(validToken, `{"settings":{"wecom.corp_id":"other"},"confirm":true}`))
	if response.Code != http.StatusForbidden || len(deniedStub.saves) != 0 {
		t.Fatalf("rbac status=%d saves=%d body=%s", response.Code, len(deniedStub.saves), response.Body.String())
	}
}

func TestDecodeAppSettingsResourcePayloadHasFiniteBoundaryErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "confirm", body: `{"settings":{},"confirm":false}`, want: "confirmation_required"},
		{name: "settings", body: `{"settings":[],"confirm":true}`, want: "settings_must_be_object"},
		{name: "trailing", body: `{"settings":{},"confirm":true}{}`, want: "payload_must_be_object"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, appSettingsResourcePath, strings.NewReader(test.body))
			_, _, got := decodeAppSettingsResourcePayload(request)
			if got != test.want {
				t.Fatalf("code=%q want=%q", got, test.want)
			}
		})
	}
}

func TestAppSettingsResourceIssuesOnlyThePUTBoundActionToken(t *testing.T) {
	session := authport.SessionRef(legacyToken(12))
	handler := &Handler{settings: &legacySettingsStub{projection: configapp.SettingsProjection{MetadataMap: map[configport.Key]configapp.SettingMetadata{}}}, auth: &legacyAuthStub{}}
	response := httptest.NewRecorder()
	request := settingsRequestContext(httptest.NewRequest(http.MethodGet, appSettingsResourcePath, nil), 7, session)
	handler.AppSettingsResource(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"admin_action_token":"`+appSettingsResourceActionToken(session)+`"`) || strings.Contains(response.Body.String(), settingsActionToken(session)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

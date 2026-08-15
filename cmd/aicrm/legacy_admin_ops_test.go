package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type legacyAdminOpsTransportStub struct {
	legacyAdminOps
	created []adminopsapp.CredentialCommand
}

func (stub *legacyAdminOpsTransportStub) CreateCredential(_ context.Context, command adminopsapp.CredentialCommand) (adminopsport.Credential, error) {
	stub.created = append(stub.created, command)
	return adminopsport.Credential{Kind: command.Kind, ClientID: command.ClientID, DisplayName: command.DisplayName, State: "active", SecretRef: "secret://adminops/direct_api_key/direct-default/12345678", SecretMask: "masked:…345678"}, nil
}

func TestAdminOpsDirectKeyRequiresSessionRBACCSRFAndActionToken(t *testing.T) {
	stub := &legacyAdminOpsTransportStub{}
	handler := &Handler{adminOps: stub, auth: &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleAdmin}}}
	tail, err := handler.Authorize(authport.CapabilityConfigSettingsManage, http.HandlerFunc(handler.AdminOps))
	if err != nil {
		t.Fatal(err)
	}
	tail, err = handler.RequireCSRF(tail)
	if err != nil {
		t.Fatal(err)
	}
	route := handler.Authenticate(tail)
	session, csrf := authport.SessionRef(legacyToken(4)), legacyToken(5)
	request := func(token string) *http.Request {
		body := `{"confirm":true,"admin_action_token":"` + token + `"}`
		r := httptest.NewRequest(http.MethodPost, "/api/admin/config/api-key/generate", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: string(session)})
		return r
	}
	response := httptest.NewRecorder()
	route.ServeHTTP(response, request(adminOpsActionToken(session, http.MethodPost, "/api/admin/config/api-key/generate")))
	if response.Code != http.StatusCreated || len(stub.created) != 1 || stub.created[0].Actor != "admin:7" {
		t.Fatalf("status=%d created=%#v", response.Code, stub.created)
	}
	if got := response.Body.String(); strings.Contains(got, "client_secret") || strings.Contains(got, "access_token") || !strings.Contains(got, `"secret_mask":"masked:`) {
		t.Fatalf("unsafe credential response: %s", got)
	}
	response = httptest.NewRecorder()
	route.ServeHTTP(response, request("wrong"))
	if response.Code != http.StatusUnauthorized || len(stub.created) != 1 {
		t.Fatalf("invalid action token status=%d created=%d", response.Code, len(stub.created))
	}
}

func TestDecodeAdminOpsPayloadRejectsTrailingJSONAndSecretMaterial(t *testing.T) {
	for _, body := range []string{`{"confirm":true}{"unexpected":true}`, `{"confirm":true,"webhook_url":"https://secret.example"}`} {
		response := httptest.NewRecorder()
		_, ok := decodeAdminOpsPayload(response, httptest.NewRequest(http.MethodPost, "/api/admin/config/api-key/generate", strings.NewReader(body)))
		if ok || response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d ok=%v", body, response.Code, ok)
		}
	}
}

func TestReleaseChangesFormOnlyAllowsReferenceForSensitiveValues(t *testing.T) {
	build := func(values url.Values) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/admin/config/releases", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		return request
	}
	changes, err := releaseChangesFromForm(build(url.Values{"key__0": {"wecom.webhook_ref"}, "value__0": {"secretref:wecom/alerts"}, "key__1": {"outbound.rate_per_second"}, "value__1": {"30"}}))
	if err != nil || changes["wecom.webhook_ref"] != "secretref:wecom/alerts" || changes["outbound.rate_per_second"] != "30" {
		t.Fatalf("changes=%#v err=%v", changes, err)
	}
	for _, values := range []url.Values{{"key__0": {"wecom.webhook_ref"}, "value__0": {"https://raw-secret.example"}}, {"key__0": {"same.key"}, "value__0": {"one"}, "key__1": {"same.key"}, "value__1": {"two"}}} {
		if _, err := releaseChangesFromForm(build(values)); err == nil {
			t.Fatalf("unsafe form was accepted: %v", values)
		}
	}
}

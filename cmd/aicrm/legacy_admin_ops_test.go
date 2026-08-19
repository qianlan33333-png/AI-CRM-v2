package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	adminopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/app"
	adminopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/adminops/port"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type legacyAdminOpsTransportStub struct {
	legacyAdminOps
	created     []adminopsapp.CredentialCommand
	credentials []adminopsport.Credential
	listErr     error
}

func (stub *legacyAdminOpsTransportStub) CreateCredential(_ context.Context, command adminopsapp.CredentialCommand) (adminopsport.Credential, error) {
	stub.created = append(stub.created, command)
	return adminopsport.Credential{Kind: command.Kind, ClientID: command.ClientID, DisplayName: command.DisplayName, State: "active", SecretRef: "secret://adminops/direct_api_key/direct-default/12345678", SecretMask: "masked:…345678"}, nil
}

func (stub *legacyAdminOpsTransportStub) ListCredentials(context.Context) ([]adminopsport.Credential, error) {
	return stub.credentials, stub.listErr
}

func TestAdminOpsAPIClientListPageUsesLocalProjectionAndSafeFilters(t *testing.T) {
	updatedAt := time.Date(2026, 8, 19, 2, 3, 4, 0, time.UTC)
	stub := &legacyAdminOpsTransportStub{credentials: []adminopsport.Credential{
		{Kind: adminopsport.CredentialAPIClient, ClientID: "alpha.client", DisplayName: "Alpha <script>alert(1)</script>", State: "active", SecretRef: "secret://must-not-render", SecretMask: "masked:must-not-render", Version: 3, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "beta.client", DisplayName: "Beta", State: "disabled", Version: 2, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "gamma.client", DisplayName: "Gamma", State: "pending_activation", Version: 1, UpdatedAt: updatedAt},
		{Kind: adminopsport.CredentialDirectAPIKey, ClientID: "direct-key", DisplayName: "Direct", State: "active", Version: 1, UpdatedAt: updatedAt},
	}}
	handler := &Handler{adminOps: stub}

	response := httptest.NewRecorder()
	handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?q=ALPHA&status=enabled", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Cache-Control"), "private") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
	for _, want := range []string{"Alpha &lt;script&gt;alert(1)&lt;/script&gt;", "/admin/config/api-clients/alpha.client", "/admin/config/api-clients/new", "共 3 个", "已启用 1 个", "已停用 1 个", "待激活 1 个"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	for _, forbidden := range []string{"beta.client", "gamma.client", "direct-key", "secret://must-not-render", "masked:must-not-render", "<script>alert(1)</script>"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unsafe or unfiltered value %q in %s", forbidden, body)
		}
	}
	for _, testCase := range []struct {
		status, want, forbidden string
	}{
		{status: "enabled", want: "alpha.client", forbidden: "beta.client"},
		{status: "disabled", want: "beta.client", forbidden: "gamma.client"},
		{status: "pending_activation", want: "gamma.client", forbidden: "beta.client"},
	} {
		filtered := httptest.NewRecorder()
		handler.AdminOps(filtered, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?status="+testCase.status, nil))
		if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), testCase.want) || strings.Contains(filtered.Body.String(), testCase.forbidden) {
			t.Fatalf("status filter=%s code=%d body=%s", testCase.status, filtered.Code, filtered.Body.String())
		}
	}
}

func TestAdminOpsAPIClientListPageRejectsBadStatusBeforeReading(t *testing.T) {
	stub := &legacyAdminOpsTransportStub{listErr: errors.New("must not be called")}
	handler := &Handler{adminOps: stub}
	response := httptest.NewRecorder()
	handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients?status=unknown", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"error":"invalid_status_filter"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminOpsAPIClientListPageFailsClosedForInvalidProjection(t *testing.T) {
	for _, credential := range []adminopsport.Credential{
		{Kind: adminopsport.CredentialAPIClient, ClientID: "broken.client", DisplayName: "Broken", State: "unknown", Version: 1, UpdatedAt: time.Now().UTC()},
		{Kind: adminopsport.CredentialAPIClient, ClientID: "..", DisplayName: "Unsafe path", State: "active", Version: 1, UpdatedAt: time.Now().UTC()},
	} {
		stub := &legacyAdminOpsTransportStub{credentials: []adminopsport.Credential{credential}}
		handler := &Handler{adminOps: stub}
		response := httptest.NewRecorder()
		handler.AdminOps(response, httptest.NewRequest(http.MethodGet, "/admin/config/api-clients", nil))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"error":"admin_ops_unavailable"`) || strings.Contains(response.Body.String(), credential.ClientID) {
			t.Fatalf("credential=%q status=%d body=%s", credential.ClientID, response.Code, response.Body.String())
		}
	}
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

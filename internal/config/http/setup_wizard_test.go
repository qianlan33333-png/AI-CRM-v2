package confighttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	configapp "github.com/qianlan33333-png/AI-CRM-v2/internal/config/app"
	configport "github.com/qianlan33333-png/AI-CRM-v2/internal/config/port"
)

func TestSetupWizardGETReturnsScopedStateAndPostBoundToken(t *testing.T) {
	application := &setupWizardApplicationStub{snapshot: testSnapshot()}
	auth := &setupWizardAuthStub{}
	handler := newSetupWizardHandler(t, application)
	session := authport.SessionRef("session-material-never-log")
	request := authorizedSetupWizardRequest(t, http.MethodGet, SetupWizardPath, nil, session, authport.CapabilityConfigOverviewRead)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.getCalls != 1 || strings.Contains(response.Body.String(), "secret-material") || !strings.Contains(response.Body.String(), `"external":false`) || !strings.Contains(response.Body.String(), `"local_only":true`) || !strings.Contains(response.Body.String(), `"runtime_applied":false`) || !strings.Contains(response.Body.String(), `"admin_action_token":"`+setupWizardActionToken(session, http.MethodPost, SetupWizardPath)+`"`) {
		t.Fatalf("GET status/calls/body=%d/%d/%s", response.Code, application.getCalls, response.Body.String())
	}
	if auth.csrfCalls != 0 || response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%#v", response.Header())
	}

	wrongCapability := authorizedSetupWizardRequest(t, http.MethodGet, SetupWizardPath, nil, session, authport.CapabilityConfigSettingsManage)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, wrongCapability)
	if response.Code != http.StatusForbidden || application.getCalls != 1 {
		t.Fatalf("GET wrong capability status/calls=%d/%d", response.Code, application.getCalls)
	}
}

func TestSetupWizardPOSTUsesPrincipalIdempotencyAndStrictLocalResult(t *testing.T) {
	requestID := strings.Join([]string{"test", "request", "one"}, "-")
	application := &setupWizardApplicationStub{result: configapp.SetupWizardSaveResult{Snapshot: testSnapshot(), Receipt: configapp.SetupWizardReceipt{IdempotencyKey: requestID, Audits: []configapp.SetupWizardAuditReceipt{}, Events: []configapp.SetupWizardEventReceipt{}}}}
	auth := &setupWizardAuthStub{}
	handler := csrfProtectedSetupWizardHandler(t, newSetupWizardHandler(t, application), auth)
	session := authport.SessionRef("session-material-never-log")
	body := `{"wecom.corp_id":"corp-1","wecom.agent_id":17,"wecom.secret":"","wecom.callback_token":"","wecom.callback_aes_key":"","ai.api_key":"","expected_digest":"` + strings.Repeat("a", 64) + `","admin_action_token":"` + setupWizardActionToken(session, http.MethodPost, SetupWizardPath) + `"}`
	request := authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(body), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-ok")
	request.Header.Set("Idempotency-Key", requestID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || application.saveCalls != 1 || auth.csrfCalls != 1 || !strings.Contains(response.Body.String(), `"external":false`) || !strings.Contains(response.Body.String(), `"local_only":true`) || !strings.Contains(response.Body.String(), `"runtime_applied":false`) {
		t.Fatalf("POST status/calls/csrf/body=%d/%d/%d/%s", response.Code, application.saveCalls, auth.csrfCalls, response.Body.String())
	}
	got := application.inputs[0]
	if got.Actor != "admin:42" || got.IdempotencyKey != requestID || got.WeComCorpID != "corp-1" || got.WeComAgentID != 17 || got.WeComSecret != "" || got.WeComCallbackToken != "" || got.WeComCallbackAESKey != "" || got.AIAPIKey != "" {
		t.Fatalf("input=%#v", got)
	}
}

func TestSetupWizardPOSTRejectsCSRFRouteTokenAndSecretWithoutLeakingIt(t *testing.T) {
	const secret = "raw-secret-must-not-leak"
	application := &setupWizardApplicationStub{saveErr: configport.ErrSecretSetting}
	auth := &setupWizardAuthStub{}
	handler := csrfProtectedSetupWizardHandler(t, newSetupWizardHandler(t, application), auth)
	session := authport.SessionRef("session-material-never-log")
	validBody := func(token string) string {
		return `{"wecom.corp_id":"corp-1","wecom.agent_id":17,"wecom.secret":"` + secret + `","wecom.callback_token":"","wecom.callback_aes_key":"","ai.api_key":"","expected_digest":"` + strings.Repeat("a", 64) + `","admin_action_token":"` + token + `"}`
	}
	request := authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(validBody(setupWizardActionToken(session, http.MethodPost, SetupWizardPath))), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "idempotency-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || application.saveCalls != 0 {
		t.Fatalf("missing csrf status/calls=%d/%d", response.Code, application.saveCalls)
	}

	request = authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(validBody("wrong")), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-ok")
	request.Header.Set("Idempotency-Key", "idempotency-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || application.saveCalls != 0 || !strings.Contains(response.Body.String(), `"invalid_action_token"`) {
		t.Fatalf("wrong token status/calls/body=%d/%d/%s", response.Code, application.saveCalls, response.Body.String())
	}

	request = authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(validBody(setupWizardActionToken(session, http.MethodPost, SetupWizardPath))), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-ok")
	request.Header.Set("Idempotency-Key", "idempotency-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || application.saveCalls != 1 || !strings.Contains(response.Body.String(), `"secret_input_forbidden"`) || strings.Contains(response.Body.String(), secret) {
		t.Fatalf("secret status/calls/body=%d/%d/%s", response.Code, application.saveCalls, response.Body.String())
	}
}

func TestSetupWizardPOSTRejectsDuplicateUnknownAndOversizedContracts(t *testing.T) {
	application := &setupWizardApplicationStub{result: configapp.SetupWizardSaveResult{Snapshot: testSnapshot()}}
	auth := &setupWizardAuthStub{}
	handler := csrfProtectedSetupWizardHandler(t, newSetupWizardHandler(t, application), auth)
	session := authport.SessionRef("session-material-never-log")
	token := setupWizardActionToken(session, http.MethodPost, SetupWizardPath)
	valid := `{"wecom.corp_id":"corp-1","wecom.agent_id":17,"wecom.secret":"","wecom.callback_token":"","wecom.callback_aes_key":"","ai.api_key":"","expected_digest":"` + strings.Repeat("a", 64) + `","admin_action_token":"` + token + `"}`
	cases := []struct {
		name        string
		contentType string
		body        string
		mutate      func(*http.Request)
	}{
		{name: "duplicate json", contentType: "application/json", body: strings.Replace(valid, `"wecom.corp_id":"corp-1",`, `"wecom.corp_id":"corp-1","wecom.corp_id":"corp-2",`, 1)},
		{name: "unknown json", contentType: "application/json", body: strings.TrimSuffix(valid, "}") + `,"payment.key":"no"}`},
		{name: "fractional agent", contentType: "application/json", body: strings.Replace(valid, `"wecom.agent_id":17`, `"wecom.agent_id":17.0`, 1)},
		{name: "duplicate idempotency", contentType: "application/json", body: valid, mutate: func(request *http.Request) { request.Header.Add("Idempotency-Key", "idempotency-second") }},
		{name: "oversized", contentType: "application/json", body: strings.Repeat("x", maximumRequestBytes+1)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(testCase.body), session, authport.CapabilityConfigSettingsManage)
			request.Header.Set("Content-Type", testCase.contentType)
			request.Header.Set("X-CSRF-Token", "csrf-ok")
			request.Header.Set("Idempotency-Key", "idempotency-contract")
			if testCase.mutate != nil {
				testCase.mutate(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if application.saveCalls != 0 {
		t.Fatalf("invalid inputs reached application %d times", application.saveCalls)
	}
}

func TestSetupWizardPOSTRejectsNonJSONAndMapsConflict(t *testing.T) {
	application := &setupWizardApplicationStub{saveErr: configapp.ErrSetupWizardConflict}
	auth := &setupWizardAuthStub{}
	handler := csrfProtectedSetupWizardHandler(t, newSetupWizardHandler(t, application), auth)
	session := authport.SessionRef("session-material-never-log")
	body := `{"wecom.corp_id":"corp-form","wecom.agent_id":18,"wecom.secret":"","wecom.callback_token":"","wecom.callback_aes_key":"","ai.api_key":"","expected_digest":"` + strings.Repeat("a", 64) + `","admin_action_token":"` + setupWizardActionToken(session, http.MethodPost, SetupWizardPath) + `"}`
	request := authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(body), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-ok")
	request.Header.Set("Idempotency-Key", "idempotency-form")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || application.saveCalls != 1 || application.inputs[0].Actor != "admin:42" || application.inputs[0].IdempotencyKey != "idempotency-form" {
		t.Fatalf("JSON status/calls/input=%d/%d/%#v", response.Code, application.saveCalls, application.inputs)
	}

	request = authorizedSetupWizardRequest(t, http.MethodPost, SetupWizardPath, strings.NewReader(body), session, authport.CapabilityConfigSettingsManage)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-CSRF-Token", "csrf-ok")
	request.Header.Set("Idempotency-Key", "idempotency-form-duplicate")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || application.saveCalls != 1 {
		t.Fatalf("non-JSON status/calls=%d/%d", response.Code, application.saveCalls)
	}
}

func newSetupWizardHandler(t *testing.T, application setupWizardApplication) *Handler {
	t.Helper()
	handler, err := NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func csrfProtectedSetupWizardHandler(t *testing.T, leaf http.Handler, auth authport.Service) http.Handler {
	t.Helper()
	handler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	protected, err := handler.RequireCSRF(leaf)
	if err != nil {
		t.Fatal(err)
	}
	return protected
}

func authorizedSetupWizardRequest(t *testing.T, method, path string, body io.Reader, session authport.SessionRef, capability authport.Capability) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 42, Role: authport.RoleAdmin}, session)
	ctx, err := authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

func testSnapshot() configapp.SetupWizardSnapshot {
	return configapp.SetupWizardSnapshot{
		ExpectedDigest: strings.Repeat("b", 64),
		Editable:       configapp.SetupWizardEditableSettings{WeComCorpID: "corp-1", WeComAgentID: 17},
		Masked: configapp.SetupWizardMaskedSettings{
			WeComSecret:         configapp.SetupWizardMaskedSetting{Configured: true, Masked: true},
			WeComCallbackToken:  configapp.SetupWizardMaskedSetting{Masked: true},
			WeComCallbackAESKey: configapp.SetupWizardMaskedSetting{Masked: true},
			AIAPIKey:            configapp.SetupWizardMaskedSetting{Masked: true},
		},
	}
}

type setupWizardApplicationStub struct {
	snapshot  configapp.SetupWizardSnapshot
	result    configapp.SetupWizardSaveResult
	getErr    error
	saveErr   error
	getCalls  int
	saveCalls int
	inputs    []configapp.SetupWizardSaveInput
}

func (stub *setupWizardApplicationStub) Get(context.Context) (configapp.SetupWizardSnapshot, error) {
	stub.getCalls++
	return stub.snapshot, stub.getErr
}

func (stub *setupWizardApplicationStub) Save(_ context.Context, input configapp.SetupWizardSaveInput) (configapp.SetupWizardSaveResult, error) {
	stub.saveCalls++
	stub.inputs = append(stub.inputs, input)
	return stub.result, stub.saveErr
}

type setupWizardAuthStub struct{ csrfCalls int }

func (stub *setupWizardAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return authport.Principal{}, errors.New("unexpected authenticate")
}

func (stub *setupWizardAuthStub) Authorize(context.Context, authport.Principal, authport.Capability) (authport.Authorization, error) {
	return authport.Authorization{}, errors.New("unexpected authorize")
}

func (stub *setupWizardAuthStub) ValidateCSRF(_ context.Context, _ authport.SessionRef, token authport.CSRFToken) error {
	stub.csrfCalls++
	if token != "csrf-ok" {
		return authport.ErrCSRFInvalid
	}
	return nil
}

func (stub *setupWizardAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return errors.New("unexpected invalidate")
}

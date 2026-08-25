package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestExternalPushRouteFragmentReadWriteAndAcceptedOnlyTest(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	application := &externalPushHTTPApplication{configuration: productport.ExternalPushConfiguration{
		ProductID: 7, ProductKind: productport.ExternalPushWeChatPay, UpdatedAt: now,
	}, test: productport.ExternalPushTest{
		ProductID: 7, ProductKind: productport.ExternalPushWeChatPay, EffectID: "eer_72", State: "accepted", CreatedAt: now,
	}}
	security := &externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 21, Role: authport.RoleAdmin}}
	handler := mustExternalPushHandler(t, application, security)

	read := externalPushRequest(http.MethodGet, "/api/admin/wechat-pay/products/7/external-push", "", "")
	readResponse := httptest.NewRecorder()
	handler.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK || application.getID != 7 || application.getKind != productport.ExternalPushWeChatPay || security.csrfCalls != 0 {
		t.Fatalf("read status/id/kind/csrf=%d/%d/%s/%d", readResponse.Code, application.getID, application.getKind, security.csrfCalls)
	}

	save := externalPushRequest(http.MethodPut, "/api/admin/wechat-pay/products/7/external-push", `{"enabled":true,"configuration_reference":"local-config-7"}`, "external-push-save-0001")
	saveResponse := httptest.NewRecorder()
	handler.ServeHTTP(saveResponse, save)
	if saveResponse.Code != http.StatusOK || !application.save.Enabled || application.save.ConfigurationReference != "local-config-7" || application.save.Actor != 21 || application.save.IdempotencyKey != "external-push-save-0001" || security.csrfCalls != 1 {
		t.Fatalf("save status/command/csrf=%d/%+v/%d", saveResponse.Code, application.save, security.csrfCalls)
	}

	testRequest := externalPushRequest(http.MethodPost, "/api/admin/wechat-pay/products/7/external-push/test", "", "external-push-test-0001")
	testResponse := httptest.NewRecorder()
	handler.ServeHTTP(testResponse, testRequest)
	if testResponse.Code != http.StatusAccepted || application.queue.ProductID != 7 || application.queue.ProductKind != productport.ExternalPushWeChatPay || application.queue.Actor != 21 || application.queue.IdempotencyKey != "external-push-test-0001" || security.csrfCalls != 2 {
		t.Fatalf("test status/command/csrf=%d/%+v/%d", testResponse.Code, application.queue, security.csrfCalls)
	}
	if want := []authport.Capability{authport.CapabilityProductsRead, authport.CapabilityProductsWrite, authport.CapabilityProductsWrite}; !slices.Equal(security.calls, want) {
		t.Fatalf("capabilities=%v want=%v", security.calls, want)
	}
	var body map[string]any
	if err := json.Unmarshal(testResponse.Body.Bytes(), &body); err != nil || body["effect_id"] != "eer_72" || body["state"] != "accepted" || body["provider_accepted"] != false || body["delivery_proven"] != false || body["real_external_call_executed"] != false || body["auto_retry_allowed"] != false {
		t.Fatalf("test response=%#v err=%v", body, err)
	}
	encoded := testResponse.Body.String()
	for _, forbidden := range []string{"receipt", "payload", "target", "identity", "mobile"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("response leaked forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func TestExternalPushRouteFragmentFailsClosedForRBACCSRFAndIdempotency(t *testing.T) {
	application := &externalPushHTTPApplication{configuration: productport.ExternalPushConfiguration{ProductID: 7, ProductKind: productport.ExternalPushServicePeriod, UpdatedAt: time.Now().UTC()}}
	for _, testCase := range []struct {
		name       string
		security   externalPushHTTPSecurity
		key        string
		body       string
		wantStatus int
	}{
		{name: "anonymous", security: externalPushHTTPSecurity{}, key: "external-push-test-0002", wantStatus: http.StatusForbidden},
		{name: "forbidden", security: externalPushHTTPSecurity{err: authport.ErrUnauthorized}, key: "external-push-test-0002", wantStatus: http.StatusForbidden},
		{name: "csrf", security: externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 2, Role: authport.RoleAdmin}, csrfErr: authport.ErrCSRFInvalid}, key: "external-push-test-0002", wantStatus: http.StatusForbidden},
		{name: "missing idempotency", security: externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 2, Role: authport.RoleAdmin}}, wantStatus: http.StatusBadRequest},
		{name: "duplicate idempotency", security: externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 2, Role: authport.RoleAdmin}}, key: "external-push-test-0002", wantStatus: http.StatusBadRequest},
		{name: "unknown field", security: externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 2, Role: authport.RoleAdmin}}, key: "external-push-test-0003", body: `{"enabled":false,"provider_url":"forbidden"}`, wantStatus: http.StatusBadRequest},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			security := testCase.security
			handler := mustExternalPushHandler(t, application, &security)
			request := externalPushRequest(http.MethodPut, "/api/admin/service-period-products/7/external-push", testCase.body, testCase.key)
			if testCase.name == "duplicate idempotency" {
				request.Header.Add("Idempotency-Key", "external-push-test-0004")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, testCase.wantStatus, response.Body.String())
			}
		})
	}
}

func TestExternalPushRouteFragmentRejectsPathAndMethodDrift(t *testing.T) {
	security := &externalPushHTTPSecurity{principal: authport.Principal{AdminUserID: 21, Role: authport.RoleAdmin}}
	handler := mustExternalPushHandler(t, &externalPushHTTPApplication{}, security)
	for _, testCase := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPost, "/api/admin/wechat-pay/products/7/external-push", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/admin/wechat-pay/products/07/external-push", http.StatusNotFound},
		{http.MethodGet, "/api/admin/wechat-pay/products/7/external-push/", http.StatusBadRequest},
		{http.MethodGet, "/api/admin/wechat-pay/products/7/external-push?x=1", http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(testCase.method, testCase.path, nil))
		if response.Code != testCase.want {
			t.Fatalf("%s %s=%d want=%d", testCase.method, testCase.path, response.Code, testCase.want)
		}
	}
}

func mustExternalPushHandler(t *testing.T, application productport.CommerceExternalPushApplication, security *externalPushHTTPSecurity) *ExternalPushHandler {
	t.Helper()
	handler, err := NewExternalPushHandler(application, security, security)
	if err != nil {
		t.Fatal(err)
	}
	if fragment, err := NewExternalPushRouteFragment(handler); err != nil || fragment != handler {
		t.Fatalf("fragment=%v err=%v", fragment, err)
	}
	return handler
}

func externalPushRequest(method, path, body, key string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPut {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	return request
}

type externalPushHTTPApplication struct {
	configuration productport.ExternalPushConfiguration
	test          productport.ExternalPushTest
	err           error
	getID         productport.ID
	getKind       productport.ExternalPushProductKind
	save          productport.SaveExternalPushConfigurationCommand
	queue         productport.QueueExternalPushTestCommand
}

func (application *externalPushHTTPApplication) GetExternalPushConfiguration(_ context.Context, id productport.ID, kind productport.ExternalPushProductKind) (productport.ExternalPushConfiguration, error) {
	application.getID, application.getKind = id, kind
	if application.err != nil {
		return productport.ExternalPushConfiguration{}, application.err
	}
	result := application.configuration
	result.ProductID, result.ProductKind = id, kind
	return result, nil
}
func (application *externalPushHTTPApplication) SaveExternalPushConfiguration(_ context.Context, command productport.SaveExternalPushConfigurationCommand) (productport.ExternalPushConfiguration, error) {
	application.save = command
	if application.err != nil {
		return productport.ExternalPushConfiguration{}, application.err
	}
	return productport.ExternalPushConfiguration{ProductID: command.ProductID, ProductKind: command.ProductKind, Enabled: command.Enabled, ConfigurationReference: command.ConfigurationReference, UpdatedAt: time.Now().UTC()}, nil
}
func (application *externalPushHTTPApplication) QueueExternalPushTest(_ context.Context, command productport.QueueExternalPushTestCommand) (productport.ExternalPushTest, error) {
	application.queue = command
	if application.err != nil {
		return productport.ExternalPushTest{}, application.err
	}
	result := application.test
	result.ProductID, result.ProductKind = command.ProductID, command.ProductKind
	return result, nil
}

type externalPushHTTPSecurity struct {
	principal authport.Principal
	err       error
	csrfErr   error
	calls     []authport.Capability
	csrfCalls int
}

func (security *externalPushHTTPSecurity) Authorize(_ context.Context, capability authport.Capability) (authport.Principal, error) {
	security.calls = append(security.calls, capability)
	if security.err != nil {
		return authport.Principal{}, security.err
	}
	return security.principal, nil
}
func (security *externalPushHTTPSecurity) Verify(_ *http.Request, _ authport.Principal) error {
	security.csrfCalls++
	return security.csrfErr
}

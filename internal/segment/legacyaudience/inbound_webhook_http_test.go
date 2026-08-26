package legacyaudience

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type inboundWebhookAuthenticatorStub struct {
	identity InboundWebhookIdentity
	err      error
	body     []byte
	calls    int
}

func (stub *inboundWebhookAuthenticatorStub) Authenticate(_ context.Context, _ *http.Request, body []byte) (InboundWebhookIdentity, error) {
	stub.calls++
	stub.body = append([]byte(nil), body...)
	return stub.identity, stub.err
}

type inboundWebhookApplicationStub struct {
	input  InboundWebhookInput
	result InboundWebhookResult
	err    error
	calls  int
}

func (stub *inboundWebhookApplicationStub) Accept(_ context.Context, input InboundWebhookInput) (InboundWebhookResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

func newInboundWebhookHTTPFixture(t *testing.T) (*InboundWebhookHandler, *inboundWebhookAuthenticatorStub, *inboundWebhookApplicationStub) {
	t.Helper()
	authenticator := &inboundWebhookAuthenticatorStub{identity: InboundWebhookIdentity{ClientID: AIAudienceWebhookClientID, TransportEventID: "transport-event-0001"}}
	application := &inboundWebhookApplicationStub{result: InboundWebhookResult{Receipt: InboundWebhookReceipt{
		ID: 7, PackageID: 42, State: InboundWebhookReceived, ExternalEventIDDigest: [32]byte{1}, PayloadDigest: [32]byte{2},
		CreatedAt: time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC),
	}}}
	handler, err := NewInboundWebhookHandler(application, authenticator)
	if err != nil {
		t.Fatalf("NewInboundWebhookHandler: %v", err)
	}
	return handler, authenticator, application
}

func performInboundWebhook(handler http.Handler, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestInboundWebhookHTTPAuthenticatesRawBodyAndReturnsRecordOnlyReceipt(t *testing.T) {
	handler, authenticator, application := newInboundWebhookHTTPFixture(t)
	body := `{"external_event_id":"business-event-0001","member_event_id":9,"status":"done","message":{"text":"ok"},"action":{}}`
	response := performInboundWebhook(handler, "/api/ai/audience/packages/42/webhook", body)

	if response.Code != http.StatusOK || authenticator.calls != 1 || string(authenticator.body) != body || application.calls != 1 {
		t.Fatalf("status=%d auth=%d app=%d body=%s", response.Code, authenticator.calls, application.calls, response.Body.String())
	}
	if application.input.PackageID != 42 || application.input.ExternalEventID != "business-event-0001" || application.input.MemberEventID == nil || *application.input.MemberEventID != 9 || application.input.Status != "done" || application.input.PayloadDigest == ([32]byte{}) {
		t.Fatalf("input=%+v", application.input)
	}
	for _, fragment := range []string{`"accepted":true`, `"deduplicated":false`, `"record_only":true`, `"real_external_call_executed":false`, `"signal":null`, `"external_effect_job_id":null`} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Fatalf("missing %s in %s", fragment, response.Body.String())
		}
	}
	if response.Header().Get("X-AICRM-Real-External-Call-Executed") != "false" {
		t.Fatalf("external effect header=%q", response.Header().Get("X-AICRM-Real-External-Call-Executed"))
	}
}

func TestInboundWebhookHTTPStrictBodyPathAndAuthentication(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		target      string
		body        string
		contentType string
		authErr     error
		status      int
	}{
		{name: "unknown field", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{"external_event_id":"business-event-0001","extra":1}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "duplicate field", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{"external_event_id":"business-event-0001","external_event_id":"business-event-0002"}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "array message", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{"external_event_id":"business-event-0001","message":[]}`, contentType: "application/json", status: http.StatusUnprocessableEntity},
		{name: "missing event", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{}`, contentType: "application/json", status: http.StatusUnprocessableEntity},
		{name: "bad package", method: http.MethodPost, target: "/api/ai/audience/packages/042/webhook", body: `{"external_event_id":"business-event-0001"}`, contentType: "application/json", status: http.StatusUnprocessableEntity},
		{name: "query", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook?x=1", body: `{"external_event_id":"business-event-0001"}`, contentType: "application/json", status: http.StatusBadRequest},
		{name: "wrong content type", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{}`, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		{name: "unauthenticated", method: http.MethodPost, target: "/api/ai/audience/packages/42/webhook", body: `{"external_event_id":"business-event-0001"}`, contentType: "application/json", authErr: ErrUnauthenticated, status: http.StatusUnauthorized},
		{name: "method", method: http.MethodGet, target: "/api/ai/audience/packages/42/webhook", contentType: "application/json", status: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, authenticator, application := newInboundWebhookHTTPFixture(t)
			authenticator.err = test.authErr
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			if test.status != http.StatusOK && application.calls != 0 {
				t.Fatalf("application calls=%d", application.calls)
			}
		})
	}
}

func TestInboundWebhookHTTPRejectsOversizedBodyBeforeAuthentication(t *testing.T) {
	handler, authenticator, application := newInboundWebhookHTTPFixture(t)
	request := httptest.NewRequest(http.MethodPost, "/api/ai/audience/packages/42/webhook", bytes.NewReader(bytes.Repeat([]byte("x"), int(MaximumRequestBodyBytes)+1)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || authenticator.calls != 0 || application.calls != 0 {
		t.Fatalf("status=%d auth=%d app=%d", response.Code, authenticator.calls, application.calls)
	}
}

type retiredSubscriptionAuthenticatorStub struct {
	err   error
	calls int
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("body must not be read") }

var _ io.Reader = failingReader{}

func (stub *retiredSubscriptionAuthenticatorStub) Authenticate(_ context.Context, _ *http.Request) error {
	stub.calls++
	return stub.err
}

func TestRetiredOutboundSubscriptionHandlerAuthenticatesBeforeGone(t *testing.T) {
	authenticator := &retiredSubscriptionAuthenticatorStub{}
	handler, err := NewRetiredOutboundSubscriptionHandler(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/ai/audience/packages/42/outbound-subscriptions", failingReader{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGone || authenticator.calls != 1 || !strings.Contains(response.Body.String(), `"webhook_configuration_retired"`) {
		t.Fatalf("status=%d auth=%d body=%s", response.Code, authenticator.calls, response.Body.String())
	}
	authenticator.err = ErrUnauthenticated
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}
}

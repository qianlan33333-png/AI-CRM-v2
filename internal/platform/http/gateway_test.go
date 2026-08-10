package platformhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGatewayAcceptsSafeRequestIDAndReplacesUnsafeValues(t *testing.T) {
	t.Parallel()

	const generatedID = "generated-id-0001"
	tests := []struct {
		name    string
		inbound string
		wantID  string
	}{
		{name: "safe opaque identifier", inbound: "req-7a.opaque:part_1", wantID: "req-7a.opaque:part_1"},
		{name: "empty", inbound: "", wantID: generatedID},
		{name: "contains slash", inbound: "customer/secret", wantID: generatedID},
		{name: "contains newline", inbound: "request\nsecret", wantID: generatedID},
		{name: "contains non ASCII", inbound: "请求-7", wantID: generatedID},
		{name: "too long", inbound: strings.Repeat("a", 65), wantID: generatedID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logs := &bytes.Buffer{}
			gateway := mustTestGateway(t, logs, GatewayOptions{NewRequestID: func() string { return generatedID }})
			handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if got := RequestID(request.Context()); got != test.wantID {
					t.Errorf("RequestID() = %q, want %q", got, test.wantID)
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			if err != nil {
				t.Fatalf("Wrap() error = %v", err)
			}

			response := serveGateway(handler, http.MethodGet, "/contacts?email=raw@example.test", test.inbound)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
			if got := response.Header().Get(RequestIDHeader); got != test.wantID {
				t.Fatalf("%s = %q, want %q", RequestIDHeader, got, test.wantID)
			}
			entry := singleAccessLog(t, logs)
			if got := entry["request_id"]; got != test.wantID {
				t.Fatalf("logged request_id = %#v, want %q", got, test.wantID)
			}
		})
	}
}

func TestGatewayFallsBackWhenConfiguredRequestIDIsUnsafe(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{NewRequestID: func() string { return "bad/request-id" }})
	handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}

	response := serveGateway(handler, http.MethodGet, "/healthz", "bad/request-id")
	requestID := response.Header().Get(RequestIDHeader)
	if !requestIDPattern.MatchString(requestID) {
		t.Fatalf("fallback request id %q is not safe", requestID)
	}
	if requestID == "bad/request-id" {
		t.Fatal("unsafe configured request id was forwarded")
	}
}

func TestGatewayWritesStableErrorsAndFailsClosedDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    ErrorCode
		wantDetails []FieldError
	}{
		{
			name:        "valid stable validation error",
			err:         NewError(CodeValidationFailed, errors.New("database password: secret-value"), FieldError{Field: "email", Reason: "invalid"}),
			wantStatus:  http.StatusUnprocessableEntity,
			wantCode:    CodeValidationFailed,
			wantDetails: []FieldError{{Field: "email", Reason: "invalid"}},
		},
		{
			name:       "unknown error code",
			err:        NewError(ErrorCode("UNSAFE_STACK_TRACE"), errors.New("private@example.test")),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
		{
			name:       "unsafe field detail",
			err:        NewError(CodeValidationFailed, errors.New("private@example.test"), FieldError{Field: "email\nsecret", Reason: "bad-value"}),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
		{
			name:       "unclassified cause",
			err:        errors.New("sql: password=private@example.test"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logs := &bytes.Buffer{}
			gateway := mustTestGateway(t, logs, GatewayOptions{})
			handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				ResponseErrorHandler(writer, request, test.err)
			}))
			if err != nil {
				t.Fatalf("Wrap() error = %v", err)
			}

			response := serveGateway(handler, http.MethodPost, "/contacts?email=private@example.test", "stable-request-1")
			payload := decodeErrorResponse(t, response)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if payload.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", payload.Code, test.wantCode)
			}
			if payload.Message != errorSpecs[test.wantCode].message {
				t.Fatalf("message = %q, want stable message %q", payload.Message, errorSpecs[test.wantCode].message)
			}
			if payload.RequestID != "stable-request-1" || response.Header().Get(RequestIDHeader) != payload.RequestID {
				t.Fatalf("request id response/header = %q/%q, want stable-request-1", payload.RequestID, response.Header().Get(RequestIDHeader))
			}
			if got, want := fmt.Sprint(payload.Details), fmt.Sprint(test.wantDetails); got != want {
				t.Fatalf("details = %s, want %s", got, want)
			}
			assertNoSensitiveText(t, response.Body.String(), "private@example.test", "secret-value", "UNSAFE_STACK_TRACE")
			assertNoSensitiveText(t, logs.String(), "private@example.test", "secret-value", "UNSAFE_STACK_TRACE")
		})
	}
}

func TestGatewayNormalizesPanicsDirectErrorsAndOversizeBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantCode   ErrorCode
		wantStatus int
	}{
		{
			name: "panic",
			handler: func(http.ResponseWriter, *http.Request) {
				panic("credential=private@example.test")
			},
			wantCode:   CodeInternal,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "direct non 2xx body",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("X-Private-Header", "private@example.test")
				http.Error(writer, "raw error: private@example.test", http.StatusNotFound)
			},
			wantCode:   CodeNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name: "buffer overflow",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				_, _ = writer.Write(bytes.Repeat([]byte("x"), maxBufferedResponseBytes+1))
			},
			wantCode:   CodeInternal,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logs := &bytes.Buffer{}
			gateway := mustTestGateway(t, logs, GatewayOptions{RoutePattern: func(*http.Request) string { return "/contacts/{contact_id}" }})
			handler, err := gateway.Wrap(test.handler)
			if err != nil {
				t.Fatalf("Wrap() error = %v", err)
			}

			response := serveGateway(handler, http.MethodGet, "/contacts/raw-person?email=private@example.test", "fault-request-1")
			payload := decodeErrorResponse(t, response)
			if response.Code != test.wantStatus || payload.Code != test.wantCode {
				t.Fatalf("status/code = %d/%q, want %d/%q", response.Code, payload.Code, test.wantStatus, test.wantCode)
			}
			if payload.Message != errorSpecs[test.wantCode].message {
				t.Fatalf("message = %q, want %q", payload.Message, errorSpecs[test.wantCode].message)
			}
			if response.Header().Get("X-Private-Header") != "" {
				t.Fatal("direct-error private header escaped normalized response")
			}
			assertNoSensitiveText(t, response.Body.String(), "private@example.test", "raw error", "credential=")
			entry := singleAccessLog(t, logs)
			if got := entry["path"]; got != "/contacts/{contact_id}" {
				t.Fatalf("logged route = %#v, want route template", got)
			}
			if got := entry["err"]; got != string(test.wantCode) {
				t.Fatalf("logged err = %#v, want %q", got, test.wantCode)
			}
			assertNoSensitiveText(t, logs.String(), "raw-person", "private@example.test", "credential=", "raw error")
		})
	}
}

func TestGatewayUsesRouteTemplatesAndOpaqueAccountContextInStructuredLogs(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{RoutePattern: func(*http.Request) string { return "/customers/{customer_id}" }})
	handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := AccountID(request.Context()); got != "anonymous" {
			t.Fatalf("AccountID before auth = %q, want anonymous", got)
		}
		ctx, err := ContextWithAccountID(request.Context(), "acct_42")
		if err != nil {
			t.Fatalf("ContextWithAccountID() error = %v", err)
		}
		if got := AccountID(ctx); got != "acct_42" {
			t.Fatalf("AccountID(new context) = %q, want acct_42", got)
		}
		if got := AccountID(request.Context()); got != "acct_42" {
			t.Fatalf("AccountID(original context) = %q, want propagated account", got)
		}
		if _, err := ContextWithAccountID(request.Context(), "acct\nprivate@example.test"); !errors.Is(err, ErrInvalidGateway) {
			t.Fatalf("invalid account error = %v, want ErrInvalidGateway", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}

	response := serveGateway(handler, http.MethodGet, "/customers/raw-person?email=private@example.test", "account-request-1")
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	entry := singleAccessLog(t, logs)
	if got := entry["account"]; got != "acct_42" {
		t.Fatalf("logged account = %#v, want acct_42", got)
	}
	if got := entry["path"]; got != "/customers/{customer_id}" {
		t.Fatalf("logged path = %#v, want route template", got)
	}
	if got := entry["method"]; got != http.MethodGet {
		t.Fatalf("logged method = %#v, want GET", got)
	}
	if got := entry["err"]; got != "" {
		t.Fatalf("logged err = %#v, want empty stable error code", got)
	}
	assertNoSensitiveText(t, logs.String(), "raw-person", "private@example.test")
}

func TestGatewayFailsClosedForUnsafeRoutePatternAndMethodInLogs(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{
		RoutePattern: func(*http.Request) string {
			return "/customers/raw-person?email=private@example.test"
		},
	})
	handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ResponseErrorHandler(writer, request, NewError(CodeConflict, errors.New("upstream error private@example.test")))
	}))
	if err != nil {
		t.Fatalf("Wrap() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/customers/raw-person?email=private@example.test", nil)
	request.Method = "GET private@example.test"
	request.Header.Set(RequestIDHeader, "unsafe-route-request-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	entry := singleAccessLog(t, logs)
	if got := entry["path"]; got != "unmatched" {
		t.Fatalf("unsafe route logged as %#v, want unmatched", got)
	}
	if got := entry["method"]; got != "UNKNOWN" {
		t.Fatalf("unsafe method logged as %#v, want UNKNOWN", got)
	}
	if got := entry["err"]; got != string(CodeConflict) {
		t.Fatalf("logged err = %#v, want %q", got, CodeConflict)
	}
	assertNoSensitiveText(t, logs.String(), "raw-person", "private@example.test", "upstream error")
}

func TestGatewaySplitMiddlewareCompositionPreservesRequestIDForRecoveryAndLogs(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{})
	recovery, err := gateway.RecoveryErrorLog(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := RequestID(request.Context()); got != "split-request-1" {
			t.Fatalf("handler request id = %q, want split-request-1", got)
		}
		RequestErrorHandler(writer, request, errors.New("raw request parse detail"))
	}))
	if err != nil {
		t.Fatalf("RecoveryErrorLog() error = %v", err)
	}
	handler, err := gateway.RequestIDMiddleware(recovery)
	if err != nil {
		t.Fatalf("RequestIDMiddleware() error = %v", err)
	}

	response := serveGateway(handler, http.MethodPost, "/events?token=private", "split-request-1")
	payload := decodeErrorResponse(t, response)
	if response.Code != http.StatusBadRequest || payload.Code != CodeMalformedRequest {
		t.Fatalf("status/code = %d/%q, want 400/%q", response.Code, payload.Code, CodeMalformedRequest)
	}
	if payload.RequestID != "split-request-1" || response.Header().Get(RequestIDHeader) != "split-request-1" {
		t.Fatalf("response request id/header = %q/%q, want split-request-1", payload.RequestID, response.Header().Get(RequestIDHeader))
	}
	entry := singleAccessLog(t, logs)
	if got := entry["request_id"]; got != "split-request-1" {
		t.Fatalf("logged request id = %#v, want split-request-1", got)
	}
	if got := entry["err"]; got != string(CodeMalformedRequest) {
		t.Fatalf("logged err = %#v, want %q", got, CodeMalformedRequest)
	}
}

func TestGatewayInvalidConfigurationFailsClosed(t *testing.T) {
	t.Parallel()

	if _, err := NewGateway(GatewayOptions{}); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("NewGateway(empty) error = %v, want ErrInvalidGateway", err)
	}

	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	gateway, err := NewGateway(GatewayOptions{Logger: logger})
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	if _, err := gateway.Wrap(nil); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("Wrap(nil) error = %v, want ErrInvalidGateway", err)
	}
	if _, err := gateway.RequestIDMiddleware(nil); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("RequestIDMiddleware(nil) error = %v, want ErrInvalidGateway", err)
	}
	if _, err := gateway.RecoveryErrorLog(nil); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("RecoveryErrorLog(nil) error = %v, want ErrInvalidGateway", err)
	}

	var nilGateway *Gateway
	if _, err := nilGateway.Wrap(http.NotFoundHandler()); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("nil Gateway Wrap() error = %v, want ErrInvalidGateway", err)
	}
	if _, err := (&Gateway{}).Wrap(http.NotFoundHandler()); !errors.Is(err, ErrInvalidGateway) {
		t.Fatalf("zero Gateway Wrap() error = %v, want ErrInvalidGateway", err)
	}
	if got := RequestID(nil); got != "" {
		t.Fatalf("RequestID(nil) = %q, want empty", got)
	}
	if got := AccountID(nil); got != "anonymous" {
		t.Fatalf("AccountID(nil) = %q, want anonymous", got)
	}
	if ctx, err := ContextWithAccountID(context.Background(), "invalid/account"); !errors.Is(err, ErrInvalidGateway) || ctx != nil {
		t.Fatalf("invalid ContextWithAccountID() = %#v, %v; want nil, ErrInvalidGateway", ctx, err)
	}
}

func mustTestGateway(t *testing.T, logs *bytes.Buffer, options GatewayOptions) *Gateway {
	t.Helper()
	if options.Logger == nil {
		options.Logger = slog.New(slog.NewJSONHandler(logs, nil))
	}
	if options.Clock == nil {
		start := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
		options.Clock = func() time.Time { return start }
	}
	gateway, err := NewGateway(options)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	return gateway
}

func serveGateway(handler http.Handler, method, target, requestID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	if requestID != "" {
		request.Header.Set(RequestIDHeader, requestID)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeErrorResponse(t *testing.T, response *httptest.ResponseRecorder) errorResponse {
	t.Helper()
	var payload errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body %q: %v", response.Body.String(), err)
	}
	return payload
}

func singleAccessLog(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(logs.String()))
	var entry map[string]any
	if err := decoder.Decode(&entry); err != nil {
		t.Fatalf("decode access log %q: %v", logs.String(), err)
	}
	if decoder.More() {
		t.Fatalf("expected one structured access log, got %q", logs.String())
	}
	if got := entry["msg"]; got != "http_access" {
		t.Fatalf("log msg = %#v, want http_access", got)
	}
	return entry
}

func assertNoSensitiveText(t *testing.T, value string, forbidden ...string) {
	t.Helper()
	for _, text := range forbidden {
		if strings.Contains(value, text) {
			t.Fatalf("unexpected sensitive text %q in %q", text, value)
		}
	}
}

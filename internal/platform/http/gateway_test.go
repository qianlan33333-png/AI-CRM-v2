package platformhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestGatewayRecoveryResponseBufferLimitsAreBoundedAtRegistration(t *testing.T) {
	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{})
	writeBytes := func(length int) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(bytes.Repeat([]byte("x"), length))
		})
	}
	serve := func(handler http.Handler) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/buffer-limit", nil))
		return response
	}

	defaultAtLimit, err := gateway.RecoveryErrorLog(writeBytes(maxBufferedResponseBytes))
	if err != nil {
		t.Fatalf("default recovery error = %v", err)
	}
	if response := serve(defaultAtLimit); response.Code != http.StatusOK || response.Body.Len() != maxBufferedResponseBytes {
		t.Fatalf("default at limit status/body=%d/%d", response.Code, response.Body.Len())
	}
	defaultOverLimit, err := gateway.RecoveryErrorLog(writeBytes(maxBufferedResponseBytes + 1))
	if err != nil {
		t.Fatalf("default overflow recovery error = %v", err)
	}
	if response := serve(defaultOverLimit); response.Code != http.StatusInternalServerError || decodeErrorResponse(t, response).Code != CodeInternal {
		t.Fatalf("default over limit status/body=%d/%d", response.Code, response.Body.Len())
	}

	customAtLimit, err := gateway.RecoveryErrorLogWithResponseBufferLimit(writeBytes(maxRouteResponseBufferBytes), maxRouteResponseBufferBytes)
	if err != nil {
		t.Fatalf("custom recovery error = %v", err)
	}
	if response := serve(customAtLimit); response.Code != http.StatusOK || response.Body.Len() != maxRouteResponseBufferBytes {
		t.Fatalf("custom at limit status/body=%d/%d", response.Code, response.Body.Len())
	}
	customOverLimit, err := gateway.RecoveryErrorLogWithResponseBufferLimit(writeBytes(maxRouteResponseBufferBytes+1), maxRouteResponseBufferBytes)
	if err != nil {
		t.Fatalf("custom overflow recovery error = %v", err)
	}
	if response := serve(customOverLimit); response.Code != http.StatusInternalServerError || decodeErrorResponse(t, response).Code != CodeInternal {
		t.Fatalf("custom over limit status/body=%d/%d", response.Code, response.Body.Len())
	}

	for _, limit := range []int{-1, 0, maxRouteResponseBufferBytes + 1} {
		if _, err := gateway.RecoveryErrorLogWithResponseBufferLimit(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), limit); !errors.Is(err, ErrInvalidGateway) {
			t.Fatalf("limit %d error = %v, want ErrInvalidGateway", limit, err)
		}
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

func TestGatewayPreservesBrowserRedirectResponses(t *testing.T) {
	t.Parallel()

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{RoutePattern: func(*http.Request) string { return "/login" }})
	handler, err := gateway.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		http.Redirect(writer, request, "/admin", http.StatusFound)
	}))
	if err != nil {
		t.Fatal(err)
	}
	response := serveGateway(handler, http.MethodGet, "/login", "redirect-request-1")
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redirect status/location/cache=%d/%q/%q body=%s", response.Code, response.Header().Get("Location"), response.Header().Get("Cache-Control"), response.Body.String())
	}
	entry := singleAccessLog(t, logs)
	if entry["status"] != float64(http.StatusFound) || entry["err"] != "" {
		t.Fatalf("redirect log = %#v", entry)
	}
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

func TestGatewayAccountBudgetDefaultsToFourAndIsolatesAccounts(t *testing.T) {
	if DefaultMaxConcurrentPerAccount != 4 {
		t.Fatalf("DefaultMaxConcurrentPerAccount = %d, want 4", DefaultMaxConcurrentPerAccount)
	}

	const (
		limitedAccount  = "acct_limited"
		isolatedAccount = "acct_isolated"
	)
	gateway := mustTestGateway(t, &bytes.Buffer{}, GatewayOptions{
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	entered := make(chan struct{}, DefaultMaxConcurrentPerAccount)
	release := make(chan struct{})
	accepted := make(chan *httptest.ResponseRecorder, DefaultMaxConcurrentPerAccount)
	pendingAccepted := DefaultMaxConcurrentPerAccount
	released := false
	defer func() {
		if !released {
			close(release)
		}
		for pendingAccepted > 0 {
			select {
			case <-accepted:
				pendingAccepted--
			case <-time.After(time.Second):
				t.Errorf("accepted requests did not finish after release")
				return
			}
		}
	}()

	handler := composeGatewayHandler(t, gateway, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if AccountID(request.Context()) == limitedAccount {
			entered <- struct{}{}
			<-release
		}
		writer.WriteHeader(http.StatusNoContent)
	}), true, false)

	for index := 0; index < DefaultMaxConcurrentPerAccount; index++ {
		request := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", fmt.Sprintf("limited-request-%d", index), limitedAccount)
		go func(request *http.Request) {
			accepted <- serveGatewayRequest(handler, request)
		}(request)
	}
	for index := 0; index < DefaultMaxConcurrentPerAccount; index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatalf("request %d did not acquire one of the default budget slots", index+1)
		}
	}

	limitedResponse := make(chan *httptest.ResponseRecorder, 1)
	limitedRequest := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", "limited-request-5", limitedAccount)
	go func() {
		limitedResponse <- serveGatewayRequest(handler, limitedRequest)
	}()
	select {
	case response := <-limitedResponse:
		assertGatewayResponse(t, response, http.StatusTooManyRequests, CodeConcurrencyLimited, "limited-request-5")
	case <-time.After(time.Second):
		t.Fatal("fifth request waited instead of receiving an immediate 429")
	}

	isolatedResponse := make(chan *httptest.ResponseRecorder, 1)
	isolatedRequest := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", "isolated-request-1", isolatedAccount)
	go func() {
		isolatedResponse <- serveGatewayRequest(handler, isolatedRequest)
	}()
	select {
	case response := <-isolatedResponse:
		assertGatewayResponse(t, response, http.StatusNoContent, "", "isolated-request-1")
	case <-time.After(time.Second):
		t.Fatal("different account was blocked by the saturated account budget")
	}

	close(release)
	released = true
	for pendingAccepted > 0 {
		select {
		case response := <-accepted:
			pendingAccepted--
			assertGatewayResponse(t, response, http.StatusNoContent, "", response.Header().Get(RequestIDHeader))
		case <-time.After(time.Second):
			t.Fatal("accepted request did not finish after release")
		}
	}
}

func TestGatewayAccountBudgetShortCircuitsAreLoggedOnceByRequestIDMiddleware(t *testing.T) {
	tests := []struct {
		name              string
		accountID         string
		holdBudgetSlot    bool
		requestID         string
		wantStatus        int
		wantCode          ErrorCode
		wantLoggedAccount string
		wantHandlerCalls  int32
	}{
		{
			name:              "concurrency limit",
			accountID:         "acct_short_circuit",
			holdBudgetSlot:    true,
			requestID:         "short-circuit-429",
			wantStatus:        http.StatusTooManyRequests,
			wantCode:          CodeConcurrencyLimited,
			wantLoggedAccount: "acct_short_circuit",
			wantHandlerCalls:  1,
		},
		{
			name:              "missing account context",
			requestID:         "short-circuit-401",
			wantStatus:        http.StatusUnauthorized,
			wantCode:          CodeUnauthenticated,
			wantLoggedAccount: "anonymous",
			wantHandlerCalls:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs := &bytes.Buffer{}
			gateway := mustTestGateway(t, logs, GatewayOptions{MaxConcurrentPerAccount: 1})
			var handlerCalls atomic.Int32
			started := make(chan struct{}, 1)
			release := make(chan struct{})
			heldResponse := make(chan *httptest.ResponseRecorder, 1)
			released := false
			heldFinished := false
			if test.holdBudgetSlot {
				defer func() {
					if !released {
						close(release)
					}
					if !heldFinished {
						select {
						case <-heldResponse:
						case <-time.After(time.Second):
							t.Errorf("held request did not finish after release")
						}
					}
				}()
			}

			handler := composeGatewayHandler(t, gateway, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				handlerCalls.Add(1)
				if test.holdBudgetSlot {
					started <- struct{}{}
					<-release
				}
				writer.WriteHeader(http.StatusNoContent)
			}), true, false)

			if test.holdBudgetSlot {
				heldRequest := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", "held-short-circuit", test.accountID)
				go func() {
					heldResponse <- serveGatewayRequest(handler, heldRequest)
				}()
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("first request did not acquire the only budget slot")
				}
			}

			request := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", test.requestID, test.accountID)
			response := serveGatewayRequest(handler, request)
			assertGatewayResponse(t, response, test.wantStatus, test.wantCode, test.requestID)
			if got := handlerCalls.Load(); got != test.wantHandlerCalls {
				t.Fatalf("handler calls = %d, want %d", got, test.wantHandlerCalls)
			}

			entry := singleAccessLog(t, logs)
			if got := entry["request_id"]; got != test.requestID {
				t.Fatalf("logged request_id = %#v, want %q", got, test.requestID)
			}
			if got := entry["status"]; got != float64(test.wantStatus) {
				t.Fatalf("logged status = %#v, want %d", got, test.wantStatus)
			}
			if got := entry["err"]; got != string(test.wantCode) {
				t.Fatalf("logged err = %#v, want %q", got, test.wantCode)
			}
			if got := entry["account"]; got != test.wantLoggedAccount {
				t.Fatalf("logged account = %#v, want %q", got, test.wantLoggedAccount)
			}

			if test.holdBudgetSlot {
				close(release)
				released = true
				select {
				case response := <-heldResponse:
					heldFinished = true
					assertGatewayResponse(t, response, http.StatusNoContent, "", "held-short-circuit")
				case <-time.After(time.Second):
					t.Fatal("held request did not finish after release")
				}
			}
		})
	}
}

func TestGatewayAccountBudgetReleasesSlotsAfterTerminalResponses(t *testing.T) {
	tests := []struct {
		name       string
		requestKey string
		handler    http.HandlerFunc
		wantStatus int
		wantCode   ErrorCode
	}{
		{
			name:       "normal response",
			requestKey: "normal",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusNoContent)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "handled error",
			requestKey: "error",
			handler: func(writer http.ResponseWriter, request *http.Request) {
				ResponseErrorHandler(writer, request, NewError(CodeConflict, errors.New("expected test error")))
			},
			wantStatus: http.StatusConflict,
			wantCode:   CodeConflict,
		},
		{
			name:       "panic recovery",
			requestKey: "panic",
			handler: func(http.ResponseWriter, *http.Request) {
				panic("expected test panic")
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := mustTestGateway(t, &bytes.Buffer{}, GatewayOptions{
				Logger:                  slog.New(slog.NewJSONHandler(io.Discard, nil)),
				MaxConcurrentPerAccount: 1,
			})
			var handlerCalls atomic.Int32
			handler := composeGatewayHandler(t, gateway, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				handlerCalls.Add(1)
				test.handler(writer, request)
			}), true, false)

			for attempt := 1; attempt <= 2; attempt++ {
				requestID := fmt.Sprintf("release-%s-%d", test.requestKey, attempt)
				request := mustGatewayRequestForAccount(t, http.MethodGet, "/contacts", requestID, "acct_release")
				response := serveGatewayRequest(handler, request)
				assertGatewayResponse(t, response, test.wantStatus, test.wantCode, requestID)
			}
			if got := handlerCalls.Load(); got != 2 {
				t.Fatalf("handler calls = %d, want 2; slot was not released after %s", got, test.name)
			}
		})
	}
}

func TestGatewayRequestTimeoutDefaultsAndRecoveryNormalizesDeadline(t *testing.T) {
	if DefaultRequestTimeout != 10*time.Second {
		t.Fatalf("DefaultRequestTimeout = %s, want 10s", DefaultRequestTimeout)
	}

	tests := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{name: "default", want: DefaultRequestTimeout},
		{name: "short injected duration", input: 5 * time.Millisecond, want: 5 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := mustTestGateway(t, &bytes.Buffer{}, GatewayOptions{
				Logger:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
				RequestTimeout: test.input,
			})
			if got := gateway.requestTimeout; got != test.want {
				t.Fatalf("gateway request timeout = %s, want %s", got, test.want)
			}
		})
	}

	logs := &bytes.Buffer{}
	gateway := mustTestGateway(t, logs, GatewayOptions{RequestTimeout: 5 * time.Millisecond})
	handler := composeGatewayHandler(t, gateway, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
		if !errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			t.Errorf("request context error = %v, want deadline exceeded", request.Context().Err())
		}
	}), false, true)
	response := serveGateway(handler, http.MethodGet, "/contacts", "timeout-request-1")
	assertGatewayResponse(t, response, http.StatusServiceUnavailable, CodeDependencyUnavailable, "timeout-request-1")
	entry := singleAccessLog(t, logs)
	if got := entry["status"]; got != float64(http.StatusServiceUnavailable) {
		t.Fatalf("logged status = %#v, want %d", got, http.StatusServiceUnavailable)
	}
	if got := entry["err"]; got != string(CodeDependencyUnavailable) {
		t.Fatalf("logged err = %#v, want %q", got, CodeDependencyUnavailable)
	}
}

func TestGatewayRejectsInvalidBudgetAndTimeoutConfiguration(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	tests := []struct {
		name    string
		options GatewayOptions
	}{
		{name: "negative max concurrent", options: GatewayOptions{MaxConcurrentPerAccount: -1}},
		{name: "negative timeout", options: GatewayOptions{RequestTimeout: -time.Nanosecond}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.options.Logger = logger
			if _, err := NewGateway(test.options); !errors.Is(err, ErrInvalidGateway) {
				t.Fatalf("NewGateway(%+v) error = %v, want ErrInvalidGateway", test.options, err)
			}
		})
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
	return serveGatewayRequest(handler, newGatewayRequest(method, target, requestID))
}

func composeGatewayHandler(t *testing.T, gateway *Gateway, next http.Handler, withAccountBudget, withTimeout bool) http.Handler {
	t.Helper()
	handler, err := gateway.RecoveryErrorLog(next)
	if err != nil {
		t.Fatalf("RecoveryErrorLog() error = %v", err)
	}
	if withTimeout {
		handler, err = gateway.TimeoutMiddleware(handler)
		if err != nil {
			t.Fatalf("TimeoutMiddleware() error = %v", err)
		}
	}
	if withAccountBudget {
		handler, err = gateway.AccountBudgetMiddleware(handler)
		if err != nil {
			t.Fatalf("AccountBudgetMiddleware() error = %v", err)
		}
	}
	handler, err = gateway.RequestIDMiddleware(handler)
	if err != nil {
		t.Fatalf("RequestIDMiddleware() error = %v", err)
	}
	return handler
}

func mustGatewayRequestForAccount(t *testing.T, method, target, requestID, accountID string) *http.Request {
	t.Helper()
	request := newGatewayRequest(method, target, requestID)
	if accountID == "" {
		return request
	}
	accountContext, err := ContextWithAccountID(request.Context(), accountID)
	if err != nil {
		t.Fatalf("ContextWithAccountID(%q) error = %v", accountID, err)
	}
	return request.WithContext(accountContext)
}

func newGatewayRequest(method, target, requestID string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if requestID != "" {
		request.Header.Set(RequestIDHeader, requestID)
	}
	return request
}

func serveGatewayRequest(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertGatewayResponse(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode ErrorCode, wantRequestID string) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d", response.Code, wantStatus)
	}
	if got := response.Header().Get(RequestIDHeader); got != wantRequestID {
		t.Fatalf("%s = %q, want %q", RequestIDHeader, got, wantRequestID)
	}
	if wantCode == "" {
		return
	}
	payload := decodeErrorResponse(t, response)
	if payload.Code != wantCode {
		t.Fatalf("error code = %q, want %q", payload.Code, wantCode)
	}
	if payload.RequestID != wantRequestID {
		t.Fatalf("error request id = %q, want %q", payload.RequestID, wantRequestID)
	}
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
	var extra map[string]any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
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

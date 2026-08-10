package p2s08

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type errorBody struct {
	Code      string                    `json:"code"`
	Message   string                    `json:"message"`
	RequestID string                    `json:"request_id"`
	Details   []platformhttp.FieldError `json:"details"`
}

func TestGatewayBlackBox(t *testing.T) {
	t.Parallel()

	t.Run("stable error and request id", func(t *testing.T) {
		response, logs := serve(t, "trace-123", "/contacts/secret-phone?token=private", func(writer http.ResponseWriter, request *http.Request) {
			ctx, err := platformhttp.ContextWithAccountID(request.Context(), "acct_42")
			if err != nil {
				t.Fatal(err)
			}
			request = request.WithContext(ctx)
			platformhttp.WriteError(writer, request, platformhttp.NewError(
				platformhttp.CodeValidationFailed,
				errors.New("sql and private-phone must stay private"),
				platformhttp.FieldError{Field: "contact.phone", Reason: "invalid_format"},
			))
		})
		assertError(t, response, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "trace-123")
		if response.Header().Get(platformhttp.RequestIDHeader) != "trace-123" {
			t.Fatalf("response request id = %q", response.Header().Get(platformhttp.RequestIDHeader))
		}
		assertNoLeak(t, response.Body.String(), "sql", "private-phone")
		entry := oneLog(t, logs)
		assertLog(t, entry, "trace-123", "acct_42", "/contacts/{contact_id}", 422, "VALIDATION_FAILED")
		assertNoLeak(t, logs, "secret-phone", "token", "private", "sql", "private-phone")
	})

	t.Run("panic is recovered without disclosure", func(t *testing.T) {
		response, logs := serve(t, "bad\nrequest-id", "/panic?secret=credential", func(http.ResponseWriter, *http.Request) {
			panic("stack credential phone-13800000000")
		})
		body := assertError(t, response, http.StatusInternalServerError, "INTERNAL_ERROR", "generated-id")
		if body.RequestID == "bad\nrequest-id" {
			t.Fatal("unsafe inbound request id was retained")
		}
		assertNoLeak(t, response.Body.String(), "stack", "credential", "13800000000")
		assertNoLeak(t, logs, "secret", "credential", "13800000000")
	})

	t.Run("raw non success response is normalized", func(t *testing.T) {
		response, logs := serve(t, "trace-404", "/missing/private-email@example.com", func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "private-email@example.com", http.StatusNotFound)
		})
		assertError(t, response, http.StatusNotFound, "NOT_FOUND", "trace-404")
		assertNoLeak(t, response.Body.String(), "private-email")
		assertNoLeak(t, logs, "private-email")
	})

	t.Run("oversized response fails closed", func(t *testing.T) {
		response, _ := serve(t, "trace-large", "/large", func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write(bytes.Repeat([]byte("x"), 8<<20+1))
		})
		assertError(t, response, http.StatusInternalServerError, "INTERNAL_ERROR", "trace-large")
	})
}

func serve(t *testing.T, requestID, target string, handler http.HandlerFunc) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	gateway, err := platformhttp.NewGateway(platformhttp.GatewayOptions{
		Logger: logger,
		NewRequestID: func() string {
			return "generated-id"
		},
		RoutePattern: func(*http.Request) string {
			if strings.HasPrefix(target, "/contacts/") {
				return "/contacts/{contact_id}"
			}
			return "/test"
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := gateway.Wrap(handler)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set(platformhttp.RequestIDHeader, requestID)
	response := httptest.NewRecorder()
	wrapped.ServeHTTP(response, request)
	return response, logBuffer.String()
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code, requestID string) errorBody {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body errorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != code {
		t.Fatalf("code = %q", body.Code)
	}
	if requestID == "generated-id" {
		if body.RequestID != requestID {
			t.Fatalf("generated request id = %q", body.RequestID)
		}
	} else if body.RequestID != requestID {
		t.Fatalf("request id = %q", body.RequestID)
	}
	if body.Message == "" {
		t.Fatal("message must be stable and non-empty")
	}
	return body
}

func oneLog(t *testing.T, logs string) map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(logs))
	if !scanner.Scan() {
		t.Fatal("missing access log")
	}
	var entry map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if scanner.Scan() {
		t.Fatal("expected exactly one access log")
	}
	return entry
}

func assertLog(t *testing.T, entry map[string]any, requestID, account, path string, status int, code string) {
	t.Helper()
	wants := map[string]any{
		"msg": "http_access", "request_id": requestID, "account": account,
		"method": "GET", "path": path, "status": float64(status), "err": code,
	}
	for key, want := range wants {
		if got := entry[key]; got != want {
			t.Fatalf("log %s = %#v, want %#v", key, got, want)
		}
	}
	if _, ok := entry["latency_ms"]; !ok {
		t.Fatal("log is missing latency_ms")
	}
}

func assertNoLeak(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			t.Fatalf("output leaked %q: %s", secret, value)
		}
	}
}

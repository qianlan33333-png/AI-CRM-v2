package main

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type outboundTaskHistoryAPIReader struct {
	calls                   int
	empty, invalid, wrongID bool
	err                     error
}

func outboundTaskHistoryAPIValue(id int64) outboundport.HistoricalOutboundTask {
	digest := sha256.Sum256([]byte("test-only-sealed-task"))
	parent, legacy := int64(7), int64(-42)
	return outboundport.HistoricalOutboundTask{
		ID: id, SourceID: -1, TaskType: "", Status: "legacy_sending", CreatedAt: time.Date(2026, 8, 28, 0, 0, 0, 123456000, time.UTC),
		BroadcastJobHistoryID: &parent, LegacyBroadcastJobID: &legacy,
		RequestPayloadDigest: digest, ResponsePayloadDigest: digest, WeComTaskIDDigest: &digest, TraceIDDigest: digest,
		SourceKeyDigest: digest, SourcePayloadDigest: digest, SourceFieldDigest: digest, RedactedRoots: []string{"request_payload"},
	}
}

func (reader *outboundTaskHistoryAPIReader) GetHistoricalOutboundTask(_ context.Context, id int64) (outboundport.HistoricalOutboundTask, error) {
	reader.calls++
	value := outboundTaskHistoryAPIValue(id)
	if reader.invalid {
		value.SourceKeyDigest = [32]byte{}
	}
	if reader.wrongID {
		value.ID++
	}
	return value, reader.err
}

func (reader *outboundTaskHistoryAPIReader) ListHistoricalOutboundTasks(ctx context.Context, _ outboundport.OutboundTaskHistoryQuery) ([]outboundport.HistoricalOutboundTask, int64, error) {
	if reader.empty {
		reader.calls++
		return nil, 0, reader.err
	}
	value, err := reader.GetHistoricalOutboundTask(ctx, 11)
	return []outboundport.HistoricalOutboundTask{value}, 1, err
}

func outboundTaskHistoryAPIRouter(t *testing.T, reader outboundport.OutboundTaskHistoryReader, role authport.Role) (http.Handler, *broadcastJobHistoryAPIAuth) {
	t.Helper()
	auth := &broadcastJobHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.outboundTaskHistory = reader
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router, auth
}

func TestFinalRouterOutboundTaskHistoryReadOnly(t *testing.T) {
	reader := &outboundTaskHistoryAPIReader{}
	router, auth := outboundTaskHistoryAPIRouter(t, reader, authport.RoleAdmin)
	for detail, path := range map[bool]string{false: "/api/admin/outbound-task-history?limit=1&offset=0", true: "/api/admin/outbound-task-history/12"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(0xc1)))
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read status=%d", response.Code)
		}
		body := broadcastJobHistoryBody(t, response)
		if body["read_only"] != true || body["source"] != "v1_history" || body["real_external_call_executed"] != false {
			t.Fatal("history boundary")
		}
		item := broadcastJobHistoryItem(t, body, detail)
		if len(item) != 6 || item["source_id"] != float64(-1) || item["task_type"] != "" || item["status"] != "legacy_sending" || item["broadcast_job_history_id"] != float64(7) {
			t.Fatal("source facts or verified parent changed")
		}
		for _, private := range []string{"digest", "redacted_roots", "legacy_broadcast_job_id", "request_payload", "response_payload", "trace_id", "wecom_task_id"} {
			if strings.Contains(response.Body.String(), private) {
				t.Fatal("private history field leaked")
			}
		}
	}
	if auth.csrfCalls != 0 || len(auth.capabilities) != 2 {
		t.Fatal("read authorization changed")
	}
	reader.empty = true
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/outbound-task-history", legacyToken(0xc2)))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatal("empty history")
	}
}

func TestFinalRouterOutboundTaskHistoryRejectsInvalidAndDenied(t *testing.T) {
	for _, role := range []authport.Role{"", authport.RoleOps} {
		reader := &outboundTaskHistoryAPIReader{}
		router, _ := outboundTaskHistoryAPIRouter(t, reader, role)
		request := httptest.NewRequest(http.MethodGet, "/api/admin/outbound-task-history", nil)
		want := 401
		if role != "" {
			request = legacyRequest(http.MethodGet, request.URL.String(), legacyToken(0xc3))
			want = 403
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != want || reader.calls != 0 {
			t.Fatal("denied access reached reader")
		}
	}
	reader := &outboundTaskHistoryAPIReader{}
	router, _ := outboundTaskHistoryAPIRouter(t, reader, authport.RoleAdmin)
	for _, suffix := range []string{"?limit=0", "?limit=101", "?limit=1&limit=2", "?offset=-1", "?unknown=1", "?limit=%zz", "/0", "/-1", "/01", "/x", "/9223372036854775808", "/12?limit=1"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/outbound-task-history"+suffix, legacyToken(0xc4)))
		if response.Code != 400 || reader.calls != 0 {
			t.Fatalf("invalid query %s status=%d", suffix, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodPost, "/api/admin/outbound-task-history", legacyToken(0xc5)))
	if response.Code >= 200 && response.Code < 300 || reader.calls != 0 {
		t.Fatal("history became writable")
	}
}

func TestFinalRouterOutboundTaskHistoryFailsClosed(t *testing.T) {
	for _, reader := range []outboundport.OutboundTaskHistoryReader{nil, (*outboundTaskHistoryAPIReader)(nil), &outboundTaskHistoryAPIReader{invalid: true}, &outboundTaskHistoryAPIReader{wrongID: true}, &outboundTaskHistoryAPIReader{err: outboundport.ErrOutboundTaskHistoryUnavailable}} {
		router, _ := outboundTaskHistoryAPIRouter(t, reader, authport.RoleAdmin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/admin/outbound-task-history/12", legacyToken(0xc6)))
		if response.Code != 503 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("invalid history did not fail closed")
		}
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

type hxcChatJobAPIStub struct {
	items []hxcport.HistoricalHXCChatJob
	total int64
	err   error
	calls int
	query hxcport.HXCChatJobHistoryQuery
}

func (s *hxcChatJobAPIStub) GetHistoricalHXCChatJob(context.Context, int64) (hxcport.HistoricalHXCChatJob, error) {
	s.calls++
	if len(s.items) == 0 {
		return hxcport.HistoricalHXCChatJob{}, s.err
	}
	return s.items[0], s.err
}
func (s *hxcChatJobAPIStub) ListHistoricalHXCChatJob(_ context.Context, q hxcport.HXCChatJobHistoryQuery) ([]hxcport.HistoricalHXCChatJob, int64, error) {
	s.calls++
	s.query = q
	return s.items, s.total, s.err
}
func hxcChatJobFixture() *hxcChatJobAPIStub {
	at := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	n := int64(-3)
	return &hxcChatJobAPIStub{total: 1, items: []hxcport.HistoricalHXCChatJob{{
		ID: 7, SourceID: -9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, SourceFieldDigest: [32]byte{3},
		QueueSourceID: &n, RequestPayloadJSON: json.RawMessage(`null`), AcceptedPayloadJSON: json.RawMessage(`{}`), CallbackPayloadJSON: json.RawMessage(`[]`), SendResultJSON: json.RawMessage(`null`),
		ExternalContactID: "private-contact", Phone: "private-phone", ExternalMessageID: "private-message", ExternalSessionID: "private-session", LaohuangTaskID: "private-task",
		ReplyText: "private-reply", ErrorCode: "private-code", ErrorMessage: "private-error", OriginalStatus: "queued", SendChannel: "wecom", CreatedAt: at, UpdatedAt: at, FinishedAtSource: "source-text-not-a-time",
	}}}
}
func hxcChatJobRouter(t *testing.T, reader hxcport.HXCChatJobHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcChatJobHistory = reader
	a, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), a, a, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func TestHXCChatJobHistoryRoutes(t *testing.T) {
	request := func(router http.Handler, method, suffix, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, legacyRequest(method, "/api/admin/hxc-history/chat-jobs"+suffix, token))
		return w
	}
	s := hxcChatJobFixture()
	router := hxcChatJobRouter(t, s, authport.RoleAdmin)
	for _, suffix := range []string{"?limit=1&offset=0", "/7"} {
		w := request(router, "GET", suffix, legacyToken(101))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read %s: %d %s", suffix, w.Code, w.Body.String())
		}
		for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":-9`, `"queue_source_id":-3`, `"member_source_id":null`, `"finished_at_source":"source-text-not-a-time"`} {
			if !strings.Contains(w.Body.String(), want) {
				t.Fatalf("missing %s", want)
			}
		}
		for _, hidden := range []string{"private-", "digest", "payload", "phone", "external_contact", "external_message", "external_session", "laohuang_task", "reply_text", "error_code", "error_message", "send_result"} {
			if strings.Contains(w.Body.String(), hidden) {
				t.Fatalf("private data exposed: %s", hidden)
			}
		}
	}
	if s.query != (hxcport.HXCChatJobHistoryQuery{Limit: 1}) {
		t.Fatal("pagination not forwarded")
	}
	for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=1&limit=2", "?customer_id=1", "?source_table=x", "?unknown=1", "?limit=%zz", "/0", "/01", "/7?limit=1"} {
		if w := request(router, "GET", suffix, legacyToken(101)); w.Code != 400 {
			t.Fatalf("invalid %s: %d", suffix, w.Code)
		}
	}
	if s.calls != 2 {
		t.Fatal("invalid input reached reader")
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if w := request(router, method, "", legacyToken(101)); w.Code >= 200 && w.Code < 300 {
			t.Fatal("write accepted")
		}
	}
	for _, tc := range []struct {
		role   authport.Role
		token  string
		status int
	}{{authport.RoleAdmin, "", 401}, {authport.Role("ops"), legacyToken(101), 403}} {
		b := hxcChatJobFixture()
		if w := request(hxcChatJobRouter(t, b, tc.role), "GET", "", tc.token); w.Code != tc.status || b.calls != 0 {
			t.Fatal("authorization boundary failed")
		}
	}
	for _, missing := range []hxcport.HXCChatJobHistoryReader{nil, (*hxcChatJobAPIStub)(nil)} {
		if w := request(hxcChatJobRouter(t, missing, authport.RoleAdmin), "GET", "", legacyToken(101)); w.Code != 503 {
			t.Fatal("nil reader accepted")
		}
	}
	for _, state := range []string{"error", "count", "hmac", "json", "wrong-id", "duplicate"} {
		b := hxcChatJobFixture()
		suffix := ""
		switch state {
		case "error":
			b.err = errors.New("private-downstream")
		case "count":
			b.total = 2
		case "hmac":
			b.items[0].SourceFieldDigest = [32]byte{}
		case "json":
			b.items[0].RequestPayloadJSON = nil
		case "wrong-id":
			suffix = "/8"
		case "duplicate":
			b.items = append(b.items, b.items[0])
			b.total = 2
		}
		w := request(hxcChatJobRouter(t, b, authport.RoleAdmin), "GET", suffix, legacyToken(101))
		if w.Code != 503 || strings.Contains(w.Body.String(), "private-downstream") {
			t.Fatalf("bad %s: %d", state, w.Code)
		}
	}
	s.items = nil
	s.total = 0
	if w := request(router, "GET", "", legacyToken(101)); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("empty response failed")
	}
}

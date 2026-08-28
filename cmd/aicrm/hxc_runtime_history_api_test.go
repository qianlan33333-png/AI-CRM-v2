package main

import (
	"context"
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

type hxcRuntimeAPIStub struct {
	calls  int
	err    error
	empty  bool
	total  int64
	sender hxcport.HistoricalHXCSenderConfig
	record hxcport.HistoricalHXCSendRecord
}

func (s *hxcRuntimeAPIStub) GetHistoricalHXCSenderConfig(context.Context, int64) (hxcport.HistoricalHXCSenderConfig, error) {
	s.calls++
	return s.sender, s.err
}
func (s *hxcRuntimeAPIStub) ListHistoricalHXCSenderConfig(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCSenderConfig, int64, error) {
	s.calls++
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCSenderConfig{s.sender}, s.total, s.err
}
func (s *hxcRuntimeAPIStub) GetHistoricalHXCSendRecord(context.Context, int64) (hxcport.HistoricalHXCSendRecord, error) {
	s.calls++
	return s.record, s.err
}
func (s *hxcRuntimeAPIStub) ListHistoricalHXCSendRecord(context.Context, hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCSendRecord, int64, error) {
	s.calls++
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCSendRecord{s.record}, s.total, s.err
}
func hxcRuntimeFixture() *hxcRuntimeAPIStub {
	id := hxcport.HistoricalHXCRuntimeIdentity{ID: 7, SourceID: -9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, SourceFieldDigest: [32]byte{3}, PrivateDigest: [32]byte{4}}
	at := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	return &hxcRuntimeAPIStub{total: 1, sender: hxcport.HistoricalHXCSenderConfig{HistoricalHXCRuntimeIdentity: id, Priority: -1, CreatedAt: at, UpdatedAt: at}, record: hxcport.HistoricalHXCSendRecord{HistoricalHXCRuntimeIdentity: id, SentCount: -2, CreatedAt: at}}
}
func hxcRuntimeRouter(t *testing.T, reader hxcport.HXCRuntimeHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcRuntimeHistory = reader
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
func TestHXCRuntimeHistoryRoutes(t *testing.T) {
	for _, kind := range []string{"sender-configs", "send-records"} {
		t.Run(kind, func(t *testing.T) {
			path := "/api/admin/hxc-history/" + kind
			s := hxcRuntimeFixture()
			router := hxcRuntimeRouter(t, s, authport.RoleAdmin)
			request := func(handler http.Handler, method, suffix, token string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, legacyRequest(method, path+suffix, token))
				return w
			}
			for _, suffix := range []string{"?limit=1&offset=0", "/7"} {
				w := request(router, "GET", suffix, legacyToken(101))
				if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("read %s: %d %s", suffix, w.Code, w.Body.String())
				}
				for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":-9`} {
					if !strings.Contains(w.Body.String(), want) {
						t.Fatalf("missing %s", want)
					}
				}
				for _, private := range []string{"digest", "sender_userid", "target_unionids", "content_preview", "task_results", "idempotency_key"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatalf("private field leaked: %s", private)
					}
				}
			}
			for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=1&limit=2", "?customer_id=1", "?source_table=x", "?unknown=1", "?limit=%zz", "/0", "/01", "/7?limit=1"} {
				if w := request(router, "GET", suffix, legacyToken(101)); w.Code != 400 {
					t.Fatalf("invalid %s: %d", suffix, w.Code)
				}
			}
			if s.calls != 2 {
				t.Fatal("invalid request reached domain")
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
				b := hxcRuntimeFixture()
				if w := request(hxcRuntimeRouter(t, b, tc.role), "GET", "", tc.token); w.Code != tc.status || b.calls != 0 {
					t.Fatal("authorization boundary failed")
				}
			}
			for _, missing := range []hxcport.HXCRuntimeHistoryReader{nil, (*hxcRuntimeAPIStub)(nil)} {
				if w := request(hxcRuntimeRouter(t, missing, authport.RoleAdmin), "GET", "", legacyToken(101)); w.Code != 503 {
					t.Fatal("nil reader accepted")
				}
			}
			for _, state := range []string{"error", "count", "private-digest", "field-digest", "wrong-id"} {
				b := hxcRuntimeFixture()
				suffix := ""
				switch state {
				case "error":
					b.err = errors.New("private-downstream")
				case "count":
					b.total = 2
				case "private-digest":
					b.sender.PrivateDigest = [32]byte{}
					b.record.PrivateDigest = [32]byte{}
				case "field-digest":
					b.sender.SourceFieldDigest = [32]byte{}
					b.record.SourceFieldDigest = [32]byte{}
				case "wrong-id":
					suffix = "/8"
				}
				w := request(hxcRuntimeRouter(t, b, authport.RoleAdmin), "GET", suffix, legacyToken(101))
				if w.Code != 503 || strings.Contains(w.Body.String(), "private-downstream") {
					t.Fatalf("bad %s: %d %s", state, w.Code, w.Body.String())
				}
			}
			s.empty = true
			s.total = 0
			if w := request(router, "GET", "", legacyToken(101)); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
				t.Fatal("empty failed")
			}
		})
	}
}

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

type hxcMemberUsageAPIStub struct {
	items []hxcport.HistoricalHXCMemberUsage
	total int64
	err   error
	calls int
	query hxcport.HXCMemberUsageHistoryQuery
}

func (s *hxcMemberUsageAPIStub) GetHistoricalHXCMemberUsage(context.Context, int64) (hxcport.HistoricalHXCMemberUsage, error) {
	s.calls++
	if len(s.items) == 0 {
		return hxcport.HistoricalHXCMemberUsage{}, s.err
	}
	return s.items[0], s.err
}
func (s *hxcMemberUsageAPIStub) ListHistoricalHXCMemberUsage(_ context.Context, q hxcport.HXCMemberUsageHistoryQuery) ([]hxcport.HistoricalHXCMemberUsage, int64, error) {
	s.calls++
	s.query = q
	return s.items, s.total, s.err
}
func hxcMemberUsageAPIFixture() *hxcMemberUsageAPIStub {
	return &hxcMemberUsageAPIStub{total: 1, items: []hxcport.HistoricalHXCMemberUsage{{
		ID: 7, Generation: -9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}, SourceFieldDigest: [32]byte{3},
		UnionID: "private-union", OwnerUserID: "private-owner", MobileHash: "private-mobile", PayloadJSON: json.RawMessage(`{"private-payload":true}`),
		IsMember: true, MembershipTier: "source-tier", ProjectedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}}}
}
func hxcMemberUsageRouter(t *testing.T, reader hxcport.HXCMemberUsageHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcMemberUsageHistory = reader
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
func TestHXCMemberUsageHistoryRoutes(t *testing.T) {
	request := func(router http.Handler, method, suffix, token string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, legacyRequest(method, "/api/admin/hxc-history/member-usage"+suffix, token))
		return w
	}
	s := hxcMemberUsageAPIFixture()
	router := hxcMemberUsageRouter(t, s, authport.RoleAdmin)
	for _, suffix := range []string{"?generation=-9&limit=1&offset=0", "/7"} {
		w := request(router, "GET", suffix, legacyToken(101))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read: %d %s", w.Code, w.Body.String())
		}
		for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"generation":-9`, `"registered_at":null`} {
			if !strings.Contains(w.Body.String(), want) {
				t.Fatalf("missing %s", want)
			}
		}
		for _, hidden := range []string{"private-", "digest", "unionid", "owner_userid", "mobile_hash", "payload", "customer_id", "staff_id"} {
			if strings.Contains(w.Body.String(), hidden) {
				t.Fatalf("private data: %s", hidden)
			}
		}
	}
	if s.query.Generation == nil || *s.query.Generation != -9 || s.query.Limit != 1 || s.query.Offset != 0 {
		t.Fatal("filter not forwarded")
	}
	for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=1&limit=2", "?generation=", "?generation=01", "?generation=-0", "?generation=1&generation=2", "?generation=9223372036854775808", "?customer_id=1", "?unknown=1", "?limit=%zz", "/0", "/01", "/7?generation=1"} {
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
		b := hxcMemberUsageAPIFixture()
		if w := request(hxcMemberUsageRouter(t, b, tc.role), "GET", "", tc.token); w.Code != tc.status || b.calls != 0 {
			t.Fatal("authorization failed")
		}
	}
	for _, missing := range []hxcport.HXCMemberUsageHistoryReader{nil, (*hxcMemberUsageAPIStub)(nil)} {
		if w := request(hxcMemberUsageRouter(t, missing, authport.RoleAdmin), "GET", "", legacyToken(101)); w.Code != 503 {
			t.Fatal("nil reader accepted")
		}
	}
	for _, state := range []string{"error", "count", "hmac", "json", "wrong-id", "duplicate", "wrong-generation"} {
		b := hxcMemberUsageAPIFixture()
		suffix := ""
		switch state {
		case "error":
			b.err = errors.New("private-downstream")
		case "count":
			b.total = 2
		case "hmac":
			b.items[0].SourceFieldDigest = [32]byte{}
		case "json":
			b.items[0].PayloadJSON = nil
		case "wrong-id":
			suffix = "/8"
		case "duplicate":
			b.items = append(b.items, b.items[0])
			b.total = 2
		case "wrong-generation":
			suffix = "?generation=1"
		}
		w := request(hxcMemberUsageRouter(t, b, authport.RoleAdmin), "GET", suffix, legacyToken(101))
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

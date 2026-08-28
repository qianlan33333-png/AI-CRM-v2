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
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type customerStateHistoryAPIStub struct {
	calls            int
	query            contactport.CustomerStateHistoryQuery
	err              error
	empty, duplicate bool
	snapshot         contactport.HistoricalCustomerStatusSnapshot
	change           contactport.HistoricalCustomerStatusChange
	term             contactport.HistoricalClassTermTagMapping
}

func (s *customerStateHistoryAPIStub) GetHistoricalCustomerStatusSnapshot(context.Context, int64) (contactport.HistoricalCustomerStatusSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}
func (s *customerStateHistoryAPIStub) ListHistoricalCustomerStatusSnapshot(_ context.Context, q contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalCustomerStatusSnapshot, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []contactport.HistoricalCustomerStatusSnapshot{s.snapshot, s.snapshot}, 2, s.err
	}
	return []contactport.HistoricalCustomerStatusSnapshot{s.snapshot}, 1, s.err
}
func (s *customerStateHistoryAPIStub) GetHistoricalCustomerStatusChange(context.Context, int64) (contactport.HistoricalCustomerStatusChange, error) {
	s.calls++
	return s.change, s.err
}
func (s *customerStateHistoryAPIStub) ListHistoricalCustomerStatusChange(_ context.Context, q contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalCustomerStatusChange, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []contactport.HistoricalCustomerStatusChange{s.change, s.change}, 2, s.err
	}
	return []contactport.HistoricalCustomerStatusChange{s.change}, 1, s.err
}
func (s *customerStateHistoryAPIStub) GetHistoricalClassTermTagMapping(context.Context, int64) (contactport.HistoricalClassTermTagMapping, error) {
	s.calls++
	return s.term, s.err
}
func (s *customerStateHistoryAPIStub) ListHistoricalClassTermTagMapping(_ context.Context, q contactport.CustomerStateHistoryQuery) ([]contactport.HistoricalClassTermTagMapping, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []contactport.HistoricalClassTermTagMapping{s.term, s.term}, 2, s.err
	}
	return []contactport.HistoricalClassTermTagMapping{s.term}, 1, s.err
}
func customerStateHistoryAPIFixture() *customerStateHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	d := func(b byte) [32]byte { var x [32]byte; x[0] = b; return x }
	return &customerStateHistoryAPIStub{snapshot: contactport.HistoricalCustomerStatusSnapshot{ID: 7, SourceKeyDigest: d(1), SourcePayloadDigest: d(2), SourceFieldDigest: d(3), SignupStatus: "", SignupLabelName: "", CustomerNameSnapshot: "PRIVATE-CUSTOMER", OwnerUserIDSnapshot: "PRIVATE-OWNER", SetByUserIDDigest: d(4), SetAt: at, WeComTagSyncErrorHash: d(5), StatusFlagsDigest: d(6), CreatedAt: at, UpdatedAt: at.Add(-time.Second), UnionID: "PRIVATE-UNION"}, change: contactport.HistoricalCustomerStatusChange{ID: 8, SourceKeyDigest: d(7), SourcePayloadDigest: d(8), SourceFieldDigest: d(9), SourceID: -1, CustomerNameSnapshot: "PRIVATE-CUSTOMER", OwnerUserIDSnapshot: "PRIVATE-OWNER", SetByUserIDDigest: d(10), SetAt: at, WeComTagSyncErrorHash: d(11), StatusFlagsDigest: d(12), CreatedAt: at, UnionID: "PRIVATE-UNION"}, term: contactport.HistoricalClassTermTagMapping{ID: 9, SourceKeyDigest: d(13), SourcePayloadDigest: d(14), SourceFieldDigest: d(15), SourceID: -2, ClassTermNo: -3, OriginalActive: false, CreatedAt: at, UpdatedAt: at.Add(-time.Second), StrategySourceID: "PRIVATE-STRATEGY", GroupSourceID: "PRIVATE-GROUP", TagSourceID: "PRIVATE-TAG"}}
}
func customerStateHistoryAPIRouter(t *testing.T, reader contactport.CustomerStateHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.customerStateHistory = reader
	a, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	r, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), a, a, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestCustomerStateHistoryRoutesReadOnly(t *testing.T) {
	for _, tc := range []struct{ path, id string }{{"snapshots", "7"}, {"changes", "8"}, {"term-tag-mappings", "9"}} {
		t.Run(tc.path, func(t *testing.T) {
			s, a := customerStateHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			r := customerStateHistoryAPIRouter(t, s, a)
			for _, path := range []string{"/api/admin/customer-state-history/" + tc.path + "?limit=1&offset=0", "/api/admin/customer-state-history/" + tc.path + "/" + tc.id} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, legacyRequest(http.MethodGet, path, legacyToken(151)))
				if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s %d", path, w.Code)
				}
				body := w.Body.String()
				for _, private := range []string{"PRIVATE-CUSTOMER", "PRIVATE-OWNER", "PRIVATE-UNION", "PRIVATE-STRATEGY", "PRIVATE-GROUP", "PRIVATE-TAG"} {
					if strings.Contains(body, private) {
						t.Fatalf("private leak %s", private)
					}
				}
				if tc.path == "snapshots" && strings.Contains(body, "source_id") {
					t.Fatal("snapshot source_id invented")
				}
			}
			if a.csrfCalls != 0 || len(a.capabilities) != 2 || a.capabilities[0] != authport.CapabilityAdminRead {
				t.Fatal("AdminRead/CSRF")
			}
			if s.query.Limit != 1 || s.query.Offset != 0 {
				t.Fatal("page")
			}
		})
	}
}
func TestCustomerStateHistoryFailClosed(t *testing.T) {
	for _, path := range []string{"snapshots", "changes", "term-tag-mappings"} {
		for _, query := range []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1"} {
			s := customerStateHistoryAPIFixture()
			w := httptest.NewRecorder()
			customerStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/customer-state-history/"+path+"?"+query, legacyToken(152)))
			if w.Code != 400 || s.calls != 0 {
				t.Fatalf("%s?%s", path, query)
			}
		}
		s := customerStateHistoryAPIFixture()
		s.duplicate = true
		w := httptest.NewRecorder()
		customerStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/customer-state-history/"+path+"?limit=2", legacyToken(153)))
		if w.Code != 503 {
			t.Fatal("duplicate")
		}
		s = customerStateHistoryAPIFixture()
		s.err = errors.New("private error")
		w = httptest.NewRecorder()
		customerStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/customer-state-history/"+path, legacyToken(154)))
		if w.Code != 503 || strings.Contains(w.Body.String(), "private error") {
			t.Fatal("error leak")
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			s = customerStateHistoryAPIFixture()
			w = httptest.NewRecorder()
			customerStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(method, "/api/admin/customer-state-history/"+path, legacyToken(155)))
			if w.Code >= 200 && w.Code < 300 || s.calls != 0 {
				t.Fatal("write")
			}
		}
	}
	for _, r := range []contactport.CustomerStateHistoryReader{nil, (*customerStateHistoryAPIStub)(nil)} {
		w := httptest.NewRecorder()
		customerStateHistoryAPIRouter(t, r, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/customer-state-history/snapshots", legacyToken(156)))
		if w.Code != 503 {
			t.Fatal("nil")
		}
	}
}

var _ contactport.CustomerStateHistoryReader = (*customerStateHistoryAPIStub)(nil)

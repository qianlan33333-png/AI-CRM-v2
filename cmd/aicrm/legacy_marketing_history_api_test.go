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
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type legacyMarketingHistoryAPIStub struct {
	calls            int
	query            segmentport.LegacyMarketingHistoryQuery
	err              error
	empty, duplicate bool
	fact0            segmentport.HistoricalLegacyMarketingState
	fact2            segmentport.HistoricalLegacyMarketingValue
}

func (s *legacyMarketingHistoryAPIStub) GetHistoricalLegacyMarketingState(context.Context, int64) (segmentport.HistoricalLegacyMarketingState, error) {
	s.calls++
	return s.fact0, s.err
}
func (s *legacyMarketingHistoryAPIStub) ListHistoricalLegacyMarketingState(_ context.Context, q segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingState, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalLegacyMarketingState{s.fact0, s.fact0}, 2, s.err
	}
	return []segmentport.HistoricalLegacyMarketingState{s.fact0}, 1, s.err
}

func (s *legacyMarketingHistoryAPIStub) GetHistoricalLegacyMarketingValue(context.Context, int64) (segmentport.HistoricalLegacyMarketingValue, error) {
	s.calls++
	return s.fact2, s.err
}
func (s *legacyMarketingHistoryAPIStub) ListHistoricalLegacyMarketingValue(_ context.Context, q segmentport.LegacyMarketingHistoryQuery) ([]segmentport.HistoricalLegacyMarketingValue, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalLegacyMarketingValue{s.fact2, s.fact2}, 2, s.err
	}
	return []segmentport.HistoricalLegacyMarketingValue{s.fact2}, 1, s.err
}

func legacyMarketingHistoryAPIFixture() *legacyMarketingHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	d := func(b byte) [32]byte { var x [32]byte; x[0] = b; return x }
	privateRef := int64(-915777)
	return &legacyMarketingHistoryAPIStub{
		fact0: segmentport.HistoricalLegacyMarketingState{ID: 7, SourceKeyDigest: d(4), SourcePayloadDigest: d(4), SourceFieldDigest: d(4), SourceID: -3, ExternalUserIDDigest: d(4), ScenarioKey: "", MarketingPhase: "", PhaseLabel: "", PhaseReason: "", LifecycleStatus: "", LastBatchSourceID: &privateRef, LastBatchStatus: "", LastBatchWindowStart: "", LastBatchWindowEnd: "", LastTriggerMessageAt: "civil source text", EnteredAt: nil, ExitedAt: nil, ExitReason: "", StatePayloadDigest: d(4), CreatedAt: at, UpdatedAt: at},
		fact2: segmentport.HistoricalLegacyMarketingValue{ID: 9, SourceKeyDigest: d(4), SourcePayloadDigest: d(4), SourceFieldDigest: d(4), SourceID: -3, ExternalUserIDDigest: d(4), ScenarioKey: "", ValueSegment: "", SegmentLabel: "", Score: -3, ScoreBreakdownDigest: d(4), StatePayloadDigest: d(4), CreatedAt: at, UpdatedAt: at},
	}
}
func legacyMarketingHistoryAPIRouter(t *testing.T, reader segmentport.LegacyMarketingHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.legacyMarketingHistory = reader
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
func TestLegacyMarketingHistoryRoutesReadOnly(t *testing.T) {
	for _, tc := range []struct{ path, id string }{{"states", "7"}, {"values", "9"}} {
		t.Run(tc.path, func(t *testing.T) {
			s, a := legacyMarketingHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			r := legacyMarketingHistoryAPIRouter(t, s, a)
			for _, path := range []string{"/api/admin/legacy-marketing-history/" + tc.path + "?limit=1&offset=0", "/api/admin/legacy-marketing-history/" + tc.path + "/" + tc.id} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, legacyRequest(http.MethodGet, path, legacyToken(151)))
				if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s %d", path, w.Code)
				}
				body := w.Body.String()
				for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "external_userid_digest", "state_payload_digest", "person_source_id", "batch_source_id", "submission_source_id", "915777"} {
					if strings.Contains(body, private) {
						t.Fatalf("private leak %s", private)
					}
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
func TestLegacyMarketingHistoryFailClosed(t *testing.T) {
	for _, path := range []string{"states", "values"} {
		for _, query := range []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1"} {
			s := legacyMarketingHistoryAPIFixture()
			w := httptest.NewRecorder()
			legacyMarketingHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/legacy-marketing-history/"+path+"?"+query, legacyToken(152)))
			if w.Code != 400 || s.calls != 0 {
				t.Fatalf("%s?%s", path, query)
			}
		}
		s := legacyMarketingHistoryAPIFixture()
		s.duplicate = true
		w := httptest.NewRecorder()
		legacyMarketingHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/legacy-marketing-history/"+path+"?limit=2", legacyToken(153)))
		if w.Code != 503 {
			t.Fatal("duplicate")
		}
		s = legacyMarketingHistoryAPIFixture()
		s.err = errors.New("private error")
		w = httptest.NewRecorder()
		legacyMarketingHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/legacy-marketing-history/"+path, legacyToken(154)))
		if w.Code != 503 || strings.Contains(w.Body.String(), "private error") {
			t.Fatal("error leak")
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			s = legacyMarketingHistoryAPIFixture()
			w = httptest.NewRecorder()
			legacyMarketingHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(method, "/api/admin/legacy-marketing-history/"+path, legacyToken(155)))
			if w.Code >= 200 && w.Code < 300 || s.calls != 0 {
				t.Fatal("write")
			}
		}
	}
	for _, r := range []segmentport.LegacyMarketingHistoryReader{nil, (*legacyMarketingHistoryAPIStub)(nil)} {
		w := httptest.NewRecorder()
		legacyMarketingHistoryAPIRouter(t, r, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/legacy-marketing-history/states", legacyToken(156)))
		if w.Code != 503 {
			t.Fatal("nil")
		}
	}
}

func TestLegacyMarketingHistoryEmptyAndAnonymous(t *testing.T) {
	for _, path := range []string{"states", "values"} {
		s := legacyMarketingHistoryAPIFixture()
		r := legacyMarketingHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/legacy-marketing-history/"+path, nil))
		if w.Code != http.StatusUnauthorized || s.calls != 0 {
			t.Fatalf("anonymous %s: %d calls=%d", path, w.Code, s.calls)
		}
		s.empty = true
		w = httptest.NewRecorder()
		r.ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/legacy-marketing-history/"+path, legacyToken(157)))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) || !strings.Contains(w.Body.String(), `"real_external_call_executed":false`) {
			t.Fatalf("empty %s: %d", path, w.Code)
		}
	}
}

var _ segmentport.LegacyMarketingHistoryReader = (*legacyMarketingHistoryAPIStub)(nil)

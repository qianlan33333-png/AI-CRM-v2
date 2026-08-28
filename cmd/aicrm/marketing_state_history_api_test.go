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

type marketingStateHistoryAPIStub struct {
	calls            int
	query            segmentport.MarketingStateHistoryQuery
	err              error
	empty, duplicate bool
	fact0            segmentport.HistoricalMarketingStateSnapshot
	fact1            segmentport.HistoricalMarketingStateChange
	fact2            segmentport.HistoricalValueSegmentSnapshot
	fact3            segmentport.HistoricalValueSegmentChange
}

func (s *marketingStateHistoryAPIStub) GetHistoricalMarketingStateSnapshot(context.Context, int64) (segmentport.HistoricalMarketingStateSnapshot, error) {
	s.calls++
	return s.fact0, s.err
}
func (s *marketingStateHistoryAPIStub) ListHistoricalMarketingStateSnapshot(_ context.Context, q segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalMarketingStateSnapshot, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalMarketingStateSnapshot{s.fact0, s.fact0}, 2, s.err
	}
	return []segmentport.HistoricalMarketingStateSnapshot{s.fact0}, 1, s.err
}
func (s *marketingStateHistoryAPIStub) GetHistoricalMarketingStateChange(context.Context, int64) (segmentport.HistoricalMarketingStateChange, error) {
	s.calls++
	return s.fact1, s.err
}
func (s *marketingStateHistoryAPIStub) ListHistoricalMarketingStateChange(_ context.Context, q segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalMarketingStateChange, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalMarketingStateChange{s.fact1, s.fact1}, 2, s.err
	}
	return []segmentport.HistoricalMarketingStateChange{s.fact1}, 1, s.err
}
func (s *marketingStateHistoryAPIStub) GetHistoricalValueSegmentSnapshot(context.Context, int64) (segmentport.HistoricalValueSegmentSnapshot, error) {
	s.calls++
	return s.fact2, s.err
}
func (s *marketingStateHistoryAPIStub) ListHistoricalValueSegmentSnapshot(_ context.Context, q segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalValueSegmentSnapshot, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalValueSegmentSnapshot{s.fact2, s.fact2}, 2, s.err
	}
	return []segmentport.HistoricalValueSegmentSnapshot{s.fact2}, 1, s.err
}
func (s *marketingStateHistoryAPIStub) GetHistoricalValueSegmentChange(context.Context, int64) (segmentport.HistoricalValueSegmentChange, error) {
	s.calls++
	return s.fact3, s.err
}
func (s *marketingStateHistoryAPIStub) ListHistoricalValueSegmentChange(_ context.Context, q segmentport.MarketingStateHistoryQuery) ([]segmentport.HistoricalValueSegmentChange, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	if s.duplicate {
		return []segmentport.HistoricalValueSegmentChange{s.fact3, s.fact3}, 2, s.err
	}
	return []segmentport.HistoricalValueSegmentChange{s.fact3}, 1, s.err
}
func marketingStateHistoryAPIFixture() *marketingStateHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	d := func(b byte) [32]byte { var x [32]byte; x[0] = b; return x }
	privateRef := int64(-915777)
	return &marketingStateHistoryAPIStub{
		fact0: segmentport.HistoricalMarketingStateSnapshot{ID: 7, SourceKeyDigest: d(2), SourcePayloadDigest: d(3), SourceFieldDigest: d(4), SourceID: -1, PersonSourceID: &privateRef, ExternalUserIDDigest: d(7), AutomationKey: "", MainStage: "", SubStage: "", Activated: false, Converted: false, EligibleForConversion: false, LifecycleStatus: "", LastActivationAt: "", LastConversionMarkedAt: "", LastMessageAt: "not a date", LastBatchSourceID: &privateRef, LastBatchStatus: "", LastBatchWindowStart: "", LastBatchWindowEnd: "", LastTriggerMessageAt: "", EnteredAt: nil, ExitedAt: nil, ExitReason: "", StatePayloadDigest: d(26), CreatedAt: at, UpdatedAt: at},
		fact1: segmentport.HistoricalMarketingStateChange{ID: 8, SourceKeyDigest: d(2), SourcePayloadDigest: d(3), SourceFieldDigest: d(4), SourceID: -1, PersonSourceID: &privateRef, BatchSourceID: &privateRef, ExternalUserIDDigest: d(8), AutomationKey: "", MainStage: "", SubStage: "", Activated: false, Converted: false, EligibleForConversion: false, LifecycleStatus: "", LastActivationAt: "", LastConversionMarkedAt: "", LastMessageAt: "not a date", ExitReason: "", ChangeReason: "", StatePayloadDigest: d(21), RecordedAt: at, CreatedAt: at},
		fact2: segmentport.HistoricalValueSegmentSnapshot{ID: 9, SourceKeyDigest: d(2), SourcePayloadDigest: d(3), SourceFieldDigest: d(4), SourceID: -1, ExternalUserIDDigest: d(6), Segment: "", SegmentRank: -3, Score: -3, ScoringVersion: "", SubmissionSourceID: &privateRef, MatchedQuestionIDsDigest: d(12), StatePayloadDigest: d(13), ComputedReason: "", EvaluatedAt: at, ComputedAt: at, CreatedAt: at, UpdatedAt: at},
		fact3: segmentport.HistoricalValueSegmentChange{ID: 10, SourceKeyDigest: d(2), SourcePayloadDigest: d(3), SourceFieldDigest: d(4), SourceID: -1, ExternalUserIDDigest: d(6), Segment: "", SegmentRank: -3, Score: -3, ScoringVersion: "", SubmissionSourceID: &privateRef, MatchedQuestionIDsDigest: d(12), StatePayloadDigest: d(13), ChangeReason: "", EvaluatedAt: at, RecordedAt: at, CreatedAt: at},
	}
}
func marketingStateHistoryAPIRouter(t *testing.T, reader segmentport.MarketingStateHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.marketingStateHistory = reader
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
func TestMarketingStateHistoryRoutesReadOnly(t *testing.T) {
	for _, tc := range []struct{ path, id string }{{"state-snapshots", "7"}, {"state-changes", "8"}, {"value-snapshots", "9"}, {"value-changes", "10"}} {
		t.Run(tc.path, func(t *testing.T) {
			s, a := marketingStateHistoryAPIFixture(), &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			r := marketingStateHistoryAPIRouter(t, s, a)
			for _, path := range []string{"/api/admin/marketing-state-history/" + tc.path + "?limit=1&offset=0", "/api/admin/marketing-state-history/" + tc.path + "/" + tc.id} {
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
func TestMarketingStateHistoryFailClosed(t *testing.T) {
	for _, path := range []string{"state-snapshots", "state-changes", "value-snapshots", "value-changes"} {
		for _, query := range []string{"limit=0", "limit=101", "offset=-1", "limit=1&limit=2", "unknown=1"} {
			s := marketingStateHistoryAPIFixture()
			w := httptest.NewRecorder()
			marketingStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/marketing-state-history/"+path+"?"+query, legacyToken(152)))
			if w.Code != 400 || s.calls != 0 {
				t.Fatalf("%s?%s", path, query)
			}
		}
		s := marketingStateHistoryAPIFixture()
		s.duplicate = true
		w := httptest.NewRecorder()
		marketingStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/marketing-state-history/"+path+"?limit=2", legacyToken(153)))
		if w.Code != 503 {
			t.Fatal("duplicate")
		}
		s = marketingStateHistoryAPIFixture()
		s.err = errors.New("private error")
		w = httptest.NewRecorder()
		marketingStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/marketing-state-history/"+path, legacyToken(154)))
		if w.Code != 503 || strings.Contains(w.Body.String(), "private error") {
			t.Fatal("error leak")
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			s = marketingStateHistoryAPIFixture()
			w = httptest.NewRecorder()
			marketingStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(method, "/api/admin/marketing-state-history/"+path, legacyToken(155)))
			if w.Code >= 200 && w.Code < 300 || s.calls != 0 {
				t.Fatal("write")
			}
		}
	}
	for _, r := range []segmentport.MarketingStateHistoryReader{nil, (*marketingStateHistoryAPIStub)(nil)} {
		w := httptest.NewRecorder()
		marketingStateHistoryAPIRouter(t, r, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/marketing-state-history/state-snapshots", legacyToken(156)))
		if w.Code != 503 {
			t.Fatal("nil")
		}
	}
}

func TestMarketingStateHistoryEmptyAndAnonymous(t *testing.T) {
	for _, path := range []string{"state-snapshots", "state-changes", "value-snapshots", "value-changes"} {
		s := marketingStateHistoryAPIFixture()
		r := marketingStateHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/marketing-state-history/"+path, nil))
		if w.Code != http.StatusUnauthorized || s.calls != 0 {
			t.Fatalf("anonymous %s: %d calls=%d", path, w.Code, s.calls)
		}
		s.empty = true
		w = httptest.NewRecorder()
		r.ServeHTTP(w, legacyRequest(http.MethodGet, "/api/admin/marketing-state-history/"+path, legacyToken(157)))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) || !strings.Contains(w.Body.String(), `"real_external_call_executed":false`) {
			t.Fatalf("empty %s: %d", path, w.Code)
		}
	}
}

var _ segmentport.MarketingStateHistoryReader = (*marketingStateHistoryAPIStub)(nil)

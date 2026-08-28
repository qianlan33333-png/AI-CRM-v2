package main

import (
	"context"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type hxcHistoryAPIStub struct {
	calls      int
	query      hxcport.HXCHistoryQuery
	err        error
	empty      bool
	total      int64
	meta       hxcport.HistoricalHXCMeta
	snapshot   hxcport.HistoricalHXCSnapshot
	activation hxcport.HistoricalHXCActivation
	lead       hxcport.HistoricalHXCLead
	batch      hxcport.HistoricalHXCBatch
}

func (s *hxcHistoryAPIStub) GetHistoricalHXCMeta(context.Context, int64) (hxcport.HistoricalHXCMeta, error) {
	s.calls++
	return s.meta, s.err
}
func (s *hxcHistoryAPIStub) ListHistoricalHXCMeta(_ context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCMeta, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCMeta{s.meta}, s.total, s.err
}
func (s *hxcHistoryAPIStub) GetHistoricalHXCSnapshot(context.Context, int64) (hxcport.HistoricalHXCSnapshot, error) {
	s.calls++
	return s.snapshot, s.err
}
func (s *hxcHistoryAPIStub) ListHistoricalHXCSnapshot(_ context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCSnapshot, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCSnapshot{s.snapshot}, s.total, s.err
}
func (s *hxcHistoryAPIStub) GetHistoricalHXCActivation(context.Context, int64) (hxcport.HistoricalHXCActivation, error) {
	s.calls++
	return s.activation, s.err
}
func (s *hxcHistoryAPIStub) ListHistoricalHXCActivation(_ context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCActivation, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCActivation{s.activation}, s.total, s.err
}
func (s *hxcHistoryAPIStub) GetHistoricalHXCLead(context.Context, int64) (hxcport.HistoricalHXCLead, error) {
	s.calls++
	return s.lead, s.err
}
func (s *hxcHistoryAPIStub) ListHistoricalHXCLead(_ context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCLead, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCLead{s.lead}, s.total, s.err
}
func (s *hxcHistoryAPIStub) GetHistoricalHXCBatch(context.Context, int64) (hxcport.HistoricalHXCBatch, error) {
	s.calls++
	return s.batch, s.err
}
func (s *hxcHistoryAPIStub) ListHistoricalHXCBatch(_ context.Context, q hxcport.HXCHistoryQuery) ([]hxcport.HistoricalHXCBatch, int64, error) {
	s.calls++
	s.query = q
	if s.empty {
		return nil, s.total, s.err
	}
	return []hxcport.HistoricalHXCBatch{s.batch}, s.total, s.err
}
func hxcHistoryAPIFixture() *hxcHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	date, batch, customer := "2024-02-29", "", int64(3)
	identity := hxcport.HistoricalHXCIdentity{ID: 7, SourceID: -9, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{2}}
	return &hxcHistoryAPIStub{total: 1,
		meta:       hxcport.HistoricalHXCMeta{HistoricalHXCIdentity: identity, StartedAt: at},
		snapshot:   hxcport.HistoricalHXCSnapshot{HistoricalHXCIdentity: identity, CustomerID: &customer, Observation: "observed_snapshot", ObservedAt: at, CRMCreatedAt: &date, LastQuestionnaireAt: &date, SubscriptionPeriodStart: &date},
		activation: hxcport.HistoricalHXCActivation{HistoricalHXCIdentity: identity, SourceTable: "public/user_ops_activation_status_source", LegacyImportBatchRef: &batch, CreatedAt: at, UpdatedAt: at},
		lead:       hxcport.HistoricalHXCLead{HistoricalHXCIdentity: identity, LegacyImportBatchRef: &batch, CreatedAt: at, UpdatedAt: at},
		batch:      hxcport.HistoricalHXCBatch{HistoricalHXCIdentity: identity, CreatedAt: at},
	}
}
func hxcHistoryAPIRouter(t *testing.T, reader hxcport.HXCHistoryReader, auth *audienceHistoryAPIAuth) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.hxcHistory = reader
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
func TestHXCHistoryFinalRoutes(t *testing.T) {
	for _, kind := range []string{"refreshes", "snapshots", "activations", "leads", "batches"} {
		t.Run(kind, func(t *testing.T) {
			s := hxcHistoryAPIFixture()
			a := &audienceHistoryAPIAuth{role: authport.RoleAdmin}
			r := hxcHistoryAPIRouter(t, s, a)
			for _, suffix := range []string{"?limit=1&offset=0", "/7"} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind+suffix, legacyToken(101)))
				if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("%s %d %s", suffix, w.Code, w.Body.String())
				}
				for _, want := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":-9`} {
					if !strings.Contains(w.Body.String(), want) {
						t.Fatalf("missing %s: %s", want, w.Body.String())
					}
				}
				if kind == "snapshots" && !strings.Contains(w.Body.String(), `"crm_created_at":"2024-02-29"`) {
					t.Fatal("DATE changed")
				}
				if (kind == "activations" || kind == "leads") && !strings.Contains(w.Body.String(), `"legacy_import_batch_ref":""`) {
					t.Fatal("empty source ref changed")
				}
			}
			if s.calls != 2 || a.csrfCalls != 0 {
				t.Fatal("read called wrong dependency")
			}
			for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=1&limit=2", "?unknown=1", "?limit=%zz", "/0", "/01", "/7?limit=1"} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind+suffix, legacyToken(101)))
				if w.Code != 400 {
					t.Fatalf("invalid %s %d", suffix, w.Code)
				}
			}
			if s.calls != 2 {
				t.Fatal("invalid query reached reader")
			}
			for _, state := range []string{"error", "count", "identity", "date", "observation"} {
				b := hxcHistoryAPIFixture()
				switch state {
				case "error":
					b.err = errors.New("private-downstream")
				case "count":
					b.total = 2
				case "identity":
					b.meta.SourceKeyDigest = [32]byte{}
					b.snapshot.SourceKeyDigest = [32]byte{}
					b.activation.SourceKeyDigest = [32]byte{}
					b.lead.SourceKeyDigest = [32]byte{}
					b.batch.SourceKeyDigest = [32]byte{}
				case "date":
					if kind != "snapshots" {
						continue
					}
					bad := "2026-02-29"
					b.snapshot.CRMCreatedAt = &bad
				case "observation":
					if kind != "snapshots" {
						continue
					}
					b.snapshot.Observation = ""
				}
				w := httptest.NewRecorder()
				hxcHistoryAPIRouter(t, b, a).ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind, legacyToken(101)))
				if w.Code != 503 || strings.Contains(w.Body.String(), "private-downstream") {
					t.Fatalf("bad %s %d %s", state, w.Code, w.Body.String())
				}
			}
			s.empty = true
			s.total = 0
			w := httptest.NewRecorder()
			r.ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind, legacyToken(101)))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
				t.Fatal("empty failed")
			}
			for _, tc := range []struct {
				role  authport.Role
				token string
				want  int
			}{{authport.RoleAdmin, "", 401}, {authport.Role("ops"), legacyToken(101), 403}} {
				b := hxcHistoryAPIFixture()
				w := httptest.NewRecorder()
				hxcHistoryAPIRouter(t, b, &audienceHistoryAPIAuth{role: tc.role}).ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind, tc.token))
				if w.Code != tc.want || b.calls != 0 {
					t.Fatal("authorization failed")
				}
			}
			for _, method := range []string{"POST", "PUT", "DELETE"} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, legacyRequest(method, "/api/admin/hxc-history/"+kind, legacyToken(101)))
				if w.Code >= 200 && w.Code < 300 {
					t.Fatal("write accepted")
				}
			}
			for _, missing := range []hxcport.HXCHistoryReader{nil, (*hxcHistoryAPIStub)(nil)} {
				w := httptest.NewRecorder()
				hxcHistoryAPIRouter(t, missing, a).ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+kind, legacyToken(101)))
				if w.Code != 503 {
					t.Fatal("missing dependency accepted")
				}
			}
		})
	}
}
func TestHXCHistoryFilterBindings(t *testing.T) {
	for _, tc := range []struct {
		path string
		want int
	}{
		{"snapshots?customer_id=3", 200}, {"snapshots?customer_id=4", 503}, {"snapshots?customer_id=03", 400}, {"snapshots?customer_id=3&customer_id=4", 400}, {"snapshots?source_table=public/user_ops_activation_status_source", 400}, {"refreshes?customer_id=3", 400}, {"leads?source_table=public/user_ops_activation_status_source", 400}, {"activations?source_table=public/user_ops_activation_status_source", 200}, {"activations?source_table=public/user_ops_huangxiaocan_activation_source", 503}, {"activations?source_table=unknown", 400}, {"activations?source_table=", 400}} {
		s := hxcHistoryAPIFixture()
		w := httptest.NewRecorder()
		hxcHistoryAPIRouter(t, s, &audienceHistoryAPIAuth{role: authport.RoleAdmin}).ServeHTTP(w, legacyRequest("GET", "/api/admin/hxc-history/"+tc.path, legacyToken(101)))
		if w.Code != tc.want {
			t.Fatalf("%s %d %s", tc.path, w.Code, w.Body.String())
		}
	}
}

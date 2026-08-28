package main

import (
	"context"
	"encoding/json"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type deferredIdentityAPIStub struct {
	contactport.DeferredIdentityHistoryReader
	empty    bool
	err      error
	query    contactport.DeferredIdentityHistoryQuery
	person   contactport.HistoricalDeferredPerson
	conflict contactport.HistoricalDeferredIdentityConflict
	missing  contactport.HistoricalMissingRootIdentity
}

func (s *deferredIdentityAPIStub) GetHistoricalDeferredPerson(context.Context, int64) (contactport.HistoricalDeferredPerson, error) {
	return s.person, s.err
}
func (s *deferredIdentityAPIStub) ListHistoricalDeferredPerson(_ context.Context, q contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalDeferredPerson, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalDeferredPerson{s.person}, 1, s.err
}
func (s *deferredIdentityAPIStub) GetHistoricalDeferredIdentityConflict(context.Context, int64) (contactport.HistoricalDeferredIdentityConflict, error) {
	return s.conflict, s.err
}
func (s *deferredIdentityAPIStub) ListHistoricalDeferredIdentityConflict(_ context.Context, q contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalDeferredIdentityConflict, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalDeferredIdentityConflict{s.conflict}, 1, s.err
}
func (s *deferredIdentityAPIStub) GetHistoricalMissingRootIdentity(context.Context, int64) (contactport.HistoricalMissingRootIdentity, error) {
	return s.missing, s.err
}
func (s *deferredIdentityAPIStub) ListHistoricalMissingRootIdentity(_ context.Context, q contactport.DeferredIdentityHistoryQuery) ([]contactport.HistoricalMissingRootIdentity, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalMissingRootIdentity{s.missing}, 1, s.err
}

func deferredIdentityAPIFixture() *deferredIdentityAPIStub {
	s := &deferredIdentityAPIStub{}
	for _, v := range []any{&s.person, &s.conflict, &s.missing} {
		fields := reflect.ValueOf(v).Elem()
		for i := 0; i < fields.NumField(); i++ {
			f := fields.Field(i)
			if f.Type() == reflect.TypeFor[[32]byte]() {
				f.Set(reflect.ValueOf([32]byte{7}))
			}
			if f.Type() == reflect.TypeFor[time.Time]() {
				f.Set(reflect.ValueOf(time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)))
			}
		}
		fields.FieldByName("ID").SetInt(7)
		fields.FieldByName("SourceID").SetInt(-1)
		fields.FieldByName("RedactedRoots").Set(reflect.ValueOf([]string{"PRIVATE_ROOT"}))
	}
	s.missing.DM01RunID = 2
	s.missing.DM01SourceHMACKeyVersion = "1"
	s.missing.QuarantineReason = "missing_customer_root"
	return s
}
func deferredIdentityAPIRouter(t *testing.T, reader contactport.DeferredIdentityHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.deferredIdentityHistory = reader
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.NotFoundHandler(), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}
func TestDeferredIdentityHistoryAPIReadOnlyPrivateAllowlist(t *testing.T) {
	keys := map[string][]string{
		"people":        {"id", "source_id", "created_at", "updated_at"},
		"conflicts":     {"id", "source_id", "conflict_type", "source_type", "status", "resolution_status", "created_at", "updated_at", "resolved_at"},
		"missing-roots": {"id", "source_id", "quarantine_reason", "type", "status", "first_seen_at", "last_seen_at", "created_at", "updated_at"},
	}
	for kind, allowed := range keys {
		for _, suffix := range []string{"?limit=5&offset=0", "/7"} {
			s := deferredIdentityAPIFixture()
			out := httptest.NewRecorder()
			deferredIdentityAPIRouter(t, s, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/deferred-identity-history/"+kind+suffix, legacyToken(101)))
			if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s: %d %s", kind, suffix, out.Code, out.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(out.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["source"] != "v1_history" || body["read_only"] != true || body["real_external_call_executed"] != false {
				t.Fatal("history envelope lost")
			}
			var row map[string]any
			if suffix[0] == '?' {
				row = body["items"].([]any)[0].(map[string]any)
				if s.query.Limit != 5 || s.query.Offset != 0 {
					t.Fatal("query lost")
				}
			} else {
				row = body["item"].(map[string]any)
			}
			if len(row) != len(allowed) {
				t.Fatalf("unexpected fields: %v", row)
			}
			for _, key := range allowed {
				if _, ok := row[key]; !ok {
					t.Fatalf("missing public field %s", key)
				}
			}
			if row["source_id"] != float64(-1) || strings.Contains(out.Body.String(), "PRIVATE_ROOT") {
				t.Fatal("source value lost or private root exposed")
			}
		}
	}
}
func TestDeferredIdentityHistoryAPIClosedFailuresAndPermissions(t *testing.T) {
	for _, kind := range []string{"people", "conflicts", "missing-roots"} {
		path := "/api/admin/deferred-identity-history/" + kind
		for _, tc := range []struct {
			suffix, token string
			role          authport.Role
			status        int
		}{
			{"", "", authport.RoleAdmin, 401}, {"", legacyToken(101), authport.RoleOps, 403},
			{"?limit=0", legacyToken(101), authport.RoleAdmin, 400}, {"?limit=2&limit=3", legacyToken(101), authport.RoleAdmin, 400},
			{"/0", legacyToken(101), authport.RoleAdmin, 400}, {"/7?mobile=private", legacyToken(101), authport.RoleAdmin, 400},
		} {
			out := httptest.NewRecorder()
			deferredIdentityAPIRouter(t, deferredIdentityAPIFixture(), tc.role).ServeHTTP(out, legacyRequest(http.MethodGet, path+tc.suffix, tc.token))
			if out.Code != tc.status {
				t.Fatalf("%s: got %d want %d", path+tc.suffix, out.Code, tc.status)
			}
		}
		s := deferredIdentityAPIFixture()
		s.empty = true
		out := httptest.NewRecorder()
		deferredIdentityAPIRouter(t, s, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 200 || !strings.Contains(out.Body.String(), `"items":[]`) {
			t.Fatalf("empty: %d %s", out.Code, out.Body.String())
		}
		s = deferredIdentityAPIFixture()
		s.err = errors.New("PRIVATE_DATABASE_ERROR")
		out = httptest.NewRecorder()
		deferredIdentityAPIRouter(t, s, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 503 || strings.Contains(out.Body.String(), "PRIVATE_DATABASE_ERROR") {
			t.Fatal("read failure did not fail closed")
		}
		var nilReader *deferredIdentityAPIStub
		out = httptest.NewRecorder()
		deferredIdentityAPIRouter(t, nilReader, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, path, legacyToken(101)))
		if out.Code != 503 {
			t.Fatalf("nil reader: %d", out.Code)
		}
		out = httptest.NewRecorder()
		deferredIdentityAPIRouter(t, deferredIdentityAPIFixture(), authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodPost, path, legacyToken(101)))
		if out.Code < 400 {
			t.Fatal("unexpected historical write route")
		}
	}
}

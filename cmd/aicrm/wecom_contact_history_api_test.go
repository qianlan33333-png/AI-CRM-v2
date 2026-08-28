package main

import (
	"context"
	"errors"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type wecomContactHistoryAPIStub struct {
	err   error
	empty bool
	query contactport.WeComContactHistoryQuery
	v0    contactport.HistoricalWeComExternalContactEventLog
	v1    contactport.HistoricalWeComExternalContactFollowUser
}

func (s *wecomContactHistoryAPIStub) GetHistoricalWeComExternalContactEventLog(context.Context, int64) (contactport.HistoricalWeComExternalContactEventLog, error) {
	return s.v0, s.err
}
func (s *wecomContactHistoryAPIStub) ListHistoricalWeComExternalContactEventLog(_ context.Context, q contactport.WeComContactHistoryQuery) ([]contactport.HistoricalWeComExternalContactEventLog, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalWeComExternalContactEventLog{s.v0}, 1, s.err
}

func (s *wecomContactHistoryAPIStub) GetHistoricalWeComExternalContactFollowUser(context.Context, int64) (contactport.HistoricalWeComExternalContactFollowUser, error) {
	return s.v1, s.err
}
func (s *wecomContactHistoryAPIStub) ListHistoricalWeComExternalContactFollowUser(_ context.Context, q contactport.WeComContactHistoryQuery) ([]contactport.HistoricalWeComExternalContactFollowUser, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalWeComExternalContactFollowUser{s.v1}, 1, s.err
}
func wecomContactHistoryAPIFixture() *wecomContactHistoryAPIStub {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	raw := int64(-1)
	way := int32(-2)
	return &wecomContactHistoryAPIStub{v0: contactport.HistoricalWeComExternalContactEventLog{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: -1, CorpIDDigest: [32]byte{1}, EventType: "observed", ChangeType: "observed", ExternalUserIDDigest: [32]byte{1}, UserIDDigest: [32]byte{1}, EventTime: &raw, EventKeyDigest: [32]byte{1}, PayloadXMLDigest: [32]byte{1}, PayloadJSONDigest: [32]byte{1}, ProcessStatus: "observed", RetryCount: -1, ErrorMessageDigest: [32]byte{1}, CreatedAt: at, UpdatedAt: at, IdentitySyncStatus: "observed", IdentitySyncErrorCodeDigest: [32]byte{1}, IdentitySyncErrorMessageDigest: [32]byte{1}, IdentitySyncResponseDigest: [32]byte{1}}, v1: contactport.HistoricalWeComExternalContactFollowUser{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: -1, CorpIDDigest: [32]byte{1}, ExternalUserIDDigest: [32]byte{1}, UserIDDigest: [32]byte{1}, RelationStatus: "observed", IsPrimary: true, RemarkDigest: [32]byte{1}, DescriptionDigest: [32]byte{1}, AddWay: &way, State: "PRIVATE-CHANNEL-SCENE", OperUserIDDigest: [32]byte{1}, CreateTime: &raw, RawFollowUserDigest: [32]byte{1}, FirstSeenAt: at, LastSeenAt: at, CreatedAt: at, UpdatedAt: at}}
}
func wecomContactHistoryAPIRouter(t *testing.T, reader contactport.WeComContactHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.wecomContactHistory = reader
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
func TestWeComContactHistoryAPIReadOnlyEnvelope(t *testing.T) {
	for _, kind := range []string{"events", "relations"} {
		for _, suffix := range []string{"?limit=5&offset=0", "/7"} {
			stub := wecomContactHistoryAPIFixture()
			router := wecomContactHistoryAPIRouter(t, stub, authport.RoleAdmin)
			out := httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind+suffix, legacyToken(101)))
			if out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s: code %d body %s", kind, suffix, out.Code, out.Body.String())
			}
			body := out.Body.String()
			for _, required := range []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"source_id":-1`} {
				if !strings.Contains(body, required) {
					t.Fatalf("missing %s", required)
				}
			}
			for _, private := range []string{"PRIVATE-CHANNEL-SCENE", "source_key_digest", "source_payload_digest", "source_field_digest", "corp_id_digest", "user_id_digest", "raw_follow_user_digest", "payload_xml_digest", "error_message_digest", `"state":`} {
				if strings.Contains(body, private) {
					t.Fatalf("private field exposed: %s", private)
				}
			}
			if suffix[0] == '?' && (stub.query.Limit != 5 || stub.query.Offset != 0) {
				t.Fatal("query lost")
			}
		}
	}
}
func TestWeComContactHistoryAPIEmptyUnavailableAndInvalid(t *testing.T) {
	for _, kind := range []string{"events", "relations"} {
		stub := wecomContactHistoryAPIFixture()
		stub.empty = true
		router := wecomContactHistoryAPIRouter(t, stub, authport.RoleAdmin)
		out := httptest.NewRecorder()
		router.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != 200 || !strings.Contains(out.Body.String(), `"items":[]`) {
			t.Fatalf("empty %d %s", out.Code, out.Body.String())
		}
		for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=2&limit=3", "?unknown=1", "/0", "/-1", "/07", "/7?limit=1"} {
			out = httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind+suffix, legacyToken(101)))
			if out.Code != 400 {
				t.Fatalf("%s expected400 got%d", suffix, out.Code)
			}
		}
		stub.err = errors.New("PRIVATE-ERROR")
		out = httptest.NewRecorder()
		router.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != 503 || strings.Contains(out.Body.String(), "PRIVATE-ERROR") {
			t.Fatal("unavailable not closed")
		}
		var nilReader *wecomContactHistoryAPIStub
		out = httptest.NewRecorder()
		wecomContactHistoryAPIRouter(t, nilReader, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != 503 {
			t.Fatal("typed nil not closed")
		}
		out = httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, nil))
		if out.Code != 401 {
			t.Fatalf("anonymous=%d", out.Code)
		}
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			out = httptest.NewRecorder()
			router.ServeHTTP(out, legacyRequest(method, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
			if out.Code >= 200 && out.Code < 300 {
				t.Fatal("write accepted")
			}
		}
	}
}

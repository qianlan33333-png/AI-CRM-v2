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

type contactReferenceHistoryAPIStub struct {
	err   error
	empty bool
	query contactport.ReferenceHistoryQuery
	b     contactport.HistoricalExternalContactBinding
	d     contactport.HistoricalWeComDirectoryMember
}

func (s *contactReferenceHistoryAPIStub) GetHistoricalExternalContactBinding(_ context.Context, _ int64) (contactport.HistoricalExternalContactBinding, error) {
	return s.b, s.err
}
func (s *contactReferenceHistoryAPIStub) ListHistoricalExternalContactBinding(_ context.Context, q contactport.ReferenceHistoryQuery) ([]contactport.HistoricalExternalContactBinding, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalExternalContactBinding{s.b}, 1, s.err
}
func (s *contactReferenceHistoryAPIStub) GetHistoricalWeComDirectoryMember(_ context.Context, _ int64) (contactport.HistoricalWeComDirectoryMember, error) {
	return s.d, s.err
}
func (s *contactReferenceHistoryAPIStub) ListHistoricalWeComDirectoryMember(_ context.Context, q contactport.ReferenceHistoryQuery) ([]contactport.HistoricalWeComDirectoryMember, int64, error) {
	s.query = q
	if s.empty {
		return nil, 0, s.err
	}
	return []contactport.HistoricalWeComDirectoryMember{s.d}, 1, s.err
}

func contactReferenceHistoryAPIFixture() *contactReferenceHistoryAPIStub {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456000, time.UTC)
	status := int32(-2)
	return &contactReferenceHistoryAPIStub{
		b: contactport.HistoricalExternalContactBinding{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, ExternalUserIDDigest: [32]byte{1}, SourcePersonID: -1, IdentityAssurance: "unresolved", FirstBoundByUserIDDigest: [32]byte{1}, FirstOwnerUserIDDigest: [32]byte{1}, LastOwnerUserIDDigest: [32]byte{1}, CreatedAt: at, UpdatedAt: at},
		d: contactport.HistoricalWeComDirectoryMember{ID: 7, SourceKeyDigest: [32]byte{1}, SourcePayloadDigest: [32]byte{1}, SourceFieldDigest: [32]byte{1}, SourceID: -1, WeComCorpIDDigest: [32]byte{1}, CorpIDDigest: [32]byte{1}, WeComUserIDDigest: [32]byte{1}, CorpAttribution: "unattributable", DisplayName: "", DepartmentIDsDigest: [32]byte{1}, DepartmentName: "", Position: "", WeComStatus: &status, IsActive: false, SyncedAt: at, RawPayloadDigest: [32]byte{1}, MobileDigest: [32]byte{1}, AvatarURLDigest: [32]byte{1}, UpdatedByDigest: [32]byte{1}, FirstSeenAt: at, LastSyncedAt: at, CreatedAt: at, UpdatedAt: at},
	}
}

func contactReferenceHistoryAPIRouter(t *testing.T, reader contactport.ReferenceHistoryReader, role authport.Role) http.Handler {
	t.Helper()
	auth := &audienceHistoryAPIAuth{role: role}
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.contactReferenceHistory = reader
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

func TestContactReferenceHistoryAPIReadOnlyEnvelope(t *testing.T) {
	for _, kind := range []string{"bindings", "directory"} {
		for _, suffix := range []string{"?limit=5&offset=0", "/7"} {
			stub := contactReferenceHistoryAPIFixture()
			out := httptest.NewRecorder()
			contactReferenceHistoryAPIRouter(t, stub, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind+suffix, legacyToken(101)))
			if out.Code != http.StatusOK || out.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s%s: code=%d body=%s", kind, suffix, out.Code, out.Body.String())
			}
			body := out.Body.String()
			required := []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`}
			if kind == "bindings" {
				required = append(required, `"source_person_id":-1`)
			} else {
				required = append(required, `"source_id":-1`)
			}
			for _, required := range required {
				if !strings.Contains(body, required) {
					t.Fatalf("missing %s", required)
				}
			}
			for _, private := range []string{"source_key_digest", "source_payload_digest", "source_field_digest", "external_user_id_digest", "wecom_user_id_digest", "raw_payload_digest", "mobile_digest", "avatar_url_digest", "updated_by_digest"} {
				if strings.Contains(body, private) {
					t.Fatalf("private field exposed: %s", private)
				}
			}
			if suffix[0] == '?' && (stub.query.Limit != 5 || stub.query.Offset != 0) {
				t.Fatalf("query lost: %+v", stub.query)
			}
		}
	}
}

func TestContactReferenceHistoryAPIClosedFailureAndNoWrites(t *testing.T) {
	for _, kind := range []string{"bindings", "directory"} {
		stub := contactReferenceHistoryAPIFixture()
		admin := contactReferenceHistoryAPIRouter(t, stub, authport.RoleAdmin)
		stub.empty = true
		out := httptest.NewRecorder()
		admin.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != http.StatusOK || !strings.Contains(out.Body.String(), `"items":[]`) {
			t.Fatalf("empty=%d %s", out.Code, out.Body.String())
		}
		for _, suffix := range []string{"?limit=0", "?limit=101", "?offset=-1", "?limit=2&limit=3", "?unknown=1", "/0", "/-1", "/07", "/7?limit=1"} {
			out = httptest.NewRecorder()
			admin.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind+suffix, legacyToken(101)))
			if out.Code != http.StatusBadRequest {
				t.Fatalf("%s expected400 got%d", suffix, out.Code)
			}
		}
		out = httptest.NewRecorder()
		admin.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind+"/8", legacyToken(101)))
		if out.Code != http.StatusServiceUnavailable {
			t.Fatalf("wrong detail id=%d", out.Code)
		}
		stub.err = errors.New("PRIVATE-ERROR")
		out = httptest.NewRecorder()
		admin.ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != http.StatusServiceUnavailable || strings.Contains(out.Body.String(), "PRIVATE-ERROR") {
			t.Fatal("reader failure not closed")
		}
		var nilReader *contactReferenceHistoryAPIStub
		out = httptest.NewRecorder()
		contactReferenceHistoryAPIRouter(t, nilReader, authport.RoleAdmin).ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != http.StatusServiceUnavailable {
			t.Fatal("typed nil reader not closed")
		}
		out = httptest.NewRecorder()
		admin.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, nil))
		if out.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous=%d", out.Code)
		}
		out = httptest.NewRecorder()
		contactReferenceHistoryAPIRouter(t, contactReferenceHistoryAPIFixture(), authport.RoleOps).ServeHTTP(out, legacyRequest(http.MethodGet, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
		if out.Code != http.StatusForbidden {
			t.Fatalf("non-admin=%d", out.Code)
		}
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			out = httptest.NewRecorder()
			admin.ServeHTTP(out, legacyRequest(method, "/api/admin/wecom-contact-history/"+kind, legacyToken(101)))
			if out.Code >= 200 && out.Code < 300 {
				t.Fatal("write accepted")
			}
		}
	}
}

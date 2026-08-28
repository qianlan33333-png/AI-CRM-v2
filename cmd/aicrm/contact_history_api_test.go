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

type contactHistoryAPIStub struct {
	err error

	sidebarRows  []contactport.HistoricalSidebarProfile
	sidebarTotal int64
	sidebarItem  contactport.HistoricalSidebarProfile
	ownerRows    []contactport.HistoricalOwnerMigrationResult
	ownerTotal   int64
	ownerItem    contactport.HistoricalOwnerMigrationResult

	calls        int
	sidebarQuery contactport.ContactHistoryQuery
	ownerQuery   contactport.ContactHistoryQuery
	sidebarID    int64
	ownerID      int64
}

func (stub *contactHistoryAPIStub) ListHistoricalSidebarProfiles(_ context.Context, query contactport.ContactHistoryQuery) ([]contactport.HistoricalSidebarProfile, int64, error) {
	stub.calls++
	stub.sidebarQuery = query
	return stub.sidebarRows, stub.sidebarTotal, stub.err
}

func (stub *contactHistoryAPIStub) GetHistoricalSidebarProfile(_ context.Context, id int64) (contactport.HistoricalSidebarProfile, error) {
	stub.calls++
	stub.sidebarID = id
	return stub.sidebarItem, stub.err
}

func (stub *contactHistoryAPIStub) ListHistoricalOwnerMigrationResults(_ context.Context, query contactport.ContactHistoryQuery) ([]contactport.HistoricalOwnerMigrationResult, int64, error) {
	stub.calls++
	stub.ownerQuery = query
	return stub.ownerRows, stub.ownerTotal, stub.err
}

func (stub *contactHistoryAPIStub) GetHistoricalOwnerMigrationResult(_ context.Context, id int64) (contactport.HistoricalOwnerMigrationResult, error) {
	stub.calls++
	stub.ownerID = id
	return stub.ownerItem, stub.err
}

type contactHistoryAPIAuth struct {
	principal authport.Principal
	csrfCalls int
}

func (stub *contactHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return stub.principal, nil
}

func (stub *contactHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.AdminUserID < 1 || principal.Role != authport.RoleAdmin || capability != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
}

func (stub *contactHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	stub.csrfCalls++
	return nil
}

func (*contactHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func contactHistoryAPIRouter(t *testing.T, history contactport.ContactHistoryReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.contactHistory = history
	authHandler, err := authhttp.NewHandler(auth)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(slog.New(slog.NewJSONHandler(io.Discard, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), authHandler, authHandler, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestContactHistoryFinalRoutesPreserveTypedReadonlyFacts(t *testing.T) {
	customerID := int64(17)
	sidebarList := contactHistorySidebarAPIValue(31, &customerID, "source preserved", "industry", "description", "needs\nfollow-up")
	sidebarDetail := contactHistorySidebarAPIValue(32, nil, "", "", "", "")
	owner := contactHistoryOwnerAPIValue(41)
	history := &contactHistoryAPIStub{
		sidebarRows:  []contactport.HistoricalSidebarProfile{sidebarList},
		sidebarTotal: 1,
		sidebarItem:  sidebarDetail,
		ownerRows:    []contactport.HistoricalOwnerMigrationResult{owner},
		ownerTotal:   1,
		ownerItem:    owner,
	}
	auth := &contactHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}}
	router := contactHistoryAPIRouter(t, history, auth)

	for _, test := range []struct {
		path string
		want []string
	}{
		{
			path: "/api/admin/contact-history/sidebar-profiles?customer_id=17&limit=1&offset=0",
			want: []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"customer_id":17`, `"needs_blockers_followup":"needs\nfollow-up"`, `"updated_at":"2026-08-28T01:02:03.123456Z"`},
		},
		{
			path: "/api/admin/contact-history/sidebar-profiles/32",
			want: []string{`"customer_id":null`, `"source":""`, `"industry_description":""`, `"updated_at":"2026-08-28T01:02:03.123456Z"`},
		},
		{
			path: "/api/admin/contact-history/owner-migration-results?limit=1&offset=0",
			want: []string{`"scope_type":"all"`, `"transfer_welcome_message":""`, `"session_relation":"unresolved"`, `"preview_relation":"resolved"`, `"created_at":"2026-08-28T01:02:03.123456Z"`, `"executed_at":"2026-08-28T02:02:03.123456Z"`},
		},
		{
			path: "/api/admin/contact-history/owner-migration-results/41",
			want: []string{`"total_rows":7`, `"wecom_success":3`, `"crm_updated":4`, `"include_wecom_transfer":false`},
		},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(101)))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status/cache=%d/%q body=%s", test.path, response.Code, response.Header().Get("Cache-Control"), response.Body.String())
		}
		for _, want := range test.want {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s missing %s in %s", test.path, want, response.Body.String())
			}
		}
		for _, forbidden := range []string{"raw_payload", "source_identifier", "unionid", "preview_token", "session_token", `"rows":`} {
			if strings.Contains(response.Body.String(), forbidden) {
				t.Fatalf("%s leaked %q: %s", test.path, forbidden, response.Body.String())
			}
		}
	}
	if history.sidebarQuery.Limit != 1 || history.sidebarQuery.Offset != 0 || history.sidebarQuery.CustomerID == nil || *history.sidebarQuery.CustomerID != customerID || history.ownerQuery.Limit != 1 || history.ownerQuery.Offset != 0 || history.ownerQuery.CustomerID != nil || history.sidebarID != 32 || history.ownerID != 41 || auth.csrfCalls != 0 {
		t.Fatalf("reader/auth input changed: %+v %+v sidebar=%d owner=%d csrf=%d", history.sidebarQuery, history.ownerQuery, history.sidebarID, history.ownerID, auth.csrfCalls)
	}
}

func TestContactHistoryFinalRoutesRejectInvalidQueriesBeforeReading(t *testing.T) {
	history := &contactHistoryAPIStub{}
	router := contactHistoryAPIRouter(t, history, &contactHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}})
	for _, path := range []string{
		"/api/admin/contact-history/sidebar-profiles?limit=0",
		"/api/admin/contact-history/sidebar-profiles?limit=101",
		"/api/admin/contact-history/sidebar-profiles?limit=1&limit=2",
		"/api/admin/contact-history/sidebar-profiles?offset=-1",
		"/api/admin/contact-history/sidebar-profiles?customer_id=01",
		"/api/admin/contact-history/sidebar-profiles?unknown=1",
		"/api/admin/contact-history/sidebar-profiles?limit=%zz",
		"/api/admin/contact-history/sidebar-profiles/0",
		"/api/admin/contact-history/sidebar-profiles/01",
		"/api/admin/contact-history/sidebar-profiles/1?limit=1",
		"/api/admin/contact-history/owner-migration-results?customer_id=17",
		"/api/admin/contact-history/owner-migration-results?offset=2147483648",
		"/api/admin/contact-history/owner-migration-results/0",
		"/api/admin/contact-history/owner-migration-results/1?offset=0",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, path, legacyToken(102)))
		if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: status/cache=%d/%q body=%s", path, response.Code, response.Header().Get("Cache-Control"), response.Body.String())
		}
	}
	if history.calls != 0 {
		t.Fatalf("invalid query invoked reader %d times", history.calls)
	}
}

func TestContactHistoryFinalRoutesFailClosedForReaderAndDataInconsistency(t *testing.T) {
	validSidebar := contactHistorySidebarAPIValue(51, nil, "", "", "", "")
	validOwner := contactHistoryOwnerAPIValue(61)
	for _, test := range []struct {
		name    string
		history contactport.ContactHistoryReader
		path    string
	}{
		{name: "typed nil reader", history: contactHistoryTypedNilReader(), path: "/api/admin/contact-history/sidebar-profiles"},
		{name: "downstream error", history: &contactHistoryAPIStub{err: errors.New("private contact history error")}, path: "/api/admin/contact-history/sidebar-profiles"},
		{name: "invalid digest", history: &contactHistoryAPIStub{sidebarRows: []contactport.HistoricalSidebarProfile{{ID: 51, UpdatedAt: validSidebar.UpdatedAt}}, sidebarTotal: 1}, path: "/api/admin/contact-history/sidebar-profiles"},
		{name: "wrong sidebar customer", history: &contactHistoryAPIStub{sidebarRows: []contactport.HistoricalSidebarProfile{validSidebar}, sidebarTotal: 1}, path: "/api/admin/contact-history/sidebar-profiles?customer_id=17"},
		{name: "wrong page total", history: &contactHistoryAPIStub{ownerRows: []contactport.HistoricalOwnerMigrationResult{validOwner}, ownerTotal: 0}, path: "/api/admin/contact-history/owner-migration-results"},
		{name: "wrong detail ID", history: &contactHistoryAPIStub{ownerItem: validOwner}, path: "/api/admin/contact-history/owner-migration-results/62"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			contactHistoryAPIRouter(t, test.history, &contactHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}}).ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(103)))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), `"items"`) || strings.Contains(response.Body.String(), `"item"`) {
				t.Fatalf("status/cache/body=%d/%q/%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
			}
		})
	}
}

func TestContactHistoryFinalRoutesRequireAdminReadAndOnlyGet(t *testing.T) {
	paths := []string{
		"/api/admin/contact-history/sidebar-profiles",
		"/api/admin/contact-history/sidebar-profiles/71",
		"/api/admin/contact-history/owner-migration-results",
		"/api/admin/contact-history/owner-migration-results/71",
	}
	for _, test := range []struct {
		name      string
		principal authport.Principal
		cookie    bool
		want      int
	}{
		{name: "anonymous", want: http.StatusUnauthorized},
		{name: "ops", principal: authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, cookie: true, want: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &contactHistoryAPIStub{}
			router := contactHistoryAPIRouter(t, history, &contactHistoryAPIAuth{principal: test.principal})
			for _, path := range paths {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				if test.cookie {
					request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: legacyToken(104)})
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != test.want {
					t.Fatalf("%s: status=%d want=%d", path, response.Code, test.want)
				}
			}
			if history.calls != 0 {
				t.Fatalf("unauthorized route invoked reader %d times", history.calls)
			}
		})
	}

	history := &contactHistoryAPIStub{}
	auth := &contactHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}}
	router := contactHistoryAPIRouter(t, history, auth)
	for _, path := range paths {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(105)))
		if response.Code < http.StatusBadRequest || response.Code >= http.StatusInternalServerError {
			t.Fatalf("%s: non-GET unexpectedly succeeded: %d", path, response.Code)
		}
	}
	if history.calls != 0 {
		t.Fatalf("non-GET invoked reader %d times", history.calls)
	}
}

func contactHistorySidebarAPIValue(id int64, customerID *int64, source, industry, description, blockers string) contactport.HistoricalSidebarProfile {
	return contactport.HistoricalSidebarProfile{
		ID: id, SourceKeyDigest: contactHistoryAPIDigest(byte(id)), CustomerID: customerID, Source: source, Industry: industry,
		IndustryDescription: description, NeedsBlockersFollowup: blockers,
		UpdatedAt: time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC), SourcePayloadDigest: contactHistoryAPIDigest(byte(id + 1)),
	}
}

func contactHistoryOwnerAPIValue(id int64) contactport.HistoricalOwnerMigrationResult {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	return contactport.HistoricalOwnerMigrationResult{
		ID: id, SourceKeyDigest: contactHistoryAPIDigest(byte(id)), ScopeType: "all", FileHash: "file-hash", PreviewHash: "preview-hash",
		TotalRows: 7, EligibleCount: 6, WeComSuccess: 3, WeComFailed: 1, CRMUpdated: 4, IncludeWeComTransfer: false,
		TransferWelcomeMessage: "", SessionRelation: "unresolved", PreviewRelation: "resolved", CreatedAt: at, ExecutedAt: at.Add(time.Hour),
		SourcePayloadDigest: contactHistoryAPIDigest(byte(id + 1)),
	}
}

func contactHistoryAPIDigest(seed byte) [32]byte {
	var digest [32]byte
	digest[0] = seed
	return digest
}

func contactHistoryTypedNilReader() contactport.ContactHistoryReader {
	var reader *contactHistoryAPIStub
	return reader
}

var _ contactport.ContactHistoryReader = (*contactHistoryAPIStub)(nil)
var _ authport.Service = (*contactHistoryAPIAuth)(nil)

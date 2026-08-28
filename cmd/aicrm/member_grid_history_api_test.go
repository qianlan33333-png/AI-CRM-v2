package main

import (
	"context"
	"crypto/sha256"
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
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type memberGridHistoryAPIStub struct {
	err error

	views      []productport.HistoricalMemberView
	viewTotal  int64
	view       productport.HistoricalMemberView
	usage      []productport.HistoricalMemberUsage
	usageTotal int64
	usageItem  productport.HistoricalMemberUsage

	viewQuery  productport.MemberGridHistoryQuery
	usageQuery productport.MemberGridHistoryQuery
	viewID     int64
	usageID    int64
	calls      int
}

func (stub *memberGridHistoryAPIStub) GetHistoricalMemberView(_ context.Context, id int64) (productport.HistoricalMemberView, error) {
	stub.calls++
	stub.viewID = id
	return stub.view, stub.err
}

func (stub *memberGridHistoryAPIStub) ListHistoricalMemberViews(_ context.Context, query productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberView, int64, error) {
	stub.calls++
	stub.viewQuery = query
	return stub.views, stub.viewTotal, stub.err
}

func (stub *memberGridHistoryAPIStub) GetHistoricalMemberUsage(_ context.Context, id int64) (productport.HistoricalMemberUsage, error) {
	stub.calls++
	stub.usageID = id
	return stub.usageItem, stub.err
}

func (stub *memberGridHistoryAPIStub) ListHistoricalMemberUsage(_ context.Context, query productport.MemberGridHistoryQuery) ([]productport.HistoricalMemberUsage, int64, error) {
	stub.calls++
	stub.usageQuery = query
	return stub.usage, stub.usageTotal, stub.err
}

type memberGridHistoryAPIAuth struct {
	principal authport.Principal
	csrfCalls int
}

func (auth *memberGridHistoryAPIAuth) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	return auth.principal, nil
}

func (auth *memberGridHistoryAPIAuth) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.AdminUserID < 1 || principal.Role != authport.RoleAdmin || capability != authport.CapabilityAdminRead {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	return authport.Authorization{Capability: authport.CapabilityAdminRead, Scope: authport.ScopeGlobal}, nil
}

func (auth *memberGridHistoryAPIAuth) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	auth.csrfCalls++
	return nil
}

func (*memberGridHistoryAPIAuth) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

func memberGridHistoryAPIRouter(t *testing.T, history productport.MemberGridHistoryReader, auth authport.Service) http.Handler {
	t.Helper()
	legacy, err := NewHandler(auth, &legacyCustomerStub{})
	if err != nil {
		t.Fatal(err)
	}
	legacy.memberGridHistory = history
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

func TestMemberGridHistoryFinalRoutesPreserveReadonlyFacts(t *testing.T) {
	productID, customerID := int64(7), int64(9)
	view := memberGridHistoryAPIView(11, &productID)
	usage := memberGridHistoryAPIUsage(12, &customerID)
	stub := &memberGridHistoryAPIStub{views: []productport.HistoricalMemberView{view}, viewTotal: 1, view: memberGridHistoryAPIView(13, nil), usage: []productport.HistoricalMemberUsage{usage}, usageTotal: 1, usageItem: memberGridHistoryAPIUsage(14, nil)}
	auth := &memberGridHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}}
	router := memberGridHistoryAPIRouter(t, stub, auth)

	for _, test := range []struct {
		path       string
		want       []string
		wantCalls  int
		wantQuery  productport.MemberGridHistoryQuery
		wantDetail int64
	}{
		{"/api/admin/member-grid-history/views?product_id=7&limit=1&offset=0", []string{`"source":"v1_history"`, `"read_only":true`, `"real_external_call_executed":false`, `"position":-2`, `"schema_version":-3`, `"version":-4`, `"is_default":false`}, 1, productport.MemberGridHistoryQuery{ProductID: &productID, Limit: 1}, 0},
		{"/api/admin/member-grid-history/views/13", []string{`"product_id":null`, `"name":""`, `"position":-2`}, 2, productport.MemberGridHistoryQuery{}, 13},
		{"/api/admin/member-grid-history/usage?customer_id=9&limit=1&offset=0", []string{`"formally_logged_in":false`, `"has_token_usage":false`, `"learning_plan_id":""`, `"last_open_at":null`, `"customer_id":9`}, 3, productport.MemberGridHistoryQuery{CustomerID: &customerID, Limit: 1}, 0},
		{"/api/admin/member-grid-history/usage/14", []string{`"customer_id":null`, `"learning_plan_current":null`, `"learning_plan_total":null`, `"last_open_at":null`}, 4, productport.MemberGridHistoryQuery{}, 14},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(71)))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" || stub.calls != test.wantCalls {
			t.Fatalf("%s status=%d calls=%d body=%s", test.path, response.Code, stub.calls, response.Body.String())
		}
		for _, want := range test.want {
			if !strings.Contains(response.Body.String(), want) {
				t.Fatalf("%s lost %s: %s", test.path, want, response.Body.String())
			}
		}
		if test.wantDetail != 0 {
			if strings.Contains(test.path, "/views/") && stub.viewID != test.wantDetail || strings.Contains(test.path, "/usage/") && stub.usageID != test.wantDetail {
				t.Fatalf("detail ID not passed for %s", test.path)
			}
		}
	}
	if stub.viewQuery.ProductID == nil || *stub.viewQuery.ProductID != productID || stub.usageQuery.CustomerID == nil || *stub.usageQuery.CustomerID != customerID || auth.csrfCalls != 0 {
		t.Fatalf("filter/csrf lost: view=%#v usage=%#v csrf=%d", stub.viewQuery, stub.usageQuery, auth.csrfCalls)
	}
}

func TestMemberGridHistoryRoutesRejectUnauthorizedQueriesAndWrites(t *testing.T) {
	paths := []string{"/api/admin/member-grid-history/views", "/api/admin/member-grid-history/views/11", "/api/admin/member-grid-history/usage", "/api/admin/member-grid-history/usage/12"}
	for _, test := range []struct {
		name, path string
		request    *http.Request
		want       int
	}{
		{"anonymous", paths[0], httptest.NewRequest(http.MethodGet, paths[0], nil), http.StatusUnauthorized},
		{"ops", paths[0], legacyRequest(http.MethodGet, paths[0], legacyToken(72)), http.StatusForbidden},
	} {
		stub := &memberGridHistoryAPIStub{}
		auth := &memberGridHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleOps}}
		if test.name == "anonymous" {
			auth.principal = authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}
		}
		response := httptest.NewRecorder()
		memberGridHistoryAPIRouter(t, stub, auth).ServeHTTP(response, test.request)
		if response.Code != test.want || stub.calls != 0 || auth.csrfCalls != 0 {
			t.Fatalf("%s status=%d calls=%d csrf=%d", test.name, response.Code, stub.calls, auth.csrfCalls)
		}
	}

	for _, path := range paths {
		stub := &memberGridHistoryAPIStub{}
		router := memberGridHistoryAPIRouter(t, stub, &memberGridHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodPost, path, legacyToken(73)))
		if response.Code < http.StatusBadRequest || response.Code >= http.StatusInternalServerError || stub.calls != 0 {
			t.Fatalf("non-GET %s status=%d calls=%d", path, response.Code, stub.calls)
		}
	}

	for _, test := range []struct{ path, query string }{
		{paths[0], "customer_id=1"}, {paths[0], "product_id=1&product_id=2"}, {paths[0], "product_id=01"}, {paths[0], "limit=0"}, {paths[0], "limit=101"}, {paths[0], "limit=1&limit=2"}, {paths[0], "offset=-1"}, {paths[0], "offset=2147483648"}, {paths[0], "unknown=1"},
		{paths[2], "product_id=1"}, {paths[2], "customer_id=0"}, {paths[2], "customer_id=1&customer_id=2"}, {paths[1], "limit=1"}, {paths[3], "offset=1"}, {"/api/admin/member-grid-history/views/0", ""}, {"/api/admin/member-grid-history/usage/not-a-number", ""},
	} {
		stub := &memberGridHistoryAPIStub{}
		router := memberGridHistoryAPIRouter(t, stub, &memberGridHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}})
		target := test.path
		if test.query != "" {
			target += "?" + test.query
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, legacyRequest(http.MethodGet, target, legacyToken(74)))
		if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" || stub.calls != 0 {
			t.Fatalf("invalid %s status=%d calls=%d", target, response.Code, stub.calls)
		}
	}
}

func TestMemberGridHistoryRoutesFailClosedForReaderAndTargetInconsistency(t *testing.T) {
	productID, customerID := int64(7), int64(9)
	validView, validUsage := memberGridHistoryAPIView(11, &productID), memberGridHistoryAPIUsage(12, &customerID)
	duplicateView := validView
	badDigest := validUsage
	badDigest.SourcePayloadDigest = [sha256.Size]byte{}
	for _, test := range []struct {
		name, path string
		stub       *memberGridHistoryAPIStub
	}{
		{"typed nil", "/api/admin/member-grid-history/views", nil},
		{"reader error", "/api/admin/member-grid-history/views", &memberGridHistoryAPIStub{err: errors.New("private source failure")}},
		{"missing detail", "/api/admin/member-grid-history/views/11", &memberGridHistoryAPIStub{}},
		{"wrong detail ID", "/api/admin/member-grid-history/usage/12", &memberGridHistoryAPIStub{usageItem: memberGridHistoryAPIUsage(13, &customerID)}},
		{"wrong product filter", "/api/admin/member-grid-history/views?product_id=7", &memberGridHistoryAPIStub{views: []productport.HistoricalMemberView{memberGridHistoryAPIView(11, nil)}, viewTotal: 1}},
		{"wrong customer filter", "/api/admin/member-grid-history/usage?customer_id=9", &memberGridHistoryAPIStub{usage: []productport.HistoricalMemberUsage{memberGridHistoryAPIUsage(12, nil)}, usageTotal: 1}},
		{"duplicate item", "/api/admin/member-grid-history/views", &memberGridHistoryAPIStub{views: []productport.HistoricalMemberView{validView, duplicateView}, viewTotal: 2}},
		{"page count", "/api/admin/member-grid-history/usage", &memberGridHistoryAPIStub{usage: []productport.HistoricalMemberUsage{validUsage}, usageTotal: 2}},
		{"invalid digest", "/api/admin/member-grid-history/usage/12", &memberGridHistoryAPIStub{usageItem: badDigest}},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := &memberGridHistoryAPIAuth{principal: authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}}
			var reader productport.MemberGridHistoryReader = test.stub
			if test.stub == nil {
				var typedNil *memberGridHistoryAPIStub
				reader = typedNil
			}
			response := httptest.NewRecorder()
			memberGridHistoryAPIRouter(t, reader, auth).ServeHTTP(response, legacyRequest(http.MethodGet, test.path, legacyToken(75)))
			if response.Code != http.StatusServiceUnavailable || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), `"items"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func memberGridHistoryAPIView(id int64, productID *int64) productport.HistoricalMemberView {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	return productport.HistoricalMemberView{
		ID: id, SourceKeyDigest: memberGridHistoryAPIDigest(byte(id)), SourceViewID: id + 100, SourceServiceProductID: id + 200, ProductID: productID,
		Name: "", Position: -2, IsDefault: false, SchemaVersion: -3, ConfigDigest: memberGridHistoryAPIDigest(byte(id + 1)), Version: -4,
		CreatedAt: at, UpdatedAt: at, SourcePayloadDigest: memberGridHistoryAPIDigest(byte(id + 2)),
	}
}

func memberGridHistoryAPIUsage(id int64, customerID *int64) productport.HistoricalMemberUsage {
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	return productport.HistoricalMemberUsage{
		ID: id, SourceKeyDigest: memberGridHistoryAPIDigest(byte(id)), CustomerID: customerID, FormallyLoggedIn: false, HasTokenUsage: false,
		LearningPlanID: "", OpenCount7D: 0, LastOpenAt: nil, RefreshedAt: at, SourcePayloadDigest: memberGridHistoryAPIDigest(byte(id + 1)),
		RecoveryEntryDigest: memberGridHistoryAPIDigest(byte(id + 2)),
	}
}

func memberGridHistoryAPIDigest(seed byte) [sha256.Size]byte {
	var result [sha256.Size]byte
	result[0] = seed
	return result
}

var _ productport.MemberGridHistoryReader = (*memberGridHistoryAPIStub)(nil)
var _ authport.Service = (*memberGridHistoryAPIAuth)(nil)

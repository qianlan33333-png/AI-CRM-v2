package membergrid

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
)

type fakeApplication struct {
	accessResponse AccessResponse
	schemaResponse SchemaResponse
	viewsResponse  MemberViewsResponse
	queryResponse  QueryResponse
	accessErr      error
	schemaErr      error
	viewsErr       error
	queryErr       error
	lastProductID  int64
	lastQuery      QueryInput
	accessCalls    int
	schemaCalls    int
	viewsCalls     int
	queryCalls     int
}

func (application *fakeApplication) Access(_ context.Context, productID int64) (AccessResponse, error) {
	application.accessCalls++
	application.lastProductID = productID
	return application.accessResponse, application.accessErr
}

func (application *fakeApplication) Schema(_ context.Context, productID int64) (SchemaResponse, error) {
	application.schemaCalls++
	application.lastProductID = productID
	return application.schemaResponse, application.schemaErr
}

func (application *fakeApplication) MemberViews(_ context.Context, productID int64) (MemberViewsResponse, error) {
	application.viewsCalls++
	application.lastProductID = productID
	return application.viewsResponse, application.viewsErr
}

func (application *fakeApplication) Query(_ context.Context, input QueryInput) (QueryResponse, error) {
	application.queryCalls++
	application.lastQuery = input
	return application.queryResponse, application.queryErr
}

func testFragment(t *testing.T, application *fakeApplication) http.Handler {
	t.Helper()
	handler, err := NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func authorizedRequest(t *testing.T, method, path, body string, role authport.Role, capability authport.Capability) *http.Request {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(method, "http://membergrid.local"+path, reader)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{
		AdminUserID: 1,
		Role:        role,
	}, authport.SessionRef("test-session"))
	if capability != "" {
		var err error
		ctx, err = authport.WithAuthorization(ctx, authport.Authorization{
			Capability: capability,
			Scope:      authport.ScopeGlobal,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return request.WithContext(ctx)
}

func assertSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q", got)
	}
}

func TestRouteFragmentServesFourClosedRoutes(t *testing.T) {
	stamp := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	application := &fakeApplication{
		accessResponse: AccessResponse{ProductID: 17, CanView: true, CanQuery: true},
		schemaResponse: SchemaResponse{ServiceProductID: 17, Columns: cloneColumns(safeColumns)},
		viewsResponse:  MemberViewsResponse{ProductID: 17, Views: append([]MemberView(nil), builtInViews...)},
		queryResponse:  QueryResponse{Rows: []MemberRow{{MemberRef: "spm_0000000000000000000009", ServiceProductID: 17, CustomerID: 9, State: "active", Source: "manual", StartsAt: stamp, Version: 1, UpdatedAt: stamp, DisplayName: "安全客户"}}, Limit: 10},
	}
	fragment := testFragment(t, application)

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		role       authport.Role
		capability authport.Capability
		wantCall   *int
	}{
		{name: "access", method: http.MethodGet, path: "/17/member-grid/access", role: authport.RoleAdmin, capability: authport.CapabilityProductsRead, wantCall: &application.accessCalls},
		{name: "schema", method: http.MethodGet, path: RoutePrefix + "/17/member-grid/schema", role: authport.RoleOps, capability: authport.CapabilityProductsRead, wantCall: &application.schemaCalls},
		{name: "views", method: http.MethodGet, path: "/17/member-views", role: authport.RoleAdmin, capability: authport.CapabilityProductsRead, wantCall: &application.viewsCalls},
		{name: "query", method: http.MethodPost, path: "/17/member-grid/query", body: `{"state":"active","source":"manual","limit":10,"cursor":""}`, role: authport.RoleOps, capability: authport.CapabilityEntitlementsRead, wantCall: &application.queryCalls},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			before := *testCase.wantCall
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, authorizedRequest(t, testCase.method, testCase.path, testCase.body, testCase.role, testCase.capability))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			assertSecurityHeaders(t, recorder)
			if *testCase.wantCall != before+1 {
				t.Fatalf("call count=%d want=%d", *testCase.wantCall, before+1)
			}
		})
	}
	if application.lastQuery.ProductID != 17 || application.lastQuery.State != StateActive || application.lastQuery.Source != SourceManual || application.lastQuery.Limit != 10 || application.lastQuery.Cursor != "" {
		t.Fatalf("query input=%+v", application.lastQuery)
	}
}

func TestQueryResponseUsesOnlyClosedSafeFields(t *testing.T) {
	stamp := time.Now().UTC()
	application := &fakeApplication{queryResponse: QueryResponse{
		Rows:  []MemberRow{{MemberRef: "spm_0000000000000000000004", ServiceProductID: 2, CustomerID: 4, State: "removed", Source: "manual", StartsAt: stamp.Add(-time.Hour), RemovedAt: &stamp, Version: 2, UpdatedAt: stamp, DisplayName: "本地客户"}},
		Limit: 1, NextCursor: "opaque", HasMore: true,
	}}
	recorder := httptest.NewRecorder()
	testFragment(t, application).ServeHTTP(recorder, authorizedRequest(t, http.MethodPost, "/2/member-grid/query", `{}`, authport.RoleAdmin, authport.CapabilityEntitlementsRead))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
	}
	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{
		"unionid", "external_userid", "order_id", "granted_by", "revoked_by", "entitlement_id", "granted_at", "revoked_at", "masked_mobile", "remark", "alliance",
		"receipt", "provider", "raw_mobile", "payment", "refund",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 4 || decoded["rows"] == nil || decoded["limit"] == nil || decoded["next_cursor"] == nil || decoded["has_more"] == nil {
		t.Fatalf("top-level fields=%v", decoded)
	}
	assertSecurityHeaders(t, recorder)
}

func TestAuthorizationFailsClosedByRoleAndCapability(t *testing.T) {
	application := &fakeApplication{accessResponse: AccessResponse{ProductID: 1}, queryResponse: QueryResponse{Rows: []MemberRow{}, Limit: 50}}
	fragment := testFragment(t, application)

	unauthenticated := httptest.NewRequest(http.MethodGet, "http://membergrid.local/1/member-grid/access", nil)
	cases := []struct {
		name       string
		request    *http.Request
		wantStatus int
	}{
		{name: "unauthenticated", request: unauthenticated, wantStatus: http.StatusUnauthorized},
		{name: "sales", request: authorizedRequest(t, http.MethodGet, "/1/member-grid/access", "", authport.RoleSales, authport.CapabilityProductsRead), wantStatus: http.StatusForbidden},
		{name: "no capability", request: authorizedRequest(t, http.MethodGet, "/1/member-grid/access", "", authport.RoleAdmin, ""), wantStatus: http.StatusForbidden},
		{name: "wrong metadata capability", request: authorizedRequest(t, http.MethodGet, "/1/member-grid/access", "", authport.RoleAdmin, authport.CapabilityEntitlementsRead), wantStatus: http.StatusForbidden},
		{name: "wrong query capability", request: authorizedRequest(t, http.MethodPost, "/1/member-grid/query", `{}`, authport.RoleAdmin, authport.CapabilityProductsRead), wantStatus: http.StatusForbidden},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, testCase.request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			assertSecurityHeaders(t, recorder)
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("noncanonical error body: %v body=%s", err, recorder.Body.String())
			}
			if response["code"] == nil || response["message"] == nil || response["request_id"] == nil {
				t.Fatalf("error fields=%v", response)
			}
		})
	}
	if application.accessCalls != 0 || application.queryCalls != 0 {
		t.Fatalf("unauthorized request reached application: access/query=%d/%d", application.accessCalls, application.queryCalls)
	}
}

func TestInvalidIDsPathsQueriesAndMethodsAreRejected(t *testing.T) {
	application := &fakeApplication{accessResponse: AccessResponse{ProductID: 1}}
	fragment := testFragment(t, application)
	for _, rawID := range []string{"0", "-1", "01", "+1", "9223372036854775808"} {
		t.Run("id_"+rawID, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, authorizedRequest(t, http.MethodGet, "/"+rawID+"/member-grid/access", "", authport.RoleAdmin, authport.CapabilityProductsRead))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			assertSecurityHeaders(t, recorder)
		})
	}

	pathCases := []struct {
		path       string
		wantStatus int
	}{
		{path: "/1/member-grid/access/extra", wantStatus: http.StatusNotFound},
		{path: "/1/member-grid/access/", wantStatus: http.StatusBadRequest},
		{path: "/1/member-grid/access?sort=id", wantStatus: http.StatusBadRequest},
		{path: "/%31/member-grid/access", wantStatus: http.StatusBadRequest},
		{path: "/1%2F2/member-grid/access", wantStatus: http.StatusBadRequest},
		{path: "/1%5C2/member-grid/access", wantStatus: http.StatusBadRequest},
	}
	for _, testCase := range pathCases {
		recorder := httptest.NewRecorder()
		fragment.ServeHTTP(recorder, authorizedRequest(t, http.MethodGet, testCase.path, "", authport.RoleAdmin, authport.CapabilityProductsRead))
		if recorder.Code != testCase.wantStatus {
			t.Errorf("path=%q status/body=%d/%s", testCase.path, recorder.Code, recorder.Body.String())
		}
		assertSecurityHeaders(t, recorder)
	}

	methodCases := []struct {
		path  string
		allow string
	}{
		{path: "/1/member-grid/access", allow: http.MethodGet},
		{path: "/1/member-grid/schema", allow: http.MethodGet},
		{path: "/1/member-views", allow: http.MethodGet},
		{path: "/1/member-grid/query", allow: http.MethodPost},
	}
	for _, testCase := range methodCases {
		method := http.MethodPost
		capability := authport.CapabilityProductsRead
		body := `{}`
		if testCase.allow == http.MethodPost {
			method = http.MethodGet
			capability = authport.CapabilityEntitlementsRead
			body = ""
		}
		recorder := httptest.NewRecorder()
		fragment.ServeHTTP(recorder, authorizedRequest(t, method, testCase.path, body, authport.RoleAdmin, capability))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != testCase.allow || recorder.Body.Len() != 0 {
			t.Errorf("path=%q status/allow/body=%d/%q/%q", testCase.path, recorder.Code, recorder.Header().Get("Allow"), recorder.Body.String())
		}
		assertSecurityHeaders(t, recorder)
	}
}

func TestQueryBodyIsClosedAndStrict(t *testing.T) {
	application := &fakeApplication{queryResponse: QueryResponse{Rows: []MemberRow{}, Limit: 50}}
	fragment := testFragment(t, application)
	invalidBodies := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "empty", body: "", wantStatus: http.StatusBadRequest},
		{name: "array", body: `[]`, wantStatus: http.StatusBadRequest},
		{name: "invalid sort", body: `{"sort":"id"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid group", body: `{"group_by":"source"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "legacy view", body: `{"view_id":"9"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "columns", body: `{"columns":["customer_id"]}`, wantStatus: http.StatusBadRequest},
		{name: "raw filter", body: `{"raw_filter":{"sql":"1=1"}}`, wantStatus: http.StatusBadRequest},
		{name: "duplicate", body: `{"limit":1,"limit":2}`, wantStatus: http.StatusBadRequest},
		{name: "invalid state", body: `{"state":"pending"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "null state", body: `{"state":null}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "all source", body: `{"source":"all"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid source", body: `{"source":"provider"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "zero limit", body: `{"limit":0}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "large limit", body: `{"limit":51}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "float limit", body: `{"limit":1.5}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "exponent limit", body: `{"limit":1e1}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "string limit", body: `{"limit":"10"}`, wantStatus: http.StatusUnprocessableEntity},
		{name: "numeric cursor", body: `{"cursor":4}`, wantStatus: http.StatusBadRequest},
		{name: "trailing", body: `{} {}`, wantStatus: http.StatusBadRequest},
		{name: "oversized", body: `{"cursor":"` + strings.Repeat("x", int(maximumQueryBodyBytes)) + `"}`, wantStatus: http.StatusBadRequest},
	}
	for _, testCase := range invalidBodies {
		t.Run(testCase.name, func(t *testing.T) {
			before := application.queryCalls
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, authorizedRequest(t, http.MethodPost, "/1/member-grid/query", testCase.body, authport.RoleAdmin, authport.CapabilityEntitlementsRead))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			if application.queryCalls != before {
				t.Fatal("invalid body reached application")
			}
			assertSecurityHeaders(t, recorder)
		})
	}

	request := authorizedRequest(t, http.MethodPost, "/1/member-grid/query", `{}`, authport.RoleAdmin, authport.CapabilityEntitlementsRead)
	request.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	fragment.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("content-type status/body=%d/%s", recorder.Code, recorder.Body.String())
	}

	for _, body := range []string{`{}`, `{"cursor":null}`, `{"cursor":""}`} {
		recorder = httptest.NewRecorder()
		fragment.ServeHTTP(recorder, authorizedRequest(t, http.MethodPost, "/1/member-grid/query", body, authport.RoleAdmin, authport.CapabilityEntitlementsRead))
		if recorder.Code != http.StatusOK || application.lastQuery.State != StateAll || application.lastQuery.Limit != DefaultLimit || application.lastQuery.Cursor != "" {
			t.Fatalf("empty cursor/default query body/status/input/response=%s/%d/%+v/%s", body, recorder.Code, application.lastQuery, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	fragment.ServeHTTP(recorder, authorizedRequest(t, http.MethodPost, "/1/member-grid/query", `{"sort":"starts_at_desc","group_by":"state"}`, authport.RoleAdmin, authport.CapabilityEntitlementsRead))
	if recorder.Code != http.StatusOK || application.lastQuery.Sort != "starts_at_desc" || application.lastQuery.GroupBy != "state" {
		t.Fatalf("selected query status/input/body=%d/%+v/%s", recorder.Code, application.lastQuery, recorder.Body.String())
	}
}

func TestApplicationFailuresUseCanonicalErrorShapeAndHeaders(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "cursor", err: ErrInvalidCursor, wantStatus: http.StatusBadRequest, wantCode: "CURSOR_INVALID"},
		{name: "database", err: errors.New("database failed"), wantStatus: http.StatusServiceUnavailable, wantCode: "DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			application := &fakeApplication{queryErr: testCase.err}
			recorder := httptest.NewRecorder()
			testFragment(t, application).ServeHTTP(recorder, authorizedRequest(t, http.MethodPost, "/3/member-grid/query", `{}`, authport.RoleAdmin, authport.CapabilityEntitlementsRead))
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status/body=%d/%s", recorder.Code, recorder.Body.String())
			}
			var body struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != testCase.wantCode || body.Message == "" || body.RequestID == "" {
				t.Fatalf("body=%+v decode_error=%v raw=%s", body, err, recorder.Body.String())
			}
			assertSecurityHeaders(t, recorder)
		})
	}
}

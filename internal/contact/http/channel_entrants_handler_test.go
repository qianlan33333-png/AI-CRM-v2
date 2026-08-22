package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

func TestChannelEntrantsHandlerReturnsExactClosedProjectionAndSecurityHeaders(t *testing.T) {
	t.Parallel()

	added := time.Date(2026, 8, 22, 10, 0, 0, 123000000, time.UTC)
	interacted := added.Add(time.Minute)
	application := &channelEntrantsHTTPTestApplication{list: func(_ context.Context, input contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error) {
		return contactapp.ChannelEntrantsResponse{
			ChannelID: input.ChannelID,
			Items: []contactapp.ChannelEntrantItem{{
				CustomerID: 91, DisplayName: "本地客户", AddedAt: added, LastInteractAt: &interacted,
			}},
			Limit: input.Limit, LocalProjection: true,
		}, nil
	}}
	handler := mustChannelEntrantsHTTPHandler(t, application)
	fragment, err := NewChannelEntrantsRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	request := channelEntrantsAuthorizedRequest(t, http.MethodGet, ChannelEntrantsRoutePrefix+"/7/contacts?limit=20", authport.RoleAdmin, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
	recorder := httptest.NewRecorder()
	fragment.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "private, no-store" ||
		recorder.Header().Get("X-Content-Type-Options") != "nosniff" ||
		recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("headers=%v", recorder.Header())
	}
	if application.calls != 1 || application.input.ChannelID != 7 || application.input.Limit != 20 || application.input.Cursor != "" {
		t.Fatalf("calls=%d input=%#v", application.calls, application.input)
	}

	var response map[string]json.RawMessage
	if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertChannelEntrantsJSONKeys(t, response, []string{
		"channel_id", "has_more", "items", "limit", "local_projection", "next_cursor", "provider_execution_eligible", "real_external_call_executed",
	})
	var items []map[string]json.RawMessage
	if err = json.Unmarshal(response["items"], &items); err != nil || len(items) != 1 {
		t.Fatalf("items=%s err=%v", response["items"], err)
	}
	assertChannelEntrantsJSONKeys(t, items[0], []string{"added_at", "customer_id", "display_name", "last_interact_at"})

	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{
		"mobile", "unionid", "external_userid", "owner_staff_id", "customers.extra",
		"\"extra\"", "wecom_userid", "access_token", "refresh_token", "raw_identity",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked forbidden %q: %s", forbidden, recorder.Body.String())
		}
	}
	if !strings.Contains(body, `"local_projection":true`) || !strings.Contains(body, `"provider_execution_eligible":false`) ||
		!strings.Contains(body, `"real_external_call_executed":false`) {
		t.Fatalf("local/external facts missing: %s", recorder.Body.String())
	}
}

func TestChannelEntrantsHandlerAcceptsOnlyCanonicalPaginationQuery(t *testing.T) {
	t.Parallel()

	application := &channelEntrantsHTTPTestApplication{list: channelEntrantsHTTPEmptyResponse}
	handler := mustChannelEntrantsHTTPHandler(t, application)

	accepted := []struct {
		name     string
		rawQuery string
		limit    int
		cursor   string
	}{
		{name: "default", rawQuery: "", limit: 20},
		{name: "minimum", rawQuery: "limit=1", limit: 1},
		{name: "maximum", rawQuery: "limit=50", limit: 50},
		{name: "cursor", rawQuery: "limit=2&cursor=ce1.opaque", limit: 2, cursor: "ce1.opaque"},
	}
	for _, testCase := range accepted {
		t.Run(testCase.name, func(t *testing.T) {
			before := application.calls
			request := channelEntrantsAuthorizedRawQueryRequest(t, testCase.rawQuery)
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "5")
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if application.calls != before+1 || application.input.Limit != testCase.limit || application.input.Cursor != testCase.cursor {
				t.Fatalf("calls=%d input=%#v", application.calls, application.input)
			}
		})
	}

	rejected := []struct {
		name     string
		rawQuery string
	}{
		{name: "unknown", rawQuery: "offset=1"},
		{name: "duplicate limit", rawQuery: "limit=1&limit=2"},
		{name: "duplicate cursor", rawQuery: "cursor=a&cursor=b"},
		{name: "empty limit", rawQuery: "limit="},
		{name: "zero", rawQuery: "limit=0"},
		{name: "negative", rawQuery: "limit=-1"},
		{name: "above maximum", rawQuery: "limit=51"},
		{name: "leading zero", rawQuery: "limit=01"},
		{name: "plus", rawQuery: "limit=+1"},
		{name: "decimal", rawQuery: "limit=1.0"},
		{name: "empty cursor", rawQuery: "cursor="},
		{name: "oversized cursor", rawQuery: "cursor=" + strings.Repeat("a", channelEntrantsMaximumCursorSize+1)},
		{name: "malformed percent", rawQuery: "limit=%"},
		{name: "invalid utf8", rawQuery: "cursor=%FF"},
		{name: "semicolon", rawQuery: "limit=1;cursor=x"},
	}
	for _, testCase := range rejected {
		t.Run(testCase.name, func(t *testing.T) {
			before := application.calls
			request := channelEntrantsAuthorizedRawQueryRequest(t, testCase.rawQuery)
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "5")
			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if application.calls != before {
				t.Fatalf("application called for rejected query: %d -> %d", before, application.calls)
			}
			assertChannelEntrantsErrorCode(t, recorder.Body.Bytes(), "VALIDATION_FAILED")
			assertChannelEntrantsSecurityHeaders(t, recorder)
		})
	}
}

func TestChannelEntrantsHandlerMapsCursorNotFoundAndUnavailableErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid cursor", err: contactapp.ErrInvalidChannelEntrantsCursor, status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED"},
		{name: "not found", err: contactapp.ErrChannelEntrantsNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "unavailable", err: contactapp.ErrChannelEntrantsUnavailable, status: http.StatusServiceUnavailable, code: "DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			application := &channelEntrantsHTTPTestApplication{list: func(context.Context, contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error) {
				return contactapp.ChannelEntrantsResponse{}, testCase.err
			}}
			handler := mustChannelEntrantsHTTPHandler(t, application)
			request := channelEntrantsAuthorizedRequest(t, http.MethodGet, "/ignored?cursor=ce1.bad", authport.RoleOps, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "6")
			if recorder.Code != testCase.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertChannelEntrantsErrorCode(t, recorder.Body.Bytes(), testCase.code)
			assertChannelEntrantsSecurityHeaders(t, recorder)
		})
	}
}

func TestChannelEntrantsHandlerRequiresAdminOrOpsCustomersReadGlobalBeforeApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		status        int
	}{
		{name: "missing principal", status: http.StatusUnauthorized},
		{name: "zero admin id", principal: &authport.Principal{Role: authport.RoleAdmin}, status: http.StatusUnauthorized},
		{name: "missing authorization", principal: &authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, status: http.StatusForbidden},
		{name: "sales", principal: &authport.Principal{AdminUserID: 1, Role: authport.RoleSales}, authorization: &authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal}, status: http.StatusForbidden},
		{name: "owner scope", principal: &authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: 9}, status: http.StatusForbidden},
		{name: "wrong capability", principal: &authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authorization: &authport.Authorization{Capability: authport.CapabilityChannelsRead, Scope: authport.ScopeGlobal}, status: http.StatusForbidden},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			application := &channelEntrantsHTTPTestApplication{list: channelEntrantsHTTPEmptyResponse}
			handler := mustChannelEntrantsHTTPHandler(t, application)
			request := httptest.NewRequest(http.MethodGet, "/ignored?limit=not-valid", nil)
			if testCase.principal != nil {
				ctx := authport.WithAuthenticatedSession(request.Context(), *testCase.principal, authport.SessionRef("test-session"))
				request = request.WithContext(ctx)
			}
			if testCase.authorization != nil {
				ctx, err := authport.WithAuthorization(request.Context(), *testCase.authorization)
				if err != nil {
					t.Fatal(err)
				}
				request = request.WithContext(ctx)
			}
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "bad-id")
			if recorder.Code != testCase.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if application.calls != 0 {
				t.Fatalf("application called %d times", application.calls)
			}
			assertChannelEntrantsSecurityHeaders(t, recorder)
		})
	}

	for _, role := range []authport.Role{authport.RoleAdmin, authport.RoleOps} {
		t.Run(string(role)+" accepted", func(t *testing.T) {
			t.Parallel()
			application := &channelEntrantsHTTPTestApplication{list: channelEntrantsHTTPEmptyResponse}
			handler := mustChannelEntrantsHTTPHandler(t, application)
			request := channelEntrantsAuthorizedRequest(t, http.MethodGet, "/ignored", role, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "8")
			if recorder.Code != http.StatusOK || application.calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, application.calls, recorder.Body.String())
			}
		})
	}
}

func TestChannelEntrantsRouteFragmentRejectsNonCanonicalPathsAndMethods(t *testing.T) {
	t.Parallel()

	application := &channelEntrantsHTTPTestApplication{list: channelEntrantsHTTPEmptyResponse}
	handler := mustChannelEntrantsHTTPHandler(t, application)
	fragment, err := NewChannelEntrantsRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}

	methodRequest := channelEntrantsAuthorizedRequest(t, http.MethodPost, ChannelEntrantsRoutePrefix+"/9/contacts", authport.RoleAdmin, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
	methodRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(methodRecorder, methodRequest)
	if methodRecorder.Code != http.StatusMethodNotAllowed || methodRecorder.Header().Get("Allow") != http.MethodGet || application.calls != 0 {
		t.Fatalf("method status=%d allow=%q calls=%d", methodRecorder.Code, methodRecorder.Header().Get("Allow"), application.calls)
	}
	assertChannelEntrantsSecurityHeaders(t, methodRecorder)

	paths := []struct {
		name   string
		path   string
		raw    string
		status int
	}{
		{name: "zero", path: ChannelEntrantsRoutePrefix + "/0/contacts", status: http.StatusUnprocessableEntity},
		{name: "negative", path: ChannelEntrantsRoutePrefix + "/-1/contacts", status: http.StatusUnprocessableEntity},
		{name: "leading zero", path: ChannelEntrantsRoutePrefix + "/01/contacts", status: http.StatusUnprocessableEntity},
		{name: "overflow", path: ChannelEntrantsRoutePrefix + "/99999999999999999999/contacts", status: http.StatusUnprocessableEntity},
		{name: "extra segment", path: ChannelEntrantsRoutePrefix + "/9/contacts/extra", status: http.StatusNotFound},
		{name: "trailing slash", path: ChannelEntrantsRoutePrefix + "/9/contacts/", status: http.StatusUnprocessableEntity},
		{name: "backslash", path: ChannelEntrantsRoutePrefix + "/9\\contacts", status: http.StatusUnprocessableEntity},
		{name: "encoded slash", path: ChannelEntrantsRoutePrefix + "/9/contacts", raw: ChannelEntrantsRoutePrefix + "/9%2Fcontacts", status: http.StatusUnprocessableEntity},
	}
	for _, testCase := range paths {
		t.Run(testCase.name, func(t *testing.T) {
			before := application.calls
			request := channelEntrantsAuthorizedRequest(t, http.MethodGet, testCase.path, authport.RoleAdmin, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
			if testCase.raw != "" {
				request.URL.RawPath = testCase.raw
			}
			recorder := httptest.NewRecorder()
			fragment.ServeHTTP(recorder, request)
			if recorder.Code != testCase.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if application.calls != before {
				t.Fatalf("application called for rejected path: %d -> %d", before, application.calls)
			}
			assertChannelEntrantsSecurityHeaders(t, recorder)
		})
	}

	relative := channelEntrantsAuthorizedRequest(t, http.MethodGet, "/10/contacts", authport.RoleOps, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
	relativeRecorder := httptest.NewRecorder()
	fragment.ServeHTTP(relativeRecorder, relative)
	if relativeRecorder.Code != http.StatusOK || application.input.ChannelID != 10 {
		t.Fatalf("relative status=%d input=%#v", relativeRecorder.Code, application.input)
	}
}

func TestChannelEntrantsHandlerRejectsMalformedApplicationProjection(t *testing.T) {
	t.Parallel()

	added := time.Date(2026, 8, 22, 11, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		response contactapp.ChannelEntrantsResponse
	}{
		{name: "nil items", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Limit: 20, LocalProjection: true}},
		{name: "wrong channel", response: contactapp.ChannelEntrantsResponse{ChannelID: 4, Items: []contactapp.ChannelEntrantItem{}, Limit: 20, LocalProjection: true}},
		{name: "not local", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Items: []contactapp.ChannelEntrantItem{}, Limit: 20}},
		{name: "external call claimed", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Items: []contactapp.ChannelEntrantItem{}, Limit: 20, LocalProjection: true, RealExternalCallExecuted: true}},
		{name: "zero customer", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Items: []contactapp.ChannelEntrantItem{{AddedAt: added}}, Limit: 20, LocalProjection: true}},
		{name: "invalid order", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Items: []contactapp.ChannelEntrantItem{{CustomerID: 1, AddedAt: added}, {CustomerID: 2, AddedAt: added}}, Limit: 20, LocalProjection: true}},
		{name: "cursor without more", response: contactapp.ChannelEntrantsResponse{ChannelID: 3, Items: []contactapp.ChannelEntrantItem{}, Limit: 20, NextCursor: "ce1.x", LocalProjection: true}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			application := &channelEntrantsHTTPTestApplication{list: func(context.Context, contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error) {
				return testCase.response, nil
			}}
			handler := mustChannelEntrantsHTTPHandler(t, application)
			request := channelEntrantsAuthorizedRequest(t, http.MethodGet, "/ignored", authport.RoleAdmin, authport.CapabilityCustomersRead, authport.ScopeGlobal, 0)
			recorder := httptest.NewRecorder()
			handler.ListChannelContacts(recorder, request, "3")
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			assertChannelEntrantsErrorCode(t, recorder.Body.Bytes(), "DEPENDENCY_UNAVAILABLE")
		})
	}
}

func TestChannelEntrantsHTTPConstructorsRejectNilDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewChannelEntrantsHandler(nil); err == nil {
		t.Fatal("nil application was accepted")
	}
	var typedNil *channelEntrantsHTTPTestApplication
	if _, err := NewChannelEntrantsHandler(typedNil); err == nil {
		t.Fatal("typed nil application was accepted")
	}
	if _, err := NewChannelEntrantsRouteFragment(nil); err == nil {
		t.Fatal("nil handler was accepted")
	}
}

type channelEntrantsHTTPTestApplication struct {
	list  func(context.Context, contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error)
	calls int
	input contactapp.ChannelEntrantsInput
}

func (application *channelEntrantsHTTPTestApplication) List(ctx context.Context, input contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error) {
	application.calls++
	application.input = input
	if application.list == nil {
		return contactapp.ChannelEntrantsResponse{}, errors.New("test application has no response")
	}
	return application.list(ctx, input)
}

func channelEntrantsHTTPEmptyResponse(_ context.Context, input contactapp.ChannelEntrantsInput) (contactapp.ChannelEntrantsResponse, error) {
	return contactapp.ChannelEntrantsResponse{
		ChannelID: input.ChannelID, Items: []contactapp.ChannelEntrantItem{}, Limit: input.Limit, LocalProjection: true,
	}, nil
}

func mustChannelEntrantsHTTPHandler(t *testing.T, application channelEntrantsApplication) *ChannelEntrantsHandler {
	t.Helper()
	handler, err := NewChannelEntrantsHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func channelEntrantsAuthorizedRequest(
	t *testing.T,
	method string,
	target string,
	role authport.Role,
	capability authport.Capability,
	scope authport.ScopeKind,
	ownerStaffID int64,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, nil)
	return channelEntrantsAuthorizeRequestContext(t, request, authport.Principal{AdminUserID: 1, Role: role}, authport.Authorization{
		Capability: capability, Scope: scope, OwnerStaffID: ownerStaffID,
	})
}

func channelEntrantsAuthorizedRawQueryRequest(t *testing.T, rawQuery string) *http.Request {
	t.Helper()
	request := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: "/ignored", RawQuery: rawQuery},
		Header: make(http.Header),
	}
	return channelEntrantsAuthorizeRequestContext(t, request, authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authport.Authorization{
		Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
	})
}

func channelEntrantsAuthorizeRequestContext(
	t *testing.T,
	request *http.Request,
	principal authport.Principal,
	authorization authport.Authorization,
) *http.Request {
	t.Helper()
	ctx := authport.WithAuthenticatedSession(request.Context(), principal, authport.SessionRef("test-session"))
	ctx, err := authport.WithAuthorization(ctx, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return request.WithContext(ctx)
}

func assertChannelEntrantsSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Header().Get("Cache-Control") != "private, no-store" ||
		recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers=%v", recorder.Header())
	}
}

func assertChannelEntrantsErrorCode(t *testing.T, body []byte, expected string) {
	t.Helper()
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, body)
	}
	if response.Code != expected {
		t.Fatalf("code=%q want=%q body=%s", response.Code, expected, body)
	}
}

func assertChannelEntrantsJSONKeys(t *testing.T, value map[string]json.RawMessage, expected []string) {
	t.Helper()
	actual := make([]string, 0, len(value))
	for key := range value {
		actual = append(actual, key)
	}
	sort.Strings(actual)
	want := append([]string(nil), expected...)
	sort.Strings(want)
	if strings.Join(actual, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("keys=%v want=%v", actual, want)
	}
}

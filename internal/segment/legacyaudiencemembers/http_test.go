package legacyaudiencemembers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type applicationStub struct {
	response ListResponse
	err      error
	inputs   []ListInput
}

func (stub *applicationStub) ListMembers(_ context.Context, input ListInput) (ListResponse, error) {
	stub.inputs = append(stub.inputs, input)
	return stub.response, stub.err
}

type securityStub struct {
	err          error
	requirements []AccessRequirement
}

func (stub *securityStub) Authorize(_ *http.Request, requirement AccessRequirement) error {
	stub.requirements = append(stub.requirements, requirement)
	return stub.err
}

func TestRouteFragmentSuccessUsesSegmentsReadAndClosedResponse(t *testing.T) {
	t.Parallel()
	enteredAt := time.Date(2026, 8, 22, 6, 5, 4, 0, time.UTC)
	application := &applicationStub{response: ListResponse{
		OK: true,
		Items: []MemberItem{{
			CustomerID:     8,
			Nickname:       "Eight",
			ExternalUserID: "wm_8",
			EnteredAt:      enteredAt,
		}},
		Total:                    3,
		Count:                    1,
		Limit:                    1,
		Offset:                   2,
		RealExternalCallExecuted: false,
	}}
	security := &securityStub{}
	fragment := mustRouteFragment(t, application, security)
	request := httptest.NewRequest(http.MethodGet, "http://example.test"+RoutePrefix+"/packages/42/members?limit=1&offset=2", nil)
	request.Header.Set("X-Request-ID", "audience-members-test")
	response := httptest.NewRecorder()

	fragment.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if !reflect.DeepEqual(security.requirements, []AccessRequirement{{Capability: CapabilitySegmentsRead, RequireCSRF: false}}) {
		t.Fatalf("security requirements = %#v", security.requirements)
	}
	if !reflect.DeepEqual(application.inputs, []ListInput{{PackageID: 42, Limit: 1, Offset: 2}}) {
		t.Fatalf("application inputs = %#v", application.inputs)
	}
	var decoded ListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, application.response) {
		t.Fatalf("response = %#v, want %#v", decoded, application.response)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"unionid", "mobile", "openid", "extra", "raw_identity", "member_count"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("response leaked forbidden field %q: %s", forbidden, body)
		}
	}
}

func TestRouteFragmentSupportsStripPrefixMountAndDefaults(t *testing.T) {
	t.Parallel()
	application := &applicationStub{response: ListResponse{
		OK:                       true,
		Items:                    []MemberItem{},
		Limit:                    DefaultLimit,
		RealExternalCallExecuted: false,
	}}
	fragment := mustRouteFragment(t, application, &securityStub{})
	request := httptest.NewRequest(http.MethodGet, "http://example.test/packages/5/members", nil)
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !reflect.DeepEqual(application.inputs, []ListInput{{PackageID: 5, Limit: DefaultLimit, Offset: 0}}) {
		t.Fatalf("inputs = %#v", application.inputs)
	}
}

func TestRouteFragmentAcceptsPaginationBoundaries(t *testing.T) {
	t.Parallel()
	application := &applicationStub{response: ListResponse{
		OK:                       true,
		Items:                    []MemberItem{},
		Limit:                    MaximumLimit,
		Offset:                   int64(^uint64(0) >> 1),
		RealExternalCallExecuted: false,
	}}
	fragment := mustRouteFragment(t, application, &securityStub{})
	request := httptest.NewRequest(http.MethodGet,
		"http://example.test"+RoutePrefix+"/packages/1/members?limit=200&offset=9223372036854775807", nil)
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	want := []ListInput{{PackageID: 1, Limit: MaximumLimit, Offset: int64(^uint64(0) >> 1)}}
	if !reflect.DeepEqual(application.inputs, want) {
		t.Fatalf("inputs = %#v, want %#v", application.inputs, want)
	}
}

func TestRouteFragmentAuthenticationAndAuthorizationFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"unauthenticated", ErrUnauthenticated, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"forbidden", ErrForbidden, http.StatusForbidden, "UNAUTHORIZED"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			application := &applicationStub{}
			fragment := mustRouteFragment(t, application, &securityStub{err: test.err})
			response := httptest.NewRecorder()
			fragment.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test"+RoutePrefix+"/packages/1/members", nil))
			assertErrorResponse(t, response, test.status, test.code)
			if len(application.inputs) != 0 {
				t.Fatal("application called after authorization failure")
			}
		})
	}
}

func TestRouteFragmentRejectsMethodAndInvalidPaths(t *testing.T) {
	t.Parallel()
	t.Run("method", func(t *testing.T) {
		t.Parallel()
		security := &securityStub{}
		fragment := mustRouteFragment(t, &applicationStub{}, security)
		response := httptest.NewRecorder()
		fragment.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "http://example.test"+RoutePrefix+"/packages/1/members", nil))
		assertErrorResponse(t, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
		if response.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("Allow = %q", response.Header().Get("Allow"))
		}
		if len(security.requirements) != 0 {
			t.Fatal("wrong method must not invoke authorization")
		}
	})

	paths := []string{
		RoutePrefix + "/packages/0/members",
		RoutePrefix + "/packages/-1/members",
		RoutePrefix + "/packages/01/members",
		RoutePrefix + "/packages/9223372036854775808/members",
		RoutePrefix + "/packages/not-an-id/members",
		RoutePrefix + "/packages/1/members/extra",
		RoutePrefix + "/packages/1/members/",
		RoutePrefix + "//packages/1/members",
		RoutePrefix + "/packages\\1\\members",
		RoutePrefix + "/packages/./members",
		RoutePrefix + "/package/1/members",
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			fragment := mustRouteFragment(t, &applicationStub{}, &securityStub{})
			response := httptest.NewRecorder()
			fragment.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test"+path, nil))
			assertErrorResponse(t, response, http.StatusNotFound, "NOT_FOUND")
		})
	}

	t.Run("encoded_path", func(t *testing.T) {
		t.Parallel()
		fragment := mustRouteFragment(t, &applicationStub{}, &securityStub{})
		request := httptest.NewRequest(http.MethodGet, "http://example.test"+RoutePrefix+"/packages%2f1/members", nil)
		response := httptest.NewRecorder()
		fragment.ServeHTTP(response, request)
		assertErrorResponse(t, response, http.StatusNotFound, "NOT_FOUND")
	})
}

func TestRouteFragmentRejectsUnknownDuplicateAndInvalidQueryAs400(t *testing.T) {
	t.Parallel()
	queries := []string{
		"unknown=1",
		"limit=1&limit=2",
		"offset=1&offset=2",
		"limit=",
		"offset=",
		"limit=0",
		"limit=201",
		"limit=01",
		"limit=-1",
		"limit=1.5",
		"offset=-1",
		"offset=01",
		"offset=9223372036854775808",
		"limit=%zz",
		"limit=%FF",
		"&limit=1",
		"limit=1&",
		"limit=1&&offset=0",
		"limit=1;offset=2",
	}
	for _, rawQuery := range queries {
		rawQuery := rawQuery
		t.Run(rawQuery, func(t *testing.T) {
			t.Parallel()
			application := &applicationStub{}
			fragment := mustRouteFragment(t, application, &securityStub{})
			path := RoutePrefix + "/packages/1/members"
			request := &http.Request{
				Method:     http.MethodGet,
				URL:        &url.URL{Path: path, RawQuery: rawQuery},
				RequestURI: path + "?" + rawQuery,
				Header:     make(http.Header),
			}
			response := httptest.NewRecorder()
			fragment.ServeHTTP(response, request)
			assertErrorResponse(t, response, http.StatusBadRequest, "MALFORMED_REQUEST")
			if len(application.inputs) != 0 {
				t.Fatal("application called for malformed query")
			}
		})
	}
}

func TestRouteFragmentMapsApplicationFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not_found", ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"invalid", ErrInvalidInput, http.StatusBadRequest, "MALFORMED_REQUEST"},
		{"database", errors.New("database failed"), http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fragment := mustRouteFragment(t, &applicationStub{err: test.err}, &securityStub{})
			response := httptest.NewRecorder()
			fragment.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test"+RoutePrefix+"/packages/1/members", nil))
			assertErrorResponse(t, response, test.status, test.code)
		})
	}
}

func TestConstructorsAndRouteSpecAreClosed(t *testing.T) {
	t.Parallel()
	if _, err := NewHandler(nil, &securityStub{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewHandler(nil) error = %v", err)
	}
	if _, err := NewHandler(&applicationStub{}, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewHandler(nil security) error = %v", err)
	}
	if _, err := NewRouteFragment(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewRouteFragment(nil) error = %v", err)
	}
	want := []RouteSpec{{
		Method:       http.MethodGet,
		Pattern:      RoutePattern,
		Capability:   CapabilitySegmentsRead,
		RequiresCSRF: false,
	}}
	if got := RouteSpecs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RouteSpecs() = %#v, want %#v", got, want)
	}
}

func mustRouteFragment(t *testing.T, application Application, security Security) http.Handler {
	t.Helper()
	handler, err := NewHandler(application, security)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := NewRouteFragment(handler)
	if err != nil {
		t.Fatal(err)
	}
	return fragment
}

func assertErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, status, recorder.Body.String())
	}
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v; body=%s", err, recorder.Body.String())
	}
	if response.Code != code || response.Message == "" || response.RequestID == "" {
		t.Fatalf("error response = %#v", response)
	}
	if recorder.Header().Get("X-Request-ID") != response.RequestID {
		t.Fatalf("X-Request-ID = %q, body request_id = %q", recorder.Header().Get("X-Request-ID"), response.RequestID)
	}
}

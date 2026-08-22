package legacyaudience

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type recordingSecurity struct {
	actor Actor
	err   error
	calls []AccessRequirement
}

func (security *recordingSecurity) Authorize(_ *http.Request, requirement AccessRequirement) (Actor, error) {
	security.calls = append(security.calls, requirement)
	if security.err != nil {
		return Actor{}, security.err
	}
	return security.actor, nil
}

func newHTTPFixture(t *testing.T) (http.Handler, *memoryWorld, *recordingSecurity) {
	t.Helper()
	world := newMemoryWorld()
	service, _ := newTestService(t, world)
	security := &recordingSecurity{actor: Actor{AdminUserID: 9}}
	handler, err := NewHandler(service, security)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	fragment, err := NewRouteFragment(handler)
	if err != nil {
		t.Fatalf("NewRouteFragment: %v", err)
	}
	return fragment, world, security
}

func performRequest(handler http.Handler, method, target, body string, write bool) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if write {
		request.Header.Set("Idempotency-Key", "http-idempotency-key-0001")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body %q: %v", response.Body.String(), err)
	}
	return body
}

func TestRouteSpecsExposeOnlyFrozenRoutes(t *testing.T) {
	specs := RouteSpecs()
	if len(specs) != 11 {
		t.Fatalf("route count = %d", len(specs))
	}
	for _, spec := range specs {
		if !strings.HasPrefix(spec.Pattern, RoutePrefix+"/") {
			t.Fatalf("route outside prefix: %+v", spec)
		}
		if spec.Method == http.MethodGet {
			if spec.Capability != CapabilitySegmentsRead || spec.RequiresCSRF {
				t.Fatalf("bad read contract: %+v", spec)
			}
		} else if spec.Capability != CapabilitySegmentsWrite || !spec.RequiresCSRF {
			t.Fatalf("bad write contract: %+v", spec)
		}
	}
}

func TestHTTPAllFrozenRoutesDispatch(t *testing.T) {
	fragment, _, security := newHTTPFixture(t)
	type requestCase struct {
		method string
		path   string
		body   string
		key    string
		status int
	}
	cases := []requestCase{
		{http.MethodGet, RoutePrefix + "/package-groups", "", "", http.StatusOK},
		{http.MethodPost, RoutePrefix + "/package-groups", `{"name":"临时分组","expected_version":0}`, "all-routes-group-create", http.StatusCreated},
		{http.MethodPatch, RoutePrefix + "/package-groups/3", `{"name":"临时分组二","expected_version":1}`, "all-routes-group-patch-", http.StatusOK},
		{http.MethodDelete, RoutePrefix + "/package-groups/3", `{"expected_version":2}`, "all-routes-group-delete", http.StatusOK},
		{http.MethodGet, RoutePrefix + "/packages?limit=10&offset=0", "", "", http.StatusOK},
		{http.MethodGet, RoutePrefix + "/packages/101", "", "", http.StatusOK},
		{http.MethodPatch, RoutePrefix + "/packages/101", `{"name":"完整路由套餐","expected_version":1}`, "all-routes-package-pat", http.StatusOK},
		{http.MethodPost, RoutePrefix + "/packages/101/copy", `{"expected_version":2}`, "all-routes-package-copy", http.StatusCreated},
		{http.MethodPost, RoutePrefix + "/packages/101/pause", `{"expected_version":2}`, "all-routes-package-paus", http.StatusOK},
		{http.MethodPost, RoutePrefix + "/packages/101/activate", `{"expected_version":3}`, "all-routes-package-acti", http.StatusOK},
		{http.MethodDelete, RoutePrefix + "/packages/101", `{"expected_version":4}`, "all-routes-package-arch", http.StatusOK},
	}
	for index, test := range cases {
		var body io.Reader
		if test.body != "" {
			body = strings.NewReader(test.body)
		}
		request := httptest.NewRequest(test.method, test.path, body)
		if test.body != "" {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", test.key)
		}
		response := httptest.NewRecorder()
		fragment.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("case %d %s %s status=%d want=%d body=%s", index, test.method, test.path, response.Code, test.status, response.Body.String())
		}
	}
	if len(security.calls) != len(cases) {
		t.Fatalf("security calls=%d want=%d", len(security.calls), len(cases))
	}
	for index, test := range cases {
		requirement := security.calls[index]
		if test.method == http.MethodGet {
			if requirement != (AccessRequirement{Capability: CapabilitySegmentsRead}) {
				t.Fatalf("case %d read requirement=%+v", index, requirement)
			}
		} else if requirement != (AccessRequirement{Capability: CapabilitySegmentsWrite, RequireCSRF: true}) {
			t.Fatalf("case %d write requirement=%+v", index, requirement)
		}
	}
}

func TestHTTPListPaginationAndPermissionContracts(t *testing.T) {
	fragment, _, security := newHTTPFixture(t)
	response := performRequest(fragment, http.MethodGet, RoutePrefix+"/packages?group_id=1&limit=1&offset=0", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := decodeBody(t, response)
	if body["local_projection"] != true || body["real_external_call_executed"] != false || body["total"] != float64(1) {
		t.Fatalf("unexpected body: %v", body)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 || items[0].(map[string]any)["member_count"] != float64(3) {
		t.Fatalf("unexpected package list: %v", body["items"])
	}
	if len(security.calls) != 1 || security.calls[0] != (AccessRequirement{Capability: CapabilitySegmentsRead}) {
		t.Fatalf("read security calls: %+v", security.calls)
	}
	if cache := response.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("cache header = %q", cache)
	}
}

func TestHTTPGroupCreateAndPackageMutations(t *testing.T) {
	fragment, world, security := newHTTPFixture(t)

	create := performRequest(fragment, http.MethodPost, RoutePrefix+"/package-groups", `{"name":"运营分组","sort_order":3,"expected_version":0}`, true)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	createBody := decodeBody(t, create)
	if createBody["local_projection"] != true || createBody["real_external_call_executed"] != false {
		t.Fatalf("create projection flags: %v", createBody)
	}
	if len(security.calls) != 1 || security.calls[0] != (AccessRequirement{Capability: CapabilitySegmentsWrite, RequireCSRF: true}) {
		t.Fatalf("write security calls: %+v", security.calls)
	}

	patchRequest := httptest.NewRequest(http.MethodPatch, RoutePrefix+"/packages/101", strings.NewReader(`{"name":"HTTP 更新","group_id":null,"expected_version":1}`))
	patchRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	patchRequest.Header.Set("Idempotency-Key", "http-package-patch-key01")
	patchResponse := httptest.NewRecorder()
	fragment.ServeHTTP(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusOK || world.packages[101].Name != "HTTP 更新" || world.packages[101].Metadata.GroupID != nil {
		t.Fatalf("patch status=%d body=%s package=%+v", patchResponse.Code, patchResponse.Body.String(), world.packages[101])
	}

	copyRequest := httptest.NewRequest(http.MethodPost, RoutePrefix+"/packages/101/copy", strings.NewReader(`{"expected_version":2}`))
	copyRequest.Header.Set("Content-Type", "application/json")
	copyRequest.Header.Set("Idempotency-Key", "http-package-copy-key001")
	copyResponse := httptest.NewRecorder()
	fragment.ServeHTTP(copyResponse, copyRequest)
	if copyResponse.Code != http.StatusCreated {
		t.Fatalf("copy status=%d body=%s", copyResponse.Code, copyResponse.Body.String())
	}
	copyBody := decodeBody(t, copyResponse)
	packageBody := copyBody["package"].(map[string]any)
	if packageBody["member_count"] != float64(0) || packageBody["lifecycle"] != string(PackagePaused) {
		t.Fatalf("copy body: %v", copyBody)
	}

	archiveRequest := httptest.NewRequest(http.MethodDelete, RoutePrefix+"/packages/101", strings.NewReader(`{"expected_version":2}`))
	archiveRequest.Header.Set("Content-Type", "application/json")
	archiveRequest.Header.Set("Idempotency-Key", "http-package-archive-01")
	archiveResponse := httptest.NewRecorder()
	fragment.ServeHTTP(archiveResponse, archiveRequest)
	if archiveResponse.Code != http.StatusOK || world.packages[101].Metadata.Lifecycle != PackageArchived {
		t.Fatalf("archive status=%d body=%s package=%+v", archiveResponse.Code, archiveResponse.Body.String(), world.packages[101])
	}
}

func TestHTTPStrictDTOQueryPathAndMethodFailures(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		target      string
		body        string
		write       bool
		contentType *string
		status      int
		allow       string
	}{
		{name: "unknown field", method: http.MethodPost, target: RoutePrefix + "/package-groups", body: `{"name":"x","expected_version":0,"opaque":{}}`, write: true, status: http.StatusBadRequest},
		{name: "duplicate field", method: http.MethodPost, target: RoutePrefix + "/package-groups", body: `{"name":"x","name":"y","expected_version":0}`, write: true, status: http.StatusBadRequest},
		{name: "empty body", method: http.MethodPost, target: RoutePrefix + "/package-groups", body: "", write: true, contentType: stringPointer("application/json"), status: http.StatusBadRequest},
		{name: "missing content type", method: http.MethodPost, target: RoutePrefix + "/package-groups", body: `{"name":"x","expected_version":0}`, write: true, contentType: stringPointer(""), status: http.StatusUnsupportedMediaType},
		{name: "wrong content type", method: http.MethodPost, target: RoutePrefix + "/package-groups", body: `{"name":"x","expected_version":0}`, write: true, contentType: stringPointer("text/plain"), status: http.StatusUnsupportedMediaType},
		{name: "leading zero id", method: http.MethodGet, target: RoutePrefix + "/packages/01", status: http.StatusUnprocessableEntity},
		{name: "limit too high", method: http.MethodGet, target: RoutePrefix + "/packages?limit=101", status: http.StatusUnprocessableEntity},
		{name: "offset too high", method: http.MethodGet, target: RoutePrefix + "/packages?offset=100001", status: http.StatusUnprocessableEntity},
		{name: "duplicate query", method: http.MethodGet, target: RoutePrefix + "/packages?limit=1&limit=2", status: http.StatusBadRequest},
		{name: "unknown query", method: http.MethodGet, target: RoutePrefix + "/packages?cursor=x", status: http.StatusBadRequest},
		{name: "query forbidden on detail", method: http.MethodGet, target: RoutePrefix + "/packages/101?limit=1", status: http.StatusBadRequest},
		{name: "trailing slash", method: http.MethodGet, target: RoutePrefix + "/packages/", status: http.StatusBadRequest},
		{name: "extra segment", method: http.MethodGet, target: RoutePrefix + "/packages/101/extra", status: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodPut, target: RoutePrefix + "/packages", status: http.StatusMethodNotAllowed, allow: http.MethodGet},
		{name: "non-object", method: http.MethodPost, target: RoutePrefix + "/packages/101/pause", body: `[]`, write: true, status: http.StatusBadRequest},
		{name: "semantic integer", method: http.MethodPost, target: RoutePrefix + "/packages/101/pause", body: `{"expected_version":1.0}`, write: true, status: http.StatusUnprocessableEntity},
		{name: "patch no mutation", method: http.MethodPatch, target: RoutePrefix + "/packages/101", body: `{"expected_version":1}`, write: true, status: http.StatusUnprocessableEntity},
		{name: "invalid refresh mode", method: http.MethodPatch, target: RoutePrefix + "/packages/101", body: `{"refresh_mode":"dynamic","expected_version":1}`, write: true, status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fragment, _, _ := newHTTPFixture(t)
			var reader io.Reader
			if test.body != "" {
				reader = strings.NewReader(test.body)
			}
			request := httptest.NewRequest(test.method, test.target, reader)
			if test.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if test.contentType != nil {
				if *test.contentType == "" {
					request.Header.Del("Content-Type")
				} else {
					request.Header.Set("Content-Type", *test.contentType)
				}
			}
			if test.write {
				request.Header.Set("Idempotency-Key", "strict-http-key-000001")
			}
			response := httptest.NewRecorder()
			fragment.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			if test.allow != "" && response.Header().Get("Allow") != test.allow {
				t.Fatalf("Allow=%q want=%q", response.Header().Get("Allow"), test.allow)
			}
			body := decodeBody(t, response)
			if body["code"] == nil || body["request_id"] == nil {
				t.Fatalf("closed error body missing fields: %v", body)
			}
		})
	}
}

func TestHTTPRejectsMissingIdempotencyOversizeAndEncodedPath(t *testing.T) {
	fragment, _, _ := newHTTPFixture(t)
	missingKey := httptest.NewRequest(http.MethodPost, RoutePrefix+"/package-groups", strings.NewReader(`{"name":"x","expected_version":0}`))
	missingKey.Header.Set("Content-Type", "application/json")
	missingResponse := httptest.NewRecorder()
	fragment.ServeHTTP(missingResponse, missingKey)
	if missingResponse.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing key status=%d body=%s", missingResponse.Code, missingResponse.Body.String())
	}

	largeName := strings.Repeat("x", int(MaximumRequestBodyBytes)+100)
	largeBody, _ := json.Marshal(map[string]any{"name": largeName, "expected_version": 0})
	largeRequest := httptest.NewRequest(http.MethodPost, RoutePrefix+"/package-groups", bytes.NewReader(largeBody))
	largeRequest.Header.Set("Content-Type", "application/json")
	largeRequest.Header.Set("Idempotency-Key", "oversize-http-key-0001")
	largeResponse := httptest.NewRecorder()
	fragment.ServeHTTP(largeResponse, largeRequest)
	if largeResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status=%d body=%s", largeResponse.Code, largeResponse.Body.String())
	}

	encodedRequest := httptest.NewRequest(http.MethodGet, RoutePrefix+"/packages/101", nil)
	encodedRequest.URL.RawPath = RoutePrefix + "/packages%2F101"
	encodedRequest.RequestURI = RoutePrefix + "/packages%2F101"
	encodedResponse := httptest.NewRecorder()
	fragment.ServeHTTP(encodedResponse, encodedRequest)
	if encodedResponse.Code != http.StatusBadRequest {
		t.Fatalf("encoded path status=%d body=%s", encodedResponse.Code, encodedResponse.Body.String())
	}
}

func TestHTTPNotFoundArchivedAndIDBoundaries(t *testing.T) {
	fragment, world, _ := newHTTPFixture(t)
	for _, path := range []string{
		RoutePrefix + "/packages/0",
		RoutePrefix + "/packages/-1",
		RoutePrefix + "/packages/01",
		RoutePrefix + "/packages/9223372036854775808",
	} {
		response := performRequest(fragment, http.MethodGet, path, "", false)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid ID path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	notFound := performRequest(fragment, http.MethodGet, RoutePrefix+"/packages/999", "", false)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d body=%s", notFound.Code, notFound.Body.String())
	}

	model := world.packages[101]
	model.Metadata.Lifecycle = PackageArchived
	model.SegmentLifecycle = segmentport.LifecycleStatusArchived
	world.packages[101] = model
	segment := world.segments[101]
	segment.LifecycleStatus = segmentport.LifecycleStatusArchived
	world.segments[101] = segment

	activate := httptest.NewRequest(http.MethodPost, RoutePrefix+"/packages/101/activate", strings.NewReader(`{"expected_version":1}`))
	activate.Header.Set("Content-Type", "application/json")
	activate.Header.Set("Idempotency-Key", "archived-http-activate-01")
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, activate)
	if response.Code != http.StatusConflict {
		t.Fatalf("archived activate status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHTTPPermissionAndCSRFFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		method string
		target string
		body   string
		write  bool
		status int
	}{
		{name: "unauthenticated read", err: ErrUnauthenticated, method: http.MethodGet, target: RoutePrefix + "/packages", status: http.StatusUnauthorized},
		{name: "forbidden read", err: ErrForbidden, method: http.MethodGet, target: RoutePrefix + "/packages", status: http.StatusForbidden},
		{name: "csrf write", err: ErrCSRFInvalid, method: http.MethodPost, target: RoutePrefix + "/package-groups", body: `{"name":"x","expected_version":0}`, write: true, status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fragment, world, security := newHTTPFixture(t)
			security.err = test.err
			response := performRequest(fragment, test.method, test.target, test.body, test.write)
			if response.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
			}
			if len(world.groups) != 2 || len(world.events) != 0 {
				t.Fatal("denied request reached business mutation")
			}
			if len(security.calls) != 1 {
				t.Fatalf("security calls=%d", len(security.calls))
			}
			if test.write && (!security.calls[0].RequireCSRF || security.calls[0].Capability != CapabilitySegmentsWrite) {
				t.Fatalf("write permission contract=%+v", security.calls[0])
			}
		})
	}
}

func TestHTTPMapsBusinessConflicts(t *testing.T) {
	fragment, _, _ := newHTTPFixture(t)
	request := httptest.NewRequest(http.MethodDelete, RoutePrefix+"/package-groups/1", strings.NewReader(`{"expected_version":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "nonempty-delete-key-001")
	response := httptest.NewRecorder()
	fragment.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := decodeBody(t, response)
	if body["code"] != "CONFLICT" {
		t.Fatalf("body=%v", body)
	}
}

func TestNewHandlerRejectsNilDependencies(t *testing.T) {
	_, err := NewHandler(nil, &recordingSecurity{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil application error=%v", err)
	}
}

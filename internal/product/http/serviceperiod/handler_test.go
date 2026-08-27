package serviceperiod

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	testKey = "service-period-idempotency-key-0001"
)

func TestHandlerRejectsMalformedIDsMethodsQueriesBodiesAndHeaders(t *testing.T) {
	application := newHTTPTestApplication()
	handler := mustHandler(t, application)

	largeBody := `{"product_code":"large","name":"` + strings.Repeat("x", maximumRequestBytes) + `","price_minor":1,"currency":"CNY","stock_quantity":0}`
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		key        string
		wantStatus int
		mutate     func(*http.Request)
	}{
		{name: "zero_id", method: http.MethodGet, path: BasePath + "/0", wantStatus: http.StatusBadRequest},
		{name: "leading_zero_id", method: http.MethodGet, path: BasePath + "/01", wantStatus: http.StatusBadRequest},
		{name: "negative_id", method: http.MethodGet, path: BasePath + "/-1", wantStatus: http.StatusBadRequest},
		{name: "nonnumeric_id", method: http.MethodGet, path: BasePath + "/abc", wantStatus: http.StatusBadRequest},
		{name: "overflow_id", method: http.MethodGet, path: BasePath + "/9223372036854775808", wantStatus: http.StatusBadRequest},
		{name: "extra_segment", method: http.MethodGet, path: BasePath + "/1/enable/extra", wantStatus: http.StatusNotFound},
		{name: "trailing_slash", method: http.MethodGet, path: BasePath + "/1/", wantStatus: http.StatusNotFound},
		{name: "backslash", method: http.MethodGet, path: BasePath + `/1\enable`, wantStatus: http.StatusBadRequest},
		{name: "encoded_slash", method: http.MethodGet, path: BasePath + "/1%2Fenable", wantStatus: http.StatusBadRequest},
		{name: "unknown_action", method: http.MethodPost, path: BasePath + "/1/publish", body: `{"expected_version":1}`, key: testKey, wantStatus: http.StatusNotFound},
		{name: "unknown_action_wrong_method_is_still_not_found", method: http.MethodGet, path: BasePath + "/1/publish", wantStatus: http.StatusNotFound},
		{name: "wrong_action_method", method: http.MethodGet, path: BasePath + "/1/enable", wantStatus: http.StatusMethodNotAllowed},
		{name: "wrong_share_method", method: http.MethodPost, path: BasePath + "/1/share", wantStatus: http.StatusMethodNotAllowed},
		{name: "share_query", method: http.MethodGet, path: BasePath + "/1/share?x=1", wantStatus: http.StatusBadRequest},
		{name: "share_body", method: http.MethodGet, path: BasePath + "/1/share", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "wrong_collection_method", method: http.MethodPatch, path: BasePath, body: `{}`, key: testKey, wantStatus: http.StatusMethodNotAllowed},
		{name: "unknown_query", method: http.MethodGet, path: BasePath + "?enabled=true", wantStatus: http.StatusBadRequest},
		{name: "duplicate_query", method: http.MethodGet, path: BasePath + "?limit=1&limit=2", wantStatus: http.StatusBadRequest},
		{name: "empty_query_value", method: http.MethodGet, path: BasePath + "?limit=", wantStatus: http.StatusBadRequest},
		{name: "encoded_query", method: http.MethodGet, path: BasePath + "?limit=%31", wantStatus: http.StatusBadRequest},
		{name: "over_limit", method: http.MethodGet, path: BasePath + "?limit=101", wantStatus: http.StatusBadRequest},
		{name: "leading_zero_offset", method: http.MethodGet, path: BasePath + "?offset=01", wantStatus: http.StatusBadRequest},
		{name: "get_body", method: http.MethodGet, path: BasePath, body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "get_item_query", method: http.MethodGet, path: BasePath + "/1?limit=1", wantStatus: http.StatusBadRequest},
		{name: "create_unknown_field", method: http.MethodPost, path: BasePath, body: `{"product_code":"p","name":"n","price_minor":1,"currency":"CNY","stock_quantity":0,"metadata":{"x":1}}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_duplicate_field", method: http.MethodPost, path: BasePath, body: `{"product_code":"p","product_code":"q","name":"n","price_minor":1,"currency":"CNY","stock_quantity":0}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_missing_required_name", method: http.MethodPost, path: BasePath, body: `{"product_code":"p","price_minor":1,"currency":"CNY","stock_quantity":0}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_missing_exact_amount", method: http.MethodPost, path: BasePath, body: `{"product_code":"p","name":"n","currency":"CNY","stock_quantity":0}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_float_amount", method: http.MethodPost, path: BasePath, body: `{"product_code":"p","name":"n","price_minor":1.0,"currency":"CNY","stock_quantity":0}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_trailing_json", method: http.MethodPost, path: BasePath, body: validCreateJSON() + `{}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "create_large_body", method: http.MethodPost, path: BasePath, body: largeBody, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "wrong_content_type", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: testKey, wantStatus: http.StatusBadRequest, mutate: func(request *http.Request) { request.Header.Set("Content-Type", "text/plain") }},
		{name: "unsupported_content_type_parameter", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: testKey, wantStatus: http.StatusBadRequest, mutate: func(request *http.Request) { request.Header.Set("Content-Type", "application/json; profile=legacy") }},
		{name: "missing_idempotency", method: http.MethodPost, path: BasePath, body: validCreateJSON(), wantStatus: http.StatusBadRequest},
		{name: "short_idempotency", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: "short", wantStatus: http.StatusBadRequest},
		{name: "long_idempotency", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: strings.Repeat("k", 129), wantStatus: http.StatusBadRequest},
		{name: "spaced_idempotency", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: " service-period-key-0001 ", wantStatus: http.StatusBadRequest},
		{name: "duplicate_idempotency", method: http.MethodPost, path: BasePath, body: validCreateJSON(), key: testKey, wantStatus: http.StatusBadRequest, mutate: func(request *http.Request) { request.Header.Add("Idempotency-Key", "service-period-second-key") }},
		{name: "update_partial_body", method: http.MethodPut, path: BasePath + "/1", body: `{"expected_version":1,"name":"partial"}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "action_unknown_field", method: http.MethodPost, path: BasePath + "/1/enable", body: `{"expected_version":1,"force":true}`, key: testKey, wantStatus: http.StatusBadRequest},
		{name: "archive_empty_body", method: http.MethodDelete, path: BasePath + "/1", body: `{}`, key: testKey, wantStatus: http.StatusBadRequest},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := newServicePeriodRequest(testCase.method, testCase.path, testCase.body, testCase.key)
			if testCase.mutate != nil {
				testCase.mutate(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != testCase.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, testCase.wantStatus, response.Body.String())
			}
		})
	}
	if application.totalCalls() != 0 {
		t.Fatalf("malformed transport reached application: calls=%d", application.totalCalls())
	}
}

func TestHandlerReturnsExistingSameOriginServicePeriodShareOnlyWhenEnabled(t *testing.T) {
	application := newHTTPTestApplication()
	application.product.Lifecycle = productport.ServicePeriodEnabled
	application.product.Enabled = true
	handler := mustHandler(t, application)

	response := serveServicePeriodRequest(handler, http.MethodGet, BasePath+"/7/share", "", "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" ||
		response.Body.String() != "{\"ok\":true,\"service_product_id\":7,\"public_path\":\"/p/service_period/7\",\"local_only\":true,\"real_external_call_executed\":false}\n" || application.getCalls != 1 {
		t.Fatalf("enabled share status/body/calls=%d/%q/%d", response.Code, response.Body.String(), application.getCalls)
	}

	application.product.Enabled = false
	application.product.Lifecycle = productport.ServicePeriodDisabled
	response = serveServicePeriodRequest(handler, http.MethodGet, BasePath+"/7/share", "", "")
	if response.Code != http.StatusNotFound || application.getCalls != 2 {
		t.Fatalf("disabled share status/body/calls=%d/%q/%d", response.Code, response.Body.String(), application.getCalls)
	}
}

func TestHandlerRoutesAllLifecycleOperationsAndReturnsOnlyClosedDTO(t *testing.T) {
	application := newHTTPTestApplication()
	handler := mustHandler(t, application)

	requests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodGet, path: BasePath + "?limit=10&offset=0", status: http.StatusOK},
		{method: http.MethodGet, path: BasePath + "/7", status: http.StatusOK},
		{method: http.MethodPost, path: BasePath, body: validCreateJSON(), status: http.StatusCreated},
		{method: http.MethodPut, path: BasePath + "/7", body: `{"expected_version":3,"name":"updated","description":"local","price_minor":1234,"currency":"cny","stock_quantity":5}`, status: http.StatusOK},
		{method: http.MethodPost, path: BasePath + "/7/enable", body: `{"expected_version":4}`, status: http.StatusOK},
		{method: http.MethodPost, path: BasePath + "/7/disable", body: `{"expected_version":5}`, status: http.StatusOK},
		{method: http.MethodPost, path: BasePath + "/7/copy", body: `{"expected_version":6}`, status: http.StatusCreated},
		{method: http.MethodDelete, path: BasePath + "/7", body: `{"expected_version":7}`, status: http.StatusOK},
	}
	for index, request := range requests {
		key := ""
		if request.method != http.MethodGet {
			key = testKey + "-" + string(rune('a'+index))
		}
		response := serveServicePeriodRequest(handler, request.method, request.path, request.body, key)
		if response.Code != request.status {
			t.Fatalf("%s %s status=%d want=%d body=%s", request.method, request.path, response.Code, request.status, response.Body.String())
		}
		body := response.Body.String()
		for _, forbidden := range []string{"legacy_admin_projection", "created_by", "metadata", "provider", "receipt", "identity", "secret", "external"} {
			if strings.Contains(strings.ToLower(body), forbidden) {
				t.Fatalf("%s %s leaked %q: %s", request.method, request.path, forbidden, body)
			}
		}
		if !strings.Contains(body, `"ok":true`) || !strings.Contains(body, `"price_minor":1234`) {
			t.Fatalf("closed response missing fields: %s", body)
		}
	}

	if application.listCalls != 1 || application.getCalls != 1 || application.createCalls != 1 || application.updateCalls != 1 || application.enabledCalls != 2 || application.copyCalls != 1 || application.archiveCalls != 1 {
		t.Fatalf("calls list/get/create/update/enabled/copy/archive=%d/%d/%d/%d/%d/%d/%d",
			application.listCalls, application.getCalls, application.createCalls, application.updateCalls, application.enabledCalls, application.copyCalls, application.archiveCalls)
	}
	if application.lastListLimit != 10 || application.lastListOffset != 0 || application.lastGetID != 7 || application.lastUpdate.ID != 7 || application.lastUpdate.ExpectedVersion != 3 || application.lastUpdate.Actor != 71 || application.lastUpdate.Currency != "cny" {
		t.Fatalf("routed values list=%d/%d get=%d update=%+v", application.lastListLimit, application.lastListOffset, application.lastGetID, application.lastUpdate)
	}
	if application.enabledCommands[0].ID != 7 || !application.enabledCommands[0].Enabled || application.enabledCommands[0].ExpectedVersion != 4 || application.enabledCommands[1].Enabled || application.enabledCommands[1].ExpectedVersion != 5 {
		t.Fatalf("enable/disable commands=%+v", application.enabledCommands)
	}
	if application.lastCopy.ID != 7 || application.lastCopy.ExpectedVersion != 6 || application.lastArchive.ID != 7 || application.lastArchive.ExpectedVersion != 7 {
		t.Fatalf("copy/archive=%+v/%+v", application.lastCopy, application.lastArchive)
	}
}

func TestHandlerMapsApplicationFailuresWithoutRetry(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "validation", err: productapp.ErrInvalidProduct, wantStatus: http.StatusUnprocessableEntity},
		{name: "not_found", err: productapp.ErrNotFound, wantStatus: http.StatusNotFound},
		{name: "conflict", err: productapp.ErrConflict, wantStatus: http.StatusConflict},
		{name: "outcome_unknown", err: productapp.ErrUnavailable, wantStatus: http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			application := newHTTPTestApplication()
			application.createErr = testCase.err
			handler := mustHandler(t, application)
			response := serveServicePeriodRequest(handler, http.MethodPost, BasePath, validCreateJSON(), testKey)
			if response.Code != testCase.wantStatus || application.createCalls != 1 {
				t.Fatalf("status=%d want=%d calls=%d body=%s", response.Code, testCase.wantStatus, application.createCalls, response.Body.String())
			}
		})
	}
}

func mustHandler(t *testing.T, application productport.ServicePeriodApplication) http.Handler {
	t.Helper()
	leaf, err := NewHandler(application)
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

func serveServicePeriodRequest(handler http.Handler, method, path, body, key string) *httptest.ResponseRecorder {
	request := newServicePeriodRequest(method, path, body, key)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func newServicePeriodRequest(method, path, body, key string) *http.Request {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	capability := authport.CapabilityProductsWrite
	if method == http.MethodGet {
		capability = authport.CapabilityProductsRead
	}
	ctx := authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 71, Role: authport.RoleOps}, "service-period-test-session")
	var err error
	ctx, err = authport.WithAuthorization(ctx, authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal})
	if err != nil {
		panic(err)
	}
	return request.WithContext(ctx)
}

func validCreateJSON() string {
	return `{"product_code":"period-http","name":"period","description":"local","price_minor":1234,"currency":"cny","stock_quantity":5}`
}

type servicePeriodHTTPApplication struct {
	product         productport.ServicePeriodProduct
	listCalls       int
	getCalls        int
	createCalls     int
	updateCalls     int
	enabledCalls    int
	copyCalls       int
	archiveCalls    int
	lastListLimit   int32
	lastListOffset  int32
	lastGetID       productport.ID
	lastCreate      productport.CreateServicePeriodProductCommand
	lastUpdate      productport.UpdateServicePeriodProductCommand
	enabledCommands []productport.SetServicePeriodProductEnabledCommand
	lastCopy        productport.CopyServicePeriodProductCommand
	lastArchive     productport.ArchiveServicePeriodProductCommand
	listErr         error
	getErr          error
	createErr       error
	updateErr       error
	enabledErr      error
	copyErr         error
	archiveErr      error
}

func newHTTPTestApplication() *servicePeriodHTTPApplication {
	stamp := time.Date(2026, 8, 21, 2, 3, 4, 0, time.UTC)
	return &servicePeriodHTTPApplication{product: productport.ServicePeriodProduct{
		ServiceProductID: 7,
		ProductCode:      "period-http",
		Name:             "period",
		Description:      "local",
		PriceMinor:       1234,
		Currency:         "CNY",
		StockQuantity:    5,
		Lifecycle:        productport.ServicePeriodDraft,
		Enabled:          false,
		Archived:         false,
		Version:          3,
		CreatedAt:        stamp,
		UpdatedAt:        stamp,
	}}
}

func (application *servicePeriodHTTPApplication) ListServicePeriodProducts(_ context.Context, limit, offset int32) (productport.ServicePeriodPage, error) {
	application.listCalls++
	application.lastListLimit = limit
	application.lastListOffset = offset
	if application.listErr != nil {
		return productport.ServicePeriodPage{}, application.listErr
	}
	return productport.ServicePeriodPage{OK: true, Items: []productport.ServicePeriodProduct{application.product}, Total: 1, Limit: limit, Offset: offset}, nil
}

func (application *servicePeriodHTTPApplication) GetServicePeriodProduct(_ context.Context, id productport.ID) (productport.ServicePeriodProduct, error) {
	application.getCalls++
	application.lastGetID = id
	return application.product, application.getErr
}

func (application *servicePeriodHTTPApplication) CreateServicePeriodProduct(_ context.Context, command productport.CreateServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	application.createCalls++
	application.lastCreate = command
	return application.product, application.createErr
}

func (application *servicePeriodHTTPApplication) UpdateServicePeriodProduct(_ context.Context, command productport.UpdateServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	application.updateCalls++
	application.lastUpdate = command
	return application.product, application.updateErr
}

func (application *servicePeriodHTTPApplication) SetServicePeriodProductEnabled(_ context.Context, command productport.SetServicePeriodProductEnabledCommand) (productport.ServicePeriodProduct, error) {
	application.enabledCalls++
	application.enabledCommands = append(application.enabledCommands, command)
	return application.product, application.enabledErr
}

func (application *servicePeriodHTTPApplication) CopyServicePeriodProduct(_ context.Context, command productport.CopyServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	application.copyCalls++
	application.lastCopy = command
	return application.product, application.copyErr
}

func (application *servicePeriodHTTPApplication) ArchiveServicePeriodProduct(_ context.Context, command productport.ArchiveServicePeriodProductCommand) (productport.ServicePeriodProduct, error) {
	application.archiveCalls++
	application.lastArchive = command
	return application.product, application.archiveErr
}

func (application *servicePeriodHTTPApplication) totalCalls() int {
	return application.listCalls + application.getCalls + application.createCalls + application.updateCalls + application.enabledCalls + application.copyCalls + application.archiveCalls
}

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	authhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/http"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestLegacyBootFlowUsesV2SessionCapabilitiesAndContactService(t *testing.T) {
	service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}
	customers := &legacyCustomerStub{result: legacyCustomerResult()}
	handler, err := NewHandler(service, customers)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	config := legacyRoute(t, handler, authport.CapabilityConfigOverviewRead, handler.ConfigOverview)
	request := legacyRequest(http.MethodGet, "/api/admin/config/overview", legacyToken(1))
	request.AddCookie(&http.Cookie{Name: "aicrm_csrf", Value: legacyToken(2)})
	response := httptest.NewRecorder()
	config.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("config status = %d, body=%s", response.Code, response.Body.String())
	}
	if cookie := response.Result().Cookies(); len(cookie) != 1 || cookie[0].Name != LegacyCSRFCookieName || cookie[0].Value != legacyToken(2) || cookie[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("legacy csrf cookie = %#v, want mirrored %q", cookie, LegacyCSRFCookieName)
	}
	var configPayload struct {
		OK       bool `json:"ok"`
		Overview struct {
			Categories []struct {
				Capabilities []string `json:"capabilities"`
			} `json:"categories"`
		} `json:"overview"`
	}
	if err := json.NewDecoder(response.Body).Decode(&configPayload); err != nil || !configPayload.OK || len(configPayload.Overview.Categories) != 1 || len(configPayload.Overview.Categories[0].Capabilities) == 0 {
		t.Fatalf("config payload = %#v, err=%v", configPayload, err)
	}

	list := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListCustomers)
	response = httptest.NewRecorder()
	list.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers?keyword=%E5%BC%A0%E4%B8%89&limit=50", legacyToken(1)))
	if response.Code != http.StatusOK {
		t.Fatalf("customer status = %d, body=%s", response.Code, response.Body.String())
	}
	if customers.calls != 1 || customers.input.Keyword != "张三" || customers.input.Limit != 50 || customers.input.OwnerStaffID != nil {
		t.Fatalf("customer service input = %#v, calls=%d", customers.input, customers.calls)
	}
	var listPayload struct {
		OK        bool `json:"ok"`
		Customers []struct {
			CustomerID   int64  `json:"customer_id"`
			CustomerName string `json:"customer_name"`
		} `json:"customers"`
		SourceStatus string `json:"source_status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listPayload); err != nil || !listPayload.OK || listPayload.SourceStatus != "v2_contact_service" || len(listPayload.Customers) != 1 || listPayload.Customers[0].CustomerID != 101 || listPayload.Customers[0].CustomerName != "张三" {
		t.Fatalf("list payload = %#v, err=%v", listPayload, err)
	}
}

func TestFinalRouterMountsLegacyBootFlowWithAccountBudget(t *testing.T) {
	service := &legacyAuthStub{principal: authport.Principal{AdminUserID: 9, Role: authport.RoleAdmin}}
	customers := &legacyCustomerStub{result: legacyCustomerResult()}
	legacy, err := NewHandler(service, customers)
	if err != nil {
		t.Fatal(err)
	}
	legacy.customerDetail = &legacyCustomerDetailStub{result: legacyCustomerDetailResult(9)}
	legacy.identityResolve = &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 101}}
	legacy.weComCorpID = "corp-fixture"
	authHandler, err := authhttp.NewHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router, err := newAPIHandlerWithCallbackAndLegacy(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		authHandler,
		authHandler,
		legacy,
	)
	if err != nil {
		t.Fatal(err)
	}

	configRequest := legacyRequest(http.MethodGet, "/api/admin/config/overview", legacyToken(6))
	configRequest.AddCookie(&http.Cookie{Name: authhttp.CSRFCookieName, Value: legacyToken(7)})
	configResponse := httptest.NewRecorder()
	router.ServeHTTP(configResponse, configRequest)
	if configResponse.Code != http.StatusOK {
		t.Fatalf("config status = %d, body=%s", configResponse.Code, configResponse.Body.String())
	}
	if cookies := configResponse.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != LegacyCSRFCookieName {
		t.Fatalf("config cookies = %#v, want mirrored legacy CSRF", cookies)
	}

	customerResponse := httptest.NewRecorder()
	router.ServeHTTP(customerResponse, legacyRequest(http.MethodGet, "/api/customers?limit=1", legacyToken(6)))
	if customerResponse.Code != http.StatusOK || customers.calls != 1 {
		t.Fatalf("customer status/calls = %d/%d, body=%s", customerResponse.Code, customers.calls, customerResponse.Body.String())
	}
	detailResponse := httptest.NewRecorder()
	router.ServeHTTP(detailResponse, legacyRequest(http.MethodGet, "/api/customers/ext-router-fixture", legacyToken(6)))
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("customer detail status = %d, body=%s", detailResponse.Code, detailResponse.Body.String())
	}
}

func TestLegacyRoutesFailClosedForExpiredSessionCSRFAndOwnerScope(t *testing.T) {
	t.Run("missing dependency", func(t *testing.T) {
		response := httptest.NewRecorder()
		(&Handler{}).ListCustomers(response, httptest.NewRequest(http.MethodGet, "/api/customers", nil))
		assertLegacyError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
	})
	t.Run("expired session", func(t *testing.T) {
		handler, err := NewHandler(&legacyAuthStub{authenticateErr: authport.ErrUnauthenticated}, &legacyCustomerStub{result: legacyCustomerResult()})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListCustomers).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers", legacyToken(3)))
		assertLegacyError(t, response, http.StatusUnauthorized, platformhttp.CodeUnauthenticated)
	})
	t.Run("missing csrf", func(t *testing.T) {
		handler, err := NewHandler(&legacyAuthStub{}, &legacyCustomerStub{result: legacyCustomerResult()})
		if err != nil {
			t.Fatal(err)
		}
		protected, err := handler.RequireCSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("csrf-protected endpoint ran") }))
		if err != nil {
			t.Fatal(err)
		}
		request := legacyRequest(http.MethodPost, "/legacy-write", legacyToken(4))
		request = request.WithContext(authport.WithAuthenticatedSession(request.Context(), authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authport.SessionRef(legacyToken(4))))
		response := httptest.NewRecorder()
		protected.ServeHTTP(response, request)
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
	})
	t.Run("owner mismatch", func(t *testing.T) {
		staffID := int64(7)
		customers := &legacyCustomerStub{result: legacyCustomerResult()}
		handler, err := NewHandler(&legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &staffID}}, customers)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListCustomers).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers?owner_userid=8", legacyToken(5)))
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
		if customers.calls != 0 {
			t.Fatalf("customer service calls = %d, want 0", customers.calls)
		}
	})
	t.Run("rbac denied", func(t *testing.T) {
		customers := &legacyCustomerStub{result: legacyCustomerResult()}
		handler, err := NewHandler(&legacyAuthStub{principal: authport.Principal{AdminUserID: 8, Role: authport.RoleOps}}, customers)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListCustomers).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers", legacyToken(8)))
		assertLegacyError(t, response, http.StatusForbidden, platformhttp.CodeUnauthorized)
		if customers.calls != 0 {
			t.Fatalf("customer service calls = %d, want 0", customers.calls)
		}
	})
	t.Run("unsafe legacy filter", func(t *testing.T) {
		customers := &legacyCustomerStub{result: legacyCustomerResult()}
		handler, err := NewHandler(&legacyAuthStub{}, customers)
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.ListCustomers).ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers?mobile=13800000000", legacyToken(9)))
		assertLegacyError(t, response, http.StatusBadRequest, platformhttp.CodeMalformedRequest)
		if customers.calls != 0 {
			t.Fatalf("customer service calls = %d, want 0", customers.calls)
		}
	})
}

func TestLegacyCustomerDetailResolvesIdentityThenReadsOwnerScopedContact(t *testing.T) {
	owner := int64(7)
	auth := &legacyAuthStub{principal: authport.Principal{AdminUserID: 7, Role: authport.RoleSales, StaffID: &owner}}
	identity := &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 101}}
	detail := &legacyCustomerDetailStub{result: legacyCustomerDetailResult(owner)}
	handler := &Handler{auth: auth, customerDetail: detail, identityResolve: identity, weComCorpID: "corp-fixture"}
	endpoint := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.GetCustomer)
	router := chi.NewRouter()
	router.Get("/api/customers/{external_userid}", endpoint.ServeHTTP)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers/ext-fixture", legacyToken(10)))
	if response.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body=%s", response.Code, response.Body.String())
	}
	if identity.calls != 1 || identity.ref.Kind != identityport.KindWeComExternalUserID || identity.ref.Scope != "wecom-corp:corp-fixture" || identity.ref.Value != "ext-fixture" {
		t.Fatalf("identity resolve = %#v calls=%d", identity.ref, identity.calls)
	}
	if detail.calls != 1 || detail.input.ID != 101 || detail.input.OwnerStaffID == nil || *detail.input.OwnerStaffID != owner {
		t.Fatalf("contact detail input = %#v calls=%d", detail.input, detail.calls)
	}
	var body struct {
		OK       bool `json:"ok"`
		Customer struct {
			ExternalUserID string   `json:"external_userid"`
			CustomerID     int64    `json:"customer_id"`
			Tags           []string `json:"tags"`
		} `json:"customer"`
		SourceStatus             string `json:"source_status"`
		FallbackUsed             bool   `json:"fallback_used"`
		RealExternalCallExecuted bool   `json:"real_external_call_executed"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || !body.OK || body.Customer.ExternalUserID != "ext-fixture" || body.Customer.CustomerID != 101 || len(body.Customer.Tags) != 1 || body.Customer.Tags[0] != "high-intent" || body.SourceStatus != "v2_identity_contact_read" || body.FallbackUsed || body.RealExternalCallExecuted {
		t.Fatalf("detail body = %#v err=%v", body, err)
	}
}

func TestLegacyCustomerDetailFailsClosedBeforeContactRead(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		identity    identityport.ResolveResult
		identityErr error
		wantStatus  int
		wantCode    platformhttp.ErrorCode
	}{
		{name: "not found", identity: identityport.ResolveResult{Status: identityport.ResolveNotFound}, wantStatus: http.StatusNotFound, wantCode: platformhttp.CodeNotFound},
		{name: "conflict", identity: identityport.ResolveResult{Status: identityport.ResolveConflict}, wantStatus: http.StatusNotFound, wantCode: platformhttp.CodeNotFound},
		{name: "identity unavailable", identityErr: identityapp.ErrIdentityResolveFailed, wantStatus: http.StatusServiceUnavailable, wantCode: platformhttp.CodeDependencyUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			detail := &legacyCustomerDetailStub{result: legacyCustomerDetailResult(1)}
			handler := &Handler{auth: &legacyAuthStub{}, customerDetail: detail, identityResolve: &legacyIdentityStub{result: testCase.identity, err: testCase.identityErr}, weComCorpID: "corp-fixture"}
			endpoint := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.GetCustomer)
			router := chi.NewRouter()
			router.Get("/api/customers/{external_userid}", endpoint.ServeHTTP)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers/ext-fixture", legacyToken(11)))
			assertLegacyError(t, response, testCase.wantStatus, testCase.wantCode)
			if detail.calls != 0 {
				t.Fatalf("contact detail calls = %d, want 0", detail.calls)
			}
		})
	}
}

func TestLegacyCustomerDetailMapsContactFailuresWithoutIdentityLeak(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		detailErr  error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{name: "not found", detailErr: contactapp.ErrCustomerNotFound, wantStatus: http.StatusNotFound, wantCode: platformhttp.CodeNotFound},
		{name: "unavailable", detailErr: contactapp.ErrCustomerDetailUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: platformhttp.CodeDependencyUnavailable},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			detail := &legacyCustomerDetailStub{err: testCase.detailErr}
			handler := &Handler{auth: &legacyAuthStub{}, customerDetail: detail, identityResolve: &legacyIdentityStub{result: identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: 101}}, weComCorpID: "corp-fixture"}
			endpoint := legacyRoute(t, handler, authport.CapabilityCustomersRead, handler.GetCustomer)
			router := chi.NewRouter()
			router.Get("/api/customers/{external_userid}", endpoint.ServeHTTP)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, legacyRequest(http.MethodGet, "/api/customers/identity-secret", legacyToken(12)))
			assertLegacyError(t, response, testCase.wantStatus, testCase.wantCode)
			if detail.calls != 1 || strings.Contains(response.Body.String(), "identity-secret") {
				t.Fatalf("detail calls/body = %d/%s", detail.calls, response.Body.String())
			}
		})
	}
}

func legacyRoute(t *testing.T, handler *Handler, capability authport.Capability, endpoint http.HandlerFunc) http.Handler {
	t.Helper()
	protected, err := handler.Authorize(capability, endpoint)
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	return handler.Authenticate(protected)
}

func legacyRequest(method, target, session string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: LegacySessionCookieName, Value: session})
	return request
}

func assertLegacyError(t *testing.T, response *httptest.ResponseRecorder, status int, code platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d, body=%s", response.Code, status, response.Body.String())
	}
	var payload struct {
		Code platformhttp.ErrorCode `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || payload.Code != code {
		t.Fatalf("error payload = %#v, err=%v, want code=%q", payload, err, code)
	}
}

func legacyToken(fill byte) string {
	return base64.RawURLEncoding.EncodeToString(filledBytes(fill, 32))
}

func filledBytes(fill byte, length int) []byte {
	value := make([]byte, length)
	for index := range value {
		value[index] = fill
	}
	return value
}

type legacyAuthStub struct {
	principal       authport.Principal
	authenticateErr error
	csrfErr         error
}

type legacyIdentityStub struct {
	result identityport.ResolveResult
	err    error
	ref    identityport.IDRef
	calls  int
}

func (stub *legacyIdentityStub) Resolve(_ context.Context, ref identityport.IDRef) (identityport.ResolveResult, error) {
	stub.calls++
	stub.ref = ref
	return stub.result, stub.err
}

type legacyCustomerDetailStub struct {
	result contactapp.CustomerDetailStoreResult
	err    error
	input  contactapp.CustomerDetailInput
	calls  int
}

func (stub *legacyCustomerDetailStub) Get(_ context.Context, input contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

func legacyCustomerDetailResult(owner int64) contactapp.CustomerDetailStoreResult {
	now := time.Date(2026, 8, 15, 1, 2, 3, 0, time.UTC)
	return contactapp.CustomerDetailStoreResult{
		Customer: contactapp.CustomerRecord{ID: 101, Name: "Ada", OwnerStaffID: &owner, CreatedAt: now, UpdatedAt: now},
		Tags:     []contactapp.CustomerTagRecord{{ID: 5, Name: "high-intent"}},
	}
}

func (stub *legacyAuthStub) Authenticate(context.Context, authport.SessionRef) (authport.Principal, error) {
	if stub.authenticateErr != nil {
		return authport.Principal{}, stub.authenticateErr
	}
	if stub.principal.AdminUserID == 0 {
		return authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, nil
	}
	return stub.principal, nil
}

func (stub *legacyAuthStub) Authorize(_ context.Context, principal authport.Principal, capability authport.Capability) (authport.Authorization, error) {
	if principal.AdminUserID < 1 {
		return authport.Authorization{}, authport.ErrUnauthorized
	}
	if capability == authport.CapabilityAdminShellRead && (principal.Role == authport.RoleAdmin || principal.Role == authport.RoleOps) {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	if principal.Role == authport.RoleAdmin {
		if capability == authport.CapabilityAuthSessionRead || capability == authport.CapabilityAuthSessionLogout {
			return authport.Authorization{Capability: capability, Scope: authport.ScopeSelf}, nil
		}
		return authport.Authorization{Capability: capability, Scope: authport.ScopeGlobal}, nil
	}
	if principal.Role == authport.RoleSales &&
		(capability == authport.CapabilityCustomersRead || capability == authport.CapabilityOutboundRead) && principal.StaffID != nil {
		return authport.Authorization{Capability: capability, Scope: authport.ScopeOwnerStaff, OwnerStaffID: *principal.StaffID}, nil
	}
	return authport.Authorization{}, authport.ErrUnauthorized
}

func (stub *legacyAuthStub) ValidateCSRF(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return stub.csrfErr
}

func (*legacyAuthStub) Invalidate(context.Context, authport.SessionRef, authport.CSRFToken) error {
	return nil
}

type legacyCustomerStub struct {
	result contactapp.CustomerListResult
	err    error
	calls  int
	input  contactapp.CustomerListInput
}

func (stub *legacyCustomerStub) List(_ context.Context, input contactapp.CustomerListInput) (contactapp.CustomerListResult, error) {
	stub.calls++
	stub.input = input
	return stub.result, stub.err
}

func legacyCustomerResult() contactapp.CustomerListResult {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	return contactapp.CustomerListResult{Items: []contactapp.CustomerRecord{{
		ID: contactport.CustomerID(101), Name: "张三", Extra: []byte(`{}`), CreatedAt: now, UpdatedAt: now,
	}}, Total: 1, Watermark: now}
}

var _ authport.Service = (*legacyAuthStub)(nil)
var _ customerListApplication = (*legacyCustomerStub)(nil)

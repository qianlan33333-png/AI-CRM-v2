package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestNewTagCatalogHandlerRejectsNilApplications(t *testing.T) {
	t.Parallel()

	if handler, err := NewTagCatalogHandler(nil); err == nil || handler != nil {
		t.Fatalf("NewTagCatalogHandler(nil) = %#v, %v; want nil and fail-closed error", handler, err)
	}
	var typedNil *tagCatalogApplicationStub
	if handler, err := NewTagCatalogHandler(typedNil); err == nil || handler != nil {
		t.Fatalf("NewTagCatalogHandler(typed nil) = %#v, %v; want nil and fail-closed error", handler, err)
	}
}

func TestTagCatalogHandlerFailsClosedForNilHandlerAndRequest(t *testing.T) {
	t.Parallel()

	application := &tagCatalogApplicationStub{records: tagCatalogValidRecords()}
	handler := newTagCatalogHandler(t, application)
	request := tagCatalogRequest(t,
		&authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)
	var nilHandler *TagCatalogHandler
	for _, testCase := range []struct {
		name   string
		invoke func(*httptest.ResponseRecorder)
	}{
		{name: "nil handler", invoke: func(response *httptest.ResponseRecorder) {
			nilHandler.ListTags(response, request)
		}},
		{name: "nil request", invoke: func(response *httptest.ResponseRecorder) {
			handler.ListTags(response, nil)
		}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			testCase.invoke(response)
			assertTagCatalogError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
		})
	}
	if application.calls != 0 {
		t.Fatalf("application calls = %d, want 0", application.calls)
	}
}

func TestTagCatalogHandlerAuthorizesAllRuntimeRolesWithoutScopingTheCatalog(t *testing.T) {
	t.Parallel()

	ownerStaffID := int64(71)
	for _, testCase := range []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
	}{
		{
			name:          "admin global",
			principal:     authport.Principal{AdminUserID: 101, Role: authport.RoleAdmin},
			authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
		},
		{
			name:          "ops global",
			principal:     authport.Principal{AdminUserID: 102, Role: authport.RoleOps},
			authorization: authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
		},
		{
			name:      "sales owner authorization reads the global catalog",
			principal: authport.Principal{AdminUserID: 103, Role: authport.RoleSales, StaffID: &ownerStaffID},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: ownerStaffID,
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &tagCatalogApplicationStub{records: tagCatalogValidRecords()}
			request := tagCatalogRequest(t, &testCase.principal, &testCase.authorization)
			response := serveTagCatalog(newTagCatalogHandler(t, application), request)

			assertTagCatalogSuccess(t, response)
			if application.calls != 1 || len(application.contexts) != 1 || application.contexts[0] != request.Context() {
				t.Fatalf("application calls/contexts = %d/%#v, want one call with exact request context", application.calls, application.contexts)
			}
		})
	}
}

func TestTagCatalogHandlerFailsClosedForAuthenticationAndScopeMismatches(t *testing.T) {
	t.Parallel()

	ownerStaffID := int64(81)
	differentOwner := int64(82)
	for _, testCase := range []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{
			name:       "missing authorization",
			principal:  &authport.Principal{AdminUserID: 201, Role: authport.RoleAdmin},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name: "missing principal",
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
		{
			name:      "wrong capability",
			principal: &authport.Principal{AdminUserID: 202, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite, Scope: authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales cannot claim global scope",
			principal: &authport.Principal{AdminUserID: 203, Role: authport.RoleSales, StaffID: &ownerStaffID},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "admin cannot claim owner scope",
			principal: &authport.Principal{AdminUserID: 204, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: ownerStaffID,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "ops cannot claim owner scope",
			principal: &authport.Principal{AdminUserID: 205, Role: authport.RoleOps},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: ownerStaffID,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales owner mismatch",
			principal: &authport.Principal{AdminUserID: 206, Role: authport.RoleSales, StaffID: &differentOwner},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: ownerStaffID,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales lacks staff identity",
			principal: &authport.Principal{AdminUserID: 207, Role: authport.RoleSales},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: ownerStaffID,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "invalid principal identifier",
			principal: &authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &tagCatalogApplicationStub{records: tagCatalogValidRecords()}
			response := serveTagCatalog(
				newTagCatalogHandler(t, application),
				tagCatalogRequest(t, testCase.principal, testCase.authorization),
			)
			assertTagCatalogError(t, response, testCase.wantStatus, testCase.wantCode)
			if application.calls != 0 {
				t.Fatalf("application calls = %d, want 0", application.calls)
			}
		})
	}
}

func TestTagCatalogHandlerMapsApplicationFailuresWithoutLeak(t *testing.T) {
	t.Parallel()

	const secret = "tag-catalog-dependency-secret"
	for _, applicationErr := range []error{
		errors.Join(contactapp.ErrTagCatalogUnavailable, errors.New(secret)),
		errors.New(secret),
	} {
		application := &tagCatalogApplicationStub{err: applicationErr}
		response := serveTagCatalog(newTagCatalogHandler(t, application), tagCatalogAdminRequest(t))
		assertTagCatalogError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
		assertTagCatalogResponseDoesNotContain(t, response, secret, "tag catalog unavailable")
		if application.calls != 1 {
			t.Fatalf("application calls = %d, want 1", application.calls)
		}
	}
}

func TestTagCatalogHandlerWritesExactGroupedAndUngroupedJSON(t *testing.T) {
	t.Parallel()

	application := &tagCatalogApplicationStub{records: tagCatalogValidRecords()}
	response := serveTagCatalog(newTagCatalogHandler(t, application), tagCatalogAdminRequest(t))
	assertTagCatalogSuccess(t, response)
	const want = "{\"items\":[{\"group_id\":31,\"group_name\":\"Lifecycle\",\"id\":81,\"name\":\"Priority\",\"sort_order\":6},{\"id\":82,\"name\":\"Ungrouped\",\"sort_order\":7}]}\n"
	if got := response.Body.String(); got != want {
		t.Fatalf("response JSON = %s, want %s", got, want)
	}
	if strings.Contains(response.Body.String(), "wecom_tag_id") {
		t.Fatalf("response leaked provider identity: %s", response.Body.String())
	}
}

func TestTagCatalogHandlerEncodesEmptyCatalogAsNonNullArray(t *testing.T) {
	t.Parallel()

	application := &tagCatalogApplicationStub{records: []contactapp.TagCatalogRecord{}}
	response := serveTagCatalog(newTagCatalogHandler(t, application), tagCatalogAdminRequest(t))
	assertTagCatalogSuccess(t, response)
	if got := response.Body.String(); got != "{\"items\":[]}\n" {
		t.Fatalf("response JSON = %q, want non-null empty items array", got)
	}
}

func TestTagCatalogHandlerRejectsInvalidApplicationRecordsWithoutLeak(t *testing.T) {
	t.Parallel()

	const secret = "tag-catalog-record-secret"
	groupID := int64(31)
	groupName := "Lifecycle"
	groupSort := int32(1)
	for _, testCase := range []struct {
		name   string
		record contactapp.TagCatalogRecord
	}{
		{name: "invalid tag identifier", record: contactapp.TagCatalogRecord{ID: 0, Name: secret}},
		{name: "empty tag name", record: contactapp.TagCatalogRecord{ID: 81, Name: ""}},
		{name: "group identifier without name", record: contactapp.TagCatalogRecord{ID: 81, GroupID: &groupID, Name: secret}},
		{name: "group name without identifier", record: contactapp.TagCatalogRecord{ID: 81, GroupName: &groupName, Name: secret}},
		{name: "group without sort order", record: contactapp.TagCatalogRecord{ID: 81, GroupID: &groupID, GroupName: &groupName, Name: secret}},
		{name: "sort order without group", record: contactapp.TagCatalogRecord{ID: 81, GroupSortOrder: &groupSort, Name: secret}},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &tagCatalogApplicationStub{records: []contactapp.TagCatalogRecord{testCase.record}}
			response := serveTagCatalog(newTagCatalogHandler(t, application), tagCatalogAdminRequest(t))
			assertTagCatalogError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			assertTagCatalogResponseDoesNotContain(t, response, secret, "tag catalog unavailable")
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

type tagCatalogApplicationStub struct {
	records  []contactapp.TagCatalogRecord
	err      error
	calls    int
	contexts []context.Context
}

var _ tagCatalogApplication = (*tagCatalogApplicationStub)(nil)

func (stub *tagCatalogApplicationStub) List(ctx context.Context) ([]contactapp.TagCatalogRecord, error) {
	stub.calls++
	stub.contexts = append(stub.contexts, ctx)
	return stub.records, stub.err
}

func newTagCatalogHandler(t *testing.T, application tagCatalogApplication) *TagCatalogHandler {
	t.Helper()
	handler, err := NewTagCatalogHandler(application)
	if err != nil {
		t.Fatalf("NewTagCatalogHandler() error = %v", err)
	}
	return handler
}

func tagCatalogRequest(t *testing.T, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	t.Helper()

	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "tag-catalog-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization(%#v) error = %v", *authorization, err)
		}
	}
	return httptest.NewRequest(http.MethodGet, "/api/v1/tags", nil).WithContext(ctx)
}

func tagCatalogAdminRequest(t *testing.T) *http.Request {
	t.Helper()
	return tagCatalogRequest(t,
		&authport.Principal{AdminUserID: 301, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)
}

func serveTagCatalog(handler *TagCatalogHandler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ListTags(response, request)
	return response
}

func assertTagCatalogSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertTagCatalogHeaders(t, response)
}

func assertTagCatalogError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertTagCatalogHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v, want code %q with stable message and request_id", body, wantCode)
	}
}

func assertTagCatalogHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertTagCatalogResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(response.Body.String(), value) {
			t.Fatalf("response leaked %q: %s", value, response.Body.String())
		}
	}
}

func tagCatalogValidRecords() []contactapp.TagCatalogRecord {
	groupID := int64(31)
	groupName := "Lifecycle"
	groupSort := int32(1)
	return []contactapp.TagCatalogRecord{
		{ID: 81, GroupID: &groupID, GroupName: &groupName, GroupSortOrder: &groupSort, Name: "Priority", SortOrder: 6},
		{ID: 82, Name: "Ungrouped", SortOrder: 7},
	}
}

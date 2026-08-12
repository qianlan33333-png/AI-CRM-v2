package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestNewCustomerDetailHandlerRejectsNilApplications(t *testing.T) {
	t.Parallel()

	if handler, err := NewCustomerDetailHandler(nil); err == nil || handler != nil {
		t.Fatalf("NewCustomerDetailHandler(nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}
	var typedNil *customerDetailApplicationStub
	if handler, err := NewCustomerDetailHandler(typedNil); err == nil || handler != nil {
		t.Fatalf("NewCustomerDetailHandler(typed nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}
}

func TestCustomerDetailHandlerFailsClosedForNilHandlerAndRequest(t *testing.T) {
	t.Parallel()

	application := &customerDetailApplicationStub{result: customerDetailValidResult()}
	handler := newCustomerDetailHandler(t, application)
	request := customerDetailRequest(t,
		&authport.Principal{AdminUserID: 41, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)
	var nilHandler *CustomerDetailHandler
	for _, testCase := range []struct {
		name   string
		invoke func(*httptest.ResponseRecorder)
	}{
		{
			name: "nil handler",
			invoke: func(response *httptest.ResponseRecorder) {
				nilHandler.GetCustomer(response, request, 71)
			},
		},
		{
			name: "nil request",
			invoke: func(response *httptest.ResponseRecorder) {
				handler.GetCustomer(response, nil, 71)
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			testCase.invoke(response)
			assertCustomerDetailError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
		})
	}
	if application.calls != 0 {
		t.Fatalf("application calls = %d, want 0", application.calls)
	}
}

func TestCustomerDetailHandlerMapsGlobalAdminAndOpsInputsExactly(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		role authport.Role
	}{
		{name: "admin", role: authport.RoleAdmin},
		{name: "ops", role: authport.RoleOps},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerDetailApplicationStub{result: customerDetailValidResult()}
			handler := newCustomerDetailHandler(t, application)
			request := customerDetailRequest(t,
				&authport.Principal{AdminUserID: 42, Role: testCase.role},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)

			response := serveCustomerDetail(handler, request, 71)
			assertCustomerDetailSuccess(t, response)
			want := contactapp.CustomerDetailInput{ID: contactport.CustomerID(71)}
			if application.calls != 1 || len(application.inputs) != 1 || !reflect.DeepEqual(application.inputs[0], want) {
				t.Fatalf("application calls/inputs = %d/%#v, want 1/%#v", application.calls, application.inputs, want)
			}
			if len(application.contexts) != 1 || application.contexts[0] != request.Context() {
				t.Fatalf("application contexts = %#v, want the exact request context", application.contexts)
			}
		})
	}
}

func TestCustomerDetailHandlerScopesSalesToAuthorizedOwner(t *testing.T) {
	t.Parallel()

	owner := int64(52)
	application := &customerDetailApplicationStub{result: customerDetailValidResult()}
	handler := newCustomerDetailHandler(t, application)
	request := customerDetailRequest(t,
		&authport.Principal{AdminUserID: 43, Role: authport.RoleSales, StaffID: &owner},
		&authport.Authorization{
			Capability:   authport.CapabilityCustomersRead,
			Scope:        authport.ScopeOwnerStaff,
			OwnerStaffID: owner,
		},
	)

	response := serveCustomerDetail(handler, request, 72)
	assertCustomerDetailSuccess(t, response)
	want := contactapp.CustomerDetailInput{ID: contactport.CustomerID(72), OwnerStaffID: &owner}
	if application.calls != 1 || len(application.inputs) != 1 || !reflect.DeepEqual(application.inputs[0], want) {
		t.Fatalf("application calls/inputs = %d/%#v, want 1/%#v", application.calls, application.inputs, want)
	}
}

func TestCustomerDetailHandlerFailsClosedForAuthenticationMismatches(t *testing.T) {
	t.Parallel()

	staff52 := int64(52)
	staff53 := int64(53)
	for _, testCase := range []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{
			name:       "missing authentication and authorization",
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:       "missing authorization",
			principal:  &authport.Principal{AdminUserID: 44, Role: authport.RoleAdmin},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name: "missing principal",
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
		{
			name:      "wrong capability",
			principal: &authport.Principal{AdminUserID: 44, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales cannot use global scope",
			principal: &authport.Principal{AdminUserID: 44, Role: authport.RoleSales, StaffID: &staff52},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "admin cannot use owner scope",
			principal: &authport.Principal{AdminUserID: 44, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: staff52,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales is missing staff",
			principal: &authport.Principal{AdminUserID: 44, Role: authport.RoleSales},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: staff52,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales staff differs from authorized owner",
			principal: &authport.Principal{AdminUserID: 44, Role: authport.RoleSales, StaffID: &staff53},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: staff52,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "principal identifier is invalid",
			principal: &authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerDetailApplicationStub{result: customerDetailValidResult()}
			response := serveCustomerDetail(
				newCustomerDetailHandler(t, application),
				customerDetailRequest(t, testCase.principal, testCase.authorization),
				71,
			)
			assertCustomerDetailError(t, response, testCase.wantStatus, testCase.wantCode)
			if application.calls != 0 {
				t.Fatalf("application calls = %d, want 0", application.calls)
			}
		})
	}
}

func TestCustomerDetailHandlerMapsInvalidIDsToNotFoundWithoutLeak(t *testing.T) {
	t.Parallel()

	const secret = "detail-input-secret"
	for _, customerID := range []generated.CustomerID{0, -19} {
		customerID := customerID
		t.Run("invalid customer identifier", func(t *testing.T) {
			application := &customerDetailApplicationStub{
				err: errors.Join(contactapp.ErrInvalidCustomerDetailQuery, errors.New(secret)),
			}
			request := customerDetailRequest(t,
				&authport.Principal{AdminUserID: 45, Role: authport.RoleAdmin},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)

			response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, customerID)
			assertCustomerDetailError(t, response, http.StatusNotFound, platformhttp.CodeNotFound)
			assertCustomerDetailResponseDoesNotContain(t, response, secret, "invalid customer detail query")
			if application.calls != 1 || len(application.inputs) != 1 || application.inputs[0].ID != contactport.CustomerID(customerID) {
				t.Fatalf("application calls/inputs = %d/%#v, want one input with ID %d", application.calls, application.inputs, customerID)
			}
		})
	}
}

func TestCustomerDetailHandlerHidesOutOfScopeCustomersAsNotFound(t *testing.T) {
	t.Parallel()

	const secret = "detail-not-found-secret"
	owner := int64(52)
	for _, testCase := range []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{
			name:      "global result is absent",
			principal: authport.Principal{AdminUserID: 46, Role: authport.RoleOps},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name:      "owner filtered result is absent",
			principal: authport.Principal{AdminUserID: 46, Role: authport.RoleSales, StaffID: &owner},
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: owner,
			},
			wantOwner: &owner,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerDetailApplicationStub{
				err: errors.Join(contactapp.ErrCustomerNotFound, errors.New(secret)),
			}
			request := customerDetailRequest(t, &testCase.principal, &testCase.authorization)

			response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, 73)
			assertCustomerDetailError(t, response, http.StatusNotFound, platformhttp.CodeNotFound)
			assertCustomerDetailResponseDoesNotContain(t, response, secret, "customer not found")
			if application.calls != 1 || len(application.inputs) != 1 || !customerDetailEqualInt64Pointer(application.inputs[0].OwnerStaffID, testCase.wantOwner) {
				t.Fatalf("application calls/inputs = %d/%#v, want one input scoped to %#v", application.calls, application.inputs, testCase.wantOwner)
			}
		})
	}
}

func TestCustomerDetailHandlerMapsDependencyFailuresToServiceUnavailableWithoutLeak(t *testing.T) {
	t.Parallel()

	const secret = "detail-dependency-secret"
	for _, testCase := range []struct {
		name string
		err  error
	}{
		{
			name: "known unavailable error",
			err:  errors.Join(contactapp.ErrCustomerDetailUnavailable, errors.New(secret)),
		},
		{
			name: "unknown application error",
			err:  errors.New(secret),
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerDetailApplicationStub{err: testCase.err}
			request := customerDetailRequest(t,
				&authport.Principal{AdminUserID: 47, Role: authport.RoleAdmin},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)

			response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, 74)
			assertCustomerDetailError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			assertCustomerDetailResponseDoesNotContain(t, response, secret, "customer detail unavailable")
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerDetailHandlerWritesExactCustomerAndTagJSON(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("CST", 8*60*60)
	avatarURL := "https://cdn.example.test/avatar.png"
	gender := int16(2)
	stageID := int64(12)
	ownerStaffID := int64(13)
	channelID := int64(14)
	addedAt := time.Date(2026, time.August, 1, 9, 8, 7, 123456789, zone)
	lastInteractAt := time.Date(2026, time.August, 2, 10, 9, 8, 223456789, zone)
	createdAt := time.Date(2026, time.August, 3, 11, 10, 9, 323456789, zone)
	updatedAt := time.Date(2026, time.August, 4, 12, 11, 10, 423456789, zone)
	groupID := int64(31)
	groupName := "Lifecycle"
	result := contactapp.CustomerDetailStoreResult{
		Customer: contactapp.CustomerRecord{
			ID:             contactport.CustomerID(71),
			Name:           "Ada Lovelace",
			AvatarURL:      &avatarURL,
			Gender:         &gender,
			StageID:        &stageID,
			OwnerStaffID:   &ownerStaffID,
			ChannelID:      &channelID,
			AddedAt:        &addedAt,
			LastInteractAt: &lastInteractAt,
			IsDeleted:      true,
			Extra:          json.RawMessage(`{"large":9007199254740993.125,"nested":{"rank":2},"active":true}`),
			CreatedAt:      createdAt,
			UpdatedAt:      updatedAt,
		},
		Tags: []contactapp.CustomerTagRecord{
			{ID: 81, GroupID: &groupID, GroupName: &groupName, Name: "Priority", SortOrder: 6},
			{ID: 82, Name: "Ungrouped", SortOrder: 7},
		},
	}
	application := &customerDetailApplicationStub{result: result}
	request := customerDetailRequest(t,
		&authport.Principal{AdminUserID: 48, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)

	response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, 71)
	assertCustomerDetailSuccess(t, response)
	const want = "{\"customer\":{\"added_at\":\"2026-08-01T01:08:07.123456789Z\",\"avatar_url\":\"https://cdn.example.test/avatar.png\",\"channel_id\":14,\"created_at\":\"2026-08-03T03:10:09.323456789Z\",\"extra\":{\"active\":true,\"large\":9007199254740993.125,\"nested\":{\"rank\":2}},\"gender\":2,\"id\":71,\"is_deleted\":true,\"last_interact_at\":\"2026-08-02T02:09:08.223456789Z\",\"name\":\"Ada Lovelace\",\"owner_staff_id\":13,\"stage_id\":12,\"updated_at\":\"2026-08-04T04:11:10.423456789Z\"},\"tags\":[{\"group_id\":31,\"group_name\":\"Lifecycle\",\"id\":81,\"name\":\"Priority\",\"sort_order\":6},{\"id\":82,\"name\":\"Ungrouped\",\"sort_order\":7}]}\n"
	if got := response.Body.String(); got != want {
		t.Fatalf("response JSON = %s, want %s", got, want)
	}
}

func TestCustomerDetailHandlerEncodesNilTagsAsEmptyArray(t *testing.T) {
	t.Parallel()

	result := customerDetailValidResult()
	result.Tags = nil
	application := &customerDetailApplicationStub{result: result}
	request := customerDetailRequest(t,
		&authport.Principal{AdminUserID: 49, Role: authport.RoleOps},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)

	response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, 75)
	assertCustomerDetailSuccess(t, response)
	var body struct {
		Tags json.RawMessage `json:"tags"`
	}
	decodeCustomerDetailJSON(t, response, &body)
	if string(body.Tags) != "[]" {
		t.Fatalf("tags JSON = %s, want nonnil empty array []", body.Tags)
	}
}

func TestCustomerDetailHandlerRejectsInvalidApplicationOutputsWithoutLeak(t *testing.T) {
	t.Parallel()

	const customerSecret = "detail-customer-output-secret"
	const tagSecret = "detail-tag-output-secret"
	const identitySecret = "detail-external-identity-secret"
	for _, testCase := range []struct {
		name      string
		result    func() contactapp.CustomerDetailStoreResult
		forbidden string
	}{
		{
			name: "invalid customer",
			result: func() contactapp.CustomerDetailStoreResult {
				result := customerDetailValidResult()
				result.Customer.ID = 0
				result.Customer.Name = customerSecret
				return result
			},
			forbidden: customerSecret,
		},
		{
			name: "invalid tag identifier",
			result: func() contactapp.CustomerDetailStoreResult {
				result := customerDetailValidResult()
				result.Tags = []contactapp.CustomerTagRecord{{ID: 0, Name: tagSecret}}
				return result
			},
			forbidden: tagSecret,
		},
		{
			name: "incomplete tag group",
			result: func() contactapp.CustomerDetailStoreResult {
				groupID := int64(31)
				result := customerDetailValidResult()
				result.Tags = []contactapp.CustomerTagRecord{{ID: 81, GroupID: &groupID, Name: tagSecret}}
				return result
			},
			forbidden: tagSecret,
		},
		{
			name: "external identity in extra",
			result: func() contactapp.CustomerDetailStoreResult {
				result := customerDetailValidResult()
				result.Customer.Extra = json.RawMessage(`{"nested":{"phone":"` + identitySecret + `"}}`)
				return result
			},
			forbidden: identitySecret,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerDetailApplicationStub{result: testCase.result()}
			request := customerDetailRequest(t,
				&authport.Principal{AdminUserID: 50, Role: authport.RoleAdmin},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)

			response := serveCustomerDetail(newCustomerDetailHandler(t, application), request, 76)
			assertCustomerDetailError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			assertCustomerDetailResponseDoesNotContain(t, response, testCase.forbidden, "invalid customer", "invalid tag")
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

type customerDetailApplicationStub struct {
	result   contactapp.CustomerDetailStoreResult
	err      error
	get      func(context.Context, contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error)
	calls    int
	contexts []context.Context
	inputs   []contactapp.CustomerDetailInput
}

var _ customerDetailApplication = (*customerDetailApplicationStub)(nil)

func (stub *customerDetailApplicationStub) Get(ctx context.Context, input contactapp.CustomerDetailInput) (contactapp.CustomerDetailStoreResult, error) {
	stub.calls++
	stub.contexts = append(stub.contexts, ctx)
	stub.inputs = append(stub.inputs, input)
	if stub.get != nil {
		return stub.get(ctx, input)
	}
	return stub.result, stub.err
}

func newCustomerDetailHandler(t *testing.T, application customerDetailApplication) *CustomerDetailHandler {
	t.Helper()
	handler, err := NewCustomerDetailHandler(application)
	if err != nil {
		t.Fatalf("NewCustomerDetailHandler() error = %v", err)
	}
	return handler
}

func customerDetailRequest(t *testing.T, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	t.Helper()

	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "customer-detail-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization(%#v) error = %v", *authorization, err)
		}
	}
	return httptest.NewRequest(http.MethodGet, "/api/v1/customers/71", nil).WithContext(ctx)
}

func serveCustomerDetail(handler *CustomerDetailHandler, request *http.Request, customerID generated.CustomerID) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.GetCustomer(response, request, customerID)
	return response
}

func assertCustomerDetailSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertCustomerDetailResponseHeaders(t, response)
}

func assertCustomerDetailError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertCustomerDetailResponseHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	decodeCustomerDetailJSON(t, response, &body)
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v, want code %q with stable message and request_id", body, wantCode)
	}
}

func assertCustomerDetailResponseHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertCustomerDetailResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(response.Body.String(), value) {
			t.Fatalf("response leaked %q: %s", value, response.Body.String())
		}
	}
}

func decodeCustomerDetailJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func customerDetailValidResult() contactapp.CustomerDetailStoreResult {
	avatarURL := "https://cdn.example.test/avatar.png"
	createdAt := time.Date(2026, time.August, 11, 8, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC)
	return contactapp.CustomerDetailStoreResult{Customer: contactapp.CustomerRecord{
		ID:        contactport.CustomerID(1),
		Name:      "valid customer",
		AvatarURL: &avatarURL,
		Extra:     json.RawMessage(`{"source":"test"}`),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}}
}

func customerDetailEqualInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

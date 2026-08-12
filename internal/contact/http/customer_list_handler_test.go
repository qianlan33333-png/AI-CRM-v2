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

func TestNewCustomerListHandlerRejectsNilApplications(t *testing.T) {
	t.Parallel()

	if handler, err := NewCustomerListHandler(nil); err == nil || handler != nil {
		t.Fatalf("NewCustomerListHandler(nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}
	var typedNil *customerListApplicationStub
	if handler, err := NewCustomerListHandler(typedNil); err == nil || handler != nil {
		t.Fatalf("NewCustomerListHandler(typed nil) = %#v, %v; want nil, fail-closed error", handler, err)
	}
}

func TestCustomerListHandlerMapsEveryQueryParameterLosslessly(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("CST", 8*60*60)
	addedAfter := time.Date(2026, time.August, 1, 9, 8, 7, 123456789, zone)
	addedBefore := time.Date(2026, time.August, 2, 10, 9, 8, 223456789, zone)
	interactedAfter := time.Date(2026, time.August, 3, 11, 10, 9, 323456789, zone)
	interactedBefore := time.Date(2026, time.August, 4, 12, 11, 10, 423456789, zone)
	params := generated.ListCustomersParams{
		Cursor:             customerListPtr("opaque.cursor-v1_A-9"),
		Limit:              customerListPtr(87),
		Keyword:            customerListPtr("  张三 / premium  "),
		OwnerStaffId:       customerListPtr(int64(11)),
		StageId:            customerListPtr(int64(12)),
		ChannelId:          customerListPtr(int64(13)),
		TagId:              customerListPtr(int64(14)),
		IsDeleted:          customerListPtr(true),
		AddedAfter:         customerListPtr(addedAfter),
		AddedBefore:        customerListPtr(addedBefore),
		LastInteractAfter:  customerListPtr(interactedAfter),
		LastInteractBefore: customerListPtr(interactedBefore),
	}
	want := contactapp.CustomerListInput{
		Cursor:             *params.Cursor,
		Limit:              int32(*params.Limit),
		Keyword:            *params.Keyword,
		OwnerStaffID:       customerListPtr(*params.OwnerStaffId),
		StageID:            customerListPtr(*params.StageId),
		ChannelID:          customerListPtr(*params.ChannelId),
		TagID:              customerListPtr(*params.TagId),
		IsDeleted:          *params.IsDeleted,
		AddedAfter:         customerListPtr(*params.AddedAfter),
		AddedBefore:        customerListPtr(*params.AddedBefore),
		LastInteractAfter:  customerListPtr(*params.LastInteractAfter),
		LastInteractBefore: customerListPtr(*params.LastInteractBefore),
	}
	application := &customerListApplicationStub{result: customerListValidResult()}
	handler := newCustomerListHandler(t, application)
	request := customerListRequest(t,
		&authport.Principal{AdminUserID: 91, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)

	response := serveCustomerList(handler, request, params)
	assertCustomerListSuccess(t, response)
	if application.calls != 1 {
		t.Fatalf("application calls = %d, want 1", application.calls)
	}
	if len(application.inputs) != 1 || !reflect.DeepEqual(application.inputs[0], want) {
		t.Fatalf("application input = %#v, want %#v", application.inputs, want)
	}
	if len(application.contexts) != 1 || application.contexts[0] == nil {
		t.Fatalf("application contexts = %#v, want one nonnil request context", application.contexts)
	}
}

func TestCustomerListHandlerAllowsGlobalAdminAndOpsWithOptionalOwner(t *testing.T) {
	t.Parallel()

	owner := int64(314)
	for _, testCase := range []struct {
		name  string
		role  authport.Role
		owner *int64
	}{
		{name: "admin without owner", role: authport.RoleAdmin},
		{name: "admin with owner", role: authport.RoleAdmin, owner: &owner},
		{name: "ops without owner", role: authport.RoleOps},
		{name: "ops with owner", role: authport.RoleOps, owner: &owner},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			application := &customerListApplicationStub{result: customerListValidResult()}
			handler := newCustomerListHandler(t, application)
			params := generated.ListCustomersParams{OwnerStaffId: testCase.owner}
			request := customerListRequest(t,
				&authport.Principal{AdminUserID: 92, Role: testCase.role},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)

			response := serveCustomerList(handler, request, params)
			assertCustomerListSuccess(t, response)
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
			if got := application.inputs[0].OwnerStaffID; !customerListEqualInt64Pointer(got, testCase.owner) {
				t.Fatalf("owner_staff_id = %#v, want %#v", got, testCase.owner)
			}
		})
	}
}

func TestCustomerListHandlerScopesSalesToAuthorizedOwner(t *testing.T) {
	t.Parallel()

	owner := int64(42)
	otherOwner := int64(43)
	for _, testCase := range []struct {
		name       string
		owner      *int64
		wantOwner  *int64
		wantStatus int
		wantCode   platformhttp.ErrorCode
		wantCalls  int
	}{
		{name: "missing owner is forced", wantOwner: &owner, wantStatus: http.StatusOK, wantCalls: 1},
		{name: "same owner is allowed", owner: &owner, wantOwner: &owner, wantStatus: http.StatusOK, wantCalls: 1},
		{name: "other owner is forbidden", owner: &otherOwner, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			application := &customerListApplicationStub{result: customerListValidResult()}
			handler := newCustomerListHandler(t, application)
			request := customerListRequest(t,
				&authport.Principal{AdminUserID: 93, Role: authport.RoleSales, StaffID: &owner},
				&authport.Authorization{
					Capability:   authport.CapabilityCustomersRead,
					Scope:        authport.ScopeOwnerStaff,
					OwnerStaffID: owner,
				},
			)

			response := serveCustomerList(handler, request, generated.ListCustomersParams{OwnerStaffId: testCase.owner})
			if testCase.wantStatus != http.StatusOK {
				assertCustomerListError(t, response, testCase.wantStatus, testCase.wantCode)
			} else {
				assertCustomerListSuccess(t, response)
				if got := application.inputs[0].OwnerStaffID; !customerListEqualInt64Pointer(got, testCase.wantOwner) {
					t.Fatalf("owner_staff_id = %#v, want %#v", got, testCase.wantOwner)
				}
			}
			if application.calls != testCase.wantCalls {
				t.Fatalf("application calls = %d, want %d", application.calls, testCase.wantCalls)
			}
		})
	}
}

func TestCustomerListHandlerFailsClosedForAuthorizationAndPrincipalMismatches(t *testing.T) {
	t.Parallel()

	staff42 := int64(42)
	staff43 := int64(43)
	for _, testCase := range []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{
			name:       "missing authentication and authorization context",
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:       "missing authorization context",
			principal:  &authport.Principal{AdminUserID: 94, Role: authport.RoleAdmin},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name: "missing principal context",
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
		{
			name:      "wrong capability",
			principal: &authport.Principal{AdminUserID: 94, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersWrite,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "self scope is not a customer list scope",
			principal: &authport.Principal{AdminUserID: 94, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityAuthSessionRead,
				Scope:      authport.ScopeSelf,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales cannot use global customer authorization",
			principal: &authport.Principal{AdminUserID: 94, Role: authport.RoleSales, StaffID: &staff42},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "admin cannot use owner staff customer authorization",
			principal: &authport.Principal{AdminUserID: 94, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: staff42,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales staff differs from authorization owner",
			principal: &authport.Principal{AdminUserID: 94, Role: authport.RoleSales, StaffID: &staff43},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomersRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: staff42,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "invalid principal identifier",
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
			t.Parallel()

			application := &customerListApplicationStub{result: customerListValidResult()}
			response := serveCustomerList(newCustomerListHandler(t, application), customerListRequest(t, testCase.principal, testCase.authorization), generated.ListCustomersParams{})
			assertCustomerListError(t, response, testCase.wantStatus, testCase.wantCode)
			if application.calls != 0 {
				t.Fatalf("application calls = %d, want 0", application.calls)
			}
		})
	}
}

func TestCustomerListHandlerClassifiesApplicationErrorsWithoutLeakingCauses(t *testing.T) {
	t.Parallel()

	const secret = "postgres://private-user:private-password@127.0.0.1/aicrm"
	for _, testCase := range []struct {
		name       string
		params     generated.ListCustomersParams
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{
			name:       "invalid cursor",
			params:     generated.ListCustomersParams{Cursor: customerListPtr("bad-cursor")},
			err:        errors.Join(contactapp.ErrInvalidCustomerListQuery, errors.New(secret)),
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeCursorInvalid,
		},
		{
			name:       "other malformed input",
			err:        errors.Join(contactapp.ErrInvalidCustomerListQuery, errors.New(secret)),
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "unavailable",
			err:        errors.Join(contactapp.ErrCustomerListUnavailable, errors.New(secret)),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			application := &customerListApplicationStub{err: testCase.err}
			request := customerListRequest(t,
				&authport.Principal{AdminUserID: 95, Role: authport.RoleAdmin},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)
			response := serveCustomerList(newCustomerListHandler(t, application), request, testCase.params)
			assertCustomerListError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerListResponseDoesNotContain(t, response, secret, "customer list unavailable", "invalid customer list query")
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerListHandlerMapsFullResponseAndPreservesExtraNumbers(t *testing.T) {
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
	watermark := time.Date(2026, time.August, 5, 13, 12, 11, 523456789, zone)
	nextCursor := "next.cursor-v1_A-9"
	result := contactapp.CustomerListResult{
		Items: []contactapp.CustomerRecord{{
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
		}},
		NextCursor:      &nextCursor,
		Total:           contactapp.CustomerListExactTotalCap,
		TotalIsEstimate: true,
		Watermark:       watermark,
	}
	application := &customerListApplicationStub{result: result}
	request := customerListRequest(t,
		&authport.Principal{AdminUserID: 96, Role: authport.RoleAdmin},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)

	response := serveCustomerList(newCustomerListHandler(t, application), request, generated.ListCustomersParams{})
	assertCustomerListSuccess(t, response)
	var body generated.CustomerListResponse
	decodeCustomerListJSON(t, response, &body)
	if len(body.Items) != 1 {
		t.Fatalf("items = %#v, want one customer", body.Items)
	}
	item := body.Items[0]
	if item.Id != 71 || item.Name != "Ada Lovelace" || !item.IsDeleted {
		t.Fatalf("mapped customer = %#v, want all scalar fields", item)
	}
	if item.AvatarUrl == nil || *item.AvatarUrl != avatarURL || item.Gender == nil || *item.Gender != int32(gender) ||
		item.StageId == nil || *item.StageId != stageID || item.OwnerStaffId == nil || *item.OwnerStaffId != ownerStaffID || item.ChannelId == nil || *item.ChannelId != channelID {
		t.Fatalf("mapped optional customer fields = %#v", item)
	}
	if item.AddedAt == nil || !item.AddedAt.Equal(addedAt) || item.LastInteractAt == nil || !item.LastInteractAt.Equal(lastInteractAt) ||
		!item.CreatedAt.Equal(createdAt) || !item.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("mapped customer times = %#v", item)
	}
	if body.NextCursor == nil || *body.NextCursor != nextCursor || body.Total != contactapp.CustomerListExactTotalCap || !body.TotalIsEstimate || !body.Watermark.Equal(watermark) {
		t.Fatalf("mapped page = %#v", body)
	}

	var raw struct {
		Items []struct {
			Extra map[string]any `json:"extra"`
		} `json:"items"`
	}
	decoder := json.NewDecoder(strings.NewReader(response.Body.String()))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		t.Fatalf("decode JSON with numbers = %v", err)
	}
	if len(raw.Items) != 1 || raw.Items[0].Extra == nil {
		t.Fatalf("extra = %#v, want JSON object", raw.Items)
	}
	large, ok := raw.Items[0].Extra["large"].(json.Number)
	if !ok || large.String() != "9007199254740993.125" {
		t.Fatalf("large JSON number = %#v, want exact 9007199254740993.125", raw.Items[0].Extra["large"])
	}
	if nested, ok := raw.Items[0].Extra["nested"].(map[string]any); !ok || nested["rank"] != json.Number("2") || raw.Items[0].Extra["active"] != true {
		t.Fatalf("nested extra = %#v, want object and preserved values", raw.Items[0].Extra)
	}
}

func TestCustomerListHandlerEncodesNilItemsAsEmptyArray(t *testing.T) {
	t.Parallel()

	result := customerListValidResult()
	result.Items = nil
	application := &customerListApplicationStub{result: result}
	request := customerListRequest(t,
		&authport.Principal{AdminUserID: 97, Role: authport.RoleOps},
		&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
	)

	response := serveCustomerList(newCustomerListHandler(t, application), request, generated.ListCustomersParams{})
	assertCustomerListSuccess(t, response)
	var body struct {
		Items json.RawMessage `json:"items"`
	}
	decodeCustomerListJSON(t, response, &body)
	if string(body.Items) != "[]" {
		t.Fatalf("items JSON = %s, want nonnil empty array []", body.Items)
	}
}

func TestCustomerListHandlerRejectsInvalidApplicationResponses(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		result func() contactapp.CustomerListResult
	}{
		{
			name: "zero customer id",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].ID = 0
				return result
			},
		},
		{
			name: "zero watermark",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Watermark = time.Time{}
				return result
			},
		},
		{
			name: "zero created at",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].CreatedAt = time.Time{}
				return result
			},
		},
		{
			name: "zero updated at",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].UpdatedAt = time.Time{}
				return result
			},
		},
		{
			name: "zero optional added at",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				zero := time.Time{}
				result.Items[0].AddedAt = &zero
				return result
			},
		},
		{
			name: "zero optional last interact at",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				zero := time.Time{}
				result.Items[0].LastInteractAt = &zero
				return result
			},
		},
		{
			name: "extra is an array not an object",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].Extra = json.RawMessage(`[]`)
				return result
			},
		},
		{
			name: "extra is null not an object",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].Extra = json.RawMessage(`null`)
				return result
			},
		},
		{
			name: "extra is malformed",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].Extra = json.RawMessage(`{"broken":`)
				return result
			},
		},
		{
			name: "extra contains external identity",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				result.Items[0].Extra = json.RawMessage(`{"nested":{"wecomTagId":"identity-secret"}}`)
				return result
			},
		},
		{
			name: "invalid avatar URL",
			result: func() contactapp.CustomerListResult {
				result := customerListResultWithOneCustomer()
				invalidURL := "https://[::1"
				result.Items[0].AvatarURL = &invalidURL
				return result
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			application := &customerListApplicationStub{result: testCase.result()}
			request := customerListRequest(t,
				&authport.Principal{AdminUserID: 98, Role: authport.RoleAdmin},
				&authport.Authorization{Capability: authport.CapabilityCustomersRead, Scope: authport.ScopeGlobal},
			)
			response := serveCustomerList(newCustomerListHandler(t, application), request, generated.ListCustomersParams{})
			assertCustomerListError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerListHandlerRejectsNilRequestContext(t *testing.T) {
	t.Parallel()

	application := &customerListApplicationStub{result: customerListValidResult()}
	handler := newCustomerListHandler(t, application)
	response := httptest.NewRecorder()

	handler.ListCustomers(response, nil, generated.ListCustomersParams{})
	assertCustomerListError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
	if application.calls != 0 {
		t.Fatalf("application calls = %d, want 0", application.calls)
	}
}

type customerListApplicationStub struct {
	result   contactapp.CustomerListResult
	err      error
	list     func(context.Context, contactapp.CustomerListInput) (contactapp.CustomerListResult, error)
	calls    int
	contexts []context.Context
	inputs   []contactapp.CustomerListInput
}

var _ customerListApplication = (*customerListApplicationStub)(nil)

func (stub *customerListApplicationStub) List(ctx context.Context, input contactapp.CustomerListInput) (contactapp.CustomerListResult, error) {
	stub.calls++
	stub.contexts = append(stub.contexts, ctx)
	stub.inputs = append(stub.inputs, input)
	if stub.list != nil {
		return stub.list(ctx, input)
	}
	return stub.result, stub.err
}

func newCustomerListHandler(t *testing.T, application customerListApplication) *CustomerListHandler {
	t.Helper()
	handler, err := NewCustomerListHandler(application)
	if err != nil {
		t.Fatalf("NewCustomerListHandler() error = %v", err)
	}
	return handler
}

func customerListRequest(t *testing.T, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	t.Helper()

	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "customer-list-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization(%#v) error = %v", *authorization, err)
		}
	}
	return httptest.NewRequest(http.MethodGet, "/api/v1/customers", nil).WithContext(ctx)
}

func serveCustomerList(handler *CustomerListHandler, request *http.Request, params generated.ListCustomersParams) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ListCustomers(response, request, params)
	return response
}

func assertCustomerListSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertCustomerListResponseHeaders(t, response)
}

func assertCustomerListError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertCustomerListResponseHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	decodeCustomerListJSON(t, response, &body)
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v, want code %q with stable message and request_id", body, wantCode)
	}
}

func assertCustomerListResponseHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertCustomerListResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, values ...string) {
	t.Helper()
	for _, value := range values {
		if value != "" && strings.Contains(response.Body.String(), value) {
			t.Fatalf("response leaked %q: %s", value, response.Body.String())
		}
	}
}

func decodeCustomerListJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func customerListValidResult() contactapp.CustomerListResult {
	return contactapp.CustomerListResult{
		Items:     []contactapp.CustomerRecord{},
		Watermark: time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC),
	}
}

func customerListResultWithOneCustomer() contactapp.CustomerListResult {
	avatarURL := "https://cdn.example.test/avatar.png"
	createdAt := time.Date(2026, time.August, 11, 8, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC)
	result := customerListValidResult()
	result.Items = []contactapp.CustomerRecord{{
		ID:        contactport.CustomerID(1),
		Name:      "valid customer",
		AvatarURL: &avatarURL,
		Extra:     json.RawMessage(`{"source":"test"}`),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}}
	return result
}

func customerListEqualInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func customerListPtr[T any](value T) *T {
	return &value
}

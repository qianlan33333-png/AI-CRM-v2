package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	generated "github.com/qianlan33333-png/AI-CRM-v2/internal/api/candidate/generated"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

func TestNewCustomerEventHandlerRejectsNilApplications(t *testing.T) {
	if handler, err := NewCustomerEventHandler(nil); err == nil || handler != nil {
		t.Fatalf("NewCustomerEventHandler(nil) = %#v, %v; want nil and fail-closed error", handler, err)
	}

	var typedNil *customerEventApplicationStub
	if handler, err := NewCustomerEventHandler(typedNil); err == nil || handler != nil {
		t.Fatalf("NewCustomerEventHandler(typed nil) = %#v, %v; want nil and fail-closed error", handler, err)
	}
}

func TestCustomerEventHandlerMapsGeneratedParametersAndScopesExactly(t *testing.T) {
	ownerStaffID := int64(71)
	cursor := generated.Cursor("opaque.cursor-v1_A-9")
	limit := generated.Limit(87)
	params := generated.ListCustomerEventsParams{Cursor: &cursor, Limit: &limit}
	callers := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{
			name:      "admin global",
			principal: authport.Principal{AdminUserID: 101, Role: authport.RoleAdmin},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomerEventsRead,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name:      "ops global",
			principal: authport.Principal{AdminUserID: 102, Role: authport.RoleOps},
			authorization: authport.Authorization{
				Capability: authport.CapabilityCustomerEventsRead,
				Scope:      authport.ScopeGlobal,
			},
		},
		{
			name:      "sales exact owner",
			principal: authport.Principal{AdminUserID: 103, Role: authport.RoleSales, StaffID: &ownerStaffID},
			authorization: authport.Authorization{
				Capability:   authport.CapabilityCustomerEventsRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: ownerStaffID,
			},
			wantOwner: &ownerStaffID,
		},
	}

	for _, caller := range callers {
		caller := caller
		t.Run(caller.name, func(t *testing.T) {
			application := &customerEventApplicationStub{result: customerEventValidResult()}
			response := serveCustomerEvent(
				newCustomerEventHandler(t, application),
				customerEventRequest(t, &caller.principal, &caller.authorization),
				41,
				params,
			)
			assertCustomerEventSuccess(t, response)
			if application.calls != 1 || len(application.inputs) != 1 || len(application.contexts) != 1 {
				t.Fatalf("application calls/inputs/contexts = %d/%d/%d, want 1/1/1", application.calls, len(application.inputs), len(application.contexts))
			}
			got := application.inputs[0]
			if got.CustomerID != contactport.CustomerID(41) || got.Cursor != string(cursor) || got.Limit != int32(limit) ||
				!customerEventEqualInt64Pointer(got.OwnerStaffID, caller.wantOwner) {
				t.Fatalf("application input = %#v, want generated customer/cursor/limit and exact scope", got)
			}
		})
	}
}

func TestCustomerEventHandlerFailsClosedForAuthorizationAndPrincipalMismatches(t *testing.T) {
	salesOwner := int64(83)
	differentOwner := int64(84)
	tests := []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{
			name:       "missing authorization",
			principal:  &authport.Principal{AdminUserID: 301, Role: authport.RoleAdmin},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name: "missing authenticated principal",
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomerEventsRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
		{
			name:      "wrong capability",
			principal: &authport.Principal{AdminUserID: 302, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomersRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales cannot claim global scope",
			principal: &authport.Principal{AdminUserID: 303, Role: authport.RoleSales, StaffID: &salesOwner},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomerEventsRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "admin cannot claim owner scope",
			principal: &authport.Principal{AdminUserID: 304, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomerEventsRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: salesOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales owner mismatch",
			principal: &authport.Principal{AdminUserID: 305, Role: authport.RoleSales, StaffID: &salesOwner},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomerEventsRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: differentOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "sales lacks staff identity",
			principal: &authport.Principal{AdminUserID: 306, Role: authport.RoleSales},
			authorization: &authport.Authorization{
				Capability:   authport.CapabilityCustomerEventsRead,
				Scope:        authport.ScopeOwnerStaff,
				OwnerStaffID: salesOwner,
			},
			wantStatus: http.StatusForbidden,
			wantCode:   platformhttp.CodeUnauthorized,
		},
		{
			name:      "invalid authenticated principal",
			principal: &authport.Principal{AdminUserID: 0, Role: authport.RoleAdmin},
			authorization: &authport.Authorization{
				Capability: authport.CapabilityCustomerEventsRead,
				Scope:      authport.ScopeGlobal,
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   platformhttp.CodeUnauthenticated,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerEventApplicationStub{result: customerEventValidResult()}
			response := serveCustomerEvent(
				newCustomerEventHandler(t, application),
				customerEventRequest(t, testCase.principal, testCase.authorization),
				41,
				generated.ListCustomerEventsParams{},
			)
			assertCustomerEventError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerEventNoCalls(t, application)
		})
	}
}

func TestCustomerEventHandlerRejectsInvalidPathLimitAndCursor(t *testing.T) {
	principal := authport.Principal{AdminUserID: 401, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	invalidCursor := generated.Cursor("not-a-valid-cursor")
	emptyCursor := generated.Cursor("")
	zeroLimit := generated.Limit(0)
	tooLargeLimit := generated.Limit(int(contactapp.CustomerListMaximumLimit) + 1)
	tests := []struct {
		name       string
		customerID generated.CustomerID
		params     generated.ListCustomerEventsParams
		result     contactapp.CustomerEventResult
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{
			name:       "zero customer path",
			customerID: 0,
			wantStatus: http.StatusNotFound,
			wantCode:   platformhttp.CodeNotFound,
		},
		{
			name:       "negative customer path",
			customerID: -9,
			wantStatus: http.StatusNotFound,
			wantCode:   platformhttp.CodeNotFound,
		},
		{
			name:       "zero limit",
			customerID: 41,
			params:     generated.ListCustomerEventsParams{Limit: &zeroLimit},
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "limit over maximum",
			customerID: 41,
			params:     generated.ListCustomerEventsParams{Limit: &tooLargeLimit},
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
		},
		{
			name:       "explicit empty cursor",
			customerID: 41,
			params:     generated.ListCustomerEventsParams{Cursor: &emptyCursor},
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeCursorInvalid,
		},
		{
			name:       "invalid opaque cursor from application",
			customerID: 41,
			params:     generated.ListCustomerEventsParams{Cursor: &invalidCursor},
			err:        errors.Join(contactapp.ErrInvalidCustomerEventQuery, errors.New("cursor source is rejected")),
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeCursorInvalid,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerEventApplicationStub{result: testCase.result, err: testCase.err}
			response := serveCustomerEvent(
				newCustomerEventHandler(t, application),
				customerEventRequest(t, &principal, &authorization),
				testCase.customerID,
				testCase.params,
			)
			assertCustomerEventError(t, response, testCase.wantStatus, testCase.wantCode)
			if testCase.customerID <= 0 || testCase.params.Limit != nil ||
				(testCase.params.Cursor != nil && *testCase.params.Cursor == "") {
				assertCustomerEventNoCalls(t, application)
				return
			}
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerEventHandlerClassifiesApplicationErrorsWithoutLeakingCauses(t *testing.T) {
	principal := authport.Principal{AdminUserID: 501, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	cursor := generated.Cursor("opaque.cursor-v1_A-9")
	tests := []struct {
		name       string
		params     generated.ListCustomerEventsParams
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
		hidden     string
	}{
		{
			name:       "not found is stable",
			err:        errors.Join(contactapp.ErrCustomerNotFound, errors.New("contact lookup detail must not escape")),
			wantStatus: http.StatusNotFound,
			wantCode:   platformhttp.CodeNotFound,
			hidden:     "contact lookup detail must not escape",
		},
		{
			name:       "invalid query without cursor is malformed",
			err:        errors.Join(contactapp.ErrInvalidCustomerEventQuery, errors.New("raw query must not escape")),
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeMalformedRequest,
			hidden:     "raw query must not escape",
		},
		{
			name:       "invalid query with cursor is cursor invalid",
			params:     generated.ListCustomerEventsParams{Cursor: &cursor},
			err:        errors.Join(contactapp.ErrInvalidCustomerEventQuery, errors.New("cursor detail must not escape")),
			wantStatus: http.StatusBadRequest,
			wantCode:   platformhttp.CodeCursorInvalid,
			hidden:     "cursor detail must not escape",
		},
		{
			name:       "dependency error is stable",
			err:        errors.New("database endpoint and query detail must not escape"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   platformhttp.CodeDependencyUnavailable,
			hidden:     "database endpoint and query detail must not escape",
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerEventApplicationStub{err: testCase.err}
			response := serveCustomerEvent(
				newCustomerEventHandler(t, application),
				customerEventRequest(t, &principal, &authorization),
				41,
				testCase.params,
			)
			assertCustomerEventError(t, response, testCase.wantStatus, testCase.wantCode)
			assertCustomerEventResponseDoesNotContain(t, response, testCase.hidden)
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerEventHandlerPreservesPayloadNumbersDeepStructureAndUTC(t *testing.T) {
	principal := authport.Principal{AdminUserID: 601, Role: authport.RoleOps}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	nextCursor := "opaque-next-cursor"
	occurredAt := time.Date(2026, time.August, 12, 16, 30, 1, 123456789, time.FixedZone("CST", 8*60*60))
	application := &customerEventApplicationStub{result: contactapp.CustomerEventResult{
		Items: []contactapp.CustomerEventRecord{{
			ID:         91,
			CustomerID: contactport.CustomerID(41),
			EventType:  "customer.stage_changed",
			Payload:    json.RawMessage(`{"large":9007199254740993.125,"nested":{"rank":2,"deep":{"exact":9007199254740993}},"active":true}`),
			Actor:      "admin:73",
			OccurredAt: occurredAt,
		}},
		NextCursor: &nextCursor,
	}}

	response := serveCustomerEvent(
		newCustomerEventHandler(t, application),
		customerEventRequest(t, &principal, &authorization),
		41,
		generated.ListCustomerEventsParams{},
	)
	assertCustomerEventSuccess(t, response)
	if application.calls != 1 {
		t.Fatalf("application calls = %d, want 1", application.calls)
	}

	var body struct {
		Items []struct {
			ID         int64          `json:"id"`
			CustomerID int64          `json:"customer_id"`
			EventType  string         `json:"event_type"`
			Payload    map[string]any `json:"payload"`
			Actor      string         `json:"actor"`
			OccurredAt time.Time      `json:"occurred_at"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	decodeCustomerEventJSONUseNumber(t, response, &body)
	if len(body.Items) != 1 || body.NextCursor == nil || *body.NextCursor != nextCursor {
		t.Fatalf("response items/next cursor = %#v/%v, want one item and %q", body.Items, body.NextCursor, nextCursor)
	}
	item := body.Items[0]
	if item.ID != 91 || item.CustomerID != 41 || item.EventType != "customer.stage_changed" || item.Actor != "admin:73" {
		t.Fatalf("response item fields = %#v, want exact generated event", item)
	}
	if item.OccurredAt.Location() != time.UTC || !item.OccurredAt.Equal(occurredAt.UTC()) {
		t.Fatalf("occurred_at = %s (%s), want UTC %s", item.OccurredAt, item.OccurredAt.Location(), occurredAt.UTC())
	}
	large, ok := item.Payload["large"].(json.Number)
	if !ok || large.String() != "9007199254740993.125" {
		t.Fatalf("large payload number = %#v, want exact JSON number", item.Payload["large"])
	}
	nested, ok := item.Payload["nested"].(map[string]any)
	if !ok || nested["rank"] != json.Number("2") || item.Payload["active"] != true {
		t.Fatalf("nested payload = %#v, want exact nested object", item.Payload)
	}
	deep, ok := nested["deep"].(map[string]any)
	if !ok || deep["exact"] != json.Number("9007199254740993") {
		t.Fatalf("deep payload = %#v, want lossless deep JSON number", nested["deep"])
	}
}

func TestCustomerEventHandlerEncodesEmptyItemsAsArray(t *testing.T) {
	principal := authport.Principal{AdminUserID: 701, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	application := &customerEventApplicationStub{result: customerEventValidResult()}
	response := serveCustomerEvent(
		newCustomerEventHandler(t, application),
		customerEventRequest(t, &principal, &authorization),
		41,
		generated.ListCustomerEventsParams{},
	)
	assertCustomerEventSuccess(t, response)
	var body struct {
		Items json.RawMessage `json:"items"`
	}
	decodeCustomerEventJSON(t, response, &body)
	if string(body.Items) != "[]" {
		t.Fatalf("items JSON = %s, want nonnil empty array []", body.Items)
	}
}

func TestCustomerEventHandlerRejectsInvalidApplicationResults(t *testing.T) {
	principal := authport.Principal{AdminUserID: 801, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	tests := []struct {
		name   string
		result func() contactapp.CustomerEventResult
		hidden string
	}{
		{
			name: "nil items",
			result: func() contactapp.CustomerEventResult {
				return contactapp.CustomerEventResult{Items: nil}
			},
		},
		{
			name: "empty next cursor",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				next := ""
				result.NextCursor = &next
				return result
			},
		},
		{
			name: "next cursor without an item",
			result: func() contactapp.CustomerEventResult {
				result := customerEventValidResult()
				next := "opaque-next"
				result.NextCursor = &next
				return result
			},
		},
		{
			name: "zero event id",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].ID = 0
				return result
			},
		},
		{
			name: "zero event customer",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].CustomerID = 0
				return result
			},
		},
		{
			name: "cross customer event",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].CustomerID = 99
				return result
			},
		},
		{
			name: "empty event type",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].EventType = ""
				return result
			},
		},
		{
			name: "empty actor",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].Actor = ""
				return result
			},
		},
		{
			name: "zero occurred at",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].OccurredAt = time.Time{}
				return result
			},
		},
		{
			name: "payload array",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].Payload = json.RawMessage(`["hidden-event-payload"]`)
				return result
			},
			hidden: "hidden-event-payload",
		},
		{
			name: "payload with trailing value",
			result: func() contactapp.CustomerEventResult {
				result := customerEventResultWithOneItem()
				result.Items[0].Payload = json.RawMessage(`{"hidden_event":"must_not_escape"} true`)
				return result
			},
			hidden: "must_not_escape",
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerEventApplicationStub{result: testCase.result()}
			response := serveCustomerEvent(
				newCustomerEventHandler(t, application),
				customerEventRequest(t, &principal, &authorization),
				41,
				generated.ListCustomerEventsParams{},
			)
			assertCustomerEventError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
			assertCustomerEventResponseDoesNotContain(t, response, testCase.hidden)
			if application.calls != 1 {
				t.Fatalf("application calls = %d, want 1", application.calls)
			}
		})
	}
}

func TestCustomerEventHandlerRejectsNilRequestBeforeApplication(t *testing.T) {
	application := &customerEventApplicationStub{result: customerEventValidResult()}
	response := httptest.NewRecorder()
	newCustomerEventHandler(t, application).ListCustomerEvents(response, nil, 41, generated.ListCustomerEventsParams{})
	assertCustomerEventError(t, response, http.StatusServiceUnavailable, platformhttp.CodeDependencyUnavailable)
	assertCustomerEventNoCalls(t, application)
}

type customerEventApplicationStub struct {
	result   contactapp.CustomerEventResult
	err      error
	list     func(context.Context, contactapp.CustomerEventInput) (contactapp.CustomerEventResult, error)
	calls    int
	contexts []context.Context
	inputs   []contactapp.CustomerEventInput
}

var _ customerEventApplication = (*customerEventApplicationStub)(nil)

func (stub *customerEventApplicationStub) List(ctx context.Context, input contactapp.CustomerEventInput) (contactapp.CustomerEventResult, error) {
	stub.calls++
	stub.contexts = append(stub.contexts, ctx)
	stub.inputs = append(stub.inputs, input)
	if stub.list != nil {
		return stub.list(ctx, input)
	}
	return stub.result, stub.err
}

func newCustomerEventHandler(t *testing.T, application customerEventApplication) *CustomerEventHandler {
	t.Helper()
	handler, err := NewCustomerEventHandler(application)
	if err != nil {
		t.Fatalf("NewCustomerEventHandler() error = %v", err)
	}
	return handler
}

func customerEventRequest(t *testing.T, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	t.Helper()
	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "customer-event-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization(%#v) error = %v", *authorization, err)
		}
	}
	return httptest.NewRequest(http.MethodGet, "/api/v1/customers/41/events", nil).WithContext(ctx)
}

func serveCustomerEvent(
	handler *CustomerEventHandler,
	request *http.Request,
	customerID generated.CustomerID,
	params generated.ListCustomerEventsParams,
) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ListCustomerEvents(response, request, customerID, params)
	return response
}

func assertCustomerEventSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	assertCustomerEventResponseHeaders(t, response)
}

func assertCustomerEventError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertCustomerEventResponseHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	decodeCustomerEventJSON(t, response, &body)
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v, want code %q with stable message and request_id", body, wantCode)
	}
}

func assertCustomerEventResponseHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func assertCustomerEventResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, value string) {
	t.Helper()
	if value != "" && strings.Contains(response.Body.String(), value) {
		t.Fatalf("response leaked %q: %s", value, response.Body.String())
	}
}

func assertCustomerEventNoCalls(t *testing.T, application *customerEventApplicationStub) {
	t.Helper()
	if application.calls != 0 || len(application.inputs) != 0 || len(application.contexts) != 0 {
		t.Fatalf("application calls/inputs/contexts = %d/%d/%d, want 0/0/0", application.calls, len(application.inputs), len(application.contexts))
	}
}

func decodeCustomerEventJSON(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func decodeCustomerEventJSONUseNumber(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(response.Body.String()))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("response JSON error = %v; body=%q", err, response.Body.String())
	}
}

func customerEventValidResult() contactapp.CustomerEventResult {
	return contactapp.CustomerEventResult{Items: []contactapp.CustomerEventRecord{}}
}

func customerEventResultWithOneItem() contactapp.CustomerEventResult {
	return contactapp.CustomerEventResult{Items: []contactapp.CustomerEventRecord{{
		ID:         91,
		CustomerID: contactport.CustomerID(41),
		EventType:  "customer.stage_changed",
		Payload:    json.RawMessage(`{"source":"test"}`),
		Actor:      "admin:73",
		OccurredAt: time.Date(2026, time.August, 12, 8, 30, 0, 0, time.UTC),
	}}}
}

func customerEventEqualInt64Pointer(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

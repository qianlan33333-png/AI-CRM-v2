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
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerContextApplicationStub struct {
	result customer360port.CustomerContext
	err    error
	calls  int
	inputs []customer360port.CustomerContextQuery
}

func (stub *customerContextApplicationStub) ReadCustomerContext(_ context.Context, input customer360port.CustomerContextQuery) (customer360port.CustomerContext, error) {
	stub.calls++
	cloned := input
	cloned.OwnerStaffID = customerContextTestCloneInt64(input.OwnerStaffID)
	stub.inputs = append(stub.inputs, cloned)
	return stub.result, stub.err
}

var _ customerContextApplication = (*customerContextApplicationStub)(nil)

func customerContextTestCloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func customerContextTestResult(customerID contactport.CustomerID) customer360port.CustomerContext {
	cursor := "opaque-next-cursor"
	group := "CRM"
	return customer360port.CustomerContext{
		Customer: customer360port.Customer{
			ID: customerID, Name: "Ada", StageID: customerContextTestInt64(17), OwnerStaffID: customerContextTestInt64(71),
			ChannelID: customerContextTestInt64(23), AddedAt: customerContextTestTimePtr(1), LastInteractAt: customerContextTestTimePtr(2),
		},
		Tags:               []customer360port.Tag{{ID: 31, GroupID: customerContextTestInt64(3), GroupName: &group, GroupSortOrder: 1, Name: "important", SortOrder: 2}},
		Timeline:           []customer360port.TimelineEntry{{ID: 41, EventType: "customer.updated", OccurredAt: customerContextTestTime(3)}},
		TimelineNextCursor: &cursor,
		Chat: customer360port.ChatSummary{
			LocalArchiveAvailable: true, Total: 1,
			Items: []customer360port.ChatEntry{{ChatType: "private", MessageType: "text", SentAt: customerContextTestTime(4)}},
		},
	}
}

func customerContextTestInt64(value int64) *int64 { return &value }

func customerContextTestTime(minute int) time.Time {
	return time.Date(2026, time.August, 20, 13, minute, 0, 0, time.FixedZone("CST", 8*60*60))
}

func customerContextTestTimePtr(minute int) *time.Time {
	value := customerContextTestTime(minute)
	return &value
}

func TestCustomerContextHandlerMapsScopedGeneratedInputAndSafeResponse(t *testing.T) {
	owner := int64(71)
	cursor := generated.Cursor("opaque-next-cursor")
	limit := generated.Limit(87)
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{name: "admin global", principal: authport.Principal{AdminUserID: 101, Role: authport.RoleAdmin}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}},
		{name: "ops global", principal: authport.Principal{AdminUserID: 102, Role: authport.RoleOps}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}},
		{name: "sales exact owner", principal: authport.Principal{AdminUserID: 103, Role: authport.RoleSales, StaffID: &owner}, authorization: authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner}, wantOwner: &owner},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerContextApplicationStub{result: customerContextTestResult(41)}
			response := serveCustomerContext(newCustomerContextHandler(t, application), customerContextRequest(t, &testCase.principal, &testCase.authorization), 41, generated.GetCustomerContextParams{Cursor: &cursor, Limit: &limit})
			assertCustomerContextSuccess(t, response)
			if application.calls != 1 || len(application.inputs) != 1 {
				t.Fatalf("application calls/inputs = %d/%d", application.calls, len(application.inputs))
			}
			input := application.inputs[0]
			if input.CustomerID != 41 || input.TimelineCursor != string(cursor) || input.TimelineLimit != int32(limit) || !reflect.DeepEqual(input.OwnerStaffID, testCase.wantOwner) {
				t.Fatalf("application input = %#v", input)
			}
			var body struct {
				Customer           map[string]any   `json:"customer"`
				Tags               []map[string]any `json:"tags"`
				Timeline           []map[string]any `json:"timeline"`
				TimelineNextCursor *string          `json:"timeline_next_cursor"`
				Chat               struct {
					Items                 []map[string]any `json:"items"`
					LocalArchiveAvailable bool             `json:"local_archive_available"`
					Total                 int64            `json:"total"`
				} `json:"chat"`
				NonAtomicSnapshot        bool `json:"non_atomic_snapshot"`
				RealExternalCallExecuted bool `json:"real_external_call_executed"`
			}
			decodeCustomerContextJSON(t, response, &body)
			if body.Customer["id"] != float64(41) || len(body.Tags) != 1 || len(body.Timeline) != 1 || body.Timeline[0]["event_type"] != "customer.updated" ||
				body.TimelineNextCursor == nil || *body.TimelineNextCursor != "opaque-next-cursor" || !body.Chat.LocalArchiveAvailable || len(body.Chat.Items) != 1 || body.Chat.Items[0]["message_type"] != "text" ||
				!body.NonAtomicSnapshot || body.RealExternalCallExecuted {
				t.Fatalf("safe response = %#v", body)
			}
			encoded := response.Body.String()
			for _, forbidden := range []string{"payload", "actor", "identity", "content", "media", "provider", "receipt", "external_user"} {
				if strings.Contains(strings.ToLower(encoded), forbidden) {
					t.Fatalf("safe response leaked %q: %s", forbidden, encoded)
				}
			}
		})
	}
}

func TestCustomerContextHandlerNormalizesNilSlicesAndUnavailableArchive(t *testing.T) {
	result := customerContextTestResult(41)
	result.Tags, result.Timeline = nil, nil
	result.TimelineNextCursor = nil
	result.Chat = customer360port.ChatSummary{LocalArchiveAvailable: false}
	application := &customerContextApplicationStub{result: result}
	principal := authport.Principal{AdminUserID: 201, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	response := serveCustomerContext(newCustomerContextHandler(t, application), customerContextRequest(t, &principal, &authorization), 41, generated.GetCustomerContextParams{})
	assertCustomerContextSuccess(t, response)
	var body struct {
		Tags     json.RawMessage `json:"tags"`
		Timeline json.RawMessage `json:"timeline"`
		Chat     struct {
			Items     json.RawMessage `json:"items"`
			Available bool            `json:"local_archive_available"`
			Total     int64           `json:"total"`
		} `json:"chat"`
	}
	decodeCustomerContextJSON(t, response, &body)
	if string(body.Tags) != "[]" || string(body.Timeline) != "[]" || string(body.Chat.Items) != "[]" || body.Chat.Available || body.Chat.Total != 0 {
		t.Fatalf("normalized arrays/chat = tags:%s timeline:%s chat:%#v", body.Tags, body.Timeline, body.Chat)
	}
}

func TestCustomerContextHandlerFailsClosedForAuthorizationAndInvalidInput(t *testing.T) {
	owner, differentOwner := int64(71), int64(72)
	validPrincipal := authport.Principal{AdminUserID: 301, Role: authport.RoleAdmin}
	validAuthorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	emptyCursor := generated.Cursor("")
	longCursor := generated.Cursor(strings.Repeat("a", 513))
	zeroLimit := generated.Limit(0)
	tooLarge := generated.Limit(201)
	tests := []struct {
		name          string
		principal     *authport.Principal
		authorization *authport.Authorization
		customerID    generated.CustomerID
		params        generated.GetCustomerContextParams
		wantStatus    int
		wantCode      platformhttp.ErrorCode
	}{
		{name: "missing authorization", principal: &validPrincipal, customerID: 41, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized},
		{name: "missing principal", authorization: &validAuthorization, customerID: 41, wantStatus: http.StatusUnauthorized, wantCode: platformhttp.CodeUnauthenticated},
		{name: "sales cannot claim global", principal: &authport.Principal{AdminUserID: 302, Role: authport.RoleSales, StaffID: &owner}, authorization: &validAuthorization, customerID: 41, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized},
		{name: "sales owner mismatch", principal: &authport.Principal{AdminUserID: 303, Role: authport.RoleSales, StaffID: &owner}, authorization: &authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: differentOwner}, customerID: 41, wantStatus: http.StatusForbidden, wantCode: platformhttp.CodeUnauthorized},
		{name: "zero customer", principal: &validPrincipal, authorization: &validAuthorization, customerID: 0, wantStatus: http.StatusNotFound, wantCode: platformhttp.CodeNotFound},
		{name: "empty cursor", principal: &validPrincipal, authorization: &validAuthorization, customerID: 41, params: generated.GetCustomerContextParams{Cursor: &emptyCursor}, wantStatus: http.StatusBadRequest, wantCode: platformhttp.CodeCursorInvalid},
		{name: "oversize cursor", principal: &validPrincipal, authorization: &validAuthorization, customerID: 41, params: generated.GetCustomerContextParams{Cursor: &longCursor}, wantStatus: http.StatusBadRequest, wantCode: platformhttp.CodeCursorInvalid},
		{name: "zero limit", principal: &validPrincipal, authorization: &validAuthorization, customerID: 41, params: generated.GetCustomerContextParams{Limit: &zeroLimit}, wantStatus: http.StatusBadRequest, wantCode: platformhttp.CodeMalformedRequest},
		{name: "limit over max", principal: &validPrincipal, authorization: &validAuthorization, customerID: 41, params: generated.GetCustomerContextParams{Limit: &tooLarge}, wantStatus: http.StatusBadRequest, wantCode: platformhttp.CodeMalformedRequest},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerContextApplicationStub{result: customerContextTestResult(41)}
			response := serveCustomerContext(newCustomerContextHandler(t, application), customerContextRequest(t, testCase.principal, testCase.authorization), testCase.customerID, testCase.params)
			assertCustomerContextError(t, response, testCase.wantStatus, testCase.wantCode)
			if application.calls != 0 {
				t.Fatalf("application calls = %d, want 0", application.calls)
			}
		})
	}
}

func TestCustomerContextHandlerMapsErrorsAndRejectsUnsafeOutputs(t *testing.T) {
	principal := authport.Principal{AdminUserID: 401, Role: authport.RoleOps}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	tests := []struct {
		name       string
		result     customer360port.CustomerContext
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
	}{
		{name: "not found", err: errors.Join(contactport.ErrCustomerReadNotFound, errors.New("customer lookup detail")), wantStatus: http.StatusNotFound, wantCode: platformhttp.CodeNotFound},
		{name: "unavailable", err: errors.Join(customer360port.ErrCustomerContextUnavailable, errors.New("database detail")), wantStatus: http.StatusServiceUnavailable, wantCode: platformhttp.CodeDependencyUnavailable},
		{name: "cross customer output", result: customerContextTestResult(42), wantStatus: http.StatusServiceUnavailable, wantCode: platformhttp.CodeDependencyUnavailable},
		{name: "chat data despite unavailable", result: func() customer360port.CustomerContext {
			result := customerContextTestResult(41)
			result.Chat.LocalArchiveAvailable = false
			return result
		}(), wantStatus: http.StatusServiceUnavailable, wantCode: platformhttp.CodeDependencyUnavailable},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerContextApplicationStub{result: testCase.result, err: testCase.err}
			response := serveCustomerContext(newCustomerContextHandler(t, application), customerContextRequest(t, &principal, &authorization), 41, generated.GetCustomerContextParams{})
			assertCustomerContextError(t, response, testCase.wantStatus, testCase.wantCode)
			for _, hidden := range []string{"customer lookup detail", "database detail", "invalid customer context"} {
				if strings.Contains(response.Body.String(), hidden) {
					t.Fatalf("error leaked %q: %s", hidden, response.Body.String())
				}
			}
		})
	}
}

func newCustomerContextHandler(t *testing.T, application customerContextApplication) *CustomerContextHandler {
	t.Helper()
	handler, err := NewCustomerContextHandler(application)
	if err != nil {
		t.Fatalf("NewCustomerContextHandler() error = %v", err)
	}
	return handler
}

func customerContextRequest(t *testing.T, principal *authport.Principal, authorization *authport.Authorization) *http.Request {
	t.Helper()
	ctx := context.Background()
	if principal != nil {
		ctx = authport.WithAuthenticatedSession(ctx, *principal, "customer-context-test-session")
	}
	if authorization != nil {
		var err error
		ctx, err = authport.WithAuthorization(ctx, *authorization)
		if err != nil {
			t.Fatalf("WithAuthorization() error = %v", err)
		}
	}
	return httptest.NewRequest(http.MethodGet, "/api/v1/customers/41/context", nil).WithContext(ctx)
}

func serveCustomerContext(handler *CustomerContextHandler, request *http.Request, customerID generated.CustomerID, params generated.GetCustomerContextParams) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.GetCustomerContext(response, request, customerID, params)
	return response
}

func assertCustomerContextSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	assertCustomerContextHeaders(t, response)
}

func assertCustomerContextError(t *testing.T, response *httptest.ResponseRecorder, wantStatus int, wantCode platformhttp.ErrorCode) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	assertCustomerContextHeaders(t, response)
	var body struct {
		Code      platformhttp.ErrorCode `json:"code"`
		Message   string                 `json:"message"`
		RequestID string                 `json:"request_id"`
	}
	decodeCustomerContextJSON(t, response, &body)
	if body.Code != wantCode || body.Message == "" || body.RequestID == "" {
		t.Fatalf("error body = %#v", body)
	}
}

func assertCustomerContextHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("headers = %#v", response.Header())
	}
}

func decodeCustomerContextJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v body=%s", err, response.Body.String())
	}
}

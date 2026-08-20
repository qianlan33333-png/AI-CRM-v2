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

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	platformhttp "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/http"
)

type customerChatActivityApplicationStub struct {
	result customer360port.CustomerChatActivityPage
	err    error
	inputs []customer360port.CustomerChatActivityQuery
}

func (stub *customerChatActivityApplicationStub) ListCustomerChatActivity(_ context.Context, input customer360port.CustomerChatActivityQuery) (customer360port.CustomerChatActivityPage, error) {
	cloned := input
	cloned.OwnerStaffID = customerContextTestCloneInt64(input.OwnerStaffID)
	stub.inputs = append(stub.inputs, cloned)
	return stub.result, stub.err
}

func customerChatActivityResult() customer360port.CustomerChatActivityPage {
	previous := "previous"
	return customer360port.CustomerChatActivityPage{
		CustomerID: 41, ChatType: "private", Total: 7, PreviousCursor: &previous,
		Items: []customer360port.CustomerChatActivityEntry{
			{ChatType: "private", MessageType: "text", SentAt: customerContextTestTime(4)},
			{ChatType: "private", MessageType: "image", SentAt: customerContextTestTime(3)},
		},
	}
}

func TestCustomerChatActivityHandlerAuthorizesAllExistingCustomerScopesAndReturnsZeroBodyProjection(t *testing.T) {
	owner := int64(71)
	tests := []struct {
		name          string
		principal     authport.Principal
		authorization authport.Authorization
		wantOwner     *int64
	}{
		{"admin", authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}, authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}, nil},
		{"ops", authport.Principal{AdminUserID: 2, Role: authport.RoleOps}, authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}, nil},
		{"sales owner", authport.Principal{AdminUserID: 3, Role: authport.RoleSales, StaffID: &owner}, authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeOwnerStaff, OwnerStaffID: owner}, &owner},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerChatActivityApplicationStub{result: customerChatActivityResult()}
			handler, err := NewCustomerChatActivityHandler(application)
			if err != nil {
				t.Fatal(err)
			}
			request := customerContextRequest(t, &testCase.principal, &testCase.authorization)
			response := httptest.NewRecorder()
			handler.GetCustomerChatActivity(response, request, 41, CustomerChatActivityQuery{ChatType: "private", Cursor: "opaque", CursorSupplied: true, Limit: 50, LimitSupplied: true})
			if response.Code != http.StatusOK || len(application.inputs) != 1 {
				t.Fatalf("status=%d body=%s inputs=%#v", response.Code, response.Body.String(), application.inputs)
			}
			input := application.inputs[0]
			if input.CustomerID != 41 || input.ChatType != "private" || input.Cursor != "opaque" || input.Limit != 50 || !reflect.DeepEqual(input.OwnerStaffID, testCase.wantOwner) {
				t.Fatalf("input=%#v", input)
			}
			var body map[string]any
			if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["customer_id"] != float64(41) || body["chat_type"] != "private" || body["non_atomic_snapshot"] != true || body["message_content_included"] != false ||
				body["identity_values_included"] != false || body["provider_receipts_included"] != false || body["real_external_call_executed"] != false {
				t.Fatalf("body=%#v", body)
			}
			encoded := strings.ToLower(response.Body.String())
			for _, forbidden := range []string{"content_masked", "external_userid", "sender", "receiver", "provider_id", "receipt_id", "source_message_id"} {
				if strings.Contains(encoded, forbidden) {
					t.Fatalf("leaked %q: %s", forbidden, encoded)
				}
			}
		})
	}
}

func TestCustomerChatActivityHandlerRejectsScopeInputAndUnsafeOutputsBeforeLeak(t *testing.T) {
	principal := authport.Principal{AdminUserID: 1, Role: authport.RoleAdmin}
	authorization := authport.Authorization{Capability: authport.CapabilityCustomerEventsRead, Scope: authport.ScopeGlobal}
	tests := []struct {
		name       string
		customerID contactport.CustomerID
		query      CustomerChatActivityQuery
		result     customer360port.CustomerChatActivityPage
		err        error
		wantStatus int
		wantCode   platformhttp.ErrorCode
		wantCalls  int
	}{
		{"zero customer", 0, CustomerChatActivityQuery{}, customer360port.CustomerChatActivityPage{}, nil, 400, platformhttp.CodeMalformedRequest, 0},
		{"invalid filter", 41, CustomerChatActivityQuery{ChatType: "room"}, customer360port.CustomerChatActivityPage{}, nil, 400, platformhttp.CodeMalformedRequest, 0},
		{"empty supplied cursor", 41, CustomerChatActivityQuery{CursorSupplied: true}, customer360port.CustomerChatActivityPage{}, nil, 400, platformhttp.CodeCursorInvalid, 0},
		{"zero supplied limit", 41, CustomerChatActivityQuery{LimitSupplied: true}, customer360port.CustomerChatActivityPage{}, nil, 400, platformhttp.CodeMalformedRequest, 0},
		{"not found", 41, CustomerChatActivityQuery{}, customer360port.CustomerChatActivityPage{}, contactport.ErrCustomerReadNotFound, 404, platformhttp.CodeNotFound, 1},
		{"unsafe filter drift", 41, CustomerChatActivityQuery{ChatType: "private"}, func() customer360port.CustomerChatActivityPage {
			value := customerChatActivityResult()
			value.ChatType = "group"
			return value
		}(), nil, 503, platformhttp.CodeDependencyUnavailable, 1},
		{"unsafe message text", 41, CustomerChatActivityQuery{ChatType: "private"}, func() customer360port.CustomerChatActivityPage {
			value := customerChatActivityResult()
			value.Items[0].MessageType = " text "
			return value
		}(), nil, 503, platformhttp.CodeDependencyUnavailable, 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			application := &customerChatActivityApplicationStub{result: testCase.result, err: testCase.err}
			handler, _ := NewCustomerChatActivityHandler(application)
			request := customerContextRequest(t, &principal, &authorization)
			response := httptest.NewRecorder()
			handler.GetCustomerChatActivity(response, request, testCase.customerID, testCase.query)
			if response.Code != testCase.wantStatus || len(application.inputs) != testCase.wantCalls {
				t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(application.inputs))
			}
			var body struct {
				Code platformhttp.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Code != testCase.wantCode {
				t.Fatalf("body=%s code=%q err=%v", response.Body.String(), body.Code, err)
			}
			for _, hidden := range []string{"customer chat activity unavailable", "invalid customer chat activity query"} {
				if strings.Contains(response.Body.String(), hidden) {
					t.Fatalf("error leaked %q: %s", hidden, response.Body.String())
				}
			}
		})
	}
}

func TestCustomerChatActivityHandlerFailsClosedWithoutAuthorization(t *testing.T) {
	application := &customerChatActivityApplicationStub{result: customerChatActivityResult()}
	handler, _ := NewCustomerChatActivityHandler(application)
	response := httptest.NewRecorder()
	handler.GetCustomerChatActivity(response, httptest.NewRequest(http.MethodGet, "/api/v1/customers/41/chat-activity", nil), 41, CustomerChatActivityQuery{})
	if response.Code != http.StatusForbidden || len(application.inputs) != 0 {
		t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(application.inputs))
	}
}

var _ customerChatActivityApplication = (*customerChatActivityApplicationStub)(nil)

func TestCustomerChatActivityErrorClassificationUsesClosedCodes(t *testing.T) {
	if !errors.Is(errors.Join(customer360port.ErrCustomerChatActivityUnavailable, errors.New("db")), customer360port.ErrCustomerChatActivityUnavailable) {
		t.Fatal("joined unavailable error must remain classifiable")
	}
}

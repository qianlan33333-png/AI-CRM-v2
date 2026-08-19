package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	customer360port "github.com/qianlan33333-png/AI-CRM-v2/internal/customer360/port"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type customerContextLocalFake struct {
	result contactport.Customer360Read
	err    error
	calls  int
	inputs []contactport.Customer360ReadInput
}

func (fake *customerContextLocalFake) ReadCustomer360(_ context.Context, input contactport.Customer360ReadInput) (contactport.Customer360Read, error) {
	fake.calls++
	cloned := input
	cloned.OwnerStaffID = customerContextTestInt64(input.OwnerStaffID)
	fake.inputs = append(fake.inputs, cloned)
	return fake.result, fake.err
}

type customerContextChatFake struct {
	result wecomport.CustomerChatSummaryPage
	err    error
	calls  int
	inputs []wecomport.CustomerChatSummaryQuery
}

func (fake *customerContextChatFake) ListCustomerChatSummaries(_ context.Context, input wecomport.CustomerChatSummaryQuery) (wecomport.CustomerChatSummaryPage, error) {
	fake.calls++
	fake.inputs = append(fake.inputs, input)
	return fake.result, fake.err
}

func customerContextTestInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func customerContextTestTime(minute int) time.Time {
	return time.Date(2026, time.August, 20, 12, minute, 0, 0, time.UTC)
}

func customerContextTestLocal(customerID contactport.CustomerID) contactport.Customer360Read {
	cursor := "timeline-next"
	group := "local CRM"
	return contactport.Customer360Read{
		Customer:           contactport.Customer360Customer{ID: customerID, Name: "Ada", StageID: customerContextTestInt64Pointer(17)},
		Tags:               []contactport.Customer360Tag{{ID: 31, GroupName: &group, Name: "important"}},
		Timeline:           []contactport.Customer360TimelineEntry{{ID: 41, EventType: "customer.updated", OccurredAt: customerContextTestTime(1)}},
		TimelineNextCursor: &cursor,
	}
}

func customerContextTestInt64Pointer(value int64) *int64 { return &value }

func customerContextTestChats() wecomport.CustomerChatSummaryPage {
	return wecomport.CustomerChatSummaryPage{
		Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "text", SentAt: customerContextTestTime(2)}},
		Total: 1, Limit: customerContextChatLimit, Offset: 0,
	}
}

func TestCustomerContextServiceReturnsOnlySafeLocalContext(t *testing.T) {
	owner := int64(71)
	local := &customerContextLocalFake{result: customerContextTestLocal(41)}
	chats := &customerContextChatFake{result: customerContextTestChats()}
	result, err := NewCustomerContextService(local, chats).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{
		CustomerID: 41, OwnerStaffID: &owner, TimelineCursor: "cursor", TimelineLimit: 2,
	})
	if err != nil {
		t.Fatalf("ReadCustomerContext() error = %v", err)
	}
	if result.Customer.ID != 41 || result.Customer.Name != "Ada" || len(result.Tags) != 1 || len(result.Timeline) != 1 ||
		!result.Chat.LocalArchiveAvailable || len(result.Chat.Items) != 1 || result.Chat.Items[0].MessageType != "text" {
		t.Fatalf("context = %#v", result)
	}
	if local.calls != 1 || chats.calls != 1 || local.inputs[0].OwnerStaffID == nil || *local.inputs[0].OwnerStaffID != owner ||
		chats.inputs[0] != (wecomport.CustomerChatSummaryQuery{CustomerID: 41, Limit: customerContextChatLimit, Offset: 0}) {
		t.Fatalf("reader inputs = local:%#v chat:%#v", local.inputs, chats.inputs)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal context: %v", marshalErr)
	}
	for _, forbidden := range []string{"identity_hint", "external_user", "payload", "content", "provider", "receipt", "media_url"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("context leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestCustomerContextServiceMarksOnlyLocalArchiveUnavailable(t *testing.T) {
	local := &customerContextLocalFake{result: customerContextTestLocal(41)}
	chats := &customerContextChatFake{err: wecomport.ErrCustomerChatSummaryUnavailable}
	result, err := NewCustomerContextService(local, chats).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{CustomerID: 41})
	if err != nil || result.Customer.ID != 41 || result.Chat.LocalArchiveAvailable || result.Chat.Items != nil || result.Chat.Total != 0 {
		t.Fatalf("context=%#v err=%v", result, err)
	}
}

func TestCustomerContextServiceFailsClosedForInvalidOrUnsafeDependencies(t *testing.T) {
	tests := []struct {
		name  string
		query customer360port.CustomerContextQuery
	}{
		{name: "zero customer", query: customer360port.CustomerContextQuery{}},
		{name: "timeline limit too large", query: customer360port.CustomerContextQuery{CustomerID: 41, TimelineLimit: 201}},
		{name: "cursor too long", query: customer360port.CustomerContextQuery{CustomerID: 41, TimelineCursor: strings.Repeat("a", 513)}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			local, chats := &customerContextLocalFake{}, &customerContextChatFake{}
			result, err := NewCustomerContextService(local, chats).ReadCustomerContext(context.Background(), testCase.query)
			if !errors.Is(err, customer360port.ErrInvalidCustomerContext) || !reflect.DeepEqual(result, customer360port.CustomerContext{}) || local.calls != 0 || chats.calls != 0 {
				t.Fatalf("result=%#v err=%v local=%d chats=%d", result, err, local.calls, chats.calls)
			}
		})
	}

	result, err := (*CustomerContextService)(nil).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{CustomerID: 41})
	if !errors.Is(err, customer360port.ErrCustomerContextUnavailable) || !reflect.DeepEqual(result, customer360port.CustomerContext{}) {
		t.Fatalf("nil service result=%#v err=%v", result, err)
	}

	unsafeChats := &customerContextChatFake{result: wecomport.CustomerChatSummaryPage{Limit: customerContextChatLimit, Items: []wecomport.CustomerChatSummary{{ChatType: "private", MessageType: "", SentAt: customerContextTestTime(3)}}}}
	result, err = NewCustomerContextService(&customerContextLocalFake{result: customerContextTestLocal(41)}, unsafeChats).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{CustomerID: 41})
	if !errors.Is(err, customer360port.ErrCustomerContextUnavailable) || !reflect.DeepEqual(result, customer360port.CustomerContext{}) {
		t.Fatalf("unsafe chat result=%#v err=%v", result, err)
	}
}

func TestCustomerContextServicePreservesCustomerNotFoundAndRejectsUnexpectedChatError(t *testing.T) {
	result, err := NewCustomerContextService(&customerContextLocalFake{err: contactport.ErrCustomerReadNotFound}, &customerContextChatFake{}).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{CustomerID: 41})
	if !errors.Is(err, contactport.ErrCustomerReadNotFound) || !reflect.DeepEqual(result, customer360port.CustomerContext{}) {
		t.Fatalf("not found result=%#v err=%v", result, err)
	}
	result, err = NewCustomerContextService(&customerContextLocalFake{result: customerContextTestLocal(41)}, &customerContextChatFake{err: errors.New("unexpected")}).ReadCustomerContext(context.Background(), customer360port.CustomerContextQuery{CustomerID: 41})
	if !errors.Is(err, customer360port.ErrCustomerContextUnavailable) || !reflect.DeepEqual(result, customer360port.CustomerContext{}) {
		t.Fatalf("unexpected chat result=%#v err=%v", result, err)
	}
}

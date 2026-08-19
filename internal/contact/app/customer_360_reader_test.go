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
)

type customer360DetailFake struct {
	result CustomerDetailStoreResult
	err    error
	calls  int
	inputs []CustomerDetailInput
}

func (fake *customer360DetailFake) Get(_ context.Context, input CustomerDetailInput) (CustomerDetailStoreResult, error) {
	fake.calls++
	fake.inputs = append(fake.inputs, cloneCustomerDetailInput(input))
	return fake.result, fake.err
}

type customer360TimelineFake struct {
	result CustomerEventResult
	err    error
	calls  int
	inputs []CustomerEventInput
}

func (fake *customer360TimelineFake) List(_ context.Context, input CustomerEventInput) (CustomerEventResult, error) {
	fake.calls++
	cloned := input
	cloned.OwnerStaffID = cloneInt64(input.OwnerStaffID)
	fake.inputs = append(fake.inputs, cloned)
	return fake.result, fake.err
}

func customer360TestInput(customerID int64, owner *int64) contactport.Customer360ReadInput {
	return contactport.Customer360ReadInput{
		CustomerID: contactport.CustomerID(customerID), OwnerStaffID: cloneInt64(owner), TimelineLimit: 2,
	}
}

func customer360TestDetail(customerID contactport.CustomerID, owner *int64) CustomerDetailStoreResult {
	return CustomerDetailStoreResult{
		Customer: CustomerRecord{
			ID: customerID, Name: "Ada", StageID: customer360Int64(17), OwnerStaffID: cloneInt64(owner), ChannelID: customer360Int64(23),
			AddedAt: customer360TimePtr(1), LastInteractAt: customer360TimePtr(2), Extra: json.RawMessage(`{"identity_hint":"must-not-leak"}`),
			CreatedAt: customer360Time(0), UpdatedAt: customer360Time(3),
		},
		Tags: []CustomerTagRecord{{ID: 31, Name: "important", SortOrder: 2}},
	}
}

func customer360TestEvents(customerID contactport.CustomerID) CustomerEventResult {
	cursor := "safe-cursor"
	return CustomerEventResult{
		Items: []CustomerEventRecord{{
			ID: 41, CustomerID: customerID, EventType: "customer.updated", Actor: "staff:external-id", OccurredAt: customer360Time(4),
			Payload: json.RawMessage(`{"external_identity":"must-not-leak","answer":"must-not-leak"}`),
		}},
		NextCursor: &cursor,
	}
}

func customer360Time(minute int) time.Time {
	return time.Date(2026, time.August, 20, 10, minute, 0, 0, time.UTC)
}

func customer360TimePtr(minute int) *time.Time {
	value := customer360Time(minute)
	return &value
}

func customer360Int64(value int64) *int64 { return &value }

func TestCustomer360ReaderMapsOnlySafeLocalCustomerFields(t *testing.T) {
	owner := int64(71)
	detail := &customer360DetailFake{result: customer360TestDetail(41, &owner)}
	timeline := &customer360TimelineFake{result: customer360TestEvents(41)}
	service := NewCustomer360ReaderService(detail, timeline)

	result, err := service.ReadCustomer360(context.Background(), customer360TestInput(41, &owner))
	if err != nil {
		t.Fatalf("ReadCustomer360() error = %v", err)
	}
	if result.Customer.ID != 41 || result.Customer.Name != "Ada" || result.Customer.StageID == nil || *result.Customer.StageID != 17 ||
		len(result.Tags) != 1 || result.Tags[0].ID != 31 || len(result.Timeline) != 1 || result.Timeline[0].ID != 41 ||
		result.Timeline[0].EventType != "customer.updated" || result.TimelineNextCursor == nil || *result.TimelineNextCursor != "safe-cursor" {
		t.Fatalf("safe result = %#v", result)
	}
	if detail.calls != 1 || timeline.calls != 1 || detail.inputs[0].OwnerStaffID == nil || *detail.inputs[0].OwnerStaffID != owner ||
		timeline.inputs[0].OwnerStaffID == nil || *timeline.inputs[0].OwnerStaffID != owner || timeline.inputs[0].Limit != 2 {
		t.Fatalf("scoped collaborator inputs = detail:%#v timeline:%#v", detail.inputs, timeline.inputs)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal safe result: %v", marshalErr)
	}
	for _, forbidden := range []string{"identity_hint", "external_identity", "must-not-leak", "staff:external-id", "Payload", "Actor"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe result leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestCustomer360ReaderRejectsInvalidRequestsBeforeCollaborators(t *testing.T) {
	zeroOwner := int64(0)
	tests := []struct {
		name  string
		ctx   context.Context
		input contactport.Customer360ReadInput
	}{
		{name: "nil context", input: customer360TestInput(41, nil)},
		{name: "zero customer", ctx: context.Background(), input: customer360TestInput(0, nil)},
		{name: "zero owner", ctx: context.Background(), input: customer360TestInput(41, &zeroOwner)},
		{name: "negative timeline limit", ctx: context.Background(), input: contactport.Customer360ReadInput{CustomerID: 41, TimelineLimit: -1}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			detail, timeline := &customer360DetailFake{}, &customer360TimelineFake{}
			result, err := NewCustomer360ReaderService(detail, timeline).ReadCustomer360(testCase.ctx, testCase.input)
			if !errors.Is(err, contactport.ErrInvalidCustomer360Read) || !reflect.DeepEqual(result, contactport.Customer360Read{}) || detail.calls != 0 || timeline.calls != 0 {
				t.Fatalf("result=%#v err=%v calls=detail:%d timeline:%d", result, err, detail.calls, timeline.calls)
			}
		})
	}
}

func TestCustomer360ReaderMapsNotFoundAndNeverReturnsPartialData(t *testing.T) {
	detail := &customer360DetailFake{result: customer360TestDetail(41, nil)}
	timeline := &customer360TimelineFake{err: ErrCustomerNotFound}
	result, err := NewCustomer360ReaderService(detail, timeline).ReadCustomer360(context.Background(), customer360TestInput(41, nil))
	if !errors.Is(err, contactport.ErrCustomerReadNotFound) || !reflect.DeepEqual(result, contactport.Customer360Read{}) || detail.calls != 1 || timeline.calls != 1 {
		t.Fatalf("result=%#v err=%v calls=detail:%d timeline:%d", result, err, detail.calls, timeline.calls)
	}

	failedDetail := &customer360DetailFake{err: errors.New("store unavailable")}
	result, err = NewCustomer360ReaderService(failedDetail, &customer360TimelineFake{}).ReadCustomer360(context.Background(), customer360TestInput(41, nil))
	if !errors.Is(err, contactport.ErrCustomer360ReadUnavailable) || !reflect.DeepEqual(result, contactport.Customer360Read{}) {
		t.Fatalf("detail failure result=%#v err=%v", result, err)
	}
}

func TestCustomer360ReaderFailsClosedWithoutDependencies(t *testing.T) {
	result, err := (*Customer360ReaderService)(nil).ReadCustomer360(context.Background(), customer360TestInput(41, nil))
	if !errors.Is(err, contactport.ErrCustomer360ReadUnavailable) || !reflect.DeepEqual(result, contactport.Customer360Read{}) {
		t.Fatalf("nil service result=%#v err=%v", result, err)
	}
}

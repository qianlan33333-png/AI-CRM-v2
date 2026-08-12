package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type c02dEventAttemptKey struct{}
type c02dEventParentKey struct{}

type c02dEventUoW struct {
	calls, callbacks int
	attempts         int
	err              error
}

func (uow *c02dEventUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		uow.callbacks++
		if err := callback(context.WithValue(ctx, c02dEventAttemptKey{}, attempt)); err != nil {
			return err
		}
	}
	return nil
}

type c02dEventStore struct {
	result CustomerEventStoreResult
	err    error

	calls        int
	queries      []CustomerEventQuery
	rawOwners    []*int64
	attempts     []int
	parentValues []string
}

func (store *c02dEventStore) ListCustomerEvents(ctx context.Context, query CustomerEventQuery) (CustomerEventStoreResult, error) {
	store.calls++
	store.rawOwners = append(store.rawOwners, query.OwnerStaffID)
	store.queries = append(store.queries, c02dEventCloneQuery(query))
	attempt, _ := ctx.Value(c02dEventAttemptKey{}).(int)
	store.attempts = append(store.attempts, attempt)
	parent, _ := ctx.Value(c02dEventParentKey{}).(string)
	store.parentValues = append(store.parentValues, parent)
	return store.result, store.err
}

func c02dEventCloneQuery(query CustomerEventQuery) CustomerEventQuery {
	cloned := query
	cloned.OwnerStaffID = c02dEventCloneInt64(query.OwnerStaffID)
	cloned.AfterOccurredAt = c02dEventCloneTime(query.AfterOccurredAt)
	cloned.AfterID = c02dEventCloneInt64(query.AfterID)
	return cloned
}

func c02dEventCloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func c02dEventCloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func c02dEventTime(minute int) time.Time {
	return time.Date(2026, time.August, 12, 10, minute, 30, 123456000, time.UTC)
}

func c02dEventInput(customerID int64, ownerStaffID *int64) CustomerEventInput {
	return CustomerEventInput{
		CustomerID:   contactport.CustomerID(customerID),
		OwnerStaffID: c02dEventCloneInt64(ownerStaffID),
	}
}

func c02dEventRecord(id int64, customerID contactport.CustomerID, occurredAt time.Time) CustomerEventRecord {
	return CustomerEventRecord{
		ID: id, CustomerID: customerID, EventType: "customer.updated",
		Payload: json.RawMessage(`{"source":"service-test"}`), Actor: "staff:7", OccurredAt: occurredAt,
	}
}

func c02dEventRequireUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrCustomerEventsUnavailable) {
		t.Fatalf("error = %v, expected unavailable", err)
	}
}

func c02dEventRequireNoCalls(t *testing.T, uow *c02dEventUoW, store *c02dEventStore) {
	t.Helper()
	if uow.calls != 0 || uow.callbacks != 0 || store.calls != 0 {
		t.Fatalf("unexpected collaborator calls: unit=%d callbacks=%d store=%d", uow.calls, uow.callbacks, store.calls)
	}
}

func c02dEventRequireZeroResult(t *testing.T, result CustomerEventResult) {
	t.Helper()
	if !reflect.DeepEqual(result, CustomerEventResult{}) {
		t.Fatalf("result = %#v, expected no partial page", result)
	}
}

func TestC02DCustomerEventServiceNormalizesLimitsAndScopes(t *testing.T) {
	owner := int64(71)
	tests := []struct {
		name      string
		input     CustomerEventInput
		wantLimit int32
	}{
		{name: "global default limit", input: c02dEventInput(41, nil), wantLimit: CustomerListDefaultLimit},
		{name: "owner lower limit", input: CustomerEventInput{CustomerID: 42, OwnerStaffID: &owner, Limit: 1}, wantLimit: 1},
		{name: "owner maximum limit", input: CustomerEventInput{CustomerID: 43, OwnerStaffID: &owner, Limit: CustomerListMaximumLimit}, wantLimit: CustomerListMaximumLimit},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &c02dEventUoW{}
			store := &c02dEventStore{result: CustomerEventStoreResult{Items: []CustomerEventRecord{
				c02dEventRecord(9, testCase.input.CustomerID, c02dEventTime(20)),
			}}}
			ctx := context.WithValue(context.Background(), c02dEventParentKey{}, "request-marker")
			result, err := NewCustomerEventService(uow, store).List(ctx, testCase.input)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(result.Items) != 1 || result.NextCursor != nil || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
			}
			if !reflect.DeepEqual(store.attempts, []int{1}) || !reflect.DeepEqual(store.parentValues, []string{"request-marker"}) {
				t.Fatalf("callback context markers = attempts:%v parent:%v", store.attempts, store.parentValues)
			}
			received := store.queries[0]
			if received.CustomerID != testCase.input.CustomerID || received.Limit != testCase.wantLimit || received.AfterOccurredAt != nil || received.AfterID != nil {
				t.Fatalf("normalized query = %#v", received)
			}
			if testCase.input.OwnerStaffID == nil {
				if received.OwnerStaffID != nil || store.rawOwners[0] != nil {
					t.Fatalf("global scope owner = query:%#v raw:%#v", received.OwnerStaffID, store.rawOwners[0])
				}
				return
			}
			if received.OwnerStaffID == nil || *received.OwnerStaffID != *testCase.input.OwnerStaffID {
				t.Fatalf("owner scope = %#v, expected %d", received.OwnerStaffID, *testCase.input.OwnerStaffID)
			}
			if store.rawOwners[0] == testCase.input.OwnerStaffID {
				t.Fatal("caller owner pointer was forwarded")
			}
		})
	}
}

func TestC02DCustomerEventServiceRejectsInvalidInputBeforeTransaction(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name      string
		ctx       context.Context
		input     CustomerEventInput
		want      error
		wantCause error
	}{
		{name: "nil context", input: c02dEventInput(41, nil), want: ErrInvalidCustomerEventQuery},
		{name: "cancelled context", ctx: cancelled, input: c02dEventInput(41, nil), want: ErrCustomerEventsUnavailable, wantCause: context.Canceled},
		{name: "zero customer", ctx: context.Background(), input: c02dEventInput(0, nil), want: ErrInvalidCustomerEventQuery},
		{name: "negative customer", ctx: context.Background(), input: c02dEventInput(-1, nil), want: ErrInvalidCustomerEventQuery},
		{name: "zero owner", ctx: context.Background(), input: c02dEventInput(41, &zero), want: ErrInvalidCustomerEventQuery},
		{name: "negative owner", ctx: context.Background(), input: c02dEventInput(41, &negative), want: ErrInvalidCustomerEventQuery},
		{name: "negative limit", ctx: context.Background(), input: CustomerEventInput{CustomerID: 41, Limit: -1}, want: ErrInvalidCustomerEventQuery},
		{name: "limit above maximum", ctx: context.Background(), input: CustomerEventInput{CustomerID: 41, Limit: CustomerListMaximumLimit + 1}, want: ErrInvalidCustomerEventQuery},
		{name: "cursor above maximum", ctx: context.Background(), input: CustomerEventInput{CustomerID: 41, Cursor: strings.Repeat("a", customerEventMaximumCursor+1)}, want: ErrInvalidCustomerEventQuery},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &c02dEventUoW{}, &c02dEventStore{}
			result, err := NewCustomerEventService(uow, store).List(testCase.ctx, testCase.input)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("List() error = %v, expected %v", err, testCase.want)
			}
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("List() error = %v, expected cause %v", err, testCase.wantCause)
			}
			c02dEventRequireZeroResult(t, result)
			c02dEventRequireNoCalls(t, uow, store)
		})
	}
}

func TestC02DCustomerEventServiceFailsClosedWithoutDependencies(t *testing.T) {
	tests := []struct {
		name    string
		service func(*c02dEventUoW, *c02dEventStore) *CustomerEventService
	}{
		{name: "nil receiver", service: func(*c02dEventUoW, *c02dEventStore) *CustomerEventService { return nil }},
		{name: "missing unit of work", service: func(_ *c02dEventUoW, store *c02dEventStore) *CustomerEventService {
			return &CustomerEventService{store: store}
		}},
		{name: "typed nil unit of work", service: func(_ *c02dEventUoW, store *c02dEventStore) *CustomerEventService {
			var typedNil *c02dEventUoW
			var dependency platformport.UnitOfWork = typedNil
			return &CustomerEventService{uow: dependency, store: store}
		}},
		{name: "missing store", service: func(uow *c02dEventUoW, _ *c02dEventStore) *CustomerEventService {
			return &CustomerEventService{uow: uow}
		}},
		{name: "typed nil store", service: func(uow *c02dEventUoW, _ *c02dEventStore) *CustomerEventService {
			var typedNil *c02dEventStore
			var dependency CustomerEventStore = typedNil
			return &CustomerEventService{uow: uow, store: dependency}
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &c02dEventUoW{}, &c02dEventStore{}
			result, err := testCase.service(uow, store).List(context.Background(), c02dEventInput(41, nil))
			c02dEventRequireUnavailable(t, err)
			c02dEventRequireZeroResult(t, result)
			c02dEventRequireNoCalls(t, uow, store)
		})
	}
}

func TestC02DCustomerEventServicePreservesNotFoundAndMapsDependencyFailures(t *testing.T) {
	input := c02dEventInput(41, nil)
	notFound := &c02dEventStore{err: fmt.Errorf("wrapped: %w", ErrCustomerNotFound)}
	result, err := NewCustomerEventService(&c02dEventUoW{}, notFound).List(context.Background(), input)
	if err != ErrCustomerNotFound {
		t.Fatalf("not found error = %v, expected exact sentinel", err)
	}
	c02dEventRequireZeroResult(t, result)
	if notFound.calls != 1 {
		t.Fatalf("not found store calls = %d, expected 1", notFound.calls)
	}

	sentinel := errors.New("dependency interrupted")
	tests := []struct {
		name       string
		uowErr     error
		storeErr   error
		callbacks  int
		storeCalls int
	}{
		{name: "unit failure", uowErr: sentinel, callbacks: 0, storeCalls: 0},
		{name: "store failure", storeErr: sentinel, callbacks: 1, storeCalls: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &c02dEventUoW{err: testCase.uowErr}
			store := &c02dEventStore{err: testCase.storeErr}
			result, err := NewCustomerEventService(uow, store).List(context.Background(), input)
			c02dEventRequireUnavailable(t, err)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, expected original dependency error", err)
			}
			c02dEventRequireZeroResult(t, result)
			if uow.calls != 1 || uow.callbacks != testCase.callbacks || store.calls != testCase.storeCalls {
				t.Fatalf("failure calls = unit:%d callbacks:%d store:%d", uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestC02DCustomerEventServiceReturnsEmptyPageAndCanonicalNextCursor(t *testing.T) {
	emptyStore := &c02dEventStore{}
	empty, err := NewCustomerEventService(&c02dEventUoW{}, emptyStore).List(context.Background(), c02dEventInput(41, nil))
	if err != nil {
		t.Fatalf("empty List() error = %v", err)
	}
	if empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != nil {
		t.Fatalf("empty result = %#v, expected non-nil empty page", empty)
	}

	firstStore := &c02dEventStore{result: CustomerEventStoreResult{
		Items: []CustomerEventRecord{
			c02dEventRecord(9, 41, c02dEventTime(20)),
			c02dEventRecord(8, 41, c02dEventTime(19)),
		},
		HasMore: true,
	}}
	first, err := NewCustomerEventService(&c02dEventUoW{}, firstStore).List(context.Background(), CustomerEventInput{CustomerID: 41, Limit: 2})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if first.NextCursor == nil || len(first.Items) != 2 {
		t.Fatalf("first result = %#v", first)
	}
	cursor := *first.NextCursor
	if strings.Contains(cursor, "=") {
		t.Fatalf("cursor %q is padded", cursor)
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != cursor {
		t.Fatalf("cursor is not canonical raw base64url: cursor=%q error=%v", cursor, err)
	}
	var payload customerEventCursor
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatalf("decode cursor = %v", err)
	}
	if payload.Version != customerEventCursorVersion || payload.Operation != customerEventCursorOperation ||
		payload.Sort != customerEventCursorSort || payload.FilterHash != customerEventFilterHash(41, nil) ||
		payload.OccurredAt != c02dEventTime(19).Format(time.RFC3339Nano) || payload.ID != 8 {
		t.Fatalf("cursor payload = %#v", payload)
	}

	nextStore := &c02dEventStore{result: CustomerEventStoreResult{Items: []CustomerEventRecord{
		c02dEventRecord(7, 41, c02dEventTime(18)),
	}}}
	next, err := NewCustomerEventService(&c02dEventUoW{}, nextStore).List(context.Background(), CustomerEventInput{CustomerID: 41, Limit: 2, Cursor: cursor})
	if err != nil {
		t.Fatalf("next List() error = %v", err)
	}
	if len(next.Items) != 1 || next.NextCursor != nil {
		t.Fatalf("next result = %#v", next)
	}
	query := nextStore.queries[0]
	if query.AfterOccurredAt == nil || !query.AfterOccurredAt.Equal(c02dEventTime(19)) || query.AfterID == nil || *query.AfterID != 8 {
		t.Fatalf("next query = %#v", query)
	}
}

func TestC02DCustomerEventServiceRejectsCursorContractViolationsBeforeTransaction(t *testing.T) {
	owner := int64(71)
	input := CustomerEventInput{CustomerID: 41, OwnerStaffID: &owner, Limit: 1}
	filter := customerEventFilterHash(input.CustomerID, input.OwnerStaffID)
	position := c02dEventTime(20)
	valid, err := encodeCustomerEventCursor(filter, position, 9)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	wrongOperation := c02dEventEncodeCursor(t, customerEventCursor{
		Version: customerEventCursorVersion, Operation: "otherOperation", Sort: customerEventCursorSort,
		FilterHash: filter, OccurredAt: position.Format(time.RFC3339Nano), ID: 9,
	})
	wrongSort := c02dEventEncodeCursor(t, customerEventCursor{
		Version: customerEventCursorVersion, Operation: customerEventCursorOperation, Sort: "id_desc",
		FilterHash: filter, OccurredAt: position.Format(time.RFC3339Nano), ID: 9,
	})
	wrongFilter := c02dEventEncodeCursor(t, customerEventCursor{
		Version: customerEventCursorVersion, Operation: customerEventCursorOperation, Sort: customerEventCursorSort,
		FilterHash: strings.Repeat("0", 64), OccurredAt: position.Format(time.RFC3339Nano), ID: 9,
	})
	wrongCustomer, err := encodeCustomerEventCursor(customerEventFilterHash(42, input.OwnerStaffID), position, 9)
	if err != nil {
		t.Fatal(err)
	}
	wrongOwner := int64(72)
	wrongOwnerCursor, err := encodeCustomerEventCursor(customerEventFilterHash(input.CustomerID, &wrongOwner), position, 9)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		cursor string
	}{
		{name: "unknown field", cursor: c02dEventRawCursor(fmt.Sprintf(`{"v":1,"operation":"%s","sort":"%s","filter":"%s","occurred_at":"%s","id":9,"unknown":true}`, customerEventCursorOperation, customerEventCursorSort, filter, position.Format(time.RFC3339Nano)))},
		{name: "duplicate field", cursor: c02dEventRawCursor(fmt.Sprintf(`{"v":1,"operation":"%s","sort":"%s","filter":"%s","occurred_at":"%s","id":9,"id":10}`, customerEventCursorOperation, customerEventCursorSort, filter, position.Format(time.RFC3339Nano)))},
		{name: "trailing JSON", cursor: c02dEventRawCursor(string(decoded) + ` {}`)},
		{name: "padded base64url", cursor: valid + "="},
		{name: "wrong operation", cursor: wrongOperation},
		{name: "wrong sort", cursor: wrongSort},
		{name: "wrong filter", cursor: wrongFilter},
		{name: "wrong customer", cursor: wrongCustomer},
		{name: "wrong owner", cursor: wrongOwnerCursor},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &c02dEventUoW{}, &c02dEventStore{}
			request := input
			request.Cursor = testCase.cursor
			result, err := NewCustomerEventService(uow, store).List(context.Background(), request)
			if !errors.Is(err, ErrInvalidCustomerEventQuery) {
				t.Fatalf("List() error = %v, expected cursor input error", err)
			}
			c02dEventRequireZeroResult(t, result)
			c02dEventRequireNoCalls(t, uow, store)
		})
	}
}

func c02dEventEncodeCursor(t *testing.T, payload customerEventCursor) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func c02dEventRawCursor(raw string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func TestC02DCustomerEventServiceFailsClosedForInvalidStoreRows(t *testing.T) {
	valid := func(id int64, occurredAt time.Time) CustomerEventRecord {
		return c02dEventRecord(id, 41, occurredAt)
	}
	nonUTC := time.Date(2026, time.August, 12, 18, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name   string
		input  CustomerEventInput
		result CustomerEventStoreResult
	}{
		{name: "has more without item", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{HasMore: true}},
		{name: "more rows than limit", input: CustomerEventInput{CustomerID: 41, Limit: 1}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(9, c02dEventTime(20)), valid(8, c02dEventTime(19))}}},
		{name: "zero event id", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(0, c02dEventTime(20))}}},
		{name: "wrong customer", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{c02dEventRecord(9, 42, c02dEventTime(20))}}},
		{name: "empty event type", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord { row := valid(9, c02dEventTime(20)); row.EventType = ""; return row }()}}},
		{name: "space padded event type", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.EventType = " customer.updated "
			return row
		}()}}},
		{name: "invalid utf8 event type", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.EventType = string([]byte{0xff})
			return row
		}()}}},
		{name: "empty actor", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord { row := valid(9, c02dEventTime(20)); row.Actor = ""; return row }()}}},
		{name: "too long actor", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.Actor = strings.Repeat("界", 201)
			return row
		}()}}},
		{name: "invalid utf8 actor", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.Actor = string([]byte{0xff})
			return row
		}()}}},
		{name: "zero occurred time", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(9, time.Time{})}}},
		{name: "non utc occurred time", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(9, nonUTC)}}},
		{name: "nil payload", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord { row := valid(9, c02dEventTime(20)); row.Payload = nil; return row }()}}},
		{name: "array payload", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.Payload = json.RawMessage(`[]`)
			return row
		}()}}},
		{name: "null payload", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.Payload = json.RawMessage(`null`)
			return row
		}()}}},
		{name: "malformed payload", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{func() CustomerEventRecord {
			row := valid(9, c02dEventTime(20))
			row.Payload = json.RawMessage(`{`)
			return row
		}()}}},
		{name: "ascending time order", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(8, c02dEventTime(19)), valid(9, c02dEventTime(20))}}},
		{name: "ascending id at same timestamp", input: CustomerEventInput{CustomerID: 41, Limit: 2}, result: CustomerEventStoreResult{Items: []CustomerEventRecord{valid(8, c02dEventTime(20)), valid(9, c02dEventTime(20))}}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &c02dEventUoW{}
			store := &c02dEventStore{result: testCase.result}
			result, err := NewCustomerEventService(uow, store).List(context.Background(), testCase.input)
			c02dEventRequireUnavailable(t, err)
			if errors.Is(err, ErrInvalidCustomerEventQuery) {
				t.Fatalf("store validation returned input error: %v", err)
			}
			c02dEventRequireZeroResult(t, result)
			if uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("invalid row calls = unit:%d callbacks:%d store:%d", uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestC02DCustomerEventServiceAcceptsSameTimestampDescendingIDsAndDeepCopies(t *testing.T) {
	zeroOffset := time.FixedZone("zero-offset", 0)
	occurredAt := time.Date(2026, time.August, 12, 10, 20, 30, 123456000, zeroOffset)
	originalPayload := json.RawMessage(`{"nested":{"source":"store"}}`)
	store := &c02dEventStore{result: CustomerEventStoreResult{Items: []CustomerEventRecord{
		{ID: 9, CustomerID: 41, EventType: "customer.updated", Payload: originalPayload, Actor: "staff:7", OccurredAt: occurredAt},
		{ID: 8, CustomerID: 41, EventType: "customer.updated", Payload: json.RawMessage(`{"source":"store"}`), Actor: "staff:7", OccurredAt: occurredAt},
	}}}
	result, err := NewCustomerEventService(&c02dEventUoW{}, store).List(context.Background(), CustomerEventInput{CustomerID: 41, Limit: 2})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 2 || result.Items[0].ID != 9 || result.Items[1].ID != 8 || result.NextCursor != nil {
		t.Fatalf("result = %#v", result)
	}
	if result.Items[0].OccurredAt.Location() != time.UTC || !result.Items[0].OccurredAt.Equal(occurredAt) {
		t.Fatalf("returned occurred time = %v (%s), expected UTC equivalent", result.Items[0].OccurredAt, result.Items[0].OccurredAt.Location())
	}
	result.Items[0].ID = 999
	result.Items[0].Payload[2] = 'X'
	if store.result.Items[0].ID != 9 || string(store.result.Items[0].Payload) != string(originalPayload) {
		t.Fatalf("store result was mutated through returned page: %#v", store.result.Items[0])
	}
}

func TestC02DCustomerEventServiceRetriesWithStableQuery(t *testing.T) {
	owner := int64(71)
	uow := &c02dEventUoW{attempts: 3}
	store := &c02dEventStore{result: CustomerEventStoreResult{Items: []CustomerEventRecord{
		c02dEventRecord(9, 41, c02dEventTime(20)),
	}}}
	result, err := NewCustomerEventService(uow, store).List(context.Background(), CustomerEventInput{CustomerID: 41, OwnerStaffID: &owner, Limit: 1})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result.Items) != 1 || uow.calls != 1 || uow.callbacks != 3 || store.calls != 3 || !reflect.DeepEqual(store.attempts, []int{1, 2, 3}) {
		t.Fatalf("retry result or calls = %#v unit=%d callbacks=%d store=%d attempts=%v", result, uow.calls, uow.callbacks, store.calls, store.attempts)
	}
	if !reflect.DeepEqual(store.queries[0], store.queries[1]) || !reflect.DeepEqual(store.queries[1], store.queries[2]) {
		t.Fatalf("retry queries changed: %#v", store.queries)
	}
}

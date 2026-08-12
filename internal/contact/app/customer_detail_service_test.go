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
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type detailServiceAttemptKey struct{}
type detailServiceParentKey struct{}

type detailServiceUoW struct {
	calls, callbacks int
	attempts         int
	err              error
	contexts         []context.Context
}

func (uow *detailServiceUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	uow.contexts = append(uow.contexts, ctx)
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		uow.callbacks++
		txCtx := context.WithValue(ctx, detailServiceAttemptKey{}, attempt)
		if err := callback(txCtx); err != nil {
			return err
		}
	}
	return nil
}

type detailServiceStore struct {
	result CustomerDetailStoreResult
	err    error

	calls        int
	inputs       []CustomerDetailInput
	snapshots    []CustomerDetailInput
	attempts     []int
	parentValues []string
	mutateInput  bool
}

func (store *detailServiceStore) GetCustomerDetail(ctx context.Context, input CustomerDetailInput) (CustomerDetailStoreResult, error) {
	store.calls++
	store.inputs = append(store.inputs, input)
	store.snapshots = append(store.snapshots, detailServiceCloneInput(input))
	attempt, _ := ctx.Value(detailServiceAttemptKey{}).(int)
	store.attempts = append(store.attempts, attempt)
	parentValue, _ := ctx.Value(detailServiceParentKey{}).(string)
	store.parentValues = append(store.parentValues, parentValue)
	if store.mutateInput && input.OwnerStaffID != nil {
		*input.OwnerStaffID = -999
	}
	return store.result, store.err
}

func detailServiceTime(minute int) time.Time {
	return time.Date(2026, time.August, 12, 9, minute, 30, 123456000, time.UTC)
}

func detailServiceInt64(value int64) *int64 {
	return &value
}

func detailServiceInt16(value int16) *int16 {
	return &value
}

func detailServiceString(value string) *string {
	return &value
}

func detailServiceTimePtr(value time.Time) *time.Time {
	return &value
}

func detailServiceInput(id int64, owner *int64) CustomerDetailInput {
	return CustomerDetailInput{ID: contactport.CustomerID(id), OwnerStaffID: detailServiceCopyInt64(owner)}
}

func detailServiceValidResult(id contactport.CustomerID, owner *int64) CustomerDetailStoreResult {
	stage, channel := int64(17), int64(23)
	gender := int16(2)
	avatar := "https://cdn.example.test/avatar.png"
	addedAt, lastInteractAt := detailServiceTime(2), detailServiceTime(3)
	return CustomerDetailStoreResult{
		Customer: CustomerRecord{
			ID:             id,
			Name:           "Ada",
			AvatarURL:      &avatar,
			Gender:         &gender,
			StageID:        &stage,
			OwnerStaffID:   detailServiceCopyInt64(owner),
			ChannelID:      &channel,
			AddedAt:        &addedAt,
			LastInteractAt: &lastInteractAt,
			Extra:          json.RawMessage(`{"tier":"gold"}`),
			CreatedAt:      detailServiceTime(0),
			UpdatedAt:      detailServiceTime(4),
		},
		Tags: []CustomerTagRecord{detailServiceTag(31, 1, 2)},
	}
}

func detailServiceTag(id int64, groupSortOrder, sortOrder int32) CustomerTagRecord {
	groupID := id + 100
	groupName := "segment"
	return CustomerTagRecord{
		ID:             id,
		GroupID:        &groupID,
		GroupName:      &groupName,
		GroupSortOrder: groupSortOrder,
		Name:           "important",
		SortOrder:      sortOrder,
	}
}

func detailServiceCopyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func detailServiceCopyInt16(value *int16) *int16 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func detailServiceCopyString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func detailServiceCopyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func detailServiceCloneInput(input CustomerDetailInput) CustomerDetailInput {
	cloned := input
	cloned.OwnerStaffID = detailServiceCopyInt64(input.OwnerStaffID)
	return cloned
}

func detailServiceCloneResult(result CustomerDetailStoreResult) CustomerDetailStoreResult {
	cloned := result
	cloned.Customer.Extra = append(json.RawMessage(nil), result.Customer.Extra...)
	cloned.Customer.AvatarURL = detailServiceCopyString(result.Customer.AvatarURL)
	cloned.Customer.Gender = detailServiceCopyInt16(result.Customer.Gender)
	cloned.Customer.StageID = detailServiceCopyInt64(result.Customer.StageID)
	cloned.Customer.OwnerStaffID = detailServiceCopyInt64(result.Customer.OwnerStaffID)
	cloned.Customer.ChannelID = detailServiceCopyInt64(result.Customer.ChannelID)
	cloned.Customer.AddedAt = detailServiceCopyTime(result.Customer.AddedAt)
	cloned.Customer.LastInteractAt = detailServiceCopyTime(result.Customer.LastInteractAt)
	cloned.Tags = make([]CustomerTagRecord, len(result.Tags))
	for index, tag := range result.Tags {
		cloned.Tags[index] = tag
		cloned.Tags[index].GroupID = detailServiceCopyInt64(tag.GroupID)
		cloned.Tags[index].GroupName = detailServiceCopyString(tag.GroupName)
	}
	return cloned
}

func detailServiceRequireUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrCustomerDetailUnavailable) {
		t.Fatalf("error = %v, expected unavailable", err)
	}
}

func detailServiceRequireNoCalls(t *testing.T, uow *detailServiceUoW, store *detailServiceStore) {
	t.Helper()
	if uow.calls != 0 || uow.callbacks != 0 || store.calls != 0 {
		t.Fatalf("unexpected collaborator calls: unit=%d callbacks=%d store=%d", uow.calls, uow.callbacks, store.calls)
	}
}

func TestCustomerDetailServiceFailsClosedWithoutDependencies(t *testing.T) {
	tests := []struct {
		name    string
		service func(*detailServiceUoW, *detailServiceStore) *CustomerDetailService
	}{
		{name: "nil receiver", service: func(*detailServiceUoW, *detailServiceStore) *CustomerDetailService { return nil }},
		{name: "missing unit of work", service: func(_ *detailServiceUoW, store *detailServiceStore) *CustomerDetailService {
			return &CustomerDetailService{store: store}
		}},
		{name: "typed nil unit of work", service: func(_ *detailServiceUoW, store *detailServiceStore) *CustomerDetailService {
			var typedNil *detailServiceUoW
			var dependency platformport.UnitOfWork = typedNil
			return &CustomerDetailService{uow: dependency, store: store}
		}},
		{name: "missing store", service: func(uow *detailServiceUoW, _ *detailServiceStore) *CustomerDetailService {
			return &CustomerDetailService{uow: uow}
		}},
		{name: "typed nil store", service: func(uow *detailServiceUoW, _ *detailServiceStore) *CustomerDetailService {
			var typedNil *detailServiceStore
			var dependency CustomerDetailStore = typedNil
			return &CustomerDetailService{uow: uow, store: dependency}
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &detailServiceUoW{}, &detailServiceStore{}
			_, err := testCase.service(uow, store).Get(context.Background(), detailServiceInput(41, nil))
			detailServiceRequireUnavailable(t, err)
			detailServiceRequireNoCalls(t, uow, store)
		})
	}
}

func TestCustomerDetailServiceRejectsInvalidRequestsBeforeTransaction(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name      string
		ctx       context.Context
		input     CustomerDetailInput
		want      error
		wantCause error
	}{
		{name: "missing context", input: detailServiceInput(41, nil), want: ErrInvalidCustomerDetailQuery},
		{name: "cancelled context", ctx: cancelled, input: detailServiceInput(41, nil), want: ErrCustomerDetailUnavailable, wantCause: context.Canceled},
		{name: "zero customer", ctx: context.Background(), input: detailServiceInput(0, nil), want: ErrInvalidCustomerDetailQuery},
		{name: "negative customer", ctx: context.Background(), input: detailServiceInput(-1, nil), want: ErrInvalidCustomerDetailQuery},
		{name: "zero owner", ctx: context.Background(), input: detailServiceInput(41, &zero), want: ErrInvalidCustomerDetailQuery},
		{name: "negative owner", ctx: context.Background(), input: detailServiceInput(41, &negative), want: ErrInvalidCustomerDetailQuery},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow, store := &detailServiceUoW{}, &detailServiceStore{}
			_, err := NewCustomerDetailService(uow, store).Get(testCase.ctx, testCase.input)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, expected %v", err, testCase.want)
			}
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("error = %v, expected cause %v", err, testCase.wantCause)
			}
			detailServiceRequireNoCalls(t, uow, store)
		})
	}
}

func TestCustomerDetailServicePassesGlobalAndOwnerScopesThroughTransaction(t *testing.T) {
	owner := int64(71)
	tests := []struct {
		name  string
		input CustomerDetailInput
	}{
		{name: "global scope", input: detailServiceInput(41, nil)},
		{name: "owner scope", input: detailServiceInput(42, &owner)},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &detailServiceUoW{}
			store := &detailServiceStore{result: detailServiceValidResult(testCase.input.ID, testCase.input.OwnerStaffID)}
			ctx := context.WithValue(context.Background(), detailServiceParentKey{}, "request-marker")
			result, err := NewCustomerDetailService(uow, store).Get(ctx, testCase.input)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if result.Customer.ID != testCase.input.ID || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
			}
			if !reflect.DeepEqual(store.attempts, []int{1}) || !reflect.DeepEqual(store.parentValues, []string{"request-marker"}) {
				t.Fatalf("callback context markers = attempts:%v parent:%v", store.attempts, store.parentValues)
			}
			received := store.inputs[0]
			if received.ID != testCase.input.ID {
				t.Fatalf("customer ID = %d, expected %d", received.ID, testCase.input.ID)
			}
			if testCase.input.OwnerStaffID == nil {
				if received.OwnerStaffID != nil {
					t.Fatalf("global scope owner = %d, expected nil", *received.OwnerStaffID)
				}
				return
			}
			if received.OwnerStaffID == nil || *received.OwnerStaffID != *testCase.input.OwnerStaffID {
				t.Fatalf("owner scope = %#v, expected %d", received.OwnerStaffID, *testCase.input.OwnerStaffID)
			}
			if received.OwnerStaffID == testCase.input.OwnerStaffID {
				t.Fatal("caller owner pointer was forwarded")
			}
		})
	}
}

func TestCustomerDetailServicePreservesNotFoundAndMapsDependencyErrors(t *testing.T) {
	input := detailServiceInput(41, nil)
	notFoundStore := &detailServiceStore{err: ErrCustomerNotFound}
	result, err := NewCustomerDetailService(&detailServiceUoW{}, notFoundStore).Get(context.Background(), input)
	if err != ErrCustomerNotFound {
		t.Fatalf("not found error = %v, expected exact sentinel", err)
	}
	if !reflect.DeepEqual(result, CustomerDetailStoreResult{}) || notFoundStore.calls != 1 {
		t.Fatalf("not found result or calls = %#v store=%d", result, notFoundStore.calls)
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
			uow := &detailServiceUoW{err: testCase.uowErr}
			store := &detailServiceStore{err: testCase.storeErr}
			result, err := NewCustomerDetailService(uow, store).Get(context.Background(), input)
			detailServiceRequireUnavailable(t, err)
			if !errors.Is(err, sentinel) {
				t.Fatalf("error = %v, expected original dependency error", err)
			}
			if !reflect.DeepEqual(result, CustomerDetailStoreResult{}) || uow.calls != 1 || uow.callbacks != testCase.callbacks || store.calls != testCase.storeCalls {
				t.Fatalf("failure result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestCustomerDetailServiceRejectsInvalidCustomerRows(t *testing.T) {
	owner := int64(71)
	input := detailServiceInput(41, &owner)
	tests := []struct {
		name   string
		mutate func(*CustomerDetailStoreResult)
	}{
		{name: "wrong customer id", mutate: func(result *CustomerDetailStoreResult) { result.Customer.ID++ }},
		{name: "owner scope lost", mutate: func(result *CustomerDetailStoreResult) { result.Customer.OwnerStaffID = nil }},
		{name: "owner scope crossed", mutate: func(result *CustomerDetailStoreResult) { result.Customer.OwnerStaffID = detailServiceInt64(72) }},
		{name: "missing created timestamp", mutate: func(result *CustomerDetailStoreResult) { result.Customer.CreatedAt = time.Time{} }},
		{name: "missing newer timestamp", mutate: func(result *CustomerDetailStoreResult) { result.Customer.UpdatedAt = time.Time{} }},
		{name: "created after newer timestamp", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.CreatedAt = result.Customer.UpdatedAt.Add(time.Nanosecond)
		}},
		{name: "empty added timestamp", mutate: func(result *CustomerDetailStoreResult) { result.Customer.AddedAt = detailServiceTimePtr(time.Time{}) }},
		{name: "empty interaction timestamp", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.LastInteractAt = detailServiceTimePtr(time.Time{})
		}},
		{name: "zero stage id", mutate: func(result *CustomerDetailStoreResult) { result.Customer.StageID = detailServiceInt64(0) }},
		{name: "negative owner id", mutate: func(result *CustomerDetailStoreResult) { result.Customer.OwnerStaffID = detailServiceInt64(-1) }},
		{name: "zero channel id", mutate: func(result *CustomerDetailStoreResult) { result.Customer.ChannelID = detailServiceInt64(0) }},
		{name: "invalid customer text", mutate: func(result *CustomerDetailStoreResult) { result.Customer.Name = string([]byte{0xff}) }},
		{name: "invalid avatar protocol", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.AvatarURL = detailServiceString("ftp://cdn.example.test/avatar.png")
		}},
		{name: "avatar user info", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.AvatarURL = detailServiceString("https://person@cdn.example.test/avatar.png")
		}},
		{name: "avatar missing host", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.AvatarURL = detailServiceString("https:///avatar.png")
		}},
		{name: "malformed extra", mutate: func(result *CustomerDetailStoreResult) { result.Customer.Extra = json.RawMessage("{") }},
		{name: "array extra", mutate: func(result *CustomerDetailStoreResult) { result.Customer.Extra = json.RawMessage("[]") }},
		{name: "null extra", mutate: func(result *CustomerDetailStoreResult) { result.Customer.Extra = json.RawMessage("null") }},
		{name: "external identity in extra", mutate: func(result *CustomerDetailStoreResult) {
			result.Customer.Extra = json.RawMessage(`{"nested":[{"open_id":"identity-secret"}]}`)
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			stored := detailServiceValidResult(input.ID, input.OwnerStaffID)
			testCase.mutate(&stored)
			uow, store := &detailServiceUoW{}, &detailServiceStore{result: stored}
			result, err := NewCustomerDetailService(uow, store).Get(context.Background(), input)
			detailServiceRequireUnavailable(t, err)
			if errors.Is(err, ErrInvalidCustomerDetailQuery) {
				t.Fatalf("row validation returned input error: %v", err)
			}
			if !reflect.DeepEqual(result, CustomerDetailStoreResult{}) || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("row validation result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestCustomerDetailServiceRejectsInvalidTagRows(t *testing.T) {
	input := detailServiceInput(41, nil)
	tests := []struct {
		name   string
		mutate func(*CustomerDetailStoreResult)
	}{
		{name: "zero tag id", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].ID = 0 }},
		{name: "negative tag id", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].ID = -1 }},
		{name: "empty tag name", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].Name = "" }},
		{name: "invalid tag text", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].Name = string([]byte{0xff}) }},
		{name: "long tag name", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].Name = strings.Repeat("界", 201) }},
		{name: "group id without name", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].GroupName = nil }},
		{name: "group name without id", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].GroupID = nil }},
		{name: "group sort without pair", mutate: func(result *CustomerDetailStoreResult) {
			result.Tags[0].GroupID = nil
			result.Tags[0].GroupName = nil
			result.Tags[0].GroupSortOrder = 1
		}},
		{name: "zero group id", mutate: func(result *CustomerDetailStoreResult) { result.Tags[0].GroupID = detailServiceInt64(0) }},
		{name: "invalid group text", mutate: func(result *CustomerDetailStoreResult) {
			result.Tags[0].GroupName = detailServiceString(string([]byte{0xff}))
		}},
		{name: "duplicate tag id", mutate: func(result *CustomerDetailStoreResult) {
			duplicate := detailServiceTag(result.Tags[0].ID, 2, 0)
			result.Tags = append(result.Tags, duplicate)
		}},
		{name: "unstable group order", mutate: func(result *CustomerDetailStoreResult) {
			result.Tags = []CustomerTagRecord{detailServiceTag(31, 2, 0), detailServiceTag(32, 1, 0)}
		}},
		{name: "unstable tag order", mutate: func(result *CustomerDetailStoreResult) {
			result.Tags = []CustomerTagRecord{detailServiceTag(32, 1, 0), detailServiceTag(31, 1, 0)}
		}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			stored := detailServiceValidResult(input.ID, nil)
			testCase.mutate(&stored)
			uow, store := &detailServiceUoW{}, &detailServiceStore{result: stored}
			result, err := NewCustomerDetailService(uow, store).Get(context.Background(), input)
			detailServiceRequireUnavailable(t, err)
			if !reflect.DeepEqual(result, CustomerDetailStoreResult{}) || uow.calls != 1 || uow.callbacks != 1 || store.calls != 1 {
				t.Fatalf("tag validation result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
			}
		})
	}
}

func TestCustomerDetailServiceAcceptsFrozenTagBoundaries(t *testing.T) {
	longGroupName := strings.Repeat("组", 201)
	emptyGroupName := ""
	groupID, emptyGroupID := int64(101), int64(102)
	input := detailServiceInput(41, nil)
	stored := detailServiceValidResult(input.ID, nil)
	stored.Tags = []CustomerTagRecord{
		{ID: 31, Name: "  local tag  ", SortOrder: 0},
		{ID: 32, GroupID: &groupID, GroupName: &longGroupName, GroupSortOrder: 1, Name: strings.Repeat("界", 200), SortOrder: 0},
		{ID: 33, GroupID: &emptyGroupID, GroupName: &emptyGroupName, GroupSortOrder: 2, Name: "remote", SortOrder: 0},
	}

	result, err := NewCustomerDetailService(&detailServiceUoW{}, &detailServiceStore{result: stored}).Get(context.Background(), input)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(result.Tags) != len(stored.Tags) || result.Tags[0].Name != stored.Tags[0].Name || *result.Tags[1].GroupName != longGroupName || *result.Tags[2].GroupName != "" {
		t.Fatalf("accepted tag result = %#v", result.Tags)
	}
}

func TestCustomerDetailServiceReturnsNonNilEmptyTags(t *testing.T) {
	input := detailServiceInput(41, nil)
	stored := detailServiceValidResult(input.ID, nil)
	stored.Tags = nil
	result, err := NewCustomerDetailService(&detailServiceUoW{}, &detailServiceStore{result: stored}).Get(context.Background(), input)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Tags == nil || len(result.Tags) != 0 {
		t.Fatalf("tags = %#v, expected non-nil empty list", result.Tags)
	}
}

func TestCustomerDetailServiceReturnsDetachedResult(t *testing.T) {
	owner := int64(71)
	input := detailServiceInput(41, &owner)
	stored := detailServiceValidResult(input.ID, input.OwnerStaffID)
	before := detailServiceCloneResult(stored)
	store := &detailServiceStore{result: stored}
	result, err := NewCustomerDetailService(&detailServiceUoW{}, store).Get(context.Background(), input)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	result.Customer.Extra[0] = '['
	*result.Customer.AvatarURL = "https://changed.example.test/avatar.png"
	*result.Customer.Gender = 9
	*result.Customer.StageID = 99
	*result.Customer.OwnerStaffID = 98
	*result.Customer.ChannelID = 97
	*result.Customer.AddedAt = detailServiceTime(50)
	*result.Customer.LastInteractAt = detailServiceTime(51)
	result.Tags[0].ID = 96
	*result.Tags[0].GroupID = 95
	*result.Tags[0].GroupName = "changed group"

	if !reflect.DeepEqual(store.result, before) {
		t.Fatalf("store result was aliased: got %#v expected %#v", store.result, before)
	}
}

func TestCustomerDetailServiceReplaysTransactionWithStableInput(t *testing.T) {
	owner := int64(71)
	input := detailServiceInput(41, &owner)
	uow := &detailServiceUoW{attempts: 3}
	store := &detailServiceStore{result: detailServiceValidResult(input.ID, input.OwnerStaffID), mutateInput: true}
	result, err := NewCustomerDetailService(uow, store).Get(context.Background(), input)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if result.Customer.ID != input.ID || uow.calls != 1 || uow.callbacks != 3 || store.calls != 3 {
		t.Fatalf("replay result or calls = %#v unit=%d callbacks=%d store=%d", result, uow.calls, uow.callbacks, store.calls)
	}
	if !reflect.DeepEqual(store.attempts, []int{1, 2, 3}) {
		t.Fatalf("transaction attempts = %v", store.attempts)
	}
	for index, snapshot := range store.snapshots {
		if snapshot.ID != input.ID || snapshot.OwnerStaffID == nil || *snapshot.OwnerStaffID != owner {
			t.Fatalf("attempt %d input = %#v", index+1, snapshot)
		}
	}
	if input.OwnerStaffID == nil || *input.OwnerStaffID != owner {
		t.Fatalf("caller input changed: %#v", input)
	}
}

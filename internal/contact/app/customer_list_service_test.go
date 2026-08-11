package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type customerListAttemptKey struct{}

type fakeCustomerListUoW struct {
	calls    int
	attempts int
	err      error
}

func (uow *fakeCustomerListUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	if uow.err != nil {
		return uow.err
	}
	attempts := uow.attempts
	if attempts == 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := callback(context.WithValue(ctx, customerListAttemptKey{}, attempt)); err != nil {
			return err
		}
	}
	return nil
}

type fakeCustomerQueryStore struct {
	result   CustomerListStoreResult
	err      error
	queries  []CustomerListQuery
	attempts []int
}

func (store *fakeCustomerQueryStore) ListCustomers(ctx context.Context, query CustomerListQuery) (CustomerListStoreResult, error) {
	store.queries = append(store.queries, cloneCustomerListQuery(query))
	attempt, _ := ctx.Value(customerListAttemptKey{}).(int)
	store.attempts = append(store.attempts, attempt)
	return store.result, store.err
}

func cloneCustomerListQuery(query CustomerListQuery) CustomerListQuery {
	query.OwnerStaffID = cloneInt64(query.OwnerStaffID)
	query.StageID = cloneInt64(query.StageID)
	query.ChannelID = cloneInt64(query.ChannelID)
	query.TagID = cloneInt64(query.TagID)
	query.AddedAfter = cloneTime(query.AddedAfter)
	query.AddedBefore = cloneTime(query.AddedBefore)
	query.LastInteractAfter = cloneTime(query.LastInteractAfter)
	query.LastInteractBefore = cloneTime(query.LastInteractBefore)
	query.AfterUpdatedAt = cloneTime(query.AfterUpdatedAt)
	if query.AfterID != nil {
		value := *query.AfterID
		query.AfterID = &value
	}
	return query
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func customerListTime(minute int) time.Time {
	return time.Date(2026, time.August, 12, 3, minute, 0, 123456000, time.UTC)
}

func validCustomerRecord(id int64, updatedAt time.Time) CustomerRecord {
	return CustomerRecord{
		ID: contactport.CustomerID(id), Name: "customer", Extra: json.RawMessage(`{}`),
		CreatedAt: customerListTime(0), UpdatedAt: updatedAt,
	}
}

func newTestCustomerListService(uow *fakeCustomerListUoW, store *fakeCustomerQueryStore) *CustomerListService {
	return &CustomerListService{uow: uow, store: store, now: func() time.Time { return customerListTime(30) }}
}

func TestCustomerListServiceNormalizesAllFiltersAndDefaults(t *testing.T) {
	owner, stage, channel, tag := int64(1), int64(2), int64(3), int64(4)
	zone := time.FixedZone("CST", 8*60*60)
	addedAfter := time.Date(2026, 8, 10, 8, 0, 0, 0, zone)
	addedBefore := time.Date(2026, 8, 11, 8, 0, 0, 0, zone)
	interactAfter := time.Date(2026, 8, 10, 9, 0, 0, 0, zone)
	interactBefore := time.Date(2026, 8, 11, 9, 0, 0, 0, zone)
	uow := &fakeCustomerListUoW{}
	store := &fakeCustomerQueryStore{result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(8, customerListTime(20))}, BoundedTotal: 1}}

	result, err := newTestCustomerListService(uow, store).List(context.Background(), CustomerListInput{
		Keyword: "  张三  ", OwnerStaffID: &owner, StageID: &stage, ChannelID: &channel, TagID: &tag,
		IsDeleted: true, AddedAfter: &addedAfter, AddedBefore: &addedBefore,
		LastInteractAfter: &interactAfter, LastInteractBefore: &interactBefore,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if uow.calls != 1 || len(store.queries) != 1 {
		t.Fatalf("calls = uow:%d store:%d, want 1/1", uow.calls, len(store.queries))
	}
	query := store.queries[0]
	if query.Keyword != "张三" || query.Limit != CustomerListDefaultLimit || query.Watermark != customerListTime(30) {
		t.Fatalf("normalized query = %#v", query)
	}
	if *query.OwnerStaffID != owner || *query.StageID != stage || *query.ChannelID != channel || *query.TagID != tag || !query.IsDeleted {
		t.Fatalf("filter query = %#v", query)
	}
	if query.AddedAfter.Location() != time.UTC || !query.AddedAfter.Equal(addedAfter) ||
		query.LastInteractBefore.Location() != time.UTC || !query.LastInteractBefore.Equal(interactBefore) {
		t.Fatalf("time filters were not normalized to UTC: %#v", query)
	}
	if result.Total != 1 || result.TotalIsEstimate || result.NextCursor != nil || result.Watermark != customerListTime(30) {
		t.Fatalf("List() result = %#v", result)
	}
}

func TestCustomerListServiceRejectsInvalidInputBeforeTransaction(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	zeroTime := time.Time{}
	after, before := customerListTime(2), customerListTime(1)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := []struct {
		name  string
		ctx   context.Context
		input CustomerListInput
		want  error
	}{
		{name: "nil context", want: ErrCustomerListUnavailable},
		{name: "cancelled context", ctx: cancelled, want: ErrCustomerListUnavailable},
		{name: "negative limit", ctx: context.Background(), input: CustomerListInput{Limit: -1}, want: ErrInvalidCustomerListQuery},
		{name: "limit above maximum", ctx: context.Background(), input: CustomerListInput{Limit: 201}, want: ErrInvalidCustomerListQuery},
		{name: "keyword too long", ctx: context.Background(), input: CustomerListInput{Keyword: strings.Repeat("界", 201)}, want: ErrInvalidCustomerListQuery},
		{name: "invalid utf8", ctx: context.Background(), input: CustomerListInput{Keyword: string([]byte{0xff})}, want: ErrInvalidCustomerListQuery},
		{name: "zero owner", ctx: context.Background(), input: CustomerListInput{OwnerStaffID: &zero}, want: ErrInvalidCustomerListQuery},
		{name: "negative stage", ctx: context.Background(), input: CustomerListInput{StageID: &negative}, want: ErrInvalidCustomerListQuery},
		{name: "zero time", ctx: context.Background(), input: CustomerListInput{AddedAfter: &zeroTime}, want: ErrInvalidCustomerListQuery},
		{name: "reversed added range", ctx: context.Background(), input: CustomerListInput{AddedAfter: &after, AddedBefore: &before}, want: ErrInvalidCustomerListQuery},
		{name: "reversed interaction range", ctx: context.Background(), input: CustomerListInput{LastInteractAfter: &after, LastInteractBefore: &before}, want: ErrInvalidCustomerListQuery},
		{name: "cursor too long", ctx: context.Background(), input: CustomerListInput{Cursor: strings.Repeat("a", 513)}, want: ErrInvalidCustomerListQuery},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			uow := &fakeCustomerListUoW{}
			store := &fakeCustomerQueryStore{}
			_, err := newTestCustomerListService(uow, store).List(testCase.ctx, testCase.input)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("List() error = %v, want %v", err, testCase.want)
			}
			if uow.calls != 0 || len(store.queries) != 0 {
				t.Fatalf("invalid input reached collaborators: uow=%d store=%d", uow.calls, len(store.queries))
			}
		})
	}
}

func TestCustomerListServiceCursorKeepsWatermarkAndBindsFilters(t *testing.T) {
	firstUoW := &fakeCustomerListUoW{}
	firstStore := &fakeCustomerQueryStore{result: CustomerListStoreResult{
		Items:        []CustomerRecord{validCustomerRecord(9, customerListTime(20)), validCustomerRecord(7, customerListTime(19))},
		BoundedTotal: CustomerListExactTotalCap + 1, HasMore: true,
	}}
	firstService := newTestCustomerListService(firstUoW, firstStore)
	firstService.now = func() time.Time { return customerListTime(30) }
	first, err := firstService.List(context.Background(), CustomerListInput{Keyword: "hello", Limit: 2})
	if err != nil {
		t.Fatalf("first List() error = %v", err)
	}
	if first.NextCursor == nil || first.Total != CustomerListExactTotalCap || !first.TotalIsEstimate {
		t.Fatalf("first result = %#v", first)
	}

	nextStore := &fakeCustomerQueryStore{result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(6, customerListTime(18))}, BoundedTotal: 9999}}
	nextService := newTestCustomerListService(&fakeCustomerListUoW{}, nextStore)
	nextService.now = func() time.Time { return customerListTime(59) }
	next, err := nextService.List(context.Background(), CustomerListInput{Keyword: "hello", Cursor: *first.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("next List() error = %v", err)
	}
	query := nextStore.queries[0]
	if query.Watermark != customerListTime(30) || query.AfterUpdatedAt == nil || *query.AfterUpdatedAt != customerListTime(19) ||
		query.AfterID == nil || *query.AfterID != 7 {
		t.Fatalf("next query = %#v", query)
	}
	if next.Watermark != first.Watermark {
		t.Fatalf("next watermark = %v, want %v", next.Watermark, first.Watermark)
	}

	badService := newTestCustomerListService(&fakeCustomerListUoW{}, &fakeCustomerQueryStore{})
	if _, err := badService.List(context.Background(), CustomerListInput{Keyword: "changed", Cursor: *first.NextCursor}); !errors.Is(err, ErrInvalidCustomerListQuery) {
		t.Fatalf("filter mismatch error = %v", err)
	}
}

func TestCustomerListServiceRejectsMalformedCursors(t *testing.T) {
	valid, err := encodeCustomerListCursor(strings.Repeat("a", 64), customerListTime(30), customerListTime(20), 9)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unknown"] = true
	unknownJSON, _ := json.Marshal(payload)
	unknown := base64.RawURLEncoding.EncodeToString(unknownJSON)
	tests := []string{"not+base64", valid + "=", unknown, base64.RawURLEncoding.EncodeToString(append(decoded, []byte(` {}`)...))}
	for _, cursor := range tests {
		query := CustomerListQuery{}
		if err := applyCustomerListCursor(&query, cursor, strings.Repeat("a", 64)); err == nil {
			t.Fatalf("applyCustomerListCursor(%q) error = nil", cursor)
		}
	}
}

func TestCustomerListServiceFailsClosedForMalformedStoreResults(t *testing.T) {
	watermark := customerListTime(30)
	tests := []struct {
		name   string
		result CustomerListStoreResult
	}{
		{name: "negative total", result: CustomerListStoreResult{BoundedTotal: -1}},
		{name: "total above bounded signal", result: CustomerListStoreResult{BoundedTotal: CustomerListExactTotalCap + 2}},
		{name: "total below returned rows", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(1, watermark)}}},
		{name: "has more empty", result: CustomerListStoreResult{HasMore: true}},
		{name: "too many rows", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(2, watermark), validCustomerRecord(1, watermark)}}},
		{name: "zero id", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(0, watermark)}}},
		{name: "future update", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(1, watermark.Add(time.Second))}}},
		{name: "invalid extra", result: CustomerListStoreResult{Items: []CustomerRecord{func() CustomerRecord {
			row := validCustomerRecord(1, watermark)
			row.Extra = json.RawMessage(`[]`)
			return row
		}()}}},
		{name: "external identity in extra", result: CustomerListStoreResult{Items: []CustomerRecord{func() CustomerRecord {
			row := validCustomerRecord(1, watermark)
			row.Extra = json.RawMessage(`{"nested":{"external_userid":"identity-secret"}}`)
			return row
		}()}}},
		{name: "wrong order", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(1, watermark.Add(-time.Minute)), validCustomerRecord(2, watermark)}}},
		{name: "duplicate order key", result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(1, watermark), validCustomerRecord(1, watermark)}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			store := &fakeCustomerQueryStore{result: testCase.result}
			_, err := newTestCustomerListService(&fakeCustomerListUoW{}, store).List(context.Background(), CustomerListInput{Limit: 1})
			if !errors.Is(err, ErrCustomerListUnavailable) || errors.Is(err, ErrInvalidCustomerListQuery) {
				t.Fatalf("List() error = %v, want unavailable", err)
			}
		})
	}
}

func TestChannelNeutralCustomerExtraRejectsExternalIdentityKeyVariants(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"nested":{"externalUserId":"secret"}}`),
		json.RawMessage(`{"nested":{"external user id":"secret"}}`),
		json.RawMessage(`{"nested":[{"wecom-tag-id":"secret"}]}`),
		json.RawMessage(`{"nested":{"alipay_user_id":"secret"}}`),
		json.RawMessage(`{"nested":{"mp_openid":"secret"}}`),
		json.RawMessage(`{"nested":{"ext:loyalty":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"ext:loyalty","value":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"unionid","value":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"openid","value":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"external_userid","value":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"mobile","value":"secret"}}`),
		json.RawMessage(`{"nested":{"kind":"unionid","k-ind":"business","value":"secret"}}`),
	} {
		for attempt := 0; attempt < 100; attempt++ {
			if IsChannelNeutralCustomerExtra(raw) {
				t.Fatalf("external identity extra was accepted on attempt %d: %s", attempt, raw)
			}
		}
	}
	if !IsChannelNeutralCustomerExtra(json.RawMessage(`{"业务字段":{"campaign_id":7},"profile":{"tier":"gold"}}`)) {
		t.Fatal("channel-neutral business extra was rejected")
	}
}

func TestCustomerListServiceRetriesWithDeterministicQuery(t *testing.T) {
	uow := &fakeCustomerListUoW{attempts: 3}
	store := &fakeCustomerQueryStore{result: CustomerListStoreResult{Items: []CustomerRecord{validCustomerRecord(1, customerListTime(20))}, BoundedTotal: 1}}
	clockCalls := 0
	service := newTestCustomerListService(uow, store)
	service.now = func() time.Time { clockCalls++; return customerListTime(30 + clockCalls - 1) }

	result, err := service.List(context.Background(), CustomerListInput{Keyword: "fixed"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if clockCalls != 1 || len(store.queries) != 3 || !reflect.DeepEqual(store.attempts, []int{1, 2, 3}) {
		t.Fatalf("retry calls = clock:%d queries:%d attempts:%v", clockCalls, len(store.queries), store.attempts)
	}
	if !reflect.DeepEqual(store.queries[0], store.queries[1]) || !reflect.DeepEqual(store.queries[1], store.queries[2]) {
		t.Fatalf("retry queries changed: %#v", store.queries)
	}
	if result.Watermark != customerListTime(30) {
		t.Fatalf("result watermark = %v", result.Watermark)
	}
}

func TestCustomerListServiceMapsDependencyErrorsAndMissingDependencies(t *testing.T) {
	sentinel := errors.New("database failed")
	tests := []struct {
		name    string
		service *CustomerListService
	}{
		{name: "nil receiver"},
		{name: "nil uow", service: &CustomerListService{store: &fakeCustomerQueryStore{}, now: time.Now}},
		{name: "nil store", service: &CustomerListService{uow: &fakeCustomerListUoW{}, now: time.Now}},
		{name: "nil clock", service: &CustomerListService{uow: &fakeCustomerListUoW{}, store: &fakeCustomerQueryStore{}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.service.List(context.Background(), CustomerListInput{})
			if !errors.Is(err, ErrCustomerListUnavailable) {
				t.Fatalf("List() error = %v", err)
			}
		})
	}

	for _, service := range []*CustomerListService{
		newTestCustomerListService(&fakeCustomerListUoW{err: sentinel}, &fakeCustomerQueryStore{}),
		newTestCustomerListService(&fakeCustomerListUoW{}, &fakeCustomerQueryStore{err: sentinel}),
	} {
		_, err := service.List(context.Background(), CustomerListInput{})
		if !errors.Is(err, ErrCustomerListUnavailable) || !errors.Is(err, sentinel) {
			t.Fatalf("dependency error = %v", err)
		}
	}
}

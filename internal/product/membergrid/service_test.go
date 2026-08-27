package membergrid

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

type testUnitOfWork struct {
	err   error
	calls int
}

func (unit *testUnitOfWork) Within(ctx context.Context, callback func(context.Context) error) error {
	unit.calls++
	if unit.err != nil {
		return unit.err
	}
	return callback(ctx)
}

type memoryStore struct {
	exists        bool
	existsErr     error
	queryErr      error
	records       []MemberRecord
	lastQuery     StoreQuery
	queryCalls    int
	invalidResult []MemberRecord
}

type legacyOnlyStore struct{ queryCalls int }

func (*legacyOnlyStore) ProductExists(context.Context, int64) (bool, error) { return true, nil }
func (store *legacyOnlyStore) QueryMembers(context.Context, StoreQuery) ([]MemberRecord, error) {
	store.queryCalls++
	return []MemberRecord{}, nil
}

func (store *memoryStore) ProductExists(context.Context, int64) (bool, error) {
	return store.exists, store.existsErr
}
func (store *memoryStore) QueryMembers(_ context.Context, query StoreQuery) ([]MemberRecord, error) {
	store.queryCalls++
	store.lastQuery = query
	if store.queryErr != nil {
		return nil, store.queryErr
	}
	if store.invalidResult != nil {
		return append([]MemberRecord(nil), store.invalidResult...), nil
	}
	filtered := make([]MemberRecord, 0, len(store.records))
	for _, record := range store.records {
		if record.ServiceProductID != query.ProductID || (query.State != StateAll && record.State != query.State) || (query.Source != SourceAny && record.Source != query.Source) || (query.After != nil && !positionBefore(record, *query.After)) {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(left, right int) bool { return recordBefore(filtered[right], filtered[left]) })
	if len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return append([]MemberRecord(nil), filtered...), nil
}

func (store *memoryStore) QuerySelectedMembers(_ context.Context, query selectedStoreQuery) ([]MemberRecord, error) {
	store.queryCalls++
	if store.queryErr != nil {
		return nil, store.queryErr
	}
	if store.invalidResult != nil {
		return append([]MemberRecord(nil), store.invalidResult...), nil
	}
	filtered := make([]MemberRecord, 0, len(store.records))
	for _, record := range store.records {
		if record.ServiceProductID != query.ProductID || (query.State != StateAll && record.State != query.State) ||
			(query.Source != SourceAny && record.Source != query.Source) || (query.After != nil && !recordAfterSelection(record, *query.After, query.Selection)) {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(left, right int) bool {
		return recordBeforeSelection(filtered[right], filtered[left], query.Selection)
	})
	if len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}
	return append([]MemberRecord(nil), filtered...), nil
}
func newTestService(t *testing.T, store Store) (*Service, *testUnitOfWork) {
	t.Helper()
	unit := &testUnitOfWork{}
	codec, err := newCursorCodec(bytes.Repeat([]byte("c"), 32), &incrementingReader{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(unit, store, codec)
	if err != nil {
		t.Fatal(err)
	}
	return service, unit
}

func TestMetadataResponsesAreClosedAndReadOnly(t *testing.T) {
	store := &memoryStore{exists: true}
	service, unit := newTestService(t, store)
	access, err := service.Access(context.Background(), 71)
	if err != nil || access.ProductID != 71 || !access.CanView || !access.CanQuery || access.CanManageViews || access.CanShare {
		t.Fatalf("access=%+v error=%v", access, err)
	}
	schema, err := service.Schema(context.Background(), 71)
	if err != nil || schema.ServiceProductID != 71 || len(schema.Columns) != 12 {
		t.Fatalf("schema=%+v error=%v", schema, err)
	}
	views, err := service.MemberViews(context.Background(), 71)
	if err != nil || views.ProductID != 71 || len(views.Views) != 1 || views.Views[0].ID != "default" || !views.Views[0].ReadOnly {
		t.Fatalf("views=%+v error=%v", views, err)
	}
	if unit.calls != 3 || store.queryCalls != 0 {
		t.Fatalf("uow/query calls=%d/%d", unit.calls, store.queryCalls)
	}
	schema.Columns[0].Key = "mutated"
	fresh, err := service.Schema(context.Background(), 71)
	if err != nil || fresh.Columns[0].Key != "member_ref" {
		t.Fatalf("schema mutable=%+v error=%v", fresh, err)
	}
}

func TestQueryCanonicalPaginationFiltersAndBoundCursor(t *testing.T) {
	stamp := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	older := stamp.Add(-time.Minute)
	store := &memoryStore{exists: true, records: []MemberRecord{
		memberRecord("spm_0000000000000000000004", 90, StateActive, SourceManual, stamp, "四号"), memberRecord("spm_0000000000000000000003", 90, StateExpired, SourcePaidOrder, stamp, "三号"), memberRecord("spm_0000000000000000000002", 90, StateRemoved, SourceManual, stamp, "二号"), memberRecord("spm_0000000000000000000001", 90, StateActive, SourceManual, older, "一号"),
	}}
	service, _ := newTestService(t, store)
	first, err := service.Query(context.Background(), QueryInput{ProductID: 90, State: StateAll, Limit: 2})
	if err != nil || !first.HasMore || len(first.Rows) != 2 || first.Rows[0].MemberRef != "spm_0000000000000000000004" || first.Rows[1].MemberRef != "spm_0000000000000000000003" {
		t.Fatalf("first=%+v error=%v", first, err)
	}
	second, err := service.Query(context.Background(), QueryInput{ProductID: 90, State: StateAll, Limit: 2, Cursor: first.NextCursor})
	if err != nil || second.HasMore || len(second.Rows) != 2 || second.Rows[0].MemberRef != "spm_0000000000000000000002" || second.Rows[1].MemberRef != "spm_0000000000000000000001" {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	for _, testCase := range []struct {
		input QueryInput
		want  int
	}{{QueryInput{ProductID: 90, State: StateAll, Source: SourceManual, Limit: 50}, 3}, {QueryInput{ProductID: 90, State: StateExpired, Limit: 50}, 1}} {
		input := testCase.input
		response, queryErr := service.Query(context.Background(), input)
		if queryErr != nil || len(response.Rows) != testCase.want {
			t.Fatalf("input=%+v response=%+v error=%v", input, response, queryErr)
		}
	}
	for _, input := range []QueryInput{{ProductID: 90, State: StateActive, Limit: 2, Cursor: first.NextCursor}, {ProductID: 90, State: StateAll, Source: SourceManual, Limit: 2, Cursor: first.NextCursor}, {ProductID: 90, State: StateAll, Limit: 3, Cursor: first.NextCursor}} {
		if _, queryErr := service.Query(context.Background(), input); !errors.Is(queryErr, ErrInvalidCursor) {
			t.Fatalf("input=%+v error=%v", input, queryErr)
		}
	}
}

func TestSelectedQuerySupportsOnlyCanonicalSortGroupDefaultViewAndBoundCursor(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	active := memberRecord("spm_0000000000000000000001", 71, StateActive, SourceManual, stamp.Add(2*time.Hour), "有效成员")
	active.StartsAt = stamp
	expired := memberRecord("spm_0000000000000000000002", 71, StateExpired, SourcePaidOrder, stamp.Add(time.Hour), "过期成员")
	expired.StartsAt = stamp.Add(3 * time.Hour)
	expiredAt := stamp.Add(3 * time.Hour)
	expired.ExpiredAt = &expiredAt
	removed := memberRecord("spm_0000000000000000000003", 71, StateRemoved, SourceManual, stamp.Add(3*time.Hour), "移除成员")
	removed.StartsAt = stamp.Add(2 * time.Hour)
	store := &memoryStore{exists: true, records: []MemberRecord{active, expired, removed}}
	service, _ := newTestService(t, store)

	byStarts, err := service.querySelected(context.Background(), QueryInput{ProductID: 71, State: StateAll, Limit: 2}, querySelection{Sort: querySortStartsAtDesc})
	if err != nil || !byStarts.HasMore || len(byStarts.Rows) != 2 || byStarts.Rows[0].MemberRef != expired.MemberRef || byStarts.Rows[1].MemberRef != removed.MemberRef || !strings.HasPrefix(byStarts.NextCursor, selectedCursorPrefix) {
		t.Fatalf("start sort response=%+v error=%v", byStarts, err)
	}
	secondByStarts, err := service.querySelected(context.Background(), QueryInput{ProductID: 71, State: StateAll, Limit: 2, Cursor: byStarts.NextCursor}, querySelection{Sort: querySortStartsAtDesc})
	if err != nil || secondByStarts.HasMore || len(secondByStarts.Rows) != 1 || secondByStarts.Rows[0].MemberRef != active.MemberRef {
		t.Fatalf("start sort continuation=%+v error=%v", secondByStarts, err)
	}

	byState, err := service.querySelected(context.Background(), QueryInput{ProductID: 71, State: StateAll, Limit: 2}, querySelection{GroupBy: queryGroupState})
	if err != nil || !byState.HasMore || len(byState.Rows) != 2 || byState.Rows[0].State != string(StateActive) || byState.Rows[1].State != string(StateExpired) {
		t.Fatalf("state group response=%+v error=%v", byState, err)
	}
	secondByState, err := service.querySelected(context.Background(), QueryInput{ProductID: 71, State: StateAll, Limit: 2, Cursor: byState.NextCursor}, querySelection{GroupBy: queryGroupState})
	if err != nil || secondByState.HasMore || len(secondByState.Rows) != 1 || secondByState.Rows[0].State != string(StateRemoved) {
		t.Fatalf("state group continuation=%+v error=%v", secondByState, err)
	}

	defaultView, err := service.querySelected(context.Background(), QueryInput{ProductID: 71, State: StateAll, Limit: 50}, querySelection{ViewID: "default"})
	if err != nil || len(defaultView.Rows) != 3 || defaultView.Rows[0].MemberRef != removed.MemberRef {
		t.Fatalf("default view=%+v error=%v", defaultView, err)
	}
	for _, testCase := range []struct {
		name      string
		input     QueryInput
		selection querySelection
	}{
		{"legacy saved view", QueryInput{ProductID: 71, State: StateAll, Limit: 2}, querySelection{ViewID: "9"}},
		{"default with filter", QueryInput{ProductID: 71, State: StateActive, Limit: 2}, querySelection{ViewID: "default"}},
		{"unknown sort", QueryInput{ProductID: 71, State: StateAll, Limit: 2}, querySelection{Sort: "granted_at_desc"}},
		{"unknown group", QueryInput{ProductID: 71, State: StateAll, Limit: 2}, querySelection{GroupBy: "source"}},
		{"cursor selection mismatch", QueryInput{ProductID: 71, State: StateAll, Limit: 2, Cursor: byStarts.NextCursor}, querySelection{GroupBy: queryGroupState}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, queryErr := service.querySelected(context.Background(), testCase.input, testCase.selection); !errors.Is(queryErr, ErrInvalidQuery) && !errors.Is(queryErr, ErrInvalidCursor) {
				t.Fatalf("input=%+v selection=%+v error=%v", testCase.input, testCase.selection, queryErr)
			}
		})
	}
}

func TestSelectedQueryDoesNotFallBackToLegacyStore(t *testing.T) {
	store := &legacyOnlyStore{}
	service, _ := newTestService(t, store)
	if _, err := service.querySelected(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 1}, querySelection{}); !errors.Is(err, ErrUnavailable) || store.queryCalls != 0 {
		t.Fatalf("error/query calls=%v/%d", err, store.queryCalls)
	}
}

func TestServiceFailsClosedForInvalidStoreFacts(t *testing.T) {
	stamp := time.Now().UTC()
	good := memberRecord("spm_0000000000000000000001", 1, StateActive, SourceManual, stamp, "x")
	for name, records := range map[string][]MemberRecord{
		"wrong product": {memberRecord("spm_0000000000000000000002", 2, StateActive, SourceManual, stamp, "x")}, "duplicate member": {good, good}, "bad order": {memberRecord("spm_0000000000000000000002", 1, StateActive, SourceManual, stamp.Add(-time.Minute), "x"), memberRecord("spm_0000000000000000000003", 1, StateActive, SourceManual, stamp, "x")}, "invalid ref": {{ServiceProductID: 1, CustomerID: 1, State: StateActive, Source: SourceManual, StartsAt: stamp, Version: 1, UpdatedAt: stamp, DisplayName: "x"}},
	} {
		t.Run(name, func(t *testing.T) {
			service, _ := newTestService(t, &memoryStore{exists: true, invalidResult: records})
			if _, err := service.Query(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 10}); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	service, _ := newTestService(t, &memoryStore{exists: true})
	for _, input := range []QueryInput{{ProductID: 0, State: StateAll, Limit: 1}, {ProductID: 1, State: "pending", Limit: 1}, {ProductID: 1, State: StateAll, Source: "all", Limit: 1}, {ProductID: 1, State: StateAll, Limit: 51}} {
		if _, err := service.Query(context.Background(), input); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("input=%+v error=%v", input, err)
		}
	}
}

func TestValidRecordsRejectsInvalidCanonicalLifecycle(t *testing.T) {
	startsAt := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	updatedAt := startsAt.Add(time.Hour)
	beforeStarts := startsAt.Add(-time.Second)
	base := memberRecord("spm_0000000000000000000001", 1, StateActive, SourceManual, updatedAt, "客户")
	for _, testCase := range []struct {
		name   string
		mutate func(*MemberRecord)
	}{
		{"illegal source", func(record *MemberRecord) { record.Source = "provider" }},
		{"expires before starts", func(record *MemberRecord) { record.ExpiresAt = &beforeStarts }},
		{"active with expired", func(record *MemberRecord) { record.ExpiredAt = &updatedAt }},
		{"active with removed", func(record *MemberRecord) { record.RemovedAt = &updatedAt }},
		{"expired missing expired at", func(record *MemberRecord) { record.State = StateExpired }},
		{"expired before starts", func(record *MemberRecord) { record.State = StateExpired; record.ExpiredAt = &beforeStarts }},
		{"expired with removed", func(record *MemberRecord) {
			record.State = StateExpired
			record.ExpiredAt = &updatedAt
			record.RemovedAt = &updatedAt
		}},
		{"removed missing removed at", func(record *MemberRecord) { record.State = StateRemoved }},
		{"removed before starts", func(record *MemberRecord) { record.State = StateRemoved; record.RemovedAt = &beforeStarts }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			record := base
			testCase.mutate(&record)
			if validRecords([]MemberRecord{record}, QueryInput{ProductID: 1, State: StateAll, Limit: 1}, nil) {
				t.Fatalf("accepted record=%+v", record)
			}
		})
	}
	removed := base
	removed.State = StateRemoved
	removed.ExpiredAt = &updatedAt
	removed.RemovedAt = &updatedAt
	if !validRecords([]MemberRecord{removed}, QueryInput{ProductID: 1, State: StateAll, Limit: 1}, nil) {
		t.Fatalf("removed member lost allowed expired_at=%+v", removed)
	}
}

func TestServiceMapsNotFoundAndDatabaseFailures(t *testing.T) {
	service, _ := newTestService(t, &memoryStore{exists: false})
	if _, err := service.Query(context.Background(), QueryInput{ProductID: 404, State: StateAll, Limit: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error=%v", err)
	}
	service, _ = newTestService(t, &memoryStore{exists: true, queryErr: errors.New("db")})
	if _, err := service.Query(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error=%v", err)
	}
}

func memberRecord(memberRef string, productID int64, state StateFilter, source SourceFilter, updatedAt time.Time, name string) MemberRecord {
	record := MemberRecord{MemberRef: memberRef, ServiceProductID: productID, CustomerID: 1, State: state, Source: source, StartsAt: updatedAt.Add(-time.Hour), Version: 1, UpdatedAt: updatedAt, DisplayName: name}
	if state == StateExpired {
		value := updatedAt
		record.ExpiredAt = &value
	}
	if state == StateRemoved {
		value := updatedAt
		record.RemovedAt = &value
	}
	return record
}

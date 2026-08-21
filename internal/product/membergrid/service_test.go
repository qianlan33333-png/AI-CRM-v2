package membergrid

import (
	"bytes"
	"context"
	"errors"
	"sort"
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
		if record.ProductID != query.ProductID || (query.State != StateAll && record.State != query.State) {
			continue
		}
		if query.After != nil && !positionBefore(record, *query.After) {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(left, right int) bool {
		if filtered[left].GrantedAt.Equal(filtered[right].GrantedAt) {
			return filtered[left].EntitlementID > filtered[right].EntitlementID
		}
		return filtered[left].GrantedAt.After(filtered[right].GrantedAt)
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
	if err != nil {
		t.Fatal(err)
	}
	if access.ProductID != 71 || !access.CanView || !access.CanQuery || access.CanManageViews || access.CanShare {
		t.Fatalf("access=%+v", access)
	}
	schema, err := service.Schema(context.Background(), 71)
	if err != nil {
		t.Fatal(err)
	}
	if schema.ProductID != 71 || len(schema.Columns) != 8 {
		t.Fatalf("schema=%+v", schema)
	}
	views, err := service.MemberViews(context.Background(), 71)
	if err != nil {
		t.Fatal(err)
	}
	if len(views.Views) != 1 || views.Views[0].ID != "default" || views.Views[0].Source != "built_in" || !views.Views[0].ReadOnly {
		t.Fatalf("views=%+v", views)
	}
	if unit.calls != 3 || store.queryCalls != 0 {
		t.Fatalf("uow/query calls=%d/%d", unit.calls, store.queryCalls)
	}

	schema.Columns[0].Key = "mutated"
	fresh, err := service.Schema(context.Background(), 71)
	if err != nil || fresh.Columns[0].Key != "entitlement_id" {
		t.Fatalf("schema mutable across calls: %+v error=%v", fresh, err)
	}
}

func TestQueryPaginatesWithStableTimestampAndIDKeyset(t *testing.T) {
	stamp := time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC)
	older := stamp.Add(-time.Minute)
	store := &memoryStore{exists: true, records: []MemberRecord{
		activeRecord(5, 90, stamp, "五号"),
		activeRecord(4, 90, stamp, "四号"),
		activeRecord(3, 90, stamp, "三号"),
		activeRecord(2, 90, older, "二号"),
	}}
	service, _ := newTestService(t, store)

	first, err := service.Query(context.Background(), QueryInput{ProductID: 90, State: StateAll, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" || len(first.Rows) != 2 ||
		first.Rows[0].EntitlementID != 5 || first.Rows[1].EntitlementID != 4 {
		t.Fatalf("first=%+v", first)
	}
	if store.lastQuery.Limit != 3 || store.lastQuery.After != nil {
		t.Fatalf("first store query=%+v", store.lastQuery)
	}

	second, err := service.Query(context.Background(), QueryInput{
		ProductID: 90, State: StateAll, Limit: 2, Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Rows) != 2 ||
		second.Rows[0].EntitlementID != 3 || second.Rows[1].EntitlementID != 2 {
		t.Fatalf("second=%+v", second)
	}
	seen := map[int64]bool{}
	for _, page := range [][]MemberRow{first.Rows, second.Rows} {
		for _, row := range page {
			if seen[row.EntitlementID] {
				t.Fatalf("duplicate entitlement %d", row.EntitlementID)
			}
			seen[row.EntitlementID] = true
		}
	}
	if len(seen) != 4 {
		t.Fatalf("pagination omitted rows: %v", seen)
	}
}

func TestQueryStateFilterAndEmptyState(t *testing.T) {
	stamp := time.Now().UTC()
	revokedAt := stamp.Add(time.Hour)
	store := &memoryStore{exists: true, records: []MemberRecord{
		activeRecord(3, 8, stamp, "active"),
		{EntitlementID: 2, ProductID: 8, State: StateRevoked, Version: 2, GrantedAt: stamp.Add(-time.Minute), RevokedAt: &revokedAt, DisplayName: "revoked"},
	}}
	service, _ := newTestService(t, store)
	response, err := service.Query(context.Background(), QueryInput{ProductID: 8, State: StateRevoked, Limit: 50})
	if err != nil || len(response.Rows) != 1 || response.Rows[0].State != "revoked" {
		t.Fatalf("response=%+v error=%v", response, err)
	}

	emptyStore := &memoryStore{exists: true}
	emptyService, _ := newTestService(t, emptyStore)
	empty, err := emptyService.Query(context.Background(), QueryInput{ProductID: 8, State: StateAll, Limit: 10})
	if err != nil || empty.Rows == nil || len(empty.Rows) != 0 || empty.HasMore || empty.NextCursor != "" {
		t.Fatalf("empty=%+v error=%v", empty, err)
	}
}

func TestServiceFailClosedForInvalidInputCursorAndStoreFacts(t *testing.T) {
	store := &memoryStore{exists: true}
	service, _ := newTestService(t, store)
	invalidInputs := []QueryInput{
		{ProductID: 0, State: StateAll, Limit: 1},
		{ProductID: 1, State: "pending", Limit: 1},
		{ProductID: 1, State: StateAll, Limit: 0},
		{ProductID: 1, State: StateAll, Limit: 51},
	}
	for _, input := range invalidInputs {
		if _, err := service.Query(context.Background(), input); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("Query(%+v) error=%v", input, err)
		}
	}
	if _, err := service.Query(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 1, Cursor: "1"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("plaintext cursor error=%v", err)
	}

	stamp := time.Now().UTC()
	cases := map[string][]MemberRecord{
		"wrong product": {activeRecord(3, 99, stamp, "x")},
		"duplicate entitlement": {
			activeRecord(3, 1, stamp, "x"),
			activeRecord(3, 1, stamp.Add(-time.Minute), "x"),
		},
		"wrong order": {
			activeRecord(2, 1, stamp.Add(-time.Minute), "x"),
			activeRecord(3, 1, stamp, "y"),
		},
		"all state row": {{EntitlementID: 1, ProductID: 1, State: StateAll, Version: 1, GrantedAt: stamp, DisplayName: "x"}},
		"active revoked": func() []MemberRecord {
			record := activeRecord(1, 1, stamp, "x")
			revoked := stamp
			record.RevokedAt = &revoked
			return []MemberRecord{record}
		}(),
		"raw mobile": func() []MemberRecord {
			record := activeRecord(1, 1, stamp, "x")
			raw := "13800138000"
			record.MaskedMobile = &raw
			return []MemberRecord{record}
		}(),
		"raw mobile with decorative mask": func() []MemberRecord {
			record := activeRecord(1, 1, stamp, "x")
			raw := "13800138000***"
			record.MaskedMobile = &raw
			return []MemberRecord{record}
		}(),
	}
	for name, records := range cases {
		t.Run(name, func(t *testing.T) {
			badStore := &memoryStore{exists: true, invalidResult: records}
			badService, _ := newTestService(t, badStore)
			if _, err := badService.Query(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 10}); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestServiceMapsNotFoundAndDatabaseFailures(t *testing.T) {
	notFound := &memoryStore{exists: false}
	service, _ := newTestService(t, notFound)
	if _, err := service.Access(context.Background(), 404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Access error=%v", err)
	}
	if _, err := service.Query(context.Background(), QueryInput{ProductID: 404, State: StateAll, Limit: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Query error=%v", err)
	}
	if notFound.queryCalls != 0 {
		t.Fatalf("queried members for missing product: %d", notFound.queryCalls)
	}

	databaseError := errors.New("database unavailable")
	for name, failing := range []*memoryStore{
		{existsErr: databaseError},
		{exists: true, queryErr: databaseError},
	} {
		failedService, _ := newTestService(t, failing)
		if _, err := failedService.Query(context.Background(), QueryInput{ProductID: 1, State: StateAll, Limit: 1}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("failure %d error=%v", name, err)
		}
	}

	unitFailureStore := &memoryStore{exists: true}
	unitFailureService, unit := newTestService(t, unitFailureStore)
	unit.err = databaseError
	if _, err := unitFailureService.Schema(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("uow failure error=%v", err)
	}
}

func activeRecord(id, productID int64, grantedAt time.Time, name string) MemberRecord {
	return MemberRecord{
		EntitlementID: id, ProductID: productID, State: StateActive,
		Version: 1, GrantedAt: grantedAt, DisplayName: name,
	}
}

package membergrid

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type fakeSQLRow struct {
	values []any
	err    error
}

func (row fakeSQLRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	return assignValues(destinations, row.values)
}

type fakeSQLRows struct {
	rows     [][]any
	position int
	scanErr  error
	rowsErr  error
	closed   bool
}

func (rows *fakeSQLRows) Next() bool { return rows.position < len(rows.rows) }
func (rows *fakeSQLRows) Scan(destinations ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	values := rows.rows[rows.position]
	rows.position++
	return assignValues(destinations, values)
}
func (rows *fakeSQLRows) Err() error { return rows.rowsErr }
func (rows *fakeSQLRows) Close()     { rows.closed = true }

type fakeSQLExecutor struct {
	row          fakeSQLRow
	rows         *fakeSQLRows
	queryErr     error
	queryRowSQL  string
	queryRowArgs []any
	querySQL     string
	queryArgs    []any
}

func (executor *fakeSQLExecutor) QueryRow(_ context.Context, sql string, arguments ...any) sqlRow {
	executor.queryRowSQL = sql
	executor.queryRowArgs = append([]any(nil), arguments...)
	return executor.row
}
func (executor *fakeSQLExecutor) Query(_ context.Context, sql string, arguments ...any) (sqlRows, error) {
	executor.querySQL = sql
	executor.queryArgs = append([]any(nil), arguments...)
	if executor.queryErr != nil {
		return nil, executor.queryErr
	}
	return executor.rows, nil
}
func repositoryForExecutor(executor sqlExecutor) *Repository {
	return &Repository{executor: func(context.Context) (sqlExecutor, error) { return executor, nil }}
}

type fakeShareQueries struct {
	currentRow productdb.CurrentMemberGridExternalShareRow
	currentErr error
	currentID  int64
	setRow     productdb.SetMemberGridExternalShareRow
	setErr     error
	setParams  productdb.SetMemberGridExternalShareParams
	lookupRow  productdb.LookupEnabledMemberGridExternalShareRow
	lookupErr  error
	lookupID   string
	summary    []productdb.SummarizePublicServicePeriodMembersRow
	summaryErr error
	summaryID  int64
	first      []productdb.ListPublicServicePeriodMembersFirstPageRow
	firstErr   error
	firstArgs  productdb.ListPublicServicePeriodMembersFirstPageParams
	after      []productdb.ListPublicServicePeriodMembersAfterRow
	afterErr   error
	afterArgs  productdb.ListPublicServicePeriodMembersAfterParams
}

func (queries *fakeShareQueries) CurrentMemberGridExternalShare(_ context.Context, id int64) (productdb.CurrentMemberGridExternalShareRow, error) {
	queries.currentID = id
	return queries.currentRow, queries.currentErr
}

func (queries *fakeShareQueries) SetMemberGridExternalShare(_ context.Context, params productdb.SetMemberGridExternalShareParams) (productdb.SetMemberGridExternalShareRow, error) {
	queries.setParams = params
	return queries.setRow, queries.setErr
}

func (queries *fakeShareQueries) LookupEnabledMemberGridExternalShare(_ context.Context, id string) (productdb.LookupEnabledMemberGridExternalShareRow, error) {
	queries.lookupID = id
	return queries.lookupRow, queries.lookupErr
}

func (queries *fakeShareQueries) SummarizePublicServicePeriodMembers(_ context.Context, id int64) ([]productdb.SummarizePublicServicePeriodMembersRow, error) {
	queries.summaryID = id
	return queries.summary, queries.summaryErr
}

func (queries *fakeShareQueries) ListPublicServicePeriodMembersFirstPage(_ context.Context, params productdb.ListPublicServicePeriodMembersFirstPageParams) ([]productdb.ListPublicServicePeriodMembersFirstPageRow, error) {
	queries.firstArgs = params
	return queries.first, queries.firstErr
}

func (queries *fakeShareQueries) ListPublicServicePeriodMembersAfter(_ context.Context, params productdb.ListPublicServicePeriodMembersAfterParams) ([]productdb.ListPublicServicePeriodMembersAfterRow, error) {
	queries.afterArgs = params
	return queries.after, queries.afterErr
}

func repositoryForShareQueries(queries shareQueries) *Repository {
	return &Repository{shareQueries: func(context.Context) (shareQueries, error) { return queries, nil }}
}

func canonicalScanRow(memberRef string, productID, customerID int64, state, source string, stamp time.Time, name string) []any {
	return []any{memberRef, productID, customerID, state, source, stamp.Add(-time.Hour), time.Unix(0, 0).UTC(), false, time.Unix(0, 0).UTC(), false, time.Unix(0, 0).UTC(), false, int64(1), stamp, name}
}
func TestRepositoryReadsCanonicalMemberAndExactCustomerRelationship(t *testing.T) {
	stamp := time.Date(2026, 8, 24, 1, 2, 3, 0, time.UTC)
	executor := &fakeSQLExecutor{row: fakeSQLRow{values: []any{true}}, rows: &fakeSQLRows{rows: [][]any{canonicalScanRow("spm_0000000000000000000001", 8, 12, "active", "manual", stamp, "精确客户")}}}
	repository := repositoryForExecutor(executor)
	exists, err := repository.ProductExists(context.Background(), 8)
	if err != nil || !exists || executor.queryRowSQL != productExistsSQL || !reflect.DeepEqual(executor.queryRowArgs, []any{int64(8)}) {
		t.Fatalf("exists/error/sql/args=%v/%v/%q/%v", exists, err, executor.queryRowSQL, executor.queryRowArgs)
	}
	members, err := repository.QueryMembers(context.Background(), StoreQuery{ProductID: 8, State: StateAll, Source: SourceAny, Limit: 10})
	if err != nil || len(members) != 1 {
		t.Fatalf("members=%+v error=%v", members, err)
	}
	member := members[0]
	if member.MemberRef != "spm_0000000000000000000001" || member.ServiceProductID != 8 || member.CustomerID != 12 || member.Source != SourceManual || member.DisplayName != "精确客户" || member.ExpiredAt != nil || member.RemovedAt != nil {
		t.Fatalf("member=%+v", member)
	}
	if executor.querySQL != firstPageSQL || !reflect.DeepEqual(executor.queryArgs, []any{int64(8), "all", "", 10}) || !executor.rows.closed {
		t.Fatalf("sql/args/closed=%q/%v/%v", executor.querySQL, executor.queryArgs, executor.rows.closed)
	}
}
func TestRepositoryUsesUpdatedAtMemberRefKeyset(t *testing.T) {
	stamp := time.Now().UTC()
	removed := stamp.Add(-time.Minute)
	values := canonicalScanRow("spm_0000000000000000000004", 3, 2, "removed", "paid_order", stamp, "已移除")
	values[10] = removed
	values[11] = true
	executor := &fakeSQLExecutor{rows: &fakeSQLRows{rows: [][]any{values}}}
	repository := repositoryForExecutor(executor)
	members, err := repository.QueryMembers(context.Background(), StoreQuery{ProductID: 3, State: StateRemoved, Source: SourcePaidOrder, Limit: 2, After: &Position{UpdatedAt: stamp, MemberRef: "spm_0000000000000000000005"}})
	if err != nil || len(members) != 1 || members[0].RemovedAt == nil || !members[0].RemovedAt.Equal(removed) {
		t.Fatalf("members=%+v error=%v", members, err)
	}
	want := []any{int64(3), "removed", "paid_order", stamp, "spm_0000000000000000000005", 2}
	if executor.querySQL != afterPageSQL || !reflect.DeepEqual(executor.queryArgs, want) {
		t.Fatalf("sql/args=%q/%v want=%q/%v", executor.querySQL, executor.queryArgs, afterPageSQL, want)
	}
}
func TestRepositoryMapsDatabaseFailures(t *testing.T) {
	databaseError := errors.New("database failed")
	cases := map[string]*fakeSQLExecutor{"exists scan": {row: fakeSQLRow{err: databaseError}}, "query": {queryErr: databaseError}, "row scan": {rows: &fakeSQLRows{rows: [][]any{{}}, scanErr: databaseError}}, "rows": {rows: &fakeSQLRows{rowsErr: databaseError}}}
	for name, executor := range cases {
		t.Run(name, func(t *testing.T) {
			repository := repositoryForExecutor(executor)
			var err error
			if name == "exists scan" {
				_, err = repository.ProductExists(context.Background(), 1)
			} else {
				_, err = repository.QueryMembers(context.Background(), StoreQuery{ProductID: 1, State: StateAll, Source: SourceAny, Limit: 1})
			}
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	providerFailure := &Repository{executor: func(context.Context) (sqlExecutor, error) { return nil, databaseError }}
	if _, err := providerFailure.ProductExists(context.Background(), 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("provider error=%v", err)
	}
}
func assignValues(destinations, values []any) error {
	if len(destinations) != len(values) {
		return errors.New("scan arity mismatch")
	}
	for index := range destinations {
		destination := reflect.ValueOf(destinations[index])
		if destination.Kind() != reflect.Pointer || destination.IsNil() {
			return errors.New("scan destination is not a pointer")
		}
		value := reflect.ValueOf(values[index])
		if !value.IsValid() || !value.Type().AssignableTo(destination.Elem().Type()) {
			return errors.New("scan type mismatch")
		}
		destination.Elem().Set(value)
	}
	return nil
}

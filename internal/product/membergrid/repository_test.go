package membergrid

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
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

func (rows *fakeSQLRows) Next() bool {
	return rows.position < len(rows.rows)
}

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

func TestRepositoryReadsExactProductCustomerRelationship(t *testing.T) {
	stamp := time.Date(2026, 8, 21, 1, 2, 3, 0, time.UTC)
	executor := &fakeSQLExecutor{
		row: fakeSQLRow{values: []any{true}},
		rows: &fakeSQLRows{rows: [][]any{
			{int64(12), int64(8), "active", int64(1), stamp, time.Unix(0, 0).UTC(), false, "精确客户"},
		}},
	}
	repository := repositoryForExecutor(executor)
	exists, err := repository.ProductExists(context.Background(), 8)
	if err != nil || !exists || executor.queryRowSQL != productExistsSQL || !reflect.DeepEqual(executor.queryRowArgs, []any{int64(8)}) {
		t.Fatalf("exists/error/sql/args=%v/%v/%q/%v", exists, err, executor.queryRowSQL, executor.queryRowArgs)
	}
	members, err := repository.QueryMembers(context.Background(), StoreQuery{ProductID: 8, State: StateAll, Limit: 10})
	if err != nil || len(members) != 1 {
		t.Fatalf("members=%+v error=%v", members, err)
	}
	member := members[0]
	if member.EntitlementID != 12 || member.ProductID != 8 || member.DisplayName != "精确客户" || member.MaskedMobile != nil || member.RevokedAt != nil {
		t.Fatalf("member=%+v", member)
	}
	if executor.querySQL != firstPageSQL || !reflect.DeepEqual(executor.queryArgs, []any{int64(8), "all", 10}) || !executor.rows.closed {
		t.Fatalf("sql/args/closed=%q/%v/%v", executor.querySQL, executor.queryArgs, executor.rows.closed)
	}
}

func TestRepositoryUsesCompositeKeysetWithoutOffset(t *testing.T) {
	stamp := time.Now().UTC()
	revoked := stamp.Add(time.Hour)
	executor := &fakeSQLExecutor{rows: &fakeSQLRows{rows: [][]any{
		{int64(4), int64(3), "revoked", int64(2), stamp.Add(-time.Minute), revoked, true, "已撤销"},
	}}}
	repository := repositoryForExecutor(executor)
	members, err := repository.QueryMembers(context.Background(), StoreQuery{
		ProductID: 3, State: StateRevoked, Limit: 2,
		After: &Position{GrantedAt: stamp, EntitlementID: 5},
	})
	if err != nil || len(members) != 1 || members[0].RevokedAt == nil || !members[0].RevokedAt.Equal(revoked) {
		t.Fatalf("members=%+v error=%v", members, err)
	}
	wantArguments := []any{int64(3), "revoked", stamp, int64(5), 2}
	if executor.querySQL != afterPageSQL || !reflect.DeepEqual(executor.queryArgs, wantArguments) {
		t.Fatalf("sql/args=%q/%v want=%q/%v", executor.querySQL, executor.queryArgs, afterPageSQL, wantArguments)
	}
}

func TestRepositoryMapsDatabaseFailures(t *testing.T) {
	databaseError := errors.New("database failed")
	cases := map[string]*fakeSQLExecutor{
		"exists scan": {row: fakeSQLRow{err: databaseError}},
		"query":       {queryErr: databaseError},
		"row scan":    {rows: &fakeSQLRows{rows: [][]any{{}}, scanErr: databaseError}},
		"rows":        {rows: &fakeSQLRows{rowsErr: databaseError}},
	}
	for name, executor := range cases {
		t.Run(name, func(t *testing.T) {
			repository := repositoryForExecutor(executor)
			var err error
			if name == "exists scan" {
				_, err = repository.ProductExists(context.Background(), 1)
			} else {
				_, err = repository.QueryMembers(context.Background(), StoreQuery{ProductID: 1, State: StateAll, Limit: 1})
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

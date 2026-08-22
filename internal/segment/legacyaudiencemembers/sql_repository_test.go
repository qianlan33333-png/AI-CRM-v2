package legacyaudiencemembers

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type sqlCall struct {
	query string
	args  []any
}

type fakeSQLProvider struct {
	reader SQLReader
	err    error
	calls  int
}

func (provider *fakeSQLProvider) Reader(context.Context) (SQLReader, error) {
	provider.calls++
	return provider.reader, provider.err
}

type fakeSQLReader struct {
	rowsQueue []SQLRow
	rowIndex  int
	rows      SQLRows
	queryErr  error
	rowCalls  []sqlCall
	queries   []sqlCall
}

func (reader *fakeSQLReader) QueryRow(_ context.Context, query string, args ...any) SQLRow {
	reader.rowCalls = append(reader.rowCalls, sqlCall{query: query, args: append([]any(nil), args...)})
	if reader.rowIndex >= len(reader.rowsQueue) {
		return valueRow{err: errors.New("unexpected QueryRow")}
	}
	row := reader.rowsQueue[reader.rowIndex]
	reader.rowIndex++
	return row
}

func (reader *fakeSQLReader) Query(_ context.Context, query string, args ...any) (SQLRows, error) {
	reader.queries = append(reader.queries, sqlCall{query: query, args: append([]any(nil), args...)})
	return reader.rows, reader.queryErr
}

type valueRow struct {
	values []any
	err    error
}

func (row valueRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	return assignValues(destinations, row.values)
}

type valueRows struct {
	rows      [][]any
	index     int
	scanErrAt int
	err       error
	closed    bool
}

func (rows *valueRows) Next() bool { return rows.index < len(rows.rows) }

func (rows *valueRows) Scan(destinations ...any) error {
	if rows.scanErrAt >= 0 && rows.index == rows.scanErrAt {
		rows.index++
		return errors.New("scan failed")
	}
	values := rows.rows[rows.index]
	rows.index++
	return assignValues(destinations, values)
}

func (rows *valueRows) Err() error { return rows.err }
func (rows *valueRows) Close()     { rows.closed = true }

func assignValues(destinations, values []any) error {
	if len(destinations) != len(values) {
		return errors.New("scan arity mismatch")
	}
	for index := range values {
		destination := reflect.ValueOf(destinations[index])
		if destination.Kind() != reflect.Pointer || destination.IsNil() {
			return errors.New("scan destination must be a pointer")
		}
		value := reflect.ValueOf(values[index])
		if !value.Type().AssignableTo(destination.Elem().Type()) {
			if value.Type().ConvertibleTo(destination.Elem().Type()) {
				value = value.Convert(destination.Elem().Type())
			} else {
				return errors.New("scan type mismatch")
			}
		}
		destination.Elem().Set(value)
	}
	return nil
}

func TestSQLRepositoryPackageExistsRequiresSegmentMetadataJoin(t *testing.T) {
	t.Parallel()
	reader := &fakeSQLReader{rowsQueue: []SQLRow{valueRow{values: []any{true}}}}
	repository, err := NewSQLRepository(&fakeSQLProvider{reader: reader})
	if err != nil {
		t.Fatal(err)
	}
	exists, err := repository.PackageExists(context.Background(), 17)
	if err != nil || !exists {
		t.Fatalf("PackageExists() = %v, %v", exists, err)
	}
	if len(reader.rowCalls) != 1 || !strings.Contains(reader.rowCalls[0].query, "JOIN public.ai_audience_package_metadata") ||
		!reflect.DeepEqual(reader.rowCalls[0].args, []any{int64(17)}) {
		t.Fatalf("existence query = %#v", reader.rowCalls)
	}
}

func TestSQLRepositoryListMembersUsesStablePageAndClosedColumns(t *testing.T) {
	t.Parallel()
	firstAt := time.Date(2026, 8, 22, 3, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(-time.Minute)
	rows := &valueRows{scanErrAt: -1, rows: [][]any{
		{int64(5), sql.NullInt64{Int64: 9, Valid: true}, sql.NullString{String: "Nine", Valid: true}, sql.NullTime{Time: firstAt, Valid: true}},
		{int64(5), sql.NullInt64{Int64: 7, Valid: true}, sql.NullString{String: "", Valid: true}, sql.NullTime{Time: secondAt, Valid: true}},
	}}
	reader := &fakeSQLReader{rows: rows}
	repository, _ := NewSQLRepository(&fakeSQLProvider{reader: reader})
	page, err := repository.ListMembers(context.Background(), 22, 2, 1)
	if err != nil {
		t.Fatalf("ListMembers() error = %v", err)
	}
	want := MemberPage{Total: 5, Items: []MemberRecord{
		{CustomerID: 9, Nickname: "Nine", EnteredAt: firstAt},
		{CustomerID: 7, Nickname: "", EnteredAt: secondAt},
	}}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("page = %#v, want %#v", page, want)
	}
	if !rows.closed {
		t.Fatal("rows were not closed")
	}
	if len(reader.rowCalls) != 0 {
		t.Fatalf("unexpected row queries = %#v", reader.rowCalls)
	}
	if len(reader.queries) != 1 {
		t.Fatalf("page queries = %#v", reader.queries)
	}
	query := reader.queries[0]
	for _, required := range []string{
		"member.customer_id", "customer.name", "member.computed_at",
		"JOIN public.customers", "ORDER BY member.computed_at DESC, member.customer_id DESC",
	} {
		if !strings.Contains(query.query, required) {
			t.Fatalf("page SQL missing %q: %s", required, query.query)
		}
	}
	for _, forbidden := range []string{"unionid", "mobile", "openid", "extra", "identities", "member_count"} {
		if strings.Contains(strings.ToLower(query.query), forbidden) {
			t.Fatalf("page SQL leaks or reads forbidden field %q: %s", forbidden, query.query)
		}
	}
	if !reflect.DeepEqual(query.args, []any{int64(22), 2, int64(1)}) {
		t.Fatalf("page args = %#v", query.args)
	}
}

func TestSQLRepositoryListMembersEmptyPageIsNonNil(t *testing.T) {
	t.Parallel()
	rows := &valueRows{scanErrAt: -1, rows: [][]any{{
		int64(0), sql.NullInt64{}, sql.NullString{}, sql.NullTime{},
	}}}
	reader := &fakeSQLReader{rows: rows}
	repository, _ := NewSQLRepository(&fakeSQLProvider{reader: reader})
	page, err := repository.ListMembers(context.Background(), 1, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("empty page = %#v", page)
	}
}

func TestSQLRepositoryErrorsFailClosed(t *testing.T) {
	t.Parallel()
	dependencyErr := errors.New("db failure")
	tests := []struct {
		name     string
		provider SQLProvider
		call     func(*SQLRepository) error
	}{
		{
			name:     "provider",
			provider: &fakeSQLProvider{err: dependencyErr},
			call: func(repository *SQLRepository) error {
				_, err := repository.PackageExists(context.Background(), 1)
				return err
			},
		},
		{
			name:     "existence_scan",
			provider: &fakeSQLProvider{reader: &fakeSQLReader{rowsQueue: []SQLRow{valueRow{err: dependencyErr}}}},
			call: func(repository *SQLRepository) error {
				_, err := repository.PackageExists(context.Background(), 1)
				return err
			},
		},
		{
			name:     "query",
			provider: &fakeSQLProvider{reader: &fakeSQLReader{queryErr: dependencyErr}},
			call: func(repository *SQLRepository) error {
				_, err := repository.ListMembers(context.Background(), 1, 1, 0)
				return err
			},
		},
		{
			name: "row_scan",
			provider: &fakeSQLProvider{reader: &fakeSQLReader{
				rows: &valueRows{rows: [][]any{{
					int64(1), sql.NullInt64{Int64: 1, Valid: true}, sql.NullString{String: "One", Valid: true}, sql.NullTime{Time: time.Now(), Valid: true},
				}}, scanErrAt: 0},
			}},
			call: func(repository *SQLRepository) error {
				_, err := repository.ListMembers(context.Background(), 1, 1, 0)
				return err
			},
		},
		{
			name: "rows_err",
			provider: &fakeSQLProvider{reader: &fakeSQLReader{
				rows: &valueRows{scanErrAt: -1, err: dependencyErr, rows: [][]any{{
					int64(0), sql.NullInt64{}, sql.NullString{}, sql.NullTime{},
				}}},
			}},
			call: func(repository *SQLRepository) error {
				_, err := repository.ListMembers(context.Background(), 1, 1, 0)
				return err
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repository, err := NewSQLRepository(test.provider)
			if err != nil {
				t.Fatal(err)
			}
			if err = test.call(repository); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestSQLRepositoryRejectsInvalidConstructionAndArguments(t *testing.T) {
	t.Parallel()
	if _, err := NewSQLRepository(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewSQLRepository(nil) error = %v", err)
	}
	repository, _ := NewSQLRepository(&fakeSQLProvider{reader: &fakeSQLReader{}})
	for _, input := range []ListInput{
		{PackageID: 0, Limit: 1},
		{PackageID: 1, Limit: 0},
		{PackageID: 1, Limit: MaximumLimit + 1},
		{PackageID: 1, Limit: 1, Offset: -1},
	} {
		if _, err := repository.ListMembers(context.Background(), input.PackageID, input.Limit, input.Offset); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("ListMembers(%#v) error = %v", input, err)
		}
	}
	if _, err := repository.PackageExists(context.Background(), 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("PackageExists(0) error = %v", err)
	}
}

package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
)

func TestChannelEntrantsRepositorySQLIsClosedStableAndIndexFriendly(t *testing.T) {
	t.Parallel()

	stateSQL := strings.ToLower(channelEntrantsChannelStateSQL)
	if !strings.Contains(stateSQL, "select ch.status") || !strings.Contains(stateSQL, "where ch.id = $1") {
		t.Fatalf("unexpected channel state SQL:\n%s", channelEntrantsChannelStateSQL)
	}
	for _, forbidden := range []string{"config", "legacy_projection", "provider", "wecom"} {
		if strings.Contains(stateSQL, forbidden) {
			t.Fatalf("channel state SQL reads forbidden %q:\n%s", forbidden, channelEntrantsChannelStateSQL)
		}
	}

	for name, statement := range map[string]string{
		"first": channelEntrantsFirstPageSQL,
		"after": channelEntrantsAfterPageSQL,
	} {
		lower := strings.ToLower(statement)
		for _, required := range []string{
			"c.id", "c.channel_id", "c.name", "c.added_at", "c.last_interact_at",
			"c.channel_id = $1", "c.is_deleted = false",
			"order by c.added_at desc, c.id desc", "limit $",
		} {
			if !strings.Contains(lower, required) {
				t.Fatalf("%s SQL is missing %q:\n%s", name, required, statement)
			}
		}
		for _, forbidden := range []string{
			"owner_staff_id", "customers.extra", "c.extra", "mobile", "unionid",
			"external_userid", "provider", "wecom", "config", "insert ", "update ", "delete ",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s SQL contains forbidden %q:\n%s", name, forbidden, statement)
			}
		}
	}
	if !strings.Contains(channelEntrantsAfterPageSQL, "(c.added_at, c.id) < ($2::timestamptz, $3::bigint)") {
		t.Fatalf("after SQL has no tuple keyset:\n%s", channelEntrantsAfterPageSQL)
	}
}

func TestChannelEntrantsRepositoryReadsOnlyChannelState(t *testing.T) {
	t.Parallel()

	executor := &channelEntrantsRepositoryFakeExecutor{
		row: &channelEntrantsRepositoryFakeRow{values: []any{"active"}},
	}
	repository := &ChannelEntrantsRepository{executor: func(context.Context) (channelEntrantsSQLExecutor, error) {
		return executor, nil
	}}
	state, err := repository.ReadChannelEntrantsChannelState(context.Background(), 81)
	if err != nil {
		t.Fatal(err)
	}
	if state != contactapp.ChannelEntrantsChannelActive {
		t.Fatalf("state=%q", state)
	}
	if executor.rowSQL != channelEntrantsChannelStateSQL || !reflect.DeepEqual(executor.rowArguments, []any{int64(81)}) {
		t.Fatalf("sql=%q args=%#v", executor.rowSQL, executor.rowArguments)
	}

	executor.row = &channelEntrantsRepositoryFakeRow{err: pgx.ErrNoRows}
	if _, err = repository.ReadChannelEntrantsChannelState(context.Background(), 82); !errors.Is(err, contactapp.ErrChannelEntrantsNotFound) {
		t.Fatalf("missing error=%v", err)
	}
	databaseErr := errors.New("database read failed")
	executor.row = &channelEntrantsRepositoryFakeRow{err: databaseErr}
	if _, err = repository.ReadChannelEntrantsChannelState(context.Background(), 83); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, databaseErr) {
		t.Fatalf("database error=%v", err)
	}
}

func TestChannelEntrantsRepositoryMapsFirstAndAfterPages(t *testing.T) {
	t.Parallel()

	added := time.Date(2026, 8, 22, 9, 0, 0, 456000000, time.FixedZone("source", 8*60*60))
	interacted := added.Add(time.Minute)
	firstRows := &channelEntrantsRepositoryFakeRows{rows: [][]any{
		{int64(31), int64(9), "甲", pgtype.Timestamptz{Time: added, Valid: true}, pgtype.Timestamptz{Time: interacted, Valid: true}},
		{int64(30), int64(9), "乙", pgtype.Timestamptz{Time: added, Valid: true}, pgtype.Timestamptz{}},
	}}
	executor := &channelEntrantsRepositoryFakeExecutor{rows: firstRows}
	repository := &ChannelEntrantsRepository{executor: func(context.Context) (channelEntrantsSQLExecutor, error) {
		return executor, nil
	}}

	records, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{
		ChannelID: 9,
		Limit:     3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].CustomerID != 31 || records[0].ChannelID != 9 ||
		records[0].DisplayName != "甲" || !records[0].AddedAt.Equal(added.UTC()) ||
		records[0].LastInteractAt == nil || !records[0].LastInteractAt.Equal(interacted.UTC()) ||
		records[1].LastInteractAt != nil {
		t.Fatalf("records=%#v", records)
	}
	if executor.querySQL != channelEntrantsFirstPageSQL ||
		!reflect.DeepEqual(executor.queryArguments, []any{int64(9), 3}) || !firstRows.closed {
		t.Fatalf("sql=%q args=%#v closed=%v", executor.querySQL, executor.queryArguments, firstRows.closed)
	}

	afterRows := &channelEntrantsRepositoryFakeRows{rows: [][]any{
		{int64(29), int64(9), "丙", pgtype.Timestamptz{Time: added.Add(-time.Second), Valid: true}, pgtype.Timestamptz{}},
	}}
	executor.rows = afterRows
	after := contactapp.ChannelEntrantsPosition{AddedAt: added, CustomerID: 30}
	records, err = repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{
		ChannelID: 9,
		Limit:     2,
		After:     &after,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].CustomerID != 29 {
		t.Fatalf("after records=%#v", records)
	}
	wantArguments := []any{int64(9), added.UTC(), int64(30), 2}
	if executor.querySQL != channelEntrantsAfterPageSQL || !reflect.DeepEqual(executor.queryArguments, wantArguments) || !afterRows.closed {
		t.Fatalf("sql=%q args=%#v want=%#v closed=%v", executor.querySQL, executor.queryArguments, wantArguments, afterRows.closed)
	}
}

func TestChannelEntrantsRepositoryPreservesMalformedTimestampForApplicationFailClosed(t *testing.T) {
	t.Parallel()

	rows := &channelEntrantsRepositoryFakeRows{rows: [][]any{
		{int64(4), int64(2), "malformed", pgtype.Timestamptz{}, pgtype.Timestamptz{}},
	}}
	repository := &ChannelEntrantsRepository{executor: func(context.Context) (channelEntrantsSQLExecutor, error) {
		return &channelEntrantsRepositoryFakeExecutor{rows: rows}, nil
	}}
	records, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || !records[0].AddedAt.IsZero() {
		t.Fatalf("records=%#v", records)
	}
}

func TestChannelEntrantsRepositoryFailsClosedOnInvalidInputsAndDatabaseFailures(t *testing.T) {
	t.Parallel()

	repository := NewChannelEntrantsRepository()
	if _, err := repository.ReadChannelEntrantsChannelState(nil, 1); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) {
		t.Fatalf("nil context state error=%v", err)
	}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) {
		t.Fatalf("invalid query error=%v", err)
	}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{
		ChannelID: 1, Limit: contactapp.ChannelEntrantsMaximumLimit + 2,
	}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) {
		t.Fatalf("oversized query error=%v", err)
	}

	executorErr := errors.New("transaction context missing")
	repository = &ChannelEntrantsRepository{executor: func(context.Context) (channelEntrantsSQLExecutor, error) {
		return nil, executorErr
	}}
	if _, err := repository.ReadChannelEntrantsChannelState(context.Background(), 1); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, executorErr) {
		t.Fatalf("executor state error=%v", err)
	}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 1, Limit: 1}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, executorErr) {
		t.Fatalf("executor list error=%v", err)
	}

	queryErr := errors.New("query failed")
	executor := &channelEntrantsRepositoryFakeExecutor{queryErr: queryErr}
	repository = &ChannelEntrantsRepository{executor: func(context.Context) (channelEntrantsSQLExecutor, error) { return executor, nil }}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 1, Limit: 1}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, queryErr) {
		t.Fatalf("query error=%v", err)
	}

	scanErr := errors.New("scan failed")
	executor.queryErr = nil
	executor.rows = &channelEntrantsRepositoryFakeRows{rows: [][]any{{int64(1)}}, scanErr: scanErr}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 1, Limit: 1}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, scanErr) {
		t.Fatalf("scan error=%v", err)
	}

	rowsErr := errors.New("rows failed")
	executor.rows = &channelEntrantsRepositoryFakeRows{terminalErr: rowsErr}
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 1, Limit: 1}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) || !errors.Is(err, rowsErr) {
		t.Fatalf("rows error=%v", err)
	}

	var typedNilRows *channelEntrantsRepositoryFakeRows
	executor.rows = typedNilRows
	if _, err := repository.ListChannelEntrants(context.Background(), contactapp.ChannelEntrantsStoreQuery{ChannelID: 1, Limit: 1}); !errors.Is(err, contactapp.ErrChannelEntrantsUnavailable) {
		t.Fatalf("typed nil rows error=%v", err)
	}
}

type channelEntrantsRepositoryFakeExecutor struct {
	row            channelEntrantsSQLRow
	rows           channelEntrantsSQLRows
	queryErr       error
	rowSQL         string
	rowArguments   []any
	querySQL       string
	queryArguments []any
}

func (executor *channelEntrantsRepositoryFakeExecutor) QueryRow(_ context.Context, sql string, arguments ...any) channelEntrantsSQLRow {
	executor.rowSQL = sql
	executor.rowArguments = append([]any(nil), arguments...)
	if executor.row == nil {
		return &channelEntrantsRepositoryFakeRow{err: errors.New("unexpected QueryRow")}
	}
	return executor.row
}

func (executor *channelEntrantsRepositoryFakeExecutor) Query(_ context.Context, sql string, arguments ...any) (channelEntrantsSQLRows, error) {
	executor.querySQL = sql
	executor.queryArguments = append([]any(nil), arguments...)
	return executor.rows, executor.queryErr
}

type channelEntrantsRepositoryFakeRow struct {
	values []any
	err    error
}

func (row *channelEntrantsRepositoryFakeRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	return channelEntrantsAssign(destinations, row.values)
}

type channelEntrantsRepositoryFakeRows struct {
	rows        [][]any
	index       int
	scanErr     error
	terminalErr error
	closed      bool
}

func (rows *channelEntrantsRepositoryFakeRows) Next() bool {
	if rows == nil || rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *channelEntrantsRepositoryFakeRows) Scan(destinations ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	if rows.index < 1 || rows.index > len(rows.rows) {
		return errors.New("scan called without current row")
	}
	return channelEntrantsAssign(destinations, rows.rows[rows.index-1])
}

func (rows *channelEntrantsRepositoryFakeRows) Err() error {
	return rows.terminalErr
}

func (rows *channelEntrantsRepositoryFakeRows) Close() {
	rows.closed = true
}

func channelEntrantsAssign(destinations []any, values []any) error {
	if len(destinations) != len(values) {
		return fmt.Errorf("scan destination/value mismatch: %d/%d", len(destinations), len(values))
	}
	for index := range destinations {
		destination := reflect.ValueOf(destinations[index])
		if destination.Kind() != reflect.Pointer || destination.IsNil() {
			return fmt.Errorf("destination %d is not a pointer", index)
		}
		value := reflect.ValueOf(values[index])
		if !value.IsValid() {
			destination.Elem().SetZero()
			continue
		}
		if !value.Type().AssignableTo(destination.Elem().Type()) {
			return fmt.Errorf("value %d type %s is not assignable to %s", index, value.Type(), destination.Elem().Type())
		}
		destination.Elem().Set(value)
	}
	return nil
}

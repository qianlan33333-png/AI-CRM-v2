package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestHistoricalChannelRelationsContactRoundTrip(t *testing.T) {
	input := historicalChannelContactFixture()
	customerID := int64(9007199254740993)
	input.CustomerID = &customerID
	stamp := pgtype.Timestamptz{Time: input.FirstEnteredAt, Valid: true}
	dbrow := contactdb.ChannelHistoricalContact{ID: 31, ChannelID: input.ChannelID, SourceContactID: input.SourceContactID,
		CustomerID: nullableInt64(input.CustomerID), OwnerReference: input.OwnerReference, EnterCount: input.EnterCount,
		FirstEnteredAt: stamp, LastEnteredAt: stamp, CreatedAt: stamp, UpdatedAt: stamp}
	tx := &channelRelationsTestTx{row: channelRelationsTestRow{values: channelContactValues(dbrow)}}
	store := &HistoricalChannelRelationsStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	got, err := store.CreateHistoricalChannelContact(context.Background(), input)
	want := input
	want.ID = dbrow.ID
	want.FirstEnteredAt = input.FirstEnteredAt.UTC().Truncate(time.Microsecond)
	want.LastEnteredAt, want.CreatedAt, want.UpdatedAt = want.FirstEnteredAt, want.FirstEnteredAt, want.FirstEnteredAt
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("contact mapping: %v", err)
	}
	stamp.Time = want.FirstEnteredAt
	if !reflect.DeepEqual(tx.args, []any{input.SourceContactID, nullableInt64(input.CustomerID), input.OwnerReference, stamp, stamp, input.EnterCount, stamp, stamp, input.ChannelID}) {
		t.Fatal("contact create arguments lost precision or UTC normalization")
	}
	assertHistoricalRelationQuery(t, tx.sql, "CreateHistoricalChannelContact")
	got, err = store.GetHistoricalChannelContact(context.Background(), dbrow.ID)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(tx.args, []any{dbrow.ID}) || !strings.Contains(tx.sql, "FOR UPDATE") {
		t.Fatalf("contact get: %v", err)
	}
	dbrow.CustomerID = pgtype.Int8{}
	got, err = historicalChannelContactRecord(dbrow)
	if err != nil || got.CustomerID != nil {
		t.Fatal("NULL customer was not preserved")
	}
}

func TestHistoricalChannelRelationsAssigneeCivilTimeAndNulls(t *testing.T) {
	input := historicalChannelAssigneeFixture()
	zero, historicalNegative := int32(0), int32(-1)
	input.RatioPercent, input.MaxScans24h = &zero, &historicalNegative
	civil := time.Date(2026, 8, 28, 18, 22, 33, 123456789, time.FixedZone("source-civil", 8*3600))
	row := contactdb.ChannelHistoricalAssignee{ID: 43, ChannelID: input.ChannelID, SourceAssigneeID: input.SourceAssigneeID,
		StaffReference: input.StaffReference, DisplayNameSnapshot: input.DisplayNameSnapshot, Priority: input.Priority, Status: input.Status,
		RatioPercent: pgtype.Int4{Int32: zero, Valid: true}, MaxScans24h: pgtype.Int4{Int32: historicalNegative, Valid: true},
		SourceCreatedAt: pgtype.Timestamp{Time: civil, Valid: true}, SourceUpdatedAt: pgtype.Timestamp{Time: civil, Valid: true}}
	tx := &channelRelationsTestTx{row: channelRelationsTestRow{values: channelAssigneeValues(row)}}
	store := &HistoricalChannelRelationsStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	want := input
	want.ID = row.ID
	got, err := store.CreateHistoricalChannelAssignee(context.Background(), input)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("assignee mapping changed civil time/nullable integer: %v", err)
	}
	parsed, _ := time.Parse(historicalChannelCivilLayout, input.SourceCreatedAt)
	stamp := pgtype.Timestamp{Time: parsed, Valid: true}
	if !reflect.DeepEqual(tx.args, []any{input.SourceAssigneeID, input.StaffReference, input.DisplayNameSnapshot, input.Priority,
		row.RatioPercent, row.MaxScans24h, input.Status, stamp, stamp, input.ChannelID}) {
		t.Fatal("assignee create did not use civil timestamp parameters")
	}
	assertHistoricalRelationQuery(t, tx.sql, "CreateHistoricalChannelAssignee")
	got, err = store.GetHistoricalChannelAssignee(context.Background(), row.ID)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(tx.args, []any{row.ID}) {
		t.Fatalf("assignee get: %v", err)
	}
	row.RatioPercent, row.MaxScans24h = pgtype.Int4{}, pgtype.Int4{}
	got, err = historicalChannelAssigneeRecord(row)
	if err != nil || got.RatioPercent != nil || got.MaxScans24h != nil {
		t.Fatal("NULL assignee limits were rewritten")
	}
}

func TestHistoricalChannelRelationsRejectInvalidInputAndRequireTransaction(t *testing.T) {
	for _, store := range []*HistoricalChannelRelationsStore{nil, {}, NewHistoricalChannelRelationsStore(), {tx: func(context.Context) (pgx.Tx, error) { return nil, nil }}} {
		for _, err := range historicalRelationCallErrors(context.Background(), store) {
			if err != contactport.ErrHistoricalChannelUnavailable {
				t.Fatal("missing caller transaction was not rejected")
			}
		}
	}
	store := &HistoricalChannelRelationsStore{tx: func(context.Context) (pgx.Tx, error) { t.Fatal("invalid input reached transaction"); return nil, nil }}
	for _, mutate := range []func(*contactport.HistoricalChannelContact){
		func(v *contactport.HistoricalChannelContact) { v.ID = 1 },
		func(v *contactport.HistoricalChannelContact) { v.ChannelID = 0 },
		func(v *contactport.HistoricalChannelContact) { v.SourceContactID = 0 },
		func(v *contactport.HistoricalChannelContact) { v.EnterCount = 0 },
		func(v *contactport.HistoricalChannelContact) { v.CustomerID = new(int64) },
		func(v *contactport.HistoricalChannelContact) { v.FirstEnteredAt = time.Time{} },
		func(v *contactport.HistoricalChannelContact) { v.LastEnteredAt = time.Time{} },
		func(v *contactport.HistoricalChannelContact) { v.CreatedAt = time.Time{} },
		func(v *contactport.HistoricalChannelContact) { v.UpdatedAt = time.Time{} },
		func(v *contactport.HistoricalChannelContact) { v.LastEnteredAt = v.FirstEnteredAt.Add(-time.Second) },
		func(v *contactport.HistoricalChannelContact) { v.UpdatedAt = v.CreatedAt.Add(-time.Second) },
	} {
		value := historicalChannelContactFixture()
		mutate(&value)
		if _, err := store.CreateHistoricalChannelContact(context.Background(), value); err != contactport.ErrHistoricalChannelInvalid {
			t.Fatal("invalid contact was accepted")
		}
	}
	for _, mutate := range []func(*contactport.HistoricalChannelAssignee){
		func(v *contactport.HistoricalChannelAssignee) { v.ID = 1 },
		func(v *contactport.HistoricalChannelAssignee) { v.ChannelID = 0 },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceAssigneeID = 0 },
		func(v *contactport.HistoricalChannelAssignee) { v.Priority = -1 },
		func(v *contactport.HistoricalChannelAssignee) { v.Status = "" },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceCreatedAt = "" },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceUpdatedAt = "2026-08-28T18:22:33Z" },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceCreatedAt += "+08:00" },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceCreatedAt = "2026-08-28T18:22:33.1234567" },
		func(v *contactport.HistoricalChannelAssignee) { v.SourceUpdatedAt = "2026-08-27T18:22:33.123456" },
	} {
		value := historicalChannelAssigneeFixture()
		mutate(&value)
		if _, err := store.CreateHistoricalChannelAssignee(context.Background(), value); err != contactport.ErrHistoricalChannelInvalid {
			t.Fatal("invalid assignee was accepted")
		}
	}
	if _, err := store.GetHistoricalChannelContact(context.Background(), 0); err != contactport.ErrHistoricalChannelInvalid {
		t.Fatal("invalid get ID")
	}
	if _, err := store.GetHistoricalChannelAssignee(context.Background(), -1); err != contactport.ErrHistoricalChannelInvalid {
		t.Fatal("invalid get ID")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, canceled} {
		for _, err := range historicalRelationCallErrors(ctx, store) {
			if err != contactport.ErrHistoricalChannelUnavailable {
				t.Fatal("invalid context reached transaction")
			}
		}
	}
}

func TestHistoricalChannelRelationsSanitizeErrorsAndInvalidStoredTimes(t *testing.T) {
	for _, test := range []struct{ cause, want error }{
		{pgx.ErrNoRows, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23505", Detail: "private"}, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23503", Detail: "private"}, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23514", Detail: "private"}, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23502", Detail: "private"}, contactport.ErrHistoricalChannelConflict},
		{errors.New("private"), contactport.ErrHistoricalChannelUnavailable},
	} {
		tx := &channelRelationsTestTx{row: channelRelationsTestRow{err: test.cause}}
		store := &HistoricalChannelRelationsStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
		for _, err := range historicalRelationCallErrors(context.Background(), store) {
			if err != test.want {
				t.Fatal("database error leaked or was misclassified")
			}
		}
	}
	for _, stamp := range []pgtype.Timestamptz{{}, {Valid: true}, {Valid: true, InfinityModifier: pgtype.Infinity}} {
		if _, err := historicalChannelContactRecord(contactdb.ChannelHistoricalContact{FirstEnteredAt: stamp}); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatal("invalid stored timestamptz was accepted")
		}
	}
	for _, stamp := range []pgtype.Timestamp{{}, {Valid: true}, {Valid: true, InfinityModifier: pgtype.Infinity}} {
		if _, err := historicalChannelAssigneeRecord(contactdb.ChannelHistoricalAssignee{SourceCreatedAt: stamp}); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatal("invalid stored civil timestamp was accepted")
		}
	}
}

func TestHistoricalChannelHistoryReaderPagingAndAssigneeOverflow(t *testing.T) {
	stamp := historicalChannelTimestamp(time.Now())
	row := contactdb.ChannelHistoricalContact{ID: 21, ChannelID: 3, SourceContactID: 99, FirstEnteredAt: stamp, LastEnteredAt: stamp, CreatedAt: stamp, UpdatedAt: stamp}
	db := &channelRelationsTestTx{row: channelRelationsTestRow{values: []any{int64(8)}}, rows: &channelRelationsTestRows{values: [][]any{channelContactValues(row)}}}
	reader := &HistoricalChannelHistoryReader{db: db}
	contacts, total, err := reader.ListHistoricalChannelContacts(context.Background(), 3, 1, 4)
	if err != nil || total != 8 || len(contacts) != 1 || contacts[0].ID != 21 || !reflect.DeepEqual(db.args, []any{int64(3), int32(4), int32(1)}) {
		t.Fatalf("reader page mapping: %v", err)
	}
	db.rows = &channelRelationsTestRows{}
	contacts, total, err = reader.ListHistoricalChannelContacts(context.Background(), 3, 100, 100)
	if err != nil || contacts == nil || len(contacts) != 0 || total != 8 {
		t.Fatal("empty page lost total")
	}
	civil := pgtype.Timestamp{Time: time.Date(2026, 8, 28, 18, 22, 33, 123456000, time.UTC), Valid: true}
	assignee := contactdb.ChannelHistoricalAssignee{ID: 22, ChannelID: 3, Status: "inactive", SourceCreatedAt: civil, SourceUpdatedAt: civil}
	for _, count := range []int{0, 1, 200, 201} {
		values := make([][]any, count)
		for i := range values {
			values[i] = channelAssigneeValues(assignee)
		}
		db.rows = &channelRelationsTestRows{values: values}
		items, err := reader.ListHistoricalChannelAssignees(context.Background(), 3)
		if count > 200 {
			if !errors.Is(err, contactport.ErrHistoricalChannelUnavailable) || !strings.Contains(err.Error(), "exceeds 200") || items != nil {
				t.Fatal("assignee history silently truncated")
			}
		} else if err != nil || len(items) != count || items == nil {
			t.Fatal("assignee list mapping")
		}
		if !strings.Contains(db.sql, "LIMIT 201") || !reflect.DeepEqual(db.args, []any{int64(3)}) {
			t.Fatal("assignee limit query changed")
		}
	}
}

func TestHistoricalChannelHistoryReaderRejectsInvalidQueriesAndErrors(t *testing.T) {
	for _, args := range [][3]int64{{0, 10, 0}, {1, 0, 0}, {1, 101, 0}, {1, 10, -1}} {
		if _, _, err := NewHistoricalChannelHistoryReader(nil).ListHistoricalChannelContacts(context.Background(), args[0], int32(args[1]), int32(args[2])); err != contactport.ErrHistoricalChannelInvalid {
			t.Fatal("invalid page accepted")
		}
	}
	if _, err := NewHistoricalChannelHistoryReader(nil).ListHistoricalChannelAssignees(context.Background(), 0); err != contactport.ErrHistoricalChannelInvalid {
		t.Fatal("invalid channel accepted")
	}
	for _, reader := range []*HistoricalChannelHistoryReader{nil, {}, NewHistoricalChannelHistoryReader(nil)} {
		if _, _, err := reader.ListHistoricalChannelContacts(context.Background(), 1, 1, 0); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatal("nil reader accepted")
		}
		if _, err := reader.ListHistoricalChannelAssignees(context.Background(), 1); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatal("nil reader accepted")
		}
	}
	for _, db := range []*channelRelationsTestTx{
		{row: channelRelationsTestRow{err: errors.New("private count error")}},
		{row: channelRelationsTestRow{values: []any{int64(0)}}, queryErr: errors.New("private query error")},
		{row: channelRelationsTestRow{values: []any{int64(0)}}, rows: &channelRelationsTestRows{err: errors.New("private rows error")}},
	} {
		if _, _, err := (&HistoricalChannelHistoryReader{db: db}).ListHistoricalChannelContacts(context.Background(), 1, 1, 0); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatal("reader error leaked")
		}
	}
}

var historicalChannelRelationsDatabase = flag.String("channel-history-test-database-url", "", "optional PostgreSQL test database migrated through 00110")

func TestHistoricalChannelRelationsPostgresRoundTripAndRollback(t *testing.T) {
	if *historicalChannelRelationsDatabase == "" {
		t.Skip("supply -channel-history-test-database-url for PostgreSQL integration")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *historicalChannelRelationsDatabase)
	if err != nil {
		t.Fatal("test database open failed")
	}
	defer pool.Close()
	rollback := errors.New("rollback historical channel fixtures")
	var channelID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		channel := historicalChannelStoreFixture()
		channel.Code = fmt.Sprintf("channel-relations-%d", time.Now().UnixNano())
		created, err := NewHistoricalChannelStore().CreateHistoricalChannel(txCtx, channel)
		if err != nil {
			return err
		}
		channelID = created.ID
		store := NewHistoricalChannelRelationsStore()
		contact := historicalChannelContactFixture()
		contact.ChannelID = channelID
		contact.SourceContactID = time.Now().UnixNano()
		gotContact, err := store.CreateHistoricalChannelContact(txCtx, contact)
		if err != nil {
			return err
		}
		readContact, err := store.GetHistoricalChannelContact(txCtx, gotContact.ID)
		if err != nil || !reflect.DeepEqual(readContact, gotContact) {
			return errors.New("contact round trip failed")
		}
		assignee := historicalChannelAssigneeFixture()
		assignee.ChannelID = channelID
		assignee.SourceAssigneeID = contact.SourceContactID
		gotAssignee, err := store.CreateHistoricalChannelAssignee(txCtx, assignee)
		if err != nil {
			return err
		}
		readAssignee, err := store.GetHistoricalChannelAssignee(txCtx, gotAssignee.ID)
		if err != nil || !reflect.DeepEqual(readAssignee, gotAssignee) || readAssignee.SourceCreatedAt != assignee.SourceCreatedAt {
			return errors.New("civil assignee round trip failed")
		}
		if _, err := store.CreateHistoricalChannelContact(txCtx, contact); err != contactport.ErrHistoricalChannelConflict {
			return errors.New("duplicate source contact accepted")
		}
		if _, err := store.CreateHistoricalChannelAssignee(txCtx, assignee); err != contactport.ErrHistoricalChannelConflict {
			return errors.New("duplicate source assignee accepted")
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		reader := &HistoricalChannelHistoryReader{db: tx}
		contacts, total, err := reader.ListHistoricalChannelContacts(txCtx, channelID, 1, 0)
		if err != nil || total != 1 || len(contacts) != 1 {
			return errors.New("contact history page failed")
		}
		assignees, err := reader.ListHistoricalChannelAssignees(txCtx, channelID)
		if err != nil || len(assignees) != 1 {
			return errors.New("assignee history list failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("historical channel PostgreSQL round trip failed")
	}
	items, total, err := NewHistoricalChannelHistoryReader(pool).ListHistoricalChannelContacts(ctx, channelID, 1, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatal("contact rollback failed")
	}
	assignees, err := NewHistoricalChannelHistoryReader(pool).ListHistoricalChannelAssignees(ctx, channelID)
	if err != nil || len(assignees) != 0 {
		t.Fatal("assignee rollback failed")
	}
}

func historicalChannelContactFixture() contactport.HistoricalChannelContact {
	stamp := time.Date(2026, 8, 28, 18, 22, 33, 123456789, time.FixedZone("offset", 8*3600))
	return contactport.HistoricalChannelContact{ChannelID: 7, SourceContactID: 9007199254740993, OwnerReference: " owner ", EnterCount: 2,
		FirstEnteredAt: stamp, LastEnteredAt: stamp, CreatedAt: stamp, UpdatedAt: stamp}
}

func historicalChannelAssigneeFixture() contactport.HistoricalChannelAssignee {
	return contactport.HistoricalChannelAssignee{ChannelID: 7, SourceAssigneeID: 9007199254740993, StaffReference: " staff ", DisplayNameSnapshot: " name ", Priority: 2,
		Status: "inactive", SourceCreatedAt: "2026-08-28T18:22:33.123456", SourceUpdatedAt: "2026-08-28T18:22:33.123456"}
}

func historicalRelationCallErrors(ctx context.Context, store *HistoricalChannelRelationsStore) []error {
	_, a := store.CreateHistoricalChannelContact(ctx, historicalChannelContactFixture())
	_, b := store.GetHistoricalChannelContact(ctx, 1)
	_, c := store.CreateHistoricalChannelAssignee(ctx, historicalChannelAssigneeFixture())
	_, d := store.GetHistoricalChannelAssignee(ctx, 1)
	return []error{a, b, c, d}
}

func assertHistoricalRelationQuery(t *testing.T, query, name string) {
	t.Helper()
	if !strings.HasPrefix(query, "-- name: "+name+" :one") || !strings.Contains(query, "c.status='inactive'") || !strings.Contains(query, "a.status='legacy_unverified'") {
		t.Fatal("historical parent guard missing")
	}
}

func channelContactValues(v contactdb.ChannelHistoricalContact) []any {
	return []any{v.ID, v.ChannelID, v.SourceContactID, v.CustomerID, v.OwnerReference, v.FirstEnteredAt, v.LastEnteredAt, v.EnterCount, v.CreatedAt, v.UpdatedAt}
}
func channelAssigneeValues(v contactdb.ChannelHistoricalAssignee) []any {
	return []any{v.ID, v.ChannelID, v.SourceAssigneeID, v.StaffReference, v.DisplayNameSnapshot, v.Priority, v.RatioPercent, v.MaxScans24h, v.Status, v.SourceCreatedAt, v.SourceUpdatedAt}
}

type channelRelationsTestTx struct {
	pgx.Tx
	row      channelRelationsTestRow
	rows     *channelRelationsTestRows
	queryErr error
	sql      string
	args     []any
}

func (tx *channelRelationsTestTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.sql, tx.args = query, args
	return tx.row
}
func (tx *channelRelationsTestTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.sql, tx.args = query, args
	return tx.rows, tx.queryErr
}

type channelRelationsTestRow struct {
	values []any
	err    error
}

func (row channelRelationsTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("wrong scan length")
	}
	for i, value := range row.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type channelRelationsTestRows struct {
	pgx.Rows
	values [][]any
	index  int
	err    error
}

func (rows *channelRelationsTestRows) Close()     {}
func (rows *channelRelationsTestRows) Err() error { return rows.err }
func (rows *channelRelationsTestRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}
func (rows *channelRelationsTestRows) Scan(dest ...any) error {
	return (channelRelationsTestRow{values: rows.values[rows.index-1]}).Scan(dest...)
}

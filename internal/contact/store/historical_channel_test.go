package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
)

func TestHistoricalChannelStoreMapsCreateAndRead(t *testing.T) {
	input := historicalChannelStoreFixture()
	want := input
	want.ID = 19
	tx := &historicalChannelTx{row: historicalChannelRow{value: contactdb.GetHistoricalChannelRow{
		ID: want.ID, Code: want.Code, Name: want.Name, Status: want.Status, Config: want.Projection,
		CreatedBy: want.CreatedBy, UpdatedBy: want.UpdatedBy, ConfigDigest: want.LegacyConfigDigest,
		CreatedAt: pgtype.Timestamptz{Time: want.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: want.UpdatedAt, Valid: true},
	}}}
	store := &HistoricalChannelStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	created, err := store.CreateHistoricalChannel(context.Background(), input)
	if err != nil || !reflect.DeepEqual(created, want) {
		t.Fatalf("create mapping mismatch: %v", err)
	}
	arguments := []any{input.Code, input.Name, []byte(input.Projection), input.CreatedBy,
		pgtype.Timestamptz{Time: input.CreatedAt, Valid: true}, pgtype.Timestamptz{Time: input.UpdatedAt, Valid: true}, input.LegacyConfigDigest}
	if !reflect.DeepEqual(tx.arguments, arguments) || tx.calls != 1 || !strings.Contains(tx.sql, "WITH inserted AS") ||
		!strings.Contains(tx.sql, "'inactive'") || !strings.Contains(tx.sql, "'legacy_unverified'") {
		t.Fatal("historical channel create did not use the single inactive/archive query")
	}
	for _, forbidden := range []string{"operation_receipts", "event_log", "external_effects", "UPDATE channels", "UPDATE customers"} {
		if strings.Contains(tx.sql, forbidden) {
			t.Fatal("historical channel create reached an unrelated write")
		}
	}
	read, err := store.GetHistoricalChannel(context.Background(), want.ID)
	if err != nil || !reflect.DeepEqual(read, want) || tx.calls != 2 || !reflect.DeepEqual(tx.arguments, []any{want.ID}) || !strings.Contains(tx.sql, "FOR UPDATE OF c, a") {
		t.Fatalf("read mapping mismatch: %v", err)
	}
}

func TestHistoricalChannelStoreRequiresCallerTransaction(t *testing.T) {
	for _, store := range []*HistoricalChannelStore{nil, {}, NewHistoricalChannelStore(), {tx: func(context.Context) (pgx.Tx, error) { return nil, nil }}} {
		if _, err := store.CreateHistoricalChannel(context.Background(), historicalChannelStoreFixture()); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatalf("create without transaction: %v", err)
		}
		if _, err := store.GetHistoricalChannel(context.Background(), 1); err != contactport.ErrHistoricalChannelUnavailable {
			t.Fatalf("read without transaction: %v", err)
		}
	}
	if _, err := NewHistoricalChannelStore().GetHistoricalChannel(nil, 1); err != contactport.ErrHistoricalChannelUnavailable {
		t.Fatalf("read without context: %v", err)
	}
}

func TestHistoricalChannelStoreRejectsInvalidInputBeforeTransaction(t *testing.T) {
	store := &HistoricalChannelStore{tx: func(context.Context) (pgx.Tx, error) {
		t.Fatal("invalid input reached transaction")
		return nil, nil
	}}
	for _, mutate := range []func(*contactport.HistoricalChannelRecord){
		func(row *contactport.HistoricalChannelRecord) { row.ID = 1 },
		func(row *contactport.HistoricalChannelRecord) { row.Status = "active" },
		func(row *contactport.HistoricalChannelRecord) { row.CreatedBy, row.UpdatedBy = 0, 0 },
		func(row *contactport.HistoricalChannelRecord) { row.UpdatedBy++ },
		func(row *contactport.HistoricalChannelRecord) { row.Code = "" },
		func(row *contactport.HistoricalChannelRecord) { row.Name = " name " },
		func(row *contactport.HistoricalChannelRecord) { row.Name = strings.Repeat("名", 201) },
		func(row *contactport.HistoricalChannelRecord) { row.Code = string([]byte{0xff}) },
		func(row *contactport.HistoricalChannelRecord) { row.CreatedAt = time.Time{} },
		func(row *contactport.HistoricalChannelRecord) { row.UpdatedAt = row.CreatedAt.Add(-time.Second) },
		func(row *contactport.HistoricalChannelRecord) { row.LegacyConfigDigest = strings.Repeat("a", 64) },
		func(row *contactport.HistoricalChannelRecord) {
			row.LegacyConfigDigest = "sha256:" + strings.Repeat("G", 64)
		},
		func(row *contactport.HistoricalChannelRecord) {
			row.LegacyConfigDigest = "sha256:" + strings.Repeat("A", 64)
		},
		func(row *contactport.HistoricalChannelRecord) { row.Projection = []byte(`{`) },
		func(row *contactport.HistoricalChannelRecord) { row.Projection = []byte(`null`) },
		func(row *contactport.HistoricalChannelRecord) { row.Projection = []byte(`[]`) },
		func(row *contactport.HistoricalChannelRecord) { row.Projection = []byte(`{}`) },
	} {
		input := historicalChannelStoreFixture()
		mutate(&input)
		if _, err := store.CreateHistoricalChannel(context.Background(), input); err != contactport.ErrHistoricalChannelInvalid {
			t.Fatalf("invalid input classification: %v", err)
		}
	}
	for _, id := range []int64{0, -1} {
		if _, err := store.GetHistoricalChannel(context.Background(), id); err != contactport.ErrHistoricalChannelInvalid {
			t.Fatalf("invalid read ID classification: %v", err)
		}
	}
}

func TestHistoricalChannelStoreSanitizesDatabaseErrors(t *testing.T) {
	for _, test := range []struct {
		cause, want error
	}{
		{pgx.ErrNoRows, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23505", Detail: "private value"}, contactport.ErrHistoricalChannelConflict},
		{&pgconn.PgError{Code: "23514", Detail: "private value"}, contactport.ErrHistoricalChannelUnavailable},
		{errors.New("private database detail"), contactport.ErrHistoricalChannelUnavailable},
	} {
		tx := &historicalChannelTx{row: historicalChannelRow{err: test.cause}}
		store := &HistoricalChannelStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
		if _, err := store.CreateHistoricalChannel(context.Background(), historicalChannelStoreFixture()); err != test.want {
			t.Fatal("create returned an unsanitized or incorrect error")
		}
		if _, err := store.GetHistoricalChannel(context.Background(), 1); err != test.want {
			t.Fatal("read returned an unsanitized or incorrect error")
		}
	}
	store := &HistoricalChannelStore{tx: func(context.Context) (pgx.Tx, error) { return nil, errors.New("private transaction detail") }}
	if _, err := store.GetHistoricalChannel(context.Background(), 1); err != contactport.ErrHistoricalChannelUnavailable {
		t.Fatal("transaction error was not sanitized")
	}
}

func historicalChannelStoreFixture() contactport.HistoricalChannelRecord {
	stamp := time.Date(2026, 8, 28, 0, 0, 0, 123000, time.UTC)
	return contactport.HistoricalChannelRecord{
		Code: "v1-test-channel", Name: "历史渠道", Status: "inactive", Projection: []byte(`{"schema_version":1,"status":"inactive"}`),
		LegacyConfigDigest: "sha256:" + strings.Repeat("a", 64), CreatedBy: 7, UpdatedBy: 7, CreatedAt: stamp, UpdatedAt: stamp.Add(time.Hour),
	}
}

type historicalChannelTx struct {
	pgx.Tx
	row       historicalChannelRow
	sql       string
	arguments []any
	calls     int
}

func (tx *historicalChannelTx) QueryRow(_ context.Context, sql string, arguments ...any) pgx.Row {
	tx.calls++
	tx.sql, tx.arguments = sql, append([]any(nil), arguments...)
	return tx.row
}

type historicalChannelRow struct {
	value contactdb.GetHistoricalChannelRow
	err   error
}

func (row historicalChannelRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	values := []any{row.value.ID, row.value.Code, row.value.Name, row.value.Status, row.value.Config,
		row.value.CreatedBy, row.value.UpdatedBy, row.value.CreatedAt, row.value.UpdatedAt, row.value.ConfigDigest}
	if len(destinations) != len(values) {
		return errors.New("unexpected scan destination count")
	}
	for index, value := range values {
		reflect.ValueOf(destinations[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

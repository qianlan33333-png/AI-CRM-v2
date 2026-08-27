package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	media "github.com/qianlan33333-png/AI-CRM-v2/internal/media"
)

func TestHistoricalMiniProgramStoreInsertsOnlyDisabledStaticDefinition(t *testing.T) {
	tx := &historicalMiniProgramTx{row: historicalMiniProgramRow{id: 81}}
	store := &HistoricalMiniProgramStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	definition := historicalMiniProgramStoreFixture(t)
	id, err := store.InsertHistoricalMiniProgram(context.Background(), definition)
	if err != nil || id != 81 {
		t.Fatalf("id/err = %d/%v", id, err)
	}
	if !strings.Contains(tx.sql, "media_miniprograms") || strings.Contains(tx.sql, "thumbnail_") || strings.Contains(tx.sql, "operation_receipts") || strings.Contains(tx.sql, "event_log") || strings.Contains(tx.sql, "UPDATE ") {
		t.Fatalf("unsafe SQL = %q", tx.sql)
	}
	if len(tx.arguments) != 10 || tx.arguments[0] != (pgtype.Int8{Int64: 18, Valid: true}) || tx.arguments[5] != int64(7) || tx.arguments[6] != int64(7) || tx.arguments[7] != int64(1) {
		t.Fatalf("arguments = %#v", tx.arguments)
	}
}

func TestHistoricalMiniProgramStoreConflictsInsteadOfOverwriting(t *testing.T) {
	tx := &historicalMiniProgramTx{row: historicalMiniProgramRow{err: &pgconn.PgError{Code: "23505"}}}
	store := &HistoricalMiniProgramStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	if _, err := store.InsertHistoricalMiniProgram(context.Background(), historicalMiniProgramStoreFixture(t)); !errors.Is(err, media.ErrHistoricalMiniProgramConflict) {
		t.Fatalf("error = %v", err)
	}
}

type historicalMiniProgramTx struct {
	pgx.Tx
	row       historicalMiniProgramRow
	sql       string
	arguments []any
}

func (tx *historicalMiniProgramTx) QueryRow(_ context.Context, sql string, arguments ...any) pgx.Row {
	tx.sql = sql
	tx.arguments = append([]any(nil), arguments...)
	return tx.row
}

type historicalMiniProgramRow struct {
	id  int64
	err error
}

func (row historicalMiniProgramRow) Scan(destinations ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(destinations) != 1 {
		return errors.New("unexpected scan destinations")
	}
	value, ok := destinations[0].(*int64)
	if !ok {
		return errors.New("unexpected scan destination")
	}
	*value = row.id
	return nil
}

func historicalMiniProgramStoreFixture(t *testing.T) media.HistoricalMiniProgramDefinition {
	t.Helper()
	stamp := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	definition, err := media.AdaptV1MiniProgramLibrary(media.V1MiniProgramLibraryRow{ID: 18, Name: "历史素材", AppID: "wx-history", PagePath: "pages/history", Title: "历史素材", CreatedAt: stamp, UpdatedAt: stamp}, "public/miniprogram_library/18", [32]byte{1}, 7)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

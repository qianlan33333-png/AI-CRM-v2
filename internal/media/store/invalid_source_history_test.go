package store

import (
	"context"
	"errors"
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
	mediadb "github.com/qianlan33333-png/AI-CRM-v2/internal/media/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var invalidAssetHistoryPostgresDSN = flag.String("media-invalid-source-history-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00133 for invalid media history rollback verification")

func TestInvalidAssetHistoryValuePreservesPrivateAndSignedFacts(t *testing.T) {
	value := invalidAssetHistoryStoreFixture(1)
	row := mediadb.MediaV1InvalidAssetHistory{ID: 4, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots, Kind: value.Kind, SourceID: value.SourceID, Name: value.Name, FileName: value.FileName, MimeType: value.MIMEType, FileSize: value.FileSize, OriginalEnabled: value.OriginalEnabled, ContentDigest: value.ContentDigest[:], CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: value.UpdatedAt, Valid: true}, QuarantineReason: value.QuarantineReason}
	actual, err := invalidAssetHistoryValue(row)
	if err != nil || actual.ID != 4 || actual.SourceID != -8 || actual.Name != "" || actual.FileName != " \n" || actual.FileSize != -1 || !reflect.DeepEqual(actual.RedactedRoots, value.RedactedRoots) || actual.PrivateDigest != value.PrivateDigest || actual.ContentDigest != value.ContentDigest {
		t.Fatalf("actual=%#v err=%v", actual, err)
	}
	row.PrivateDigest = row.PrivateDigest[:31]
	if _, err := invalidAssetHistoryValue(row); !errors.Is(err, mediaport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("short private digest accepted: %v", err)
	}
}

func TestInvalidAssetHistoryStoreAndReaderFailClosed(t *testing.T) {
	if _, err := NewInvalidSourceHistoryStore().CreateHistoricalInvalidAsset(context.Background(), invalidAssetHistoryStoreFixture(2)); !errors.Is(err, mediaport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("caller transaction escaped: %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*InvalidSourceHistoryReader{nil, NewInvalidSourceHistoryReader(nil), NewInvalidSourceHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalInvalidAsset(context.Background(), mediaport.InvalidSourceHistoryQuery{Limit: 1}); !errors.Is(err, mediaport.ErrInvalidSourceHistoryUnavailable) {
			t.Fatalf("reader=%#v err=%v", reader, err)
		}
	}
	for _, page := range []mediaport.InvalidSourceHistoryQuery{{Limit: 0}, {Limit: 201}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewInvalidSourceHistoryReader(nil).ListHistoricalInvalidAsset(context.Background(), page); !errors.Is(err, mediaport.ErrInvalidSourceHistoryInvalid) {
			t.Fatalf("page=%+v err=%v", page, err)
		}
	}
}

func TestInvalidAssetHistoryReaderPrefersCallerTransaction(t *testing.T) {
	tx := &invalidAssetHistoryCallerTx{}
	err := platformstore.NewUnitOfWork(invalidAssetHistoryCallerBeginner{tx: tx}).Within(context.Background(), func(ctx context.Context) error { _, err := NewInvalidSourceHistoryReader(nil).queries(ctx); return err })
	if err != nil {
		t.Fatalf("caller transaction was not used: %v", err)
	}
}

func TestInvalidAssetHistoryPostgresRoundTripRollback(t *testing.T) {
	if *invalidAssetHistoryPostgresDSN == "" {
		t.Skip("set -media-invalid-source-history-postgres-dsn for isolated schema 133 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *invalidAssetHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := mediadb.New(pool)
	before, err := queries.CountHistoricalInvalidAsset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	forced := errors.New("invalid asset history forced rollback")
	var ids []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewInvalidSourceHistoryStore(), NewInvalidSourceHistoryReader(nil)
		for first := byte(11); first <= 12; first++ {
			created, err := store.CreateHistoricalInvalidAsset(txCtx, invalidAssetHistoryStoreFixture(first))
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			loaded, err := reader.GetHistoricalInvalidAsset(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, created) {
				return errors.New("media invalid asset caller transaction read mismatch")
			}
			tx, err := platformstore.TxFromContext(txCtx)
			if err != nil {
				return err
			}
			loaded, err = NewInvalidSourceHistoryReader(tx).GetHistoricalInvalidAsset(context.Background(), created.ID)
			if err != nil || !reflect.DeepEqual(loaded, created) {
				return errors.New("media invalid asset bare transaction read mismatch")
			}
			if first == 11 {
				if _, err := store.CreateHistoricalInvalidAsset(txCtx, invalidAssetHistoryStoreFixture(first)); !errors.Is(err, mediaport.ErrInvalidSourceHistoryConflict) {
					return errors.New("media invalid asset duplicate was accepted")
				}
			}
		}
		items, total, err := reader.ListHistoricalInvalidAsset(txCtx, mediaport.InvalidSourceHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+2 || len(items) != 1 || items[0].ID != ids[1] {
			return errors.New("media invalid asset page mismatch")
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback err=%v", err)
	}
	after, err := queries.CountHistoricalInvalidAsset(ctx)
	if err != nil || after != before {
		t.Fatalf("before=%d after=%d err=%v", before, after, err)
	}
}

func invalidAssetHistoryStoreFixture(first byte) mediaport.HistoricalInvalidAsset {
	value := mediaport.HistoricalInvalidAsset{Kind: "attachment", SourceID: -8, Name: "", FileName: " \n", MIMEType: "", FileSize: -1, OriginalEnabled: false, RedactedRoots: []string{"payload", "content"}, CreatedAt: time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 29, 9, 11, 12, 123456000, time.UTC), QuarantineReason: "invalid_static_media_definition"}
	for index := range value.SourceKeyDigest {
		value.SourceKeyDigest[index] = first + 1
		value.SourcePayloadDigest[index] = first + 2
		value.SourceFieldDigest[index] = first + 3
		value.PrivateDigest[index] = first + 4
		value.ContentDigest[index] = first + 5
	}
	return value
}

type invalidAssetHistoryCallerBeginner struct{ tx pgx.Tx }

func (b invalidAssetHistoryCallerBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return b.tx, nil
}

type invalidAssetHistoryCallerTx struct{ pgx.Tx }

func (*invalidAssetHistoryCallerTx) Commit(context.Context) error   { return nil }
func (*invalidAssetHistoryCallerTx) Rollback(context.Context) error { return nil }

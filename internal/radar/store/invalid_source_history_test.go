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
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radardb "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store/generated"
)

var invalidRadarLinkHistoryPostgresDSN = flag.String("radar-invalid-source-history-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00133 for invalid radar history rollback verification")

func TestInvalidRadarLinkHistoryValuePreservesPrivateAndSignedFacts(t *testing.T) {
	value := invalidRadarLinkHistoryStoreFixture(1)
	row := radardb.RadarV1InvalidLinkHistory{ID: 4, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], PrivateDigest: value.PrivateDigest[:], RedactedRoots: value.RedactedRoots, SourceID: value.SourceID, Code: value.Code, Title: value.Title, DestinationUrlDigest: value.DestinationURLDigest[:], CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: value.UpdatedAt, Valid: true}, QuarantineReason: value.QuarantineReason}
	actual, err := invalidRadarLinkHistoryValue(row)
	if err != nil || actual.ID != 4 || actual.SourceID != -9 || actual.Code != "" || actual.Title != " \n" || actual.PrivateDigest != value.PrivateDigest || actual.DestinationURLDigest != value.DestinationURLDigest || !reflect.DeepEqual(actual.RedactedRoots, value.RedactedRoots) {
		t.Fatalf("actual=%#v err=%v", actual, err)
	}
	row.DestinationUrlDigest = row.DestinationUrlDigest[:31]
	if _, err := invalidRadarLinkHistoryValue(row); !errors.Is(err, radarport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("short destination digest accepted: %v", err)
	}
}
func TestInvalidRadarLinkHistoryStoreAndReaderFailClosed(t *testing.T) {
	if _, err := NewInvalidSourceHistoryStore().CreateHistoricalInvalidRadarLink(context.Background(), invalidRadarLinkHistoryStoreFixture(2)); !errors.Is(err, radarport.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("caller transaction escaped: %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*InvalidSourceHistoryReader{nil, NewInvalidSourceHistoryReader(nil), NewInvalidSourceHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalInvalidRadarLink(context.Background(), radarport.InvalidSourceHistoryQuery{Limit: 1}); !errors.Is(err, radarport.ErrInvalidSourceHistoryUnavailable) {
			t.Fatalf("reader=%#v err=%v", reader, err)
		}
	}
	for _, page := range []radarport.InvalidSourceHistoryQuery{{Limit: 0}, {Limit: 201}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewInvalidSourceHistoryReader(nil).ListHistoricalInvalidRadarLink(context.Background(), page); !errors.Is(err, radarport.ErrInvalidSourceHistoryInvalid) {
			t.Fatalf("page=%+v err=%v", page, err)
		}
	}
}
func TestInvalidRadarLinkHistoryReaderPrefersCallerTransaction(t *testing.T) {
	tx := &invalidRadarLinkHistoryCallerTx{}
	err := platformstore.NewUnitOfWork(invalidRadarLinkHistoryCallerBeginner{tx: tx}).Within(context.Background(), func(ctx context.Context) error { _, err := NewInvalidSourceHistoryReader(nil).queries(ctx); return err })
	if err != nil {
		t.Fatalf("caller transaction was not used: %v", err)
	}
}
func TestInvalidRadarLinkHistoryPostgresRoundTripRollback(t *testing.T) {
	if *invalidRadarLinkHistoryPostgresDSN == "" {
		t.Skip("set -radar-invalid-source-history-postgres-dsn for isolated schema 133 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *invalidRadarLinkHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := radardb.New(pool)
	before, err := queries.CountHistoricalInvalidRadarLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	forced := errors.New("invalid radar link history forced rollback")
	var ids []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewInvalidSourceHistoryStore(), NewInvalidSourceHistoryReader(nil)
		for first := byte(11); first <= 12; first++ {
			created, err := store.CreateHistoricalInvalidRadarLink(txCtx, invalidRadarLinkHistoryStoreFixture(first))
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
			loaded, err := reader.GetHistoricalInvalidRadarLink(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, created) {
				return errors.New("radar invalid link caller transaction read mismatch")
			}
			tx, err := platformstore.TxFromContext(txCtx)
			if err != nil {
				return err
			}
			loaded, err = NewInvalidSourceHistoryReader(tx).GetHistoricalInvalidRadarLink(context.Background(), created.ID)
			if err != nil || !reflect.DeepEqual(loaded, created) {
				return errors.New("radar invalid link bare transaction read mismatch")
			}
			if first == 11 {
				if _, err := store.CreateHistoricalInvalidRadarLink(txCtx, invalidRadarLinkHistoryStoreFixture(first)); !errors.Is(err, radarport.ErrInvalidSourceHistoryConflict) {
					return errors.New("radar invalid link duplicate was accepted")
				}
			}
		}
		items, total, err := reader.ListHistoricalInvalidRadarLink(txCtx, radarport.InvalidSourceHistoryQuery{Limit: 1, Offset: int32(before + 1)})
		if err != nil || total != before+2 || len(items) != 1 || items[0].ID != ids[1] {
			return errors.New("radar invalid link page mismatch")
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback err=%v", err)
	}
	after, err := queries.CountHistoricalInvalidRadarLink(ctx)
	if err != nil || after != before {
		t.Fatalf("before=%d after=%d err=%v", before, after, err)
	}
}
func invalidRadarLinkHistoryStoreFixture(first byte) radarport.HistoricalInvalidRadarLink {
	value := radarport.HistoricalInvalidRadarLink{SourceID: -9, Code: "", Title: " \n", RedactedRoots: []string{"destination"}, CreatedAt: time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC), UpdatedAt: time.Date(2026, 8, 29, 9, 11, 12, 123456000, time.UTC), QuarantineReason: "invalid_radar_definition"}
	for index := range value.SourceKeyDigest {
		value.SourceKeyDigest[index] = first + 1
		value.SourcePayloadDigest[index] = first + 2
		value.SourceFieldDigest[index] = first + 3
		value.PrivateDigest[index] = first + 4
		value.DestinationURLDigest[index] = first + 5
	}
	return value
}

type invalidRadarLinkHistoryCallerBeginner struct{ tx pgx.Tx }

func (b invalidRadarLinkHistoryCallerBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return b.tx, nil
}

type invalidRadarLinkHistoryCallerTx struct{ pgx.Tx }

func (*invalidRadarLinkHistoryCallerTx) Commit(context.Context) error   { return nil }
func (*invalidRadarLinkHistoryCallerTx) Rollback(context.Context) error { return nil }

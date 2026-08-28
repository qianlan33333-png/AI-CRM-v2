package store

import (
	"context"
	"errors"
	"flag"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	radardb "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store/generated"
)

var radarClickHistoryPostgresDSN = flag.String("radar-click-history-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00126 for Radar click history rollback verification")

func TestRadarClickHistoryValuePreservesNilReferences(t *testing.T) {
	value := radarClickHistoryStoreFixture(1)
	row := radardb.RadarV1ClickHistory{ID: 4, SourceID: value.SourceID, LinkSourceID: value.LinkSourceID, Code: value.Code, RawStage: value.RawStage,
		SourceChannel: value.SourceChannel, TargetTypeSnapshot: value.TargetTypeSnapshot, SourceChannelSnapshot: value.SourceChannelSnapshot, ErrorCode: value.ErrorCode,
		CreatedAt: pgtype.Timestamptz{Time: value.CreatedAt, Valid: true}, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		OpenIDDigest: value.OpenIDDigest[:], UnionIDDigest: value.UnionIDDigest[:], ExternalUserIDDigest: value.ExternalUserIDDigest[:], CampaignIDDigest: value.CampaignIDDigest[:],
		StaffIDDigest: value.StaffIDDigest[:], UserAgentDigest: value.UserAgentDigest[:], IpDigest: value.IPDigest[:], PersonIDDigest: value.PersonIDDigest[:],
		IpHashDigest: value.IPHashDigest[:], CampaignSnapshotDigest: value.CampaignSnapshotDigest[:], StaffSnapshotDigest: value.StaffSnapshotDigest[:], RefererDigest: value.RefererDigest[:], QueryParamsDigest: value.QueryParamsDigest[:]}
	actual, err := radarClickHistoryValue(row)
	if err != nil || actual.ID != 4 || actual.RadarLinkID != nil || actual.CustomerID != nil || !actual.CreatedAt.Equal(value.CreatedAt) {
		t.Fatalf("actual=%+v err=%v", actual, err)
	}
}

func TestRadarClickHistoryStoreAndReaderFailClosed(t *testing.T) {
	value := radarClickHistoryStoreFixture(2)
	if _, err := NewRadarClickHistoryStore().CreateHistoricalRadarClick(context.Background(), value); !errors.Is(err, radarport.ErrRadarClickHistoryUnavailable) {
		t.Fatalf("caller tx escaped: %v", err)
	}
	var nilPool *pgxpool.Pool
	for _, reader := range []*RadarClickHistoryReader{nil, NewRadarClickHistoryReader(nil), NewRadarClickHistoryReader(nilPool)} {
		if _, _, err := reader.ListHistoricalRadarClick(context.Background(), radarport.RadarClickHistoryQuery{Limit: 1}); !errors.Is(err, radarport.ErrRadarClickHistoryUnavailable) {
			t.Fatalf("reader=%#v err=%v", reader, err)
		}
	}
	reader := NewRadarClickHistoryReader(nil)
	for _, query := range []radarport.RadarClickHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := reader.ListHistoricalRadarClick(context.Background(), query); !errors.Is(err, radarport.ErrRadarClickHistoryInvalid) {
			t.Fatalf("query=%+v err=%v", query, err)
		}
	}
}

func TestRadarClickHistoryPostgresRoundTripRollback(t *testing.T) {
	if *radarClickHistoryPostgresDSN == "" {
		t.Skip("set -radar-click-history-postgres-dsn for isolated migration 00126 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *radarClickHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	before, err := radardb.New(pool).CountHistoricalRadarClick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	forced := errors.New("radar click history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewRadarClickHistoryStore(), NewRadarClickHistoryReader(nil)
		created, err := store.CreateHistoricalRadarClick(txCtx, radarClickHistoryStoreFixture(10))
		if err != nil {
			return err
		}
		loaded, err := reader.GetHistoricalRadarClick(txCtx, created.ID)
		if err != nil || !reflect.DeepEqual(loaded, created) {
			return errors.New("radar click reader mismatch")
		}
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		if loaded, err := NewRadarClickHistoryReader(tx).GetHistoricalRadarClick(context.Background(), created.ID); err != nil || !reflect.DeepEqual(loaded, created) {
			return errors.New("radar click bare transaction reader mismatch")
		}
		items, total, err := reader.ListHistoricalRadarClick(txCtx, radarport.RadarClickHistoryQuery{Limit: 10})
		if err != nil || total < 1 || len(items) == 0 {
			return errors.New("radar click list mismatch")
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback err=%v", err)
	}
	after, err := radardb.New(pool).CountHistoricalRadarClick(ctx)
	if err != nil || after != before {
		t.Fatalf("before=%d after=%d err=%v", before, after, err)
	}
}

func radarClickHistoryStoreFixture(first byte) radarport.HistoricalRadarClick {
	value := radarport.HistoricalRadarClick{SourceID: int64(first), LinkSourceID: int64(first) + 100, Code: "", RawStage: "opened", SourceChannel: "", TargetTypeSnapshot: "", SourceChannelSnapshot: "", ErrorCode: "", CreatedAt: time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)}
	for index := range value.SourceKeyDigest {
		value.SourceKeyDigest[index] = first + 1
		value.SourcePayloadDigest[index] = first + 2
		value.SourceFieldDigest[index] = first + 3
		value.OpenIDDigest[index] = first + 4
		value.UnionIDDigest[index] = first + 5
		value.ExternalUserIDDigest[index] = first + 6
		value.CampaignIDDigest[index] = first + 7
		value.StaffIDDigest[index] = first + 8
		value.UserAgentDigest[index] = first + 9
		value.IPDigest[index] = first + 10
		value.PersonIDDigest[index] = first + 11
		value.IPHashDigest[index] = first + 12
		value.CampaignSnapshotDigest[index] = first + 13
		value.StaffSnapshotDigest[index] = first + 14
		value.RefererDigest[index] = first + 15
		value.QueryParamsDigest[index] = first + 16
	}
	return value
}

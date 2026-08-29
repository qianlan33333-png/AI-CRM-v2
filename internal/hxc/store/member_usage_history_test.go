package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	hxc "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	hxcdb "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var hxcMemberUsageHistoryPostgresDSN = flag.String("hxc-member-usage-history-postgres-dsn", "", "isolated PostgreSQL DSN with migration 00138 for HXC member usage history rollback verification")

func TestHXCMemberUsageHistoryMappingAndBoundaries(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	value := memberUsageStoreFixture(-7, at)
	row := hxcdb.HxcV1MemberUsageHistory{
		ID: 11, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:],
		Generation: value.Generation, Unionid: value.UnionID, OwnerUserid: value.OwnerUserID, MobileHash: value.MobileHash,
		IsMember: value.IsMember, IsRegistered: value.IsRegistered, RegisteredAt: pts(value.RegisteredAt), HasRealUsage: value.HasRealUsage,
		FirstUsedAt: pts(value.FirstUsedAt), LastUsedAt: pts(value.LastUsedAt), MemberSince: pts(value.MemberSince), MembershipExpiresAt: pts(value.MembershipExpiresAt),
		MembershipTier: value.MembershipTier, MembershipStatus: value.MembershipStatus, MembershipSource: value.MembershipSource,
		RegistrationSource: value.RegistrationSource, UsageSource: value.UsageSource, UpdatedAt: pts(value.UpdatedAt), PayloadJson: string(value.PayloadJSON), ProjectedAt: ts(value.ProjectedAt),
	}
	want := value
	want.ID = row.ID
	got, err := hxcMemberUsage(row)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("member usage mapping: got=%#v err=%v", got, err)
	}
	if _, err = NewHXCHistoryStore().GetHistoricalHXCMemberUsage(ctx, 1); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatal("store escaped caller transaction")
	}
	invalid := value
	invalid.SourceFieldDigest = [32]byte{}
	if _, err = NewHXCHistoryStore().CreateHistoricalHXCMemberUsage(ctx, invalid); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
		t.Fatal("invalid value reached database")
	}
	for _, query := range []hxc.HXCMemberUsageHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err = NewHXCHistoryReader(nil).ListHistoricalHXCMemberUsage(ctx, query); !errors.Is(err, hxc.ErrHXCHistoryInvalid) {
			t.Fatalf("invalid query accepted: %#v err=%v", query, err)
		}
	}
	if _, _, err = NewHXCHistoryReader(nil).ListHistoricalHXCMemberUsage(ctx, hxc.HXCMemberUsageHistoryQuery{Limit: 1}); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatal("nil reader did not fail closed")
	}
}

func TestHXCMemberUsageHistoryRejectsCorruptRows(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	value := memberUsageStoreFixture(0, at)
	row := hxcdb.HxcV1MemberUsageHistory{ID: 1, SourceKeyDigest: value.SourceKeyDigest[:], SourcePayloadDigest: value.SourcePayloadDigest[:], SourceFieldDigest: value.SourceFieldDigest[:], ProjectedAt: pgtype.Timestamptz{Time: at, Valid: true}, PayloadJson: "not json"}
	if _, err := hxcMemberUsage(row); !errors.Is(err, hxc.ErrHXCHistoryUnavailable) {
		t.Fatalf("corrupt row err=%v", err)
	}
}

func TestHXCMemberUsageHistoryPostgresRoundTripRollback(t *testing.T) {
	if *hxcMemberUsageHistoryPostgresDSN == "" {
		t.Skip("set -hxc-member-usage-history-postgres-dsn for isolated migration 00138 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *hxcMemberUsageHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	before, err := hxcdb.New(pool).CountHistoricalHXCMemberUsage(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	forced := errors.New("HXC member usage history forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store, reader := NewHXCHistoryStore(), NewHXCHistoryReader(nil)
		first, err := store.CreateHistoricalHXCMemberUsage(txCtx, memberUsageStoreFixture(-7, at))
		if err != nil {
			return fmt.Errorf("create first: %w", err)
		}
		if loaded, err := reader.GetHistoricalHXCMemberUsage(txCtx, first.ID); err != nil || !reflect.DeepEqual(loaded, first) {
			return fmt.Errorf("get first: %v", err)
		}
		second, err := store.CreateHistoricalHXCMemberUsage(txCtx, memberUsageStoreFixture(0, at.Add(time.Second)))
		if err != nil {
			return fmt.Errorf("create second: %w", err)
		}
		generation := int64(-7)
		items, total, err := reader.ListHistoricalHXCMemberUsage(txCtx, hxc.HXCMemberUsageHistoryQuery{Generation: &generation, Limit: 1})
		if err != nil || total != 1 || len(items) != 1 || !reflect.DeepEqual(items[0], first) {
			return fmt.Errorf("generation list: total=%d items=%d err=%v", total, len(items), err)
		}
		all, allTotal, err := reader.ListHistoricalHXCMemberUsage(txCtx, hxc.HXCMemberUsageHistoryQuery{Limit: 1, Offset: 1})
		if err != nil || allTotal != 2 || len(all) != 1 || all[0].ID != second.ID {
			return fmt.Errorf("pagination: total=%d items=%d err=%v", allTotal, len(all), err)
		}
		duplicate := memberUsageStoreFixture(-7, at)
		if _, err = store.CreateHistoricalHXCMemberUsage(txCtx, duplicate); !errors.Is(err, hxc.ErrHXCHistoryConflict) {
			return fmt.Errorf("duplicate source key: %v", err)
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := hxcdb.New(pool).CountHistoricalHXCMemberUsage(ctx, pgtype.Int8{})
	if err != nil || after != before {
		t.Fatalf("rollback count: before=%d after=%d err=%v", before, after, err)
	}
}

func memberUsageStoreFixture(generation int64, at time.Time) hxc.HistoricalHXCMemberUsage {
	registered := at.Add(-6 * time.Hour)
	last := at.Add(-5 * time.Hour)
	expires := at.Add(24 * time.Hour)
	updated := at.Add(-time.Hour)
	var key, payload, field [32]byte
	key[0], payload[0], field[0] = byte(generation+8), byte(generation+28), byte(generation+48)
	return hxc.HistoricalHXCMemberUsage{
		SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, Generation: generation,
		UnionID: "union-private", OwnerUserID: "owner-private", MobileHash: "mobile-private", IsMember: true, IsRegistered: false,
		RegisteredAt: &registered, HasRealUsage: true, LastUsedAt: &last, MembershipExpiresAt: &expires,
		MembershipTier: "legacy-tier", MembershipStatus: "expired", MembershipSource: "v1", RegistrationSource: "v1-registration", UsageSource: "v1-usage", UpdatedAt: &updated,
		PayloadJSON: []byte("{\"opaque\":[1,2],\"nested\":null}"), ProjectedAt: at,
	}
}

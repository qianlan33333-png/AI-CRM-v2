package store

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

var memberGridHistoryPostgresDSN = flag.String("member-grid-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 117 rollback verification")

func TestMemberGridHistoryValuePreservesNullableHistoricalFacts(t *testing.T) {
	at := time.Date(2026, 8, 27, 10, 11, 12, 123456000, time.UTC)
	view := productdb.ProductV1MemberViewHistory{ID: 10, SourceViewID: 11, SourceServiceProductID: 12, Name: "", Position: -1, SchemaVersion: -2, Version: -3,
		CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}, SourceKeyDigest: memberGridHistoryDigest(1), ConfigDigest: memberGridHistoryDigest(2), SourcePayloadDigest: memberGridHistoryDigest(3)}
	viewValue, err := memberGridHistoryView(view)
	if err != nil || viewValue.ProductID != nil || viewValue.Name != "" || viewValue.Position != -1 || viewValue.SchemaVersion != -2 || viewValue.Version != -3 || viewValue.UpdatedAt != at {
		t.Fatalf("lost member view history: %v", err)
	}
	product := int64(8)
	view.ProductID = pgtype.Int8{Int64: product, Valid: true}
	view.Name = " \nmember view\t "
	viewValue, err = memberGridHistoryView(view)
	if err != nil || viewValue.ProductID == nil || *viewValue.ProductID != product || viewValue.Name != view.Name {
		t.Fatalf("member view changed: %v", err)
	}
	usage := productdb.ProductV1MemberUsageHistory{ID: 20, FormallyLoggedIn: true, HasTokenUsage: true, LearningPlanID: "", OpenCount7d: 0,
		RefreshedAt: pgtype.Timestamptz{Time: at, Valid: true}, SourceKeyDigest: memberGridHistoryDigest(4), SourcePayloadDigest: memberGridHistoryDigest(5), RecoveryEntryDigest: memberGridHistoryDigest(6)}
	usageValue, err := memberGridHistoryUsage(usage)
	if err != nil || usageValue.CustomerID != nil || usageValue.LearningPlanCurrent != nil || usageValue.LearningPlanTotal != nil || usageValue.LastOpenAt != nil || usageValue.LearningPlanID != "" {
		t.Fatalf("lost nullable member usage history: %v", err)
	}
	usage.LastOpenAt = pgtype.Timestamptz{Time: at, Valid: true}
	if _, err = memberGridHistoryUsage(usage); err != nil {
		t.Fatal(err)
	}
	usage.RefreshedAt.InfinityModifier = pgtype.Infinity
	if _, err = memberGridHistoryUsage(usage); !errors.Is(err, productport.ErrMemberGridHistoryUnavailable) {
		t.Fatal("infinite source time accepted")
	}
}

func TestMemberGridHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewMemberGridHistoryStore().GetHistoricalMemberView(ctx, 1); !errors.Is(err, productport.ErrMemberGridHistoryUnavailable) {
		t.Fatal("read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*MemberGridHistoryReader{nil, NewMemberGridHistoryReader(nil), NewMemberGridHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalMemberViews(ctx, productport.MemberGridHistoryQuery{Limit: 20}); !errors.Is(err, productport.ErrMemberGridHistoryUnavailable) {
			t.Fatal("nil reader did not fail closed")
		}
	}
	for _, query := range []productport.MemberGridHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberViews(ctx, query); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
			t.Fatal("invalid view page accepted")
		}
		if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberUsage(ctx, query); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
			t.Fatal("invalid usage page accepted")
		}
	}
	invalid := int64(0)
	valid := int64(1)
	if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberViews(ctx, productport.MemberGridHistoryQuery{ProductID: &invalid, Limit: 1}); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
		t.Fatal("invalid view product accepted")
	}
	if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberUsage(ctx, productport.MemberGridHistoryQuery{CustomerID: &invalid, Limit: 1}); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
		t.Fatal("invalid usage customer accepted")
	}
	if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberViews(ctx, productport.MemberGridHistoryQuery{CustomerID: &valid, Limit: 1}); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
		t.Fatal("view customer filter was ignored")
	}
	if _, _, err := NewMemberGridHistoryReader(nil).ListHistoricalMemberUsage(ctx, productport.MemberGridHistoryQuery{ProductID: &valid, Limit: 1}); !errors.Is(err, productport.ErrMemberGridHistoryInvalid) {
		t.Fatal("usage product filter was ignored")
	}
}

func TestMemberGridHistoryPostgresRoundTripRollback(t *testing.T) {
	if *memberGridHistoryPostgresDSN == "" {
		t.Skip("set -member-grid-history-store-postgres-dsn for isolated schema 117 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *memberGridHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := productdb.New(pool)
	viewBefore, err := queries.CountHistoricalMemberViews(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	usageBefore, err := queries.CountHistoricalMemberUsage(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("member grid history forced rollback")
	var viewIDs, usageIDs []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		at := time.Date(2026, 8, 27, 10, 11, 12, 123456000, time.UTC)
		product, err := productdb.New(tx).CreateProduct(txCtx, productdb.CreateProductParams{ProductCode: "member-grid-history-test", Name: "member grid history test", Description: "", PriceMinor: 0, Currency: "CNY", StockQuantity: 0, CreatedBy: 1, CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, LegacyAdminProjection: []byte(`{"schema_version":1}`)})
		if err != nil {
			return err
		}
		productID := product.ID
		customer := int64(900000000)
		viewFilterBefore, err := productdb.New(tx).CountHistoricalMemberViews(txCtx, memberGridHistoryInt(&productID))
		if err != nil {
			return err
		}
		usageFilterBefore, err := productdb.New(tx).CountHistoricalMemberUsage(txCtx, memberGridHistoryInt(&customer))
		if err != nil {
			return err
		}
		store, reader := NewMemberGridHistoryStore(), NewMemberGridHistoryReader(tx)
		for i := 0; i < 3; i++ {
			value := memberGridHistoryViewFixture(byte(i+1), at)
			if i > 0 {
				value.ProductID = &productID
			}
			if i == 2 {
				value.Name = " \nmember view\t "
			}
			created, err := store.CreateHistoricalMemberView(txCtx, value)
			if err != nil {
				return err
			}
			viewIDs = append(viewIDs, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(value, created) {
				return errors.New("SQL member view create changed historical fields")
			}
			loaded, err := reader.GetHistoricalMemberView(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("SQL member view round trip failed: %v", err)
			}
		}
		for i := 0; i < 3; i++ {
			value := memberGridHistoryUsageFixture(byte(i+10), at)
			if i > 0 {
				value.CustomerID = &customer
			}
			if i == 2 {
				value.LearningPlanID = " \nplan\t "
			}
			created, err := store.CreateHistoricalMemberUsage(txCtx, value)
			if err != nil {
				return err
			}
			usageIDs = append(usageIDs, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(value, created) {
				return errors.New("SQL member usage create changed historical fields")
			}
			loaded, err := reader.GetHistoricalMemberUsage(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("SQL member usage round trip failed: %v", err)
			}
		}
		views, total, err := reader.ListHistoricalMemberViews(txCtx, productport.MemberGridHistoryQuery{ProductID: &productID, Limit: 1, Offset: int32(viewFilterBefore + 1)})
		if err != nil || total != viewFilterBefore+2 || len(views) != 1 || views[0].ID != viewIDs[2] || views[0].Name != " \nmember view\t " {
			return fmt.Errorf("SQL member view paging failed: total=%d count=%d err=%v", total, len(views), err)
		}
		usage, total, err := reader.ListHistoricalMemberUsage(txCtx, productport.MemberGridHistoryQuery{CustomerID: &customer, Limit: 1, Offset: int32(usageFilterBefore + 1)})
		if err != nil || total != usageFilterBefore+2 || len(usage) != 1 || usage[0].ID != usageIDs[2] || usage[0].LearningPlanID != " \nplan\t " {
			return fmt.Errorf("SQL member usage paging failed: total=%d count=%d err=%v", total, len(usage), err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	viewAfter, err := queries.CountHistoricalMemberViews(ctx, pgtype.Int8{})
	if err != nil || viewAfter != viewBefore {
		t.Fatal("forced rollback did not preserve member view history")
	}
	usageAfter, err := queries.CountHistoricalMemberUsage(ctx, pgtype.Int8{})
	if err != nil || usageAfter != usageBefore {
		t.Fatal("forced rollback did not preserve member usage history")
	}
	for _, id := range append(viewIDs, usageIDs...) {
		if id == 0 {
			t.Fatal("missing generated id")
		}
	}
	for _, id := range viewIDs {
		if _, err := queries.GetHistoricalMemberView(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back member view remained")
		}
	}
	for _, id := range usageIDs {
		if _, err := queries.GetHistoricalMemberUsage(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back member usage remained")
		}
	}
}

func memberGridHistoryDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}

func memberGridHistoryViewFixture(first byte, at time.Time) productport.HistoricalMemberView {
	var source, config, payload [32]byte
	copy(source[:], memberGridHistoryDigest(first))
	copy(config[:], memberGridHistoryDigest(first+30))
	copy(payload[:], memberGridHistoryDigest(first+60))
	return productport.HistoricalMemberView{SourceKeyDigest: source, SourceViewID: int64(first), SourceServiceProductID: int64(first) + 100, Name: "", Position: -1, SchemaVersion: -2, ConfigDigest: config, Version: -3, CreatedAt: at, UpdatedAt: at, SourcePayloadDigest: payload}
}

func memberGridHistoryUsageFixture(first byte, at time.Time) productport.HistoricalMemberUsage {
	var source, payload, recovery [32]byte
	copy(source[:], memberGridHistoryDigest(first))
	copy(payload[:], memberGridHistoryDigest(first+30))
	copy(recovery[:], memberGridHistoryDigest(first+60))
	return productport.HistoricalMemberUsage{SourceKeyDigest: source, FormallyLoggedIn: true, HasTokenUsage: true, LearningPlanID: "", OpenCount7D: 0, RefreshedAt: at, SourcePayloadDigest: payload, RecoveryEntryDigest: recovery}
}

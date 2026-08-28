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
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var contactHistoryPostgresDSN = flag.String("contact-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 116 rollback verification")

func TestContactHistoryValuePreservesNullableAndEmptyHistoricalFacts(t *testing.T) {
	at := time.Date(2026, 8, 27, 10, 11, 12, 123456000, time.UTC)
	sidebar := contactdb.ContactV1SidebarProfileHistory{ID: 10, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}, SourceKeyDigest: contactHistoryDigest(1), SourcePayloadDigest: contactHistoryDigest(2)}
	value, err := contactHistorySidebarValue(sidebar)
	if err != nil || value.CustomerID != nil || value.Source != "" || value.UpdatedAt != at {
		t.Fatalf("lost nullable sidebar history: %v", err)
	}
	customer := int64(8)
	sidebar.CustomerID = pgtype.Int8{Int64: customer, Valid: true}
	sidebar.IndustryDescription = " \nhistory\t "
	value, err = contactHistorySidebarValue(sidebar)
	if err != nil || value.CustomerID == nil || *value.CustomerID != customer || value.IndustryDescription != sidebar.IndustryDescription {
		t.Fatalf("sidebar history changed: %v", err)
	}
	owner := contactdb.ContactV1OwnerMigrationResultHistory{ID: 11, SourceKeyDigest: contactHistoryDigest(3), SourcePayloadDigest: contactHistoryDigest(4), SessionRelation: "unresolved", PreviewRelation: "resolved", CreatedAt: pgtype.Timestamptz{Time: at, Valid: true}, ExecutedAt: pgtype.Timestamptz{Time: at, Valid: true}}
	ownerValue, err := contactHistoryOwnerValue(owner)
	if err != nil || ownerValue.TransferWelcomeMessage != "" || ownerValue.CreatedAt != at || ownerValue.ExecutedAt != at {
		t.Fatalf("lost owner historical facts: %v", err)
	}
	owner.CreatedAt.InfinityModifier = pgtype.Infinity
	if _, err = contactHistoryOwnerValue(owner); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
		t.Fatal("infinite source time accepted")
	}
}

func TestContactHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewContactHistoryStore().GetHistoricalSidebarProfile(ctx, 1); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
		t.Fatal("read escaped caller transaction")
	}
	var pool *pgxpool.Pool
	for _, reader := range []*ContactHistoryReader{nil, NewContactHistoryReader(nil), NewContactHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalSidebarProfiles(ctx, contactport.ContactHistoryQuery{Limit: 20}); !errors.Is(err, contactport.ErrContactHistoryUnavailable) {
			t.Fatal("nil reader did not fail closed")
		}
	}
	for _, query := range []contactport.ContactHistoryQuery{{Limit: 0}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewContactHistoryReader(nil).ListHistoricalSidebarProfiles(ctx, query); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
			t.Fatal("invalid sidebar page accepted")
		}
	}
	invalidCustomer := int64(0)
	if _, _, err := NewContactHistoryReader(nil).ListHistoricalSidebarProfiles(ctx, contactport.ContactHistoryQuery{CustomerID: &invalidCustomer, Limit: 1}); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
		t.Fatal("invalid sidebar customer accepted")
	}
	validCustomer := int64(1)
	if _, _, err := NewContactHistoryReader(nil).ListHistoricalOwnerMigrationResults(ctx, contactport.ContactHistoryQuery{CustomerID: &validCustomer, Limit: 1}); !errors.Is(err, contactport.ErrContactHistoryInvalid) {
		t.Fatal("owner customer filter was ignored")
	}
}

func TestContactHistoryPostgresRoundTripRollback(t *testing.T) {
	if *contactHistoryPostgresDSN == "" {
		t.Skip("set -contact-history-store-postgres-dsn for isolated schema 116 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *contactHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := contactdb.New(pool)
	sidebarBefore, err := queries.CountHistoricalSidebarProfiles(ctx, pgtype.Int8{})
	if err != nil {
		t.Fatal(err)
	}
	ownerBefore, err := queries.CountHistoricalOwnerMigrationResults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("contact history forced rollback")
	var sidebarIDs, ownerIDs []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		store, reader := NewContactHistoryStore(), NewContactHistoryReader(tx)
		at := time.Date(2026, 8, 27, 10, 11, 12, 123456000, time.UTC)
		customer := int64(999999999)
		for i := 0; i < 3; i++ {
			value := contactHistorySidebarFixture(byte(i+1), at)
			if i > 0 {
				value.CustomerID = &customer
			}
			if i == 2 {
				value.IndustryDescription = " \nhistory\t "
			}
			created, err := store.CreateHistoricalSidebarProfile(txCtx, value)
			if err != nil {
				return err
			}
			sidebarIDs = append(sidebarIDs, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(value, created) {
				return errors.New("SQL sidebar create changed historical fields")
			}
			loaded, err := reader.GetHistoricalSidebarProfile(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("SQL sidebar round trip failed: %v", err)
			}
		}
		for i := 0; i < 2; i++ {
			value := contactHistoryOwnerFixture(byte(i+10), at)
			if i == 1 {
				value.TransferWelcomeMessage = " \nwelcome\t "
			}
			created, err := store.CreateHistoricalOwnerMigrationResult(txCtx, value)
			if err != nil {
				return err
			}
			ownerIDs = append(ownerIDs, created.ID)
			value.ID = created.ID
			if !reflect.DeepEqual(value, created) {
				return errors.New("SQL owner create changed historical fields")
			}
			loaded, err := reader.GetHistoricalOwnerMigrationResult(txCtx, created.ID)
			if err != nil || !reflect.DeepEqual(loaded, value) {
				return fmt.Errorf("SQL owner round trip failed: %v", err)
			}
		}
		items, total, err := reader.ListHistoricalSidebarProfiles(txCtx, contactport.ContactHistoryQuery{CustomerID: &customer, Limit: 1, Offset: 1})
		if err != nil || total != 2 || len(items) != 1 || items[0].ID != sidebarIDs[2] || items[0].IndustryDescription != " \nhistory\t " {
			return fmt.Errorf("SQL sidebar paging failed: total=%d count=%d err=%v", total, len(items), err)
		}
		owners, total, err := reader.ListHistoricalOwnerMigrationResults(txCtx, contactport.ContactHistoryQuery{Limit: 1, Offset: int32(ownerBefore + 1)})
		if err != nil || total != ownerBefore+2 || len(owners) != 1 || owners[0].ID != ownerIDs[1] || owners[0].TransferWelcomeMessage != " \nwelcome\t " {
			return fmt.Errorf("SQL owner paging failed: total=%d count=%d err=%v", total, len(owners), err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	sidebarAfter, err := queries.CountHistoricalSidebarProfiles(ctx, pgtype.Int8{})
	if err != nil || sidebarAfter != sidebarBefore {
		t.Fatal("forced rollback did not preserve sidebar history")
	}
	ownerAfter, err := queries.CountHistoricalOwnerMigrationResults(ctx)
	if err != nil || ownerAfter != ownerBefore {
		t.Fatal("forced rollback did not preserve owner history")
	}
	for _, id := range append(sidebarIDs, ownerIDs...) {
		if id == 0 {
			t.Fatal("missing generated id")
		}
	}
	for _, id := range sidebarIDs {
		if _, err := queries.GetHistoricalSidebarProfile(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back sidebar row remained")
		}
	}
	for _, id := range ownerIDs {
		if _, err := queries.GetHistoricalOwnerMigrationResult(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("rolled back owner row remained")
		}
	}
}

func contactHistoryDigest(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}

func contactHistorySidebarFixture(first byte, at time.Time) contactport.HistoricalSidebarProfile {
	var source, payload [32]byte
	copy(source[:], contactHistoryDigest(first))
	copy(payload[:], contactHistoryDigest(first+30))
	return contactport.HistoricalSidebarProfile{SourceKeyDigest: source, Source: "", Industry: "", IndustryDescription: "", NeedsBlockersFollowup: "", UpdatedAt: at, SourcePayloadDigest: payload}
}

func contactHistoryOwnerFixture(first byte, at time.Time) contactport.HistoricalOwnerMigrationResult {
	var source, payload [32]byte
	copy(source[:], contactHistoryDigest(first))
	copy(payload[:], contactHistoryDigest(first+30))
	return contactport.HistoricalOwnerMigrationResult{SourceKeyDigest: source, ScopeType: "", FileHash: "", PreviewHash: "", SessionRelation: "unresolved", PreviewRelation: "unresolved", CreatedAt: at, ExecutedAt: at, SourcePayloadDigest: payload}
}

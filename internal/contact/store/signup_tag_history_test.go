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
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var signupTagHistoryPostgresDSN = flag.String("signup-tag-history-store-postgres-dsn", "", "isolated PostgreSQL DSN for schema 120 rollback verification")

func TestSignupTagHistoryValuePreservesHistoricalFields(t *testing.T) {
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
	row := contactdb.ContactV1SignupTagRule{ID: 10, SourceKeyDigest: signupTagHistoryBytes(1), SourcePayloadDigest: signupTagHistoryBytes(2), TagSourceID: "v1/tag/7", TagName: " \n标签\t ", SignupStatus: "", OriginalActive: false, UpdatedAt: pgtype.Timestamptz{Time: at, Valid: true}}
	value, err := signupTagHistoryValue(row)
	if err != nil || value.ID != 10 || value.TagSourceID != row.TagSourceID || value.TagName != row.TagName || value.SignupStatus != "" || value.OriginalActive || !value.UpdatedAt.Equal(at) {
		t.Fatalf("value=%+v err=%v", value, err)
	}
	row.UpdatedAt.InfinityModifier = pgtype.Infinity
	if _, err = signupTagHistoryValue(row); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
		t.Fatalf("infinite time error=%v", err)
	}
}

func TestSignupTagHistoryStoreRequiresCallerTransactionAndStrictPage(t *testing.T) {
	ctx := context.Background()
	if _, err := NewSignupTagHistoryStore().GetHistoricalSignupTagRule(ctx, 1); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
		t.Fatalf("caller transaction escaped error=%v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*SignupTagHistoryReader{nil, NewSignupTagHistoryReader(nil), NewSignupTagHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalSignupTagRules(ctx, 20, 0); !errors.Is(err, contactport.ErrSignupTagHistoryUnavailable) {
			t.Fatalf("nil reader error=%v", err)
		}
	}
	for _, page := range [][2]int32{{0, 0}, {101, 0}, {1, -1}} {
		if _, _, err := NewSignupTagHistoryReader(nil).ListHistoricalSignupTagRules(ctx, page[0], page[1]); !errors.Is(err, contactport.ErrSignupTagHistoryInvalid) {
			t.Fatalf("page=%v error=%v", page, err)
		}
	}
}

func TestSignupTagHistoryPostgresRoundTripRollback(t *testing.T) {
	if *signupTagHistoryPostgresDSN == "" {
		t.Skip("set -signup-tag-history-store-postgres-dsn for isolated schema 120 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *signupTagHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := contactdb.New(pool)
	before, err := queries.CountHistoricalSignupTagRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("signup tag history forced rollback")
	var ids []int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		store, reader := NewSignupTagHistoryStore(), NewSignupTagHistoryReader(tx)
		at := time.Date(2026, 8, 28, 10, 11, 12, 123456000, time.UTC)
		for index := 0; index < 2; index++ {
			value := signupTagHistoryStoreFixture(byte(index+1), at.Add(time.Duration(index)*time.Second))
			if index == 1 {
				value.TagName = " \nhistory\t "
			}
			created, createErr := store.CreateHistoricalSignupTagRule(txCtx, value)
			if createErr != nil {
				return createErr
			}
			value.ID = created.ID
			if !reflect.DeepEqual(created, value) {
				return errors.New("SQL signup tag create changed history")
			}
			loaded, getErr := reader.GetHistoricalSignupTagRule(txCtx, created.ID)
			if getErr != nil || !reflect.DeepEqual(loaded, value) {
				return errors.New("SQL signup tag round trip failed")
			}
			ids = append(ids, created.ID)
		}
		items, total, listErr := reader.ListHistoricalSignupTagRules(txCtx, 1, int32(before+1))
		if listErr != nil || total != before+2 || len(items) != 1 || items[0].ID != ids[1] || items[0].TagName != " \nhistory\t " {
			return errors.New("SQL signup tag page failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback transaction: %v", err)
	}
	after, err := queries.CountHistoricalSignupTagRules(ctx)
	if err != nil || after != before {
		t.Fatalf("rollback count before=%d after=%d err=%v", before, after, err)
	}
	for _, id := range ids {
		if id < 1 {
			t.Fatal("missing generated id")
		}
		if _, err := queries.GetHistoricalSignupTagRule(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("rolled-back row %d remained err=%v", id, err)
		}
	}
}

func signupTagHistoryBytes(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}

func signupTagHistoryStoreFixture(first byte, updatedAt time.Time) contactport.HistoricalSignupTagRule {
	var source, payload [32]byte
	copy(source[:], signupTagHistoryBytes(first))
	copy(payload[:], signupTagHistoryBytes(first+30))
	return contactport.HistoricalSignupTagRule{SourceKeyDigest: source, SourcePayloadDigest: payload, TagSourceID: "v1/tag", TagName: "", SignupStatus: "", OriginalActive: false, UpdatedAt: updatedAt}
}

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
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var wecomContactHistoryPostgresDSN = flag.String("wecom-contact-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 125 rollback verification")

func TestWeComContactHistoryStrictReaderAndMapping(t *testing.T) {
	ctx := context.Background()
	if _, err := NewWeComContactHistoryStore().GetHistoricalWeComExternalContactEventLog(ctx, 1); !errors.Is(err, contact.ErrWeComContactHistoryUnavailable) {
		t.Fatal(err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*WeComContactHistoryReader{nil, NewWeComContactHistoryReader(nil), NewWeComContactHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalWeComExternalContactEventLog(ctx, contact.WeComContactHistoryQuery{Limit: 1}); !errors.Is(err, contact.ErrWeComContactHistoryUnavailable) {
			t.Fatal(err)
		}
	}
	for _, query := range []contact.WeComContactHistoryQuery{{}, {Limit: 101}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewWeComContactHistoryReader(nil).ListHistoricalWeComExternalContactFollowUser(ctx, query); !errors.Is(err, contact.ErrWeComContactHistoryInvalid) {
			t.Fatal(err)
		}
	}
	at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
	event, err := wecomEventValue(contactdb.ContactV1WecomEventLogHistory{ID: 1, SourceKeyDigest: wecomBytes(1), SourcePayloadDigest: wecomBytes(2), SourceFieldDigest: wecomBytes(3), SourceID: -7, CorpIDDigest: wecomBytes(4), ExternalUserIDDigest: wecomBytes(5), UserIDDigest: wecomBytes(6), EventTime: pgtype.Int8{}, EventKeyDigest: wecomBytes(7), PayloadXmlDigest: wecomBytes(8), PayloadJsonDigest: wecomBytes(9), RetryCount: -2, ErrorMessageDigest: wecomBytes(10), CreatedAt: wecomStoredTime(at), UpdatedAt: wecomStoredTime(at.Add(-time.Second)), IdentitySyncErrorCodeDigest: wecomBytes(11), IdentitySyncErrorMessageDigest: wecomBytes(12), IdentitySyncResponseDigest: wecomBytes(13)})
	if err != nil || event.SourceID != -7 || event.EventTime != nil || event.RetryCount != -2 {
		t.Fatalf("event=%+v err=%v", event, err)
	}
	addWay, createTime := int32(-4), int64(-5)
	follow, err := wecomFollowValue(contactdb.ContactV1WecomFollowUserHistory{ID: 2, SourceKeyDigest: wecomBytes(14), SourcePayloadDigest: wecomBytes(15), SourceFieldDigest: wecomBytes(16), SourceID: 0, CorpIDDigest: wecomBytes(17), ExternalUserIDDigest: wecomBytes(18), UserIDDigest: wecomBytes(19), RemarkDigest: wecomBytes(20), DescriptionDigest: wecomBytes(21), AddWay: pgtype.Int4{Int32: addWay, Valid: true}, State: "private-state", OperUserIDDigest: wecomBytes(22), CreateTime: pgtype.Int8{Int64: createTime, Valid: true}, RawFollowUserDigest: wecomBytes(23), FirstSeenAt: wecomStoredTime(at), LastSeenAt: wecomStoredTime(at.Add(-time.Second)), CreatedAt: wecomStoredTime(at), UpdatedAt: wecomStoredTime(at.Add(-2 * time.Second))})
	if err != nil || follow.SourceID != 0 || follow.AddWay == nil || *follow.AddWay != -4 || follow.CreateTime == nil || *follow.CreateTime != -5 || follow.State != "private-state" {
		t.Fatalf("follow=%+v err=%v", follow, err)
	}
}

func TestWeComContactHistoryPostgresRoundTripRollback(t *testing.T) {
	if *wecomContactHistoryPostgresDSN == "" {
		t.Skip("set -wecom-contact-history-postgres-dsn for isolated schema 125 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *wecomContactHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := contactdb.New(pool)
	beforeEvents, err := queries.CountHistoricalWeComExternalContactEventLog(ctx)
	if err != nil {
		t.Fatal("wecom-history stage=count-events")
	}
	beforeFollows, err := queries.CountHistoricalWeComExternalContactFollowUser(ctx)
	if err != nil {
		t.Fatal("wecom-history stage=count-follows")
	}
	rollback := errors.New("wecom contact rollback")
	var eventID, followID int64
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		store := NewWeComContactHistoryStore()
		tx, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return fmt.Errorf("wecom-history stage=tx: %w", err)
		}
		poolReader, txReader := NewWeComContactHistoryReader(pool), NewWeComContactHistoryReader(tx)
		at := time.Date(2026, 8, 28, 1, 2, 3, 123456000, time.UTC)
		event, err := store.CreateHistoricalWeComExternalContactEventLog(txCtx, wecomStoreEvent(at))
		if err != nil {
			return fmt.Errorf("wecom-history stage=event-create: %w", err)
		}
		eventID = event.ID
		if loaded, err := poolReader.GetHistoricalWeComExternalContactEventLog(txCtx, event.ID); err != nil || !reflect.DeepEqual(loaded, event) {
			return fmt.Errorf("wecom-history stage=event-caller-tx: %w", err)
		}
		if loaded, err := txReader.GetHistoricalWeComExternalContactEventLog(context.Background(), event.ID); err != nil || !reflect.DeepEqual(loaded, event) {
			return fmt.Errorf("wecom-history stage=event-bare-tx: %w", err)
		}
		follow, err := store.CreateHistoricalWeComExternalContactFollowUser(txCtx, wecomStoreFollow(at))
		if err != nil {
			return fmt.Errorf("wecom-history stage=follow-create: %w", err)
		}
		followID = follow.ID
		if loaded, err := txReader.GetHistoricalWeComExternalContactFollowUser(context.Background(), follow.ID); err != nil || !reflect.DeepEqual(loaded, follow) {
			return fmt.Errorf("wecom-history stage=follow-bare-tx: %w", err)
		}
		if values, total, err := poolReader.ListHistoricalWeComExternalContactEventLog(txCtx, contact.WeComContactHistoryQuery{Limit: 1, Offset: int32(beforeEvents + 1)}); err != nil || total != beforeEvents+1 || len(values) != 0 {
			return fmt.Errorf("wecom-history stage=page: %w", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	afterEvents, eventErr := queries.CountHistoricalWeComExternalContactEventLog(ctx)
	afterFollows, followErr := queries.CountHistoricalWeComExternalContactFollowUser(ctx)
	if eventErr != nil || followErr != nil || afterEvents != beforeEvents || afterFollows != beforeFollows {
		t.Fatal("wecom-history rollback retained rows")
	}
	if _, err := queries.GetHistoricalWeComExternalContactEventLog(ctx, eventID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("event row remains after rollback")
	}
	if _, err := queries.GetHistoricalWeComExternalContactFollowUser(ctx, followID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("follow row remains after rollback")
	}
}

func wecomBytes(value byte) []byte   { result := make([]byte, 32); result[0] = value; return result }
func wecomArray(value byte) [32]byte { var result [32]byte; result[0] = value; return result }
func wecomStoredTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func wecomStoreEvent(at time.Time) contact.HistoricalWeComExternalContactEventLog {
	eventTime := int64(-8)
	return contact.HistoricalWeComExternalContactEventLog{SourceKeyDigest: wecomArray(1), SourcePayloadDigest: wecomArray(2), SourceFieldDigest: wecomArray(3), SourceID: -7, CorpIDDigest: wecomArray(4), ExternalUserIDDigest: wecomArray(5), UserIDDigest: wecomArray(6), EventTime: &eventTime, EventKeyDigest: wecomArray(7), PayloadXMLDigest: wecomArray(8), PayloadJSONDigest: wecomArray(9), RetryCount: -2, ErrorMessageDigest: wecomArray(10), CreatedAt: at, UpdatedAt: at.Add(-time.Second), IdentitySyncErrorCodeDigest: wecomArray(11), IdentitySyncErrorMessageDigest: wecomArray(12), IdentitySyncResponseDigest: wecomArray(13)}
}
func wecomStoreFollow(at time.Time) contact.HistoricalWeComExternalContactFollowUser {
	addWay := int32(-4)
	return contact.HistoricalWeComExternalContactFollowUser{SourceKeyDigest: wecomArray(14), SourcePayloadDigest: wecomArray(15), SourceFieldDigest: wecomArray(16), SourceID: 0, CorpIDDigest: wecomArray(17), ExternalUserIDDigest: wecomArray(18), UserIDDigest: wecomArray(19), RemarkDigest: wecomArray(20), DescriptionDigest: wecomArray(21), AddWay: &addWay, State: "private-state", OperUserIDDigest: wecomArray(22), RawFollowUserDigest: wecomArray(23), FirstSeenAt: at, LastSeenAt: at.Add(-time.Second), CreatedAt: at, UpdatedAt: at.Add(-2 * time.Second)}
}

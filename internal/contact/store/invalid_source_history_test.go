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
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var invalidSourceHistoryPostgresDSN = flag.String("contact-invalid-source-history-postgres-dsn", "", "isolated PostgreSQL DSN for schema 133 rollback verification")

func TestInvalidSourceHistoryValuesPreserveSignedEmptyAndPrivateFields(t *testing.T) {
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
	tag, err := invalidSourceHistoryUnboundTagValue(contactdb.ContactV1UnboundTagHistory{ID: 1, SourceKeyDigest: invalidSourceHistoryBytes(1), SourcePayloadDigest: invalidSourceHistoryBytes(2), SourceFieldDigest: invalidSourceHistoryBytes(3), PrivateDigest: invalidSourceHistoryBytes(4), RedactedRoots: []string{}, TagSourceID: "", UnionIDDigest: invalidSourceHistoryBytes(5), CreatedAt: invalidSourceHistoryStoredTimestamp(at), QuarantineReason: "invalid_contact_tag"})
	if err != nil || tag.TagSourceID != "" || tag.RedactedRoots == nil || tag.CreatedAt != at {
		t.Fatalf("unbound tag changed: %#v err=%v", tag, err)
	}
	channel, err := invalidSourceHistoryInvalidChannelValue(contactdb.ContactV1InvalidChannelHistory{ID: 2, SourceKeyDigest: invalidSourceHistoryBytes(6), SourcePayloadDigest: invalidSourceHistoryBytes(7), SourceFieldDigest: invalidSourceHistoryBytes(8), PrivateDigest: invalidSourceHistoryBytes(9), RedactedRoots: []string{"source"}, SourceID: -9, Code: "", Name: "", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: invalidSourceHistoryStoredTimestamp(at), UpdatedAt: invalidSourceHistoryStoredTimestamp(at.Add(-time.Second)), QuarantineReason: "invalid_channel_definition"})
	if err != nil || channel.SourceID != -9 || channel.Code != "" || !reflect.DeepEqual(channel.RedactedRoots, []string{"source"}) {
		t.Fatalf("channel changed: %#v err=%v", channel, err)
	}
}

func TestInvalidSourceHistoryStoreRequiresTransactionAndStrictPage(t *testing.T) {
	if _, err := NewInvalidSourceHistoryStore().GetHistoricalUnboundTag(context.Background(), 1); !errors.Is(err, contact.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("store escaped tx: %v", err)
	}
	var pool *pgxpool.Pool
	for _, reader := range []*InvalidSourceHistoryReader{nil, NewInvalidSourceHistoryReader(nil), NewInvalidSourceHistoryReader(pool)} {
		if _, _, err := reader.ListHistoricalInvalidChannel(context.Background(), contact.InvalidSourceHistoryQuery{Limit: 20}); !errors.Is(err, contact.ErrInvalidSourceHistoryUnavailable) {
			t.Fatalf("nil reader=%v", err)
		}
	}
	for _, query := range []contact.InvalidSourceHistoryQuery{{Limit: 0}, {Limit: 201}, {Limit: 1, Offset: -1}} {
		if _, _, err := NewInvalidSourceHistoryReader(nil).ListHistoricalUnboundTag(context.Background(), query); !errors.Is(err, contact.ErrInvalidSourceHistoryInvalid) {
			t.Fatalf("invalid page=%v", err)
		}
	}
}

func TestInvalidSourceHistoryPostgresRoundTripRollback(t *testing.T) {
	if *invalidSourceHistoryPostgresDSN == "" {
		t.Skip("set -contact-invalid-source-history-postgres-dsn for isolated schema 133 test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *invalidSourceHistoryPostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	queries := contactdb.New(pool)
	tagBefore, err := queries.CountHistoricalUnboundTag(ctx)
	if err != nil {
		t.Fatal(err)
	}
	channelBefore, err := queries.CountHistoricalInvalidChannel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("forced rollback")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		tx, txErr := platformstore.TxFromContext(txCtx)
		if txErr != nil {
			return txErr
		}
		store, reader := NewInvalidSourceHistoryStore(), NewInvalidSourceHistoryReader(tx)
		at := time.Date(2026, 8, 29, 10, 11, 12, 123456000, time.UTC)
		tag := invalidSourceHistoryTagStoreFixture(1, at)
		createdTag, createErr := store.CreateHistoricalUnboundTag(txCtx, tag)
		if createErr != nil {
			return createErr
		}
		tag.ID = createdTag.ID
		if !reflect.DeepEqual(tag, createdTag) {
			return errors.New("tag create changed source fact")
		}
		loadedTag, getErr := reader.GetHistoricalUnboundTag(txCtx, tag.ID)
		if getErr != nil || !reflect.DeepEqual(tag, loadedTag) {
			return errors.New("tag readback changed source fact")
		}
		channel := invalidSourceHistoryChannelStoreFixture(10, at)
		createdChannel, createErr := store.CreateHistoricalInvalidChannel(txCtx, channel)
		if createErr != nil {
			return createErr
		}
		channel.ID = createdChannel.ID
		if !reflect.DeepEqual(channel, createdChannel) {
			return errors.New("channel create changed source fact")
		}
		loadedChannel, getErr := reader.GetHistoricalInvalidChannel(txCtx, channel.ID)
		if getErr != nil || !reflect.DeepEqual(channel, loadedChannel) {
			return errors.New("channel readback changed source fact")
		}
		items, total, listErr := reader.ListHistoricalInvalidChannel(txCtx, contact.InvalidSourceHistoryQuery{Limit: 1})
		if listErr != nil || total != channelBefore+1 || len(items) != 1 || items[0].ID != channel.ID {
			return errors.New("channel list failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback=%v", err)
	}
	tagAfter, err := queries.CountHistoricalUnboundTag(ctx)
	if err != nil || tagAfter != tagBefore {
		t.Fatal("tag rollback leaked")
	}
	channelAfter, err := queries.CountHistoricalInvalidChannel(ctx)
	if err != nil || channelAfter != channelBefore {
		t.Fatal("channel rollback leaked")
	}
}

func invalidSourceHistoryBytes(first byte) []byte {
	value := make([]byte, 32)
	value[0] = first
	return value
}
func invalidSourceHistoryArray(first byte) [32]byte {
	var value [32]byte
	value[0] = first
	return value
}
func invalidSourceHistoryStoredTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func invalidSourceHistoryTagStoreFixture(first byte, at time.Time) contact.HistoricalUnboundTag {
	return contact.HistoricalUnboundTag{SourceKeyDigest: invalidSourceHistoryArray(first), SourcePayloadDigest: invalidSourceHistoryArray(first + 1), SourceFieldDigest: invalidSourceHistoryArray(first + 2), PrivateDigest: invalidSourceHistoryArray(first + 3), RedactedRoots: []string{}, TagSourceID: "", UnionIDDigest: invalidSourceHistoryArray(first + 4), CreatedAt: at, QuarantineReason: "invalid_contact_tag"}
}
func invalidSourceHistoryChannelStoreFixture(first byte, at time.Time) contact.HistoricalInvalidChannel {
	return contact.HistoricalInvalidChannel{SourceKeyDigest: invalidSourceHistoryArray(first), SourcePayloadDigest: invalidSourceHistoryArray(first + 1), SourceFieldDigest: invalidSourceHistoryArray(first + 2), PrivateDigest: invalidSourceHistoryArray(first + 3), RedactedRoots: []string{"source"}, SourceID: -1, Code: "", Name: "name", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: at, UpdatedAt: at.Add(-time.Second), QuarantineReason: "invalid_channel_definition"}
}

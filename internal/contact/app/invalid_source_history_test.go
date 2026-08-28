package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

func TestInvalidSourceHistoryWriterCreatesReplaysAndDetectsPrivateDrift(t *testing.T) {
	store := &invalidSourceHistoryStoreFake{}
	journal := &invalidSourceHistoryJournalFake{receipts: map[string]contact.InvalidSourceHistoryReceipt{}}
	writer := NewInvalidSourceHistoryWriter(store, journal)
	tagSource, tag := invalidSourceHistoryTagFixture("tag")
	first, err := writer.ImportHistoricalUnboundTag(context.Background(), tagSource, tag)
	if err != nil || first.Kind != invalidSourceHistoryUnboundTagKind || first.Replayed || first.TargetID != 41 || store.tagCreates != 1 {
		t.Fatalf("first unbound tag receipt=%#v err=%v", first, err)
	}
	replay, err := writer.ImportHistoricalUnboundTag(context.Background(), tagSource, tag)
	if err != nil || !replay.Replayed || store.tagCreates != 1 || store.tagGets != 1 {
		t.Fatalf("unbound tag replay=%#v err=%v", replay, err)
	}
	changed := store.tags[first.TargetID]
	changed.PrivateDigest[0]++
	store.tags[first.TargetID] = changed
	if _, err := writer.ImportHistoricalUnboundTag(context.Background(), tagSource, tag); !errors.Is(err, contact.ErrInvalidSourceHistoryConflict) {
		t.Fatalf("private digest drift=%v", err)
	}

	channelSource, channel := invalidSourceHistoryChannelFixture("channel")
	channel.SourceID = -7
	channel.Code = ""
	first, err = writer.ImportHistoricalInvalidChannel(context.Background(), channelSource, channel)
	if err != nil || first.Kind != invalidSourceHistoryInvalidChannelKind || first.Replayed || first.TargetID != 61 || store.channelCreates != 1 {
		t.Fatalf("first invalid channel receipt=%#v err=%v", first, err)
	}
	replay, err = writer.ImportHistoricalInvalidChannel(context.Background(), channelSource, channel)
	if err != nil || !replay.Replayed || store.channelCreates != 1 || store.channelGets != 1 {
		t.Fatalf("invalid channel replay=%#v err=%v", replay, err)
	}
	changedChannel := store.channels[first.TargetID]
	changedChannel.RedactedRoots = []string{"changed"}
	store.channels[first.TargetID] = changedChannel
	if _, err := writer.ImportHistoricalInvalidChannel(context.Background(), channelSource, channel); !errors.Is(err, contact.ErrInvalidSourceHistoryConflict) {
		t.Fatalf("roots drift=%v", err)
	}
}

func TestInvalidSourceHistoryWriterRejectsUnsafeInputAndForwardsCallerContext(t *testing.T) {
	tagSource, tag := invalidSourceHistoryTagFixture("tag")
	channelSource, channel := invalidSourceHistoryChannelFixture("channel")
	for _, mutate := range []func(*contact.HistoricalUnboundTag){
		func(value *contact.HistoricalUnboundTag) { value.UnionIDDigest = [32]byte{} },
		func(value *contact.HistoricalUnboundTag) { value.QuarantineReason = "other" },
		func(value *contact.HistoricalUnboundTag) { value.TagSourceID = string([]byte{0xff}) },
	} {
		value := tag
		mutate(&value)
		if _, err := NewInvalidSourceHistoryWriter(&invalidSourceHistoryStoreFake{}, &invalidSourceHistoryJournalFake{}).ImportHistoricalUnboundTag(context.Background(), tagSource, value); !errors.Is(err, contact.ErrInvalidSourceHistoryInvalid) {
			t.Fatalf("unsafe tag accepted: %v", err)
		}
	}
	for _, mutate := range []func(*contact.HistoricalInvalidChannel){
		func(value *contact.HistoricalInvalidChannel) { value.CreatedAt = time.Time{} },
		func(value *contact.HistoricalInvalidChannel) { value.Name = "bad\x00" },
		func(value *contact.HistoricalInvalidChannel) { value.QuarantineReason = "other" },
	} {
		value := channel
		mutate(&value)
		if _, err := NewInvalidSourceHistoryWriter(&invalidSourceHistoryStoreFake{}, &invalidSourceHistoryJournalFake{}).ImportHistoricalInvalidChannel(context.Background(), channelSource, value); !errors.Is(err, contact.ErrInvalidSourceHistoryInvalid) {
			t.Fatalf("unsafe channel accepted: %v", err)
		}
	}
	store := &invalidSourceHistoryStoreFake{requireContext: true}
	if _, err := NewInvalidSourceHistoryWriter(store, &invalidSourceHistoryJournalFake{}).ImportHistoricalUnboundTag(context.Background(), tagSource, tag); !errors.Is(err, contact.ErrInvalidSourceHistoryUnavailable) {
		t.Fatalf("caller context bypass=%v", err)
	}
	ctx := context.WithValue(context.Background(), invalidSourceHistoryContextKey{}, "caller")
	if _, err := NewInvalidSourceHistoryWriter(store, &invalidSourceHistoryJournalFake{receipts: map[string]contact.InvalidSourceHistoryReceipt{}}).ImportHistoricalInvalidChannel(ctx, channelSource, channel); err != nil {
		t.Fatalf("caller context not forwarded: %v", err)
	}
}

func TestInvalidSourceHistoryDigestIncludesAllPrivateFields(t *testing.T) {
	_, tag := invalidSourceHistoryTagFixture("tag")
	baseTag := withHistoricalUnboundTagID(tag, 1)
	digest, err := DigestHistoricalUnboundTag(baseTag)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contact.HistoricalUnboundTag){
		func(value *contact.HistoricalUnboundTag) { value.SourceFieldDigest[0]++ },
		func(value *contact.HistoricalUnboundTag) { value.PrivateDigest[0]++ },
		func(value *contact.HistoricalUnboundTag) { value.UnionIDDigest[0]++ },
		func(value *contact.HistoricalUnboundTag) { value.RedactedRoots = []string{"root"} },
		func(value *contact.HistoricalUnboundTag) { value.TagSourceID = "changed" },
	} {
		value := baseTag
		mutate(&value)
		changed, changeErr := DigestHistoricalUnboundTag(value)
		if changeErr != nil || changed == digest {
			t.Fatalf("tag digest did not cover mutation: %v", changeErr)
		}
	}
	_, channel := invalidSourceHistoryChannelFixture("channel")
	baseChannel := withHistoricalInvalidChannelID(channel, 1)
	digest, err = DigestHistoricalInvalidChannel(baseChannel)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contact.HistoricalInvalidChannel){
		func(value *contact.HistoricalInvalidChannel) { value.SourceFieldDigest[0]++ },
		func(value *contact.HistoricalInvalidChannel) { value.PrivateDigest[0]++ },
		func(value *contact.HistoricalInvalidChannel) { value.RedactedRoots = []string{"root"} },
		func(value *contact.HistoricalInvalidChannel) { value.SourceID-- },
		func(value *contact.HistoricalInvalidChannel) { value.Code = "changed" },
		func(value *contact.HistoricalInvalidChannel) { value.UpdatedAt = value.UpdatedAt.Add(time.Microsecond) },
	} {
		value := baseChannel
		mutate(&value)
		changed, changeErr := DigestHistoricalInvalidChannel(value)
		if changeErr != nil || changed == digest {
			t.Fatalf("channel digest did not cover mutation: %v", changeErr)
		}
	}
}

type invalidSourceHistoryStoreFake struct {
	tags                                             map[int64]contact.HistoricalUnboundTag
	channels                                         map[int64]contact.HistoricalInvalidChannel
	tagCreates, tagGets, channelCreates, channelGets int
	requireContext                                   bool
}

func (store *invalidSourceHistoryStoreFake) valid(ctx context.Context) bool {
	return !store.requireContext || ctx.Value(invalidSourceHistoryContextKey{}) == "caller"
}
func (store *invalidSourceHistoryStoreFake) CreateHistoricalUnboundTag(ctx context.Context, value contact.HistoricalUnboundTag) (contact.HistoricalUnboundTag, error) {
	if !store.valid(ctx) {
		return contact.HistoricalUnboundTag{}, errors.New("caller transaction required")
	}
	if store.tags == nil {
		store.tags = map[int64]contact.HistoricalUnboundTag{}
	}
	store.tagCreates++
	value.ID = 41
	store.tags[value.ID] = value
	return value, nil
}
func (store *invalidSourceHistoryStoreFake) GetHistoricalUnboundTag(ctx context.Context, id int64) (contact.HistoricalUnboundTag, error) {
	if !store.valid(ctx) {
		return contact.HistoricalUnboundTag{}, errors.New("caller transaction required")
	}
	store.tagGets++
	value, ok := store.tags[id]
	if !ok {
		return contact.HistoricalUnboundTag{}, contact.ErrInvalidSourceHistoryConflict
	}
	return value, nil
}
func (store *invalidSourceHistoryStoreFake) CreateHistoricalInvalidChannel(ctx context.Context, value contact.HistoricalInvalidChannel) (contact.HistoricalInvalidChannel, error) {
	if !store.valid(ctx) {
		return contact.HistoricalInvalidChannel{}, errors.New("caller transaction required")
	}
	if store.channels == nil {
		store.channels = map[int64]contact.HistoricalInvalidChannel{}
	}
	store.channelCreates++
	value.ID = 61
	store.channels[value.ID] = value
	return value, nil
}
func (store *invalidSourceHistoryStoreFake) GetHistoricalInvalidChannel(ctx context.Context, id int64) (contact.HistoricalInvalidChannel, error) {
	if !store.valid(ctx) {
		return contact.HistoricalInvalidChannel{}, errors.New("caller transaction required")
	}
	store.channelGets++
	value, ok := store.channels[id]
	if !ok {
		return contact.HistoricalInvalidChannel{}, contact.ErrInvalidSourceHistoryConflict
	}
	return value, nil
}

type invalidSourceHistoryJournalFake struct {
	receipts map[string]contact.InvalidSourceHistoryReceipt
}

func (journal *invalidSourceHistoryJournalFake) LoadInvalidSourceHistory(_ context.Context, kind, source string) (contact.InvalidSourceHistoryReceipt, bool, error) {
	value, ok := journal.receipts[kind+"/"+source]
	return value, ok, nil
}
func (journal *invalidSourceHistoryJournalFake) RecordInvalidSourceHistory(_ context.Context, receipt contact.InvalidSourceHistoryReceipt) error {
	if journal.receipts == nil {
		journal.receipts = map[string]contact.InvalidSourceHistoryReceipt{}
	}
	key := receipt.Kind + "/" + receipt.SourceIdentifier
	if _, ok := journal.receipts[key]; ok {
		return contact.ErrInvalidSourceHistoryConflict
	}
	journal.receipts[key] = receipt
	return nil
}

type invalidSourceHistoryContextKey struct{}

func invalidSourceHistoryTagFixture(seed string) (string, contact.HistoricalUnboundTag) {
	key := sha256.Sum256([]byte(seed + "-key"))
	payload := sha256.Sum256([]byte(seed + "-payload"))
	field := sha256.Sum256([]byte(seed + "-field"))
	private := sha256.Sum256([]byte(seed + "-private"))
	union := sha256.Sum256([]byte(seed + "-union"))
	return hex.EncodeToString(key[:]), contact.HistoricalUnboundTag{SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, TagSourceID: "", UnionIDDigest: union, CreatedAt: time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("+8", 8*60*60)), QuarantineReason: "invalid_contact_tag"}
}
func invalidSourceHistoryChannelFixture(seed string) (string, contact.HistoricalInvalidChannel) {
	key := sha256.Sum256([]byte(seed + "-key"))
	payload := sha256.Sum256([]byte(seed + "-payload"))
	field := sha256.Sum256([]byte(seed + "-field"))
	private := sha256.Sum256([]byte(seed + "-private"))
	at := time.Date(2026, 8, 29, 10, 11, 12, 123456789, time.FixedZone("+8", 8*60*60))
	return hex.EncodeToString(key[:]), contact.HistoricalInvalidChannel{SourceKeyDigest: key, SourcePayloadDigest: payload, SourceFieldDigest: field, PrivateDigest: private, SourceID: 0, Code: "code", Name: "name", ChannelType: "qrcode", CarrierType: "qrcode", CreatedAt: at, UpdatedAt: at.Add(-time.Second), QuarantineReason: "invalid_channel_definition"}
}

package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func TestMessageHistoryWriterCreatesAndReplaysCallerTarget(t *testing.T) {
	store := &messageHistoryStoreFake{values: map[int64]wecomport.HistoricalMessage{}}
	journal := &messageHistoryJournalFake{}
	writer := NewMessageHistoryWriter(store, journal)
	value := messageHistoryValue(t, "explicit_offset")
	payload := value.SourcePayloadDigest

	first, err := writer.Write(context.Background(), "source-1", payload, value)
	if err != nil || first.Replayed || first.TargetID != 41 || store.creates != 1 || journal.records != 1 {
		t.Fatalf("first receipt=%#v err=%v creates/records=%d/%d", first, err, store.creates, journal.records)
	}
	second, err := writer.Write(context.Background(), "source-1", payload, value)
	if err != nil || !second.Replayed || second.SourceIdentifier != first.SourceIdentifier || second.PayloadDigest != first.PayloadDigest || second.TargetID != first.TargetID || second.TargetDigest != first.TargetDigest || store.creates != 1 || store.gets != 1 || journal.records != 1 {
		t.Fatalf("replay receipt=%#v first=%#v err=%v creates/gets/records=%d/%d/%d", second, first, err, store.creates, store.gets, journal.records)
	}
	stored := store.values[first.TargetID]
	if stored.Sequence != nil || stored.CustomerID != nil || stored.ContentMasked != nil || stored.CreatedAt.Nanosecond()%1000 != 0 || stored.SentAt == nil || stored.SentAt.Nanosecond()%1000 != 0 {
		t.Fatal("timestamps_not_normalized_to_microseconds")
	}
}

func TestMessageHistoryWriterRejectsPayloadAndTargetDrift(t *testing.T) {
	store := &messageHistoryStoreFake{values: map[int64]wecomport.HistoricalMessage{}}
	journal := &messageHistoryJournalFake{}
	writer := NewMessageHistoryWriter(store, journal)
	value := messageHistoryValue(t, "explicit_offset")
	if _, err := writer.Write(context.Background(), "source-1", sha256.Sum256([]byte("other")), value); !errors.Is(err, wecomport.ErrMessageHistoryInvalid) {
		t.Fatalf("payload mismatch err=%v", err)
	}
	receipt, err := writer.Write(context.Background(), "source-1", value.SourcePayloadDigest, value)
	if err != nil {
		t.Fatal(err)
	}
	changed := store.values[receipt.TargetID]
	changed.MessageType = "mutated"
	store.values[receipt.TargetID] = changed
	if _, err = writer.Write(context.Background(), "source-1", value.SourcePayloadDigest, value); !errors.Is(err, wecomport.ErrMessageHistoryConflict) {
		t.Fatalf("target drift err=%v", err)
	}
}

func TestMessageHistoryWriterPreservesNullableAndCivilClock(t *testing.T) {
	store := &messageHistoryStoreFake{values: map[int64]wecomport.HistoricalMessage{}}
	journal := &messageHistoryJournalFake{}
	writer := NewMessageHistoryWriter(store, journal)
	value := messageHistoryValue(t, "civil_unzoned")
	value.Sequence = ptr(int64(-7))
	value.CustomerID, value.ContentMasked = nil, ptr(" \n")
	receipt, err := writer.Write(context.Background(), "source-civil", value.SourcePayloadDigest, value)
	if err != nil {
		t.Fatal(err)
	}
	stored := store.values[receipt.TargetID]
	if stored.Sequence == nil || *stored.Sequence != -7 || stored.CustomerID != nil || stored.ContentMasked == nil || *stored.ContentMasked != " \n" || stored.SentAt != nil || stored.OriginalSendTime != "2026-08-27 13:36:01" {
		t.Fatalf("nullable/civil fact changed: %#v", stored)
	}
}

func TestMessageHistoryWriterRejectsUnsafeTextTimeAndCallerContext(t *testing.T) {
	for _, change := range []func(*wecomport.HistoricalMessage){
		func(value *wecomport.HistoricalMessage) { value.ChatType = "private\x00" },
		func(value *wecomport.HistoricalMessage) { value.ContentMasked = ptr(string([]byte{0xff})) },
		func(value *wecomport.HistoricalMessage) {
			value.SendTimeBasis, value.SentAt = "civil_unzoned", ptr(time.Now())
		},
		func(value *wecomport.HistoricalMessage) { value.OriginalSendTime = "2026-08-27T13:36:01Z" },
	} {
		value := messageHistoryValue(t, "explicit_offset")
		change(&value)
		if _, err := NewMessageHistoryWriter(&messageHistoryStoreFake{}, &messageHistoryJournalFake{}).Write(context.Background(), "source", value.SourcePayloadDigest, value); !errors.Is(err, wecomport.ErrMessageHistoryInvalid) {
			t.Fatalf("unsafe source accepted err=%v", err)
		}
	}
	store := &messageHistoryStoreFake{values: map[int64]wecomport.HistoricalMessage{}, requireContext: true}
	if _, err := NewMessageHistoryWriter(store, &messageHistoryJournalFake{}).Write(context.Background(), "source", messageHistoryValue(t, "explicit_offset").SourcePayloadDigest, messageHistoryValue(t, "explicit_offset")); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatalf("missing caller transaction context err=%v", err)
	}
	ctx := context.WithValue(context.Background(), messageHistoryContextKey{}, "caller")
	value := messageHistoryValue(t, "explicit_offset")
	if _, err := NewMessageHistoryWriter(store, &messageHistoryJournalFake{}).Write(ctx, "source", value.SourcePayloadDigest, value); err != nil {
		t.Fatalf("caller transaction context not forwarded err=%v", err)
	}
	var typedNilStore *messageHistoryStoreFake
	if _, err := NewMessageHistoryWriter(typedNilStore, &messageHistoryJournalFake{}).Write(context.Background(), "source", messageHistoryValue(t, "explicit_offset").SourcePayloadDigest, messageHistoryValue(t, "explicit_offset")); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatalf("typed nil store err=%v", err)
	}
	var typedNilJournal *messageHistoryJournalFake
	if _, err := NewMessageHistoryWriter(&messageHistoryStoreFake{}, typedNilJournal).Write(context.Background(), "source", messageHistoryValue(t, "explicit_offset").SourcePayloadDigest, messageHistoryValue(t, "explicit_offset")); !errors.Is(err, wecomport.ErrMessageHistoryUnavailable) {
		t.Fatalf("typed nil journal err=%v", err)
	}
}

type messageHistoryStoreFake struct {
	values         map[int64]wecomport.HistoricalMessage
	creates, gets  int
	requireContext bool
}

func (store *messageHistoryStoreFake) CreateHistoricalMessage(ctx context.Context, value wecomport.HistoricalMessage) (wecomport.HistoricalMessage, error) {
	if store.requireContext && ctx.Value(messageHistoryContextKey{}) != "caller" {
		return wecomport.HistoricalMessage{}, errors.New("caller transaction context required")
	}
	store.creates++
	if store.values == nil {
		store.values = map[int64]wecomport.HistoricalMessage{}
	}
	value.ID = 41
	store.values[value.ID] = value
	return value, nil
}

func (store *messageHistoryStoreFake) GetHistoricalMessage(ctx context.Context, id int64) (wecomport.HistoricalMessage, error) {
	if store.requireContext && ctx.Value(messageHistoryContextKey{}) != "caller" {
		return wecomport.HistoricalMessage{}, errors.New("caller transaction context required")
	}
	store.gets++
	value, found := store.values[id]
	if !found {
		return wecomport.HistoricalMessage{}, wecomport.ErrMessageHistoryConflict
	}
	return value, nil
}

type messageHistoryJournalFake struct {
	receipt wecomport.MessageHistoryReceipt
	found   bool
	records int
}

func (journal *messageHistoryJournalFake) LoadMessageHistory(_ context.Context, source string) (wecomport.MessageHistoryReceipt, bool, error) {
	if journal.found && journal.receipt.SourceIdentifier != source {
		return wecomport.MessageHistoryReceipt{}, false, wecomport.ErrMessageHistoryConflict
	}
	return journal.receipt, journal.found, nil
}

func (journal *messageHistoryJournalFake) RecordMessageHistory(_ context.Context, receipt wecomport.MessageHistoryReceipt) error {
	if journal.found {
		return wecomport.ErrMessageHistoryConflict
	}
	journal.receipt, journal.found, journal.records = receipt, true, journal.records+1
	return nil
}

type messageHistoryContextKey struct{}

func messageHistoryValue(t *testing.T, basis string) wecomport.HistoricalMessage {
	t.Helper()
	payload := sha256.Sum256([]byte("source-payload"))
	created := time.Date(2026, 8, 27, 13, 36, 1, 123456789, time.FixedZone("+8", 8*60*60))
	value := wecomport.HistoricalMessage{SourceID: 9, ChatType: "private", MessageType: "text", OriginalSendTime: "2026-08-27T21:36:01.123456789+08:00", SendTimeBasis: basis, CreatedAt: created, SourcePayloadDigest: payload}
	if basis == "civil_unzoned" {
		value.OriginalSendTime, value.SentAt = "2026-08-27 13:36:01", nil
	} else {
		sent := time.Date(2026, 8, 27, 13, 36, 1, 123456789, time.UTC)
		value.SentAt = &sent
	}
	return value
}

func ptr[T any](value T) *T { return &value }

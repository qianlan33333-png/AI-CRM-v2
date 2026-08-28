package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	mediaport "github.com/qianlan33333-png/AI-CRM-v2/internal/media/port"
)

func TestStaticMediaHistoryWriterReplaysAndDetectsTargetDrift(t *testing.T) {
	store := &staticMediaHistoryStoreFake{}
	journal := &staticMediaHistoryJournalFake{}
	writer, err := NewStaticMediaHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := staticMediaHistoryFixture()
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.ImportGroupInvite(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID != 1 {
		t.Fatalf("first import = %#v, %v", first, err)
	}
	if store.value.SourceID != -7 || *store.value.RoomBaseSourceID != -3 || store.value.Name != "" {
		t.Fatal("signed or blank historical source facts changed")
	}
	second, err := writer.ImportGroupInvite(context.Background(), source, value)
	if err != nil || !second.Replayed || store.createCalls != 1 {
		t.Fatalf("replay = %#v, create=%d, err=%v", second, store.createCalls, err)
	}
	store.value.Title = "changed"
	if _, err := writer.ImportGroupInvite(context.Background(), source, value); !errors.Is(err, mediaport.ErrStaticMediaHistoryConflict) {
		t.Fatalf("target drift = %v", err)
	}
}

func TestStaticMediaHistoryWriterRejectsInvalidSourceBeforeEffects(t *testing.T) {
	store := &staticMediaHistoryStoreFake{}
	journal := &staticMediaHistoryJournalFake{}
	writer, err := NewStaticMediaHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := staticMediaHistoryFixture()
	for _, source := range []string{"", hex.EncodeToString(value.SourceKeyDigest[:])[:63], "AA" + hex.EncodeToString(value.SourceKeyDigest[:])[2:]} {
		if _, err := writer.ImportGroupInvite(context.Background(), source, value); !errors.Is(err, mediaport.ErrStaticMediaHistoryInvalid) {
			t.Fatalf("invalid source %q: %v", source, err)
		}
	}
	value.Description = "bad\x00text"
	if _, err := writer.ImportGroupInvite(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, mediaport.ErrStaticMediaHistoryInvalid) {
		t.Fatalf("NUL text accepted: %v", err)
	}
	if store.createCalls != 0 || journal.recordCalls != 0 {
		t.Fatal("invalid input reached an effect")
	}
	var nilStore *staticMediaHistoryStoreFake
	if _, err := NewStaticMediaHistoryWriter(nilStore, journal); !errors.Is(err, mediaport.ErrStaticMediaHistoryUnavailable) {
		t.Fatalf("typed nil store: %v", err)
	}
}

type staticMediaHistoryStoreFake struct {
	value       mediaport.HistoricalGroupInvite
	createCalls int
}

func (store *staticMediaHistoryStoreFake) CreateHistoricalGroupInvite(_ context.Context, value mediaport.HistoricalGroupInvite) (mediaport.HistoricalGroupInvite, error) {
	store.createCalls++
	value.ID = 1
	store.value = value
	return value, nil
}
func (store *staticMediaHistoryStoreFake) GetHistoricalGroupInvite(_ context.Context, id int64) (mediaport.HistoricalGroupInvite, error) {
	if id != store.value.ID || id == 0 {
		return mediaport.HistoricalGroupInvite{}, mediaport.ErrStaticMediaHistoryConflict
	}
	return store.value, nil
}

type staticMediaHistoryJournalFake struct {
	receipt     mediaport.StaticMediaHistoryReceipt
	found       bool
	recordCalls int
}

func (journal *staticMediaHistoryJournalFake) LoadStaticMediaHistory(_ context.Context, kind, source string) (mediaport.StaticMediaHistoryReceipt, bool, error) {
	if journal.found && journal.receipt.Kind == kind && journal.receipt.SourceIdentifier == source {
		return journal.receipt, true, nil
	}
	return mediaport.StaticMediaHistoryReceipt{}, false, nil
}
func (journal *staticMediaHistoryJournalFake) RecordStaticMediaHistory(_ context.Context, receipt mediaport.StaticMediaHistoryReceipt) error {
	journal.recordCalls++
	journal.receipt, journal.found = receipt, true
	return nil
}

func staticMediaHistoryFixture() mediaport.HistoricalGroupInvite {
	var source, payload [sha256.Size]byte
	source[0], payload[0] = 1, 2
	room := int64(-3)
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456789, time.FixedZone("legacy", 8*60*60))
	return mediaport.HistoricalGroupInvite{SourceID: -7, SourceKeyDigest: source, SourcePayloadDigest: payload, Name: "", Title: "title", Description: "\n", OriginalState: "old", OriginalAutoCreate: true, RoomBaseName: "", RoomBaseSourceID: &room, OriginalEnabled: false, OriginalBindingState: "unbound", CreatedAt: at, UpdatedAt: at}
}

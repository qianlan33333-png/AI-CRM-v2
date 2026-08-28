package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestStaticProductHistoryWriterReplaysAndDetectsTargetDrift(t *testing.T) {
	store := &staticProductHistoryStoreFake{}
	journal := &staticProductHistoryJournalFake{}
	writer, err := NewStaticProductHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := staticProductHistoryFixture()
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.ImportProductPageSlice(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID != 1 {
		t.Fatalf("first import = %#v, %v", first, err)
	}
	if store.value.SourceID != -7 || store.value.ProductSourceID != 0 || store.value.ImageSourceID != -3 || store.value.SortOrder != -4 {
		t.Fatal("signed or zero historical source facts changed")
	}
	second, err := writer.ImportProductPageSlice(context.Background(), source, value)
	if err != nil || !second.Replayed || store.createCalls != 1 {
		t.Fatalf("replay = %#v, create=%d, err=%v", second, store.createCalls, err)
	}
	store.value.SortOrder = 9
	if _, err := writer.ImportProductPageSlice(context.Background(), source, value); !errors.Is(err, productport.ErrStaticProductHistoryConflict) {
		t.Fatalf("target drift = %v", err)
	}
}

func TestStaticProductHistoryWriterRejectsInvalidSourceBeforeEffects(t *testing.T) {
	store := &staticProductHistoryStoreFake{}
	journal := &staticProductHistoryJournalFake{}
	writer, err := NewStaticProductHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := staticProductHistoryFixture()
	if _, err := writer.ImportProductPageSlice(context.Background(), "AA"+hex.EncodeToString(value.SourceKeyDigest[:])[2:], value); !errors.Is(err, productport.ErrStaticProductHistoryInvalid) {
		t.Fatalf("uppercase source accepted: %v", err)
	}
	value.SourcePayloadDigest = [sha256.Size]byte{}
	if _, err := writer.ImportProductPageSlice(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, productport.ErrStaticProductHistoryInvalid) {
		t.Fatalf("zero payload accepted: %v", err)
	}
	if store.createCalls != 0 || journal.recordCalls != 0 {
		t.Fatal("invalid input reached an effect")
	}
	var nilJournal *staticProductHistoryJournalFake
	if _, err := NewStaticProductHistoryWriter(store, nilJournal); !errors.Is(err, productport.ErrStaticProductHistoryUnavailable) {
		t.Fatalf("typed nil journal: %v", err)
	}
}

type staticProductHistoryStoreFake struct {
	value       productport.HistoricalProductPageSlice
	createCalls int
}

func (store *staticProductHistoryStoreFake) CreateHistoricalProductPageSlice(_ context.Context, value productport.HistoricalProductPageSlice) (productport.HistoricalProductPageSlice, error) {
	store.createCalls++
	value.ID = 1
	store.value = value
	return value, nil
}
func (store *staticProductHistoryStoreFake) GetHistoricalProductPageSlice(_ context.Context, id int64) (productport.HistoricalProductPageSlice, error) {
	if id != store.value.ID || id == 0 {
		return productport.HistoricalProductPageSlice{}, productport.ErrStaticProductHistoryConflict
	}
	return store.value, nil
}

type staticProductHistoryJournalFake struct {
	receipt     productport.StaticProductHistoryReceipt
	found       bool
	recordCalls int
}

func (journal *staticProductHistoryJournalFake) LoadStaticProductHistory(_ context.Context, kind, source string) (productport.StaticProductHistoryReceipt, bool, error) {
	if journal.found && journal.receipt.Kind == kind && journal.receipt.SourceIdentifier == source {
		return journal.receipt, true, nil
	}
	return productport.StaticProductHistoryReceipt{}, false, nil
}
func (journal *staticProductHistoryJournalFake) RecordStaticProductHistory(_ context.Context, receipt productport.StaticProductHistoryReceipt) error {
	journal.recordCalls++
	journal.receipt, journal.found = receipt, true
	return nil
}

func staticProductHistoryFixture() productport.HistoricalProductPageSlice {
	var source, payload [sha256.Size]byte
	source[0], payload[0] = 1, 2
	at := time.Date(2026, 8, 28, 10, 11, 12, 123456789, time.FixedZone("legacy", 8*60*60))
	return productport.HistoricalProductPageSlice{SourceID: -7, SourceKeyDigest: source, SourcePayloadDigest: payload, ProductSourceID: 0, ImageSourceID: -3, SortOrder: -4, OriginalEnabled: false, CreatedAt: at, UpdatedAt: at}
}

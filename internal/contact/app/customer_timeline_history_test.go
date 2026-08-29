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

func TestCustomerTimelineHistoryWriterCreatesAndReplaysVerifiedTarget(t *testing.T) {
	store := &customerTimelineHistoryStoreFake{}
	journal := &customerTimelineHistoryJournalFake{}
	writer, err := NewCustomerTimelineHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	value := testCustomerTimelineHistoryFact()
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	first, err := writer.ImportHistoricalCustomerTimelineEvent(context.Background(), source, value)
	if err != nil || first.Replayed || first.TargetID < 1 || len(store.rows) != 1 || len(journal.receipts) != 1 {
		t.Fatalf("first import receipt=%#v rows=%d journal=%d err=%v", first, len(store.rows), len(journal.receipts), err)
	}
	stored := store.rows[first.TargetID]
	if stored.SourceID != -7 || stored.EventID != "event" || stored.Title != "private title" || stored.Summary != "private summary" || string(stored.MetadataJSON) != "null" || stored.UnionID != "private-union" || stored.CustomerID != nil {
		t.Fatalf("store did not preserve private fact or nil customer reference: %#v", stored)
	}
	replay, err := writer.ImportHistoricalCustomerTimelineEvent(context.Background(), source, value)
	if err != nil || !replay.Replayed || replay.TargetID != first.TargetID || len(store.rows) != 1 {
		t.Fatalf("replay receipt=%#v rows=%d err=%v", replay, len(store.rows), err)
	}
}

func TestCustomerTimelineHistoryWriterRejectsPrivateTargetDrift(t *testing.T) {
	store := &customerTimelineHistoryStoreFake{}
	journal := &customerTimelineHistoryJournalFake{}
	writer, _ := NewCustomerTimelineHistoryWriter(store, journal)
	value := testCustomerTimelineHistoryFact()
	source := hex.EncodeToString(value.SourceKeyDigest[:])
	if _, err := writer.ImportHistoricalCustomerTimelineEvent(context.Background(), source, value); err != nil {
		t.Fatal(err)
	}
	stored := store.rows[1]
	stored.Summary = "changed private summary"
	store.rows[1] = stored
	if _, err := writer.ImportHistoricalCustomerTimelineEvent(context.Background(), source, value); !errors.Is(err, contact.ErrCustomerTimelineHistoryConflict) {
		t.Fatalf("private target drift error=%v", err)
	}
}

func TestCustomerTimelineHistoryWriterFailsClosedBeforeStore(t *testing.T) {
	store := &customerTimelineHistoryStoreFake{}
	journal := &customerTimelineHistoryJournalFake{}
	writer, _ := NewCustomerTimelineHistoryWriter(store, journal)
	for _, mutate := range []func(*contact.HistoricalCustomerTimelineEvent){
		func(value *contact.HistoricalCustomerTimelineEvent) { value.SourcePayloadDigest = [32]byte{} },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.MetadataJSON = []byte("not-json") },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.Title = "bad\x00title" },
		func(value *contact.HistoricalCustomerTimelineEvent) { invalid := int64(0); value.CustomerID = &invalid },
	} {
		value := testCustomerTimelineHistoryFact()
		mutate(&value)
		if _, err := writer.ImportHistoricalCustomerTimelineEvent(context.Background(), hex.EncodeToString(value.SourceKeyDigest[:]), value); !errors.Is(err, contact.ErrCustomerTimelineHistoryInvalid) {
			t.Fatalf("invalid source error=%v", err)
		}
	}
	if len(store.rows) != 0 || len(journal.receipts) != 0 {
		t.Fatal("invalid source reached target store or journal")
	}
	if _, err := NewCustomerTimelineHistoryWriter((*customerTimelineHistoryStoreFake)(nil), journal); !errors.Is(err, contact.ErrCustomerTimelineHistoryUnavailable) {
		t.Fatalf("typed nil store error=%v", err)
	}
}

func TestHistoricalCustomerTimelineEventDigestCoversPrivateFields(t *testing.T) {
	base := testCustomerTimelineHistoryFact()
	base.ID = 1
	base.EventTime = base.EventTime.UTC().Truncate(time.Microsecond)
	base.CreatedAt = base.CreatedAt.UTC().Truncate(time.Microsecond)
	want, err := HistoricalCustomerTimelineEventDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*contact.HistoricalCustomerTimelineEvent){
		func(value *contact.HistoricalCustomerTimelineEvent) { value.UnionID = "other" },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.Title = "other" },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.Summary = "other" },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.MetadataJSON = []byte(`{"other":true}`) },
		func(value *contact.HistoricalCustomerTimelineEvent) { value.SourceFieldDigest[0] ^= 0xff },
	} {
		changed := base
		changed.MetadataJSON = append([]byte(nil), base.MetadataJSON...)
		mutate(&changed)
		got, digestErr := HistoricalCustomerTimelineEventDigest(changed)
		if digestErr != nil || got == want {
			t.Fatalf("private field omitted from digest: digest=%x err=%v", got, digestErr)
		}
	}
}

func testCustomerTimelineHistoryFact() contact.HistoricalCustomerTimelineEvent {
	at := time.Date(2026, 8, 29, 1, 2, 3, 123456789, time.FixedZone("source", 8*3600))
	return contact.HistoricalCustomerTimelineEvent{
		SourceKeyDigest: sha256.Sum256([]byte("source")), SourcePayloadDigest: sha256.Sum256([]byte("payload")), SourceFieldDigest: sha256.Sum256([]byte("field")),
		SourceID: -7, EventID: "event", EventType: "legacy", EventTime: at, Title: "private title", Summary: "private summary",
		SourceTable: "legacy_table", SourceValue: "-3", MetadataJSON: []byte(`null`), CreatedAt: at.Add(time.Second), UnionID: "private-union",
	}
}

type customerTimelineHistoryStoreFake struct {
	rows map[int64]contact.HistoricalCustomerTimelineEvent
	next int64
}

func (store *customerTimelineHistoryStoreFake) CreateHistoricalCustomerTimelineEvent(_ context.Context, value contact.HistoricalCustomerTimelineEvent) (contact.HistoricalCustomerTimelineEvent, error) {
	if store.rows == nil {
		store.rows, store.next = map[int64]contact.HistoricalCustomerTimelineEvent{}, 1
	}
	value.ID = store.next
	store.next++
	store.rows[value.ID] = value
	return value, nil
}

func (store *customerTimelineHistoryStoreFake) GetHistoricalCustomerTimelineEvent(_ context.Context, id int64) (contact.HistoricalCustomerTimelineEvent, error) {
	value, found := store.rows[id]
	if !found {
		return contact.HistoricalCustomerTimelineEvent{}, errors.New("missing")
	}
	return value, nil
}

type customerTimelineHistoryJournalFake struct {
	receipts map[string]contact.CustomerTimelineHistoryReceipt
}

func (journal *customerTimelineHistoryJournalFake) LoadCustomerTimelineHistory(_ context.Context, kind, source string) (contact.CustomerTimelineHistoryReceipt, bool, error) {
	value, found := journal.receipts[kind+":"+source]
	return value, found, nil
}

func (journal *customerTimelineHistoryJournalFake) RecordCustomerTimelineHistory(_ context.Context, value contact.CustomerTimelineHistoryReceipt) error {
	if journal.receipts == nil {
		journal.receipts = map[string]contact.CustomerTimelineHistoryReceipt{}
	}
	journal.receipts[value.Kind+":"+value.SourceIdentifier] = value
	return nil
}

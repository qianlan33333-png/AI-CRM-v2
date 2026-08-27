package campaign

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHistoricalDefinitionWriterInsertsDisabledDefinitionThenReplays(t *testing.T) {
	store := &historicalDefinitionMemoryStore{}
	journal := &historicalDefinitionMemoryJournal{receipts: map[string]HistoricalDefinitionReceipt{}}
	writer, err := NewHistoricalDefinitionWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	definition := historicalDefinitionFixture()
	receipt, err := writer.Import(context.Background(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Replayed || receipt.OriginalApprovalStatus != "pending_review" || receipt.OriginalRuntimeStatus != "draft" || receipt.TargetCampaignCode != definition.Campaign.Code {
		t.Fatalf("receipt = %#v", receipt)
	}
	if len(store.values) != 1 || store.values[0].Campaign.ApprovalStatus != ApprovalRejected || store.values[0].Campaign.RuntimeStatus != RuntimePaused || store.values[0].Campaign.Version != 1 {
		t.Fatalf("stored = %#v", store.values)
	}
	if store.plans != 0 || store.commands != 0 || store.events != 0 || store.providers != 0 {
		t.Fatalf("side effects = %#v", store)
	}
	replayed, err := writer.Import(context.Background(), definition)
	if err != nil || !replayed.Replayed || len(store.values) != 1 || journal.records != 1 {
		t.Fatalf("replay/result/store/records = %#v/%#v/%d/%d", err, replayed, len(store.values), journal.records)
	}
}

func TestHistoricalDefinitionWriterRejectsDigestDriftAndExistingUserCampaign(t *testing.T) {
	t.Run("digest drift", func(t *testing.T) {
		store := &historicalDefinitionMemoryStore{}
		definition := historicalDefinitionFixture()
		journal := &historicalDefinitionMemoryJournal{receipts: map[string]HistoricalDefinitionReceipt{
			definition.SourceIdentifier: {SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest, OriginalApprovalStatus: definition.OriginalApprovalStatus, OriginalRuntimeStatus: definition.OriginalRuntimeStatus, TargetCampaignCode: definition.Campaign.Code},
		}}
		writer, _ := NewHistoricalDefinitionWriter(store, journal)
		definition.PayloadDigest[0]++
		if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrHistoricalDefinitionConflict) || len(store.values) != 0 {
			t.Fatalf("error/store = %v/%#v", err, store.values)
		}
	})

	t.Run("existing user campaign", func(t *testing.T) {
		store := &historicalDefinitionMemoryStore{insertErr: ErrHistoricalDefinitionConflict}
		journal := &historicalDefinitionMemoryJournal{receipts: map[string]HistoricalDefinitionReceipt{}}
		writer, _ := NewHistoricalDefinitionWriter(store, journal)
		if _, err := writer.Import(context.Background(), historicalDefinitionFixture()); !errors.Is(err, ErrHistoricalDefinitionConflict) || journal.records != 0 {
			t.Fatalf("error/records = %v/%d", err, journal.records)
		}
	})
}

func TestHistoricalDefinitionWriterRejectsInvalidDefinition(t *testing.T) {
	store := &historicalDefinitionMemoryStore{}
	journal := &historicalDefinitionMemoryJournal{receipts: map[string]HistoricalDefinitionReceipt{}}
	writer, _ := NewHistoricalDefinitionWriter(store, journal)
	definition := historicalDefinitionFixture()
	definition.Steps[0].Index = 2
	if _, err := writer.Import(context.Background(), definition); !errors.Is(err, ErrUnavailable) || len(store.values) != 0 || journal.records != 0 {
		t.Fatalf("error/store/records = %v/%#v/%d", err, store.values, journal.records)
	}
}

type historicalDefinitionMemoryStore struct {
	values                             []HistoricalDefinition
	insertErr                          error
	plans, commands, events, providers int
}

func (store *historicalDefinitionMemoryStore) InsertHistoricalDefinition(_ context.Context, definition HistoricalDefinition) error {
	if store.insertErr != nil {
		return store.insertErr
	}
	store.values = append(store.values, definition)
	return nil
}

type historicalDefinitionMemoryJournal struct {
	receipts map[string]HistoricalDefinitionReceipt
	records  int
}

func (journal *historicalDefinitionMemoryJournal) LoadHistoricalDefinition(_ context.Context, source string) (HistoricalDefinitionReceipt, bool, error) {
	value, found := journal.receipts[source]
	return value, found, nil
}

func (journal *historicalDefinitionMemoryJournal) RecordHistoricalDefinition(_ context.Context, receipt HistoricalDefinitionReceipt) error {
	journal.receipts[receipt.SourceIdentifier] = receipt
	journal.records++
	return nil
}

func historicalDefinitionFixture() HistoricalDefinition {
	stamp := time.Date(2026, 8, 28, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	return HistoricalDefinition{
		SourceIdentifier: "campaigns/opaque-source-key", PayloadDigest: [32]byte{1}, OriginalApprovalStatus: "pending_review", OriginalRuntimeStatus: "draft",
		Campaign: Campaign{Code: "v1-history-campaign", Name: "历史 Campaign", ApprovalStatus: ApprovalDraft, RuntimeStatus: RuntimeIdle, Version: 1, CreatedBy: 7, UpdatedBy: 7, CreatedAt: stamp, UpdatedAt: stamp},
		Steps:    []Step{{Index: 1, DelayMinutes: 90, Content: "历史内容"}},
	}
}

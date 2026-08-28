package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestCampaignHistoryJournalRoundTripsExactKinds(t *testing.T) {
	values := map[string]*campaignHistoryTerminalFake{}
	journals := map[string]campaignHistoryTerminalJournal{}
	for kind := range campaignHistoryScopes {
		values[kind] = newCampaignHistoryTerminalFake()
		journals[kind] = values[kind]
	}
	journal, err := newCampaignHistoryJournal(journals)
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []string{campaignHistorySegmentsKind, campaignHistoryMembersKind, campaignHistoryPlansKind, campaignHistoryRecipientsKind, campaignHistoryMessagesKind} {
		receipt := campaignHistoryReceipt(byte(index + 1))
		if err := journal.RecordCampaignHistory(context.Background(), kind, receipt); err != nil {
			t.Fatalf("record %s: %v", kind, err)
		}
		got, found, err := journal.LoadCampaignHistory(context.Background(), kind, receipt.SourceIdentifier)
		if err != nil || !found || got != receipt {
			t.Fatalf("load %s=%#v/%v/%v", kind, got, found, err)
		}
	}
	for kind, value := range values {
		if len(value.values) != 1 {
			t.Fatalf("scope %s got %d receipts", kind, len(value.values))
		}
	}
}

func TestCampaignHistoryJournalRejectsUnsafeReceiptAndWrongScope(t *testing.T) {
	values := map[string]campaignHistoryTerminalJournal{}
	for kind := range campaignHistoryScopes {
		values[kind] = newCampaignHistoryTerminalFake()
	}
	journal, err := newCampaignHistoryJournal(values)
	if err != nil {
		t.Fatal(err)
	}
	receipt := campaignHistoryReceipt(1)
	for _, invalid := range []campaignport.CampaignHistoryReceipt{
		{SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest, Replayed: true},
		{SourceIdentifier: receipt.SourceIdentifier, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest},
		{SourceIdentifier: receipt.SourceIdentifier, PayloadDigest: receipt.PayloadDigest, TargetDigest: receipt.TargetDigest},
		{SourceIdentifier: "invalid", PayloadDigest: receipt.PayloadDigest, TargetID: receipt.TargetID, TargetDigest: receipt.TargetDigest},
	} {
		if err := journal.RecordCampaignHistory(context.Background(), campaignHistorySegmentsKind, invalid); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
			t.Fatalf("invalid receipt error=%v", err)
		}
	}
	if err := journal.RecordCampaignHistory(context.Background(), "unknown", receipt); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("unknown kind error=%v", err)
	}
	values[campaignHistorySegmentsKind].(*campaignHistoryTerminalFake).values[receipt.SourceIdentifier] = TerminalReceipt{
		SourceKeyDigest: campaignHistoryDigest(1), PayloadDigest: receipt.PayloadDigest, Disposition: "quarantine", Reason: "shape", Metadata: map[string]any{},
	}
	if _, _, err := journal.LoadCampaignHistory(context.Background(), campaignHistorySegmentsKind, receipt.SourceIdentifier); !errors.Is(err, campaignport.ErrCampaignHistoryConflict) {
		t.Fatalf("terminal disposition error=%v", err)
	}

	segments, members, plans, recipients, messages := campaignHistoryScopedJournal(campaignHistorySegmentsKind, "run"), campaignHistoryScopedJournal(campaignHistoryMembersKind, "run"), campaignHistoryScopedJournal(campaignHistoryPlansKind, "run"), campaignHistoryScopedJournal(campaignHistoryRecipientsKind, "run"), campaignHistoryScopedJournal(campaignHistoryMessagesKind, "run")
	if _, err := NewCampaignHistoryJournal(segments, members, plans, recipients, messages); err != nil {
		t.Fatal(err)
	}
	messages.scope.ArchiveRunID = "other"
	if _, err := NewCampaignHistoryJournal(segments, members, plans, recipients, messages); !errors.Is(err, campaignport.ErrCampaignHistoryInvalid) {
		t.Fatalf("mixed run error=%v", err)
	}
}

type campaignHistoryTerminalFake struct{ values map[string]TerminalReceipt }

func newCampaignHistoryTerminalFake() *campaignHistoryTerminalFake {
	return &campaignHistoryTerminalFake{values: map[string]TerminalReceipt{}}
}

func (journal *campaignHistoryTerminalFake) LoadTerminal(_ context.Context, source string) (TerminalReceipt, bool, error) {
	value, found := journal.values[source]
	return value, found, nil
}

func (journal *campaignHistoryTerminalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if found, exists := journal.values[key]; exists && !sameCampaignHistoryTerminal(found, receipt) {
		return ErrConflict
	}
	journal.values[key] = receipt
	return nil
}

func campaignHistoryReceipt(first byte) campaignport.CampaignHistoryReceipt {
	return campaignport.CampaignHistoryReceipt{SourceIdentifier: SourceIdentifier(campaignHistoryDigest(first)), PayloadDigest: campaignHistoryDigest(first + 10), TargetID: int64(first) + 40, TargetDigest: campaignHistoryDigest(first + 20)}
}

func campaignHistoryDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}

func campaignHistoryScopedJournal(kind, archiveRun string) *Journal {
	scope := campaignHistoryScopes[kind]
	return &Journal{scope: Scope{ImportVersion: campaignHistoryImportVersion, ArchiveRunID: archiveRun, AdapterID: v1archive.DefaultAdapterID, TableID: scope[0], TargetDomain: campaignHistoryTargetDomain, TargetTable: scope[1]}, tx: func(context.Context) (pgx.Tx, error) {
		return nil, nil
	}}
}

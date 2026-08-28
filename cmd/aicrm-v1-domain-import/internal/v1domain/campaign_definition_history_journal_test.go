package v1domain

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/jackc/pgx/v5"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestNewCampaignDefinitionHistoryJournalRequiresExactScopes(t *testing.T) {
	definitions := campaignDefinitionHistoryTestJournal(campaignDefinitionHistoryDefinitionTable, campaignDefinitionHistoryDefinitionTarget, "run")
	steps := campaignDefinitionHistoryTestJournal(campaignDefinitionHistoryStepTable, campaignDefinitionHistoryStepTarget, "run")
	journal, err := NewCampaignDefinitionHistoryJournal(definitions, steps)
	if err != nil {
		t.Fatalf("new journal: %v", err)
	}
	if err = journal.ValidateCampaignDefinitionHistoryImportScope("run"); err != nil {
		t.Fatalf("validate scope: %v", err)
	}
	if err = journal.ValidateCampaignDefinitionHistoryImportScope("other"); err == nil {
		t.Fatal("cross-run scope accepted")
	}
	wrong := campaignDefinitionHistoryTestJournal(campaignDefinitionHistoryStepTable, campaignDefinitionHistoryDefinitionTarget, "run")
	if _, err = NewCampaignDefinitionHistoryJournal(definitions, wrong); err == nil {
		t.Fatal("wrong target scope accepted")
	}
}

func TestCampaignDefinitionHistoryTerminalReceiptRoundTrip(t *testing.T) {
	key := sha256.Sum256([]byte("source"))
	payload := sha256.Sum256([]byte("payload"))
	target := sha256.Sum256([]byte("target"))
	receipt := campaignport.CampaignHistoryReceipt{SourceIdentifier: SourceIdentifier(key), PayloadDigest: payload, TargetID: 9, TargetDigest: target}
	terminal, err := campaignDefinitionHistoryTerminalFromReceipt(campaignDefinitionHistoryDefinitionKind, receipt)
	if err != nil {
		t.Fatalf("receipt to terminal: %v", err)
	}
	if terminal.SourceKeyDigest != key || terminal.PayloadDigest != payload || terminal.Disposition != "import" || terminal.TargetID != "9" || terminal.TargetDigest != target {
		t.Fatalf("terminal mismatch: %#v", terminal)
	}
	actual, err := campaignDefinitionHistoryReceiptFromTerminal(campaignDefinitionHistoryDefinitionKind, receipt.SourceIdentifier, terminal)
	if err != nil {
		t.Fatalf("terminal to receipt: %v", err)
	}
	if actual != receipt {
		t.Fatalf("receipt mismatch: got %#v want %#v", actual, receipt)
	}
	receipt.Replayed = true
	if _, err = campaignDefinitionHistoryTerminalFromReceipt(campaignDefinitionHistoryStepKind, receipt); err == nil {
		t.Fatal("replayed owner receipt accepted for new terminal")
	}
}

func campaignDefinitionHistoryTestJournal(table, target, run string) *Journal {
	return &Journal{
		scope: Scope{
			ImportVersion: campaignDefinitionHistoryImportVersion,
			ArchiveRunID:  run,
			AdapterID:     v1archive.DefaultAdapterID,
			TableID:       table,
			TargetDomain:  campaignDefinitionHistoryDomain,
			TargetTable:   target,
		},
		tx: func(context.Context) (pgx.Tx, error) { return nil, nil },
	}
}

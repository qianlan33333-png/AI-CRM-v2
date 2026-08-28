package v1domain

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestNewMarketingStateHistoryJournalPinsFourScopes(t *testing.T) {
	values := make([]*Journal, len(marketingStateHistoryScopes))
	for index, scope := range marketingStateHistoryScopes {
		journal, err := NewJournal(Scope{ImportVersion: marketingStateHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: marketingStateHistoryDomain, TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		values[index] = journal
	}
	journal, err := NewMarketingStateHistoryJournal(values[0], values[1], values[2], values[3])
	if err != nil || journal.ValidateMarketingStateHistoryImportScope("archive-run") != nil {
		t.Fatalf("journal=%v err=%v", journal, err)
	}
	if journal.ValidateMarketingStateHistoryImportScope("other-run") == nil {
		t.Fatal("wrong run accepted")
	}
	if _, _, err := journal.LoadMarketingStateHistoryTerminal(context.Background(), "bad", SourceIdentifier(sha256.Sum256([]byte("key")))); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestNewMarketingStateHistoryJournalRejectsCrossedTarget(t *testing.T) {
	values := make([]*Journal, len(marketingStateHistoryScopes))
	for index, scope := range marketingStateHistoryScopes {
		target := scope.target
		if index == 1 {
			target = marketingStateSnapshotTarget
		}
		journal, err := NewJournal(Scope{ImportVersion: marketingStateHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: marketingStateHistoryDomain, TargetTable: target})
		if err != nil {
			t.Fatal(err)
		}
		values[index] = journal
	}
	if journal, err := NewMarketingStateHistoryJournal(values[0], values[1], values[2], values[3]); err == nil || journal != nil {
		t.Fatalf("crossed target accepted: %v", journal)
	}
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestNewCustomerStateHistoryJournalPinsThreeScopes(t *testing.T) {
	values := make([]*Journal, len(customerStateHistoryScopes))
	for index, scope := range customerStateHistoryScopes {
		journal, err := NewJournal(Scope{ImportVersion: customerStateHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: customerStateHistoryDomain, TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		values[index] = journal
	}
	journal, err := NewCustomerStateHistoryJournal(values[0], values[1], values[2])
	if err != nil || journal.ValidateCustomerStateHistoryImportScope("archive-run") != nil {
		t.Fatalf("journal=%v err=%v", journal, err)
	}
	if journal.ValidateCustomerStateHistoryImportScope("other-run") == nil {
		t.Fatal("wrong run accepted")
	}
	if _, _, err := journal.LoadCustomerStateHistoryTerminal(context.Background(), "bad", SourceIdentifier(sha256.Sum256([]byte("key")))); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestNewCustomerStateHistoryJournalRejectsCrossedTarget(t *testing.T) {
	values := make([]*Journal, len(customerStateHistoryScopes))
	for index, scope := range customerStateHistoryScopes {
		target := scope.target
		if index == 1 {
			target = customerStateHistorySnapshotTarget
		}
		journal, err := NewJournal(Scope{ImportVersion: customerStateHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: customerStateHistoryDomain, TargetTable: target})
		if err != nil {
			t.Fatal(err)
		}
		values[index] = journal
	}
	if journal, err := NewCustomerStateHistoryJournal(values[0], values[1], values[2]); err == nil || journal != nil {
		t.Fatalf("crossed target accepted: %v", journal)
	}
}

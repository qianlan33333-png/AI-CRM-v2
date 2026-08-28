package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contacthistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func contactHistoryScopeFixture(tableID, targetTable string) Scope {
	return Scope{ImportVersion: contactHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: tableID, TargetDomain: "contact", TargetTable: targetTable}
}

func contactHistoryJournalFixture(t *testing.T) *ContactHistoryJournal {
	t.Helper()
	sidebar, err := NewJournal(contactHistoryScopeFixture(v1contacthistory.SidebarProfileFieldsTableID, contactHistorySidebarTargetTable))
	if err != nil {
		t.Fatal("sidebar_journal_failed")
	}
	results, err := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationResultsTableID, contactHistoryOwnerResultTargetTable))
	if err != nil {
		t.Fatal("result_journal_failed")
	}
	sessions, err := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationSessionsTableID, contactHistoryContextTargetTable))
	if err != nil {
		t.Fatal("session_journal_failed")
	}
	previews, err := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationPreviewsTableID, contactHistoryContextTargetTable))
	if err != nil {
		t.Fatal("preview_journal_failed")
	}
	journal, err := NewContactHistoryJournal(sidebar, results, sessions, previews)
	if err != nil {
		t.Fatal("create_journal_failed")
	}
	return journal
}

func TestContactHistoryJournalPinsAllFourScopes(t *testing.T) {
	journal := contactHistoryJournalFixture(t)
	if err := journal.ValidateContactHistoryImportScope("archive-run"); err != nil {
		t.Fatal("valid_scope_rejected")
	}
	for name, mutate := range map[string]func(*Scope){
		"version": func(scope *Scope) { scope.ImportVersion = "v1-contact-history-a2" },
		"run":     func(scope *Scope) { scope.ArchiveRunID = "other-run" },
		"adapter": func(scope *Scope) { scope.AdapterID = "other-adapter" },
		"domain":  func(scope *Scope) { scope.TargetDomain = "wecom" },
		"target":  func(scope *Scope) { scope.TargetTable = "other_history" },
	} {
		t.Run(name, func(t *testing.T) {
			scope := contactHistoryScopeFixture(v1contacthistory.SidebarProfileFieldsTableID, contactHistorySidebarTargetTable)
			mutate(&scope)
			sidebar, err := NewJournal(scope)
			if err != nil {
				t.Fatal("create_mutated_scope_failed")
			}
			results, _ := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationResultsTableID, contactHistoryOwnerResultTargetTable))
			sessions, _ := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationSessionsTableID, contactHistoryContextTargetTable))
			previews, _ := NewJournal(contactHistoryScopeFixture(v1contacthistory.OwnerMigrationPreviewsTableID, contactHistoryContextTargetTable))
			if candidate, err := NewContactHistoryJournal(sidebar, results, sessions, previews); err == nil || candidate != nil {
				t.Fatal("cross_scope_accepted")
			}
		})
	}
	var missing *ContactHistoryJournal
	if err := missing.ValidateContactHistoryImportScope("archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("nil_scope_accepted")
	}
}

func TestContactHistoryJournalReceiptRequiresExactImportTerminal(t *testing.T) {
	receipt := contactport.ContactHistoryReceipt{Kind: contactport.ContactHistorySidebar,
		SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte("contact-history-source"))), PayloadDigest: sha256.Sum256([]byte("payload")),
		TargetID: 17, TargetDigest: sha256.Sum256([]byte("target"))}
	terminal, err := contactHistoryTerminalFromReceipt(receipt)
	if err != nil {
		t.Fatal("terminal_encode_failed")
	}
	actual, err := contactHistoryReceiptFromTerminal(receipt.Kind, receipt.SourceIdentifier, terminal)
	if err != nil || actual != receipt {
		t.Fatal("receipt_roundtrip_failed")
	}
	for _, mutate := range []func(*TerminalReceipt){
		func(value *TerminalReceipt) { value.Disposition = "archive" },
		func(value *TerminalReceipt) { value.Reason = "context" },
		func(value *TerminalReceipt) { value.SourceKeyDigest[0]++ },
		func(value *TerminalReceipt) { value.TargetID = "017" },
		func(value *TerminalReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		func(value *TerminalReceipt) { value.Metadata = map[string]any{"unexpected": true} },
	} {
		bad := terminal
		mutate(&bad)
		if _, err := contactHistoryReceiptFromTerminal(receipt.Kind, receipt.SourceIdentifier, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe_terminal_accepted")
		}
	}
	for _, mutate := range []func(*contactport.ContactHistoryReceipt){
		func(value *contactport.ContactHistoryReceipt) { value.Kind = "unknown" },
		func(value *contactport.ContactHistoryReceipt) { value.SourceIdentifier = "bad" },
		func(value *contactport.ContactHistoryReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
		func(value *contactport.ContactHistoryReceipt) { value.TargetID = 0 },
		func(value *contactport.ContactHistoryReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		func(value *contactport.ContactHistoryReceipt) { value.Replayed = true },
	} {
		bad := receipt
		mutate(&bad)
		if _, err := contactHistoryTerminalFromReceipt(bad); !errors.Is(err, ErrInvalidScope) {
			t.Fatal("unsafe_receipt_accepted")
		}
	}
}

func TestContactHistoryJournalRejectsWorkOutsideCallerTransaction(t *testing.T) {
	journal := contactHistoryJournalFixture(t)
	if _, _, err := journal.LoadContactHistory(context.Background(), contactport.ContactHistorySidebar, SourceIdentifier(sha256.Sum256([]byte("source")))); err == nil {
		t.Fatal("missing_transaction_accepted")
	}
}

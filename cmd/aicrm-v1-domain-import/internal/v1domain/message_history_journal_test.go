package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

func messageHistoryScopeFixture() Scope {
	return Scope{ImportVersion: messageHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: messageHistoryTableID, TargetDomain: "wecom", TargetTable: messageHistoryTargetTable}
}

func messageHistoryReceiptFixture() wecomport.MessageHistoryReceipt {
	return wecomport.MessageHistoryReceipt{SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte("message-source"))),
		PayloadDigest: sha256.Sum256([]byte("message-payload")), TargetID: 17, TargetDigest: sha256.Sum256([]byte("message-target"))}
}

func TestMessageHistoryJournalPinsScope(t *testing.T) {
	journal, err := NewJournal(messageHistoryScopeFixture())
	if err != nil {
		t.Fatal("create_journal_failed")
	}
	if err = journal.ValidateMessageHistoryImportScope("archive-run"); err != nil {
		t.Fatal("valid_scope_rejected")
	}
	for name, mutate := range map[string]func(*Scope){
		"version": func(scope *Scope) { scope.ImportVersion = "v1-message-history-a2" },
		"run":     func(scope *Scope) { scope.ArchiveRunID = "other-run" },
		"adapter": func(scope *Scope) { scope.AdapterID = "other-adapter" },
		"table":   func(scope *Scope) { scope.TableID = "public/other_messages" },
		"domain":  func(scope *Scope) { scope.TargetDomain = "contact" },
		"target":  func(scope *Scope) { scope.TargetTable = "messages" },
	} {
		t.Run(name, func(t *testing.T) {
			scope := messageHistoryScopeFixture()
			mutate(&scope)
			candidate, err := NewJournal(scope)
			if err != nil {
				t.Fatal("create_scope_failed")
			}
			if err = candidate.ValidateMessageHistoryImportScope("archive-run"); !errors.Is(err, ErrInvalidScope) {
				t.Fatal("cross_scope_accepted")
			}
		})
	}
	var missing *Journal
	if err = missing.ValidateMessageHistoryImportScope("archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("nil_scope_accepted")
	}
}

func TestMessageHistoryJournalReceiptRequiresExactImportTerminal(t *testing.T) {
	want := messageHistoryReceiptFixture()
	terminal, err := messageHistoryTerminalFromReceipt(want)
	if err != nil {
		t.Fatal("terminal_encode_failed")
	}
	got, err := messageHistoryReceiptFromTerminal(want.SourceIdentifier, terminal)
	if err != nil || got != want {
		t.Fatal("receipt_round_trip_failed")
	}
	for _, mutate := range []func(*TerminalReceipt){
		func(value *TerminalReceipt) { value.Disposition = "quarantine" },
		func(value *TerminalReceipt) { value.Reason = "unexpected" },
		func(value *TerminalReceipt) { value.SourceKeyDigest[0]++ },
		func(value *TerminalReceipt) { value.TargetID = "017" },
		func(value *TerminalReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		func(value *TerminalReceipt) { value.Metadata = map[string]any{"unexpected": true} },
	} {
		bad := terminal
		mutate(&bad)
		if _, err = messageHistoryReceiptFromTerminal(want.SourceIdentifier, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe_terminal_accepted")
		}
	}
	for _, mutate := range []func(*wecomport.MessageHistoryReceipt){
		func(value *wecomport.MessageHistoryReceipt) { value.SourceIdentifier = "not-a-digest" },
		func(value *wecomport.MessageHistoryReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
		func(value *wecomport.MessageHistoryReceipt) { value.TargetID = 0 },
		func(value *wecomport.MessageHistoryReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		func(value *wecomport.MessageHistoryReceipt) { value.Replayed = true },
	} {
		bad := want
		mutate(&bad)
		if _, err = messageHistoryTerminalFromReceipt(bad); !errors.Is(err, ErrInvalidScope) {
			t.Fatal("unsafe_receipt_accepted")
		}
	}
}

func TestMessageHistoryJournalRejectsWorkOutsideCallerTransaction(t *testing.T) {
	journal, err := NewJournal(messageHistoryScopeFixture())
	if err != nil {
		t.Fatal("create_journal_failed")
	}
	if _, _, err = journal.LoadMessageHistory(context.Background(), messageHistoryReceiptFixture().SourceIdentifier); err == nil {
		t.Fatal("missing_transaction_accepted")
	}
}

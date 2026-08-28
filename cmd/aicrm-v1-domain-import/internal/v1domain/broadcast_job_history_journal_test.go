package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

func broadcastJobHistoryScopeFixture() Scope {
	return Scope{ImportVersion: broadcastJobHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
		TableID: broadcastJobHistoryTableID, TargetDomain: "outbound", TargetTable: broadcastJobHistoryTargetTable}
}

func broadcastJobHistoryReceiptFixture() outboundport.BroadcastJobHistoryReceipt {
	return outboundport.BroadcastJobHistoryReceipt{SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte("broadcast-job-source"))),
		PayloadDigest: sha256.Sum256([]byte("broadcast-job-payload")), TargetID: 17, TargetDigest: sha256.Sum256([]byte("broadcast-job-target"))}
}

func TestBroadcastJobHistoryJournalPinsExactScope(t *testing.T) {
	journal, err := NewJournal(broadcastJobHistoryScopeFixture())
	if err != nil || journal.ValidateBroadcastJobHistoryImportScope("archive-run") != nil {
		t.Fatal("valid_scope_rejected")
	}
	for name, mutate := range map[string]func(*Scope){
		"version": func(scope *Scope) { scope.ImportVersion = "v1-broadcast-job-history-a2" },
		"run":     func(scope *Scope) { scope.ArchiveRunID = "other-run" },
		"adapter": func(scope *Scope) { scope.AdapterID = "other-adapter" },
		"table":   func(scope *Scope) { scope.TableID = "public/other" },
		"domain":  func(scope *Scope) { scope.TargetDomain = "campaign" },
		"target":  func(scope *Scope) { scope.TargetTable = "other_history" },
	} {
		t.Run(name, func(t *testing.T) {
			scope := broadcastJobHistoryScopeFixture()
			mutate(&scope)
			candidate, createErr := NewJournal(scope)
			if createErr != nil || !errors.Is(candidate.ValidateBroadcastJobHistoryImportScope("archive-run"), ErrInvalidScope) {
				t.Fatal("cross_scope_accepted")
			}
		})
	}
	var missing *Journal
	if !errors.Is(missing.ValidateBroadcastJobHistoryImportScope("archive-run"), ErrInvalidScope) {
		t.Fatal("nil_scope_accepted")
	}
}

func TestBroadcastJobHistoryJournalRequiresCanonicalImportReceipt(t *testing.T) {
	want := broadcastJobHistoryReceiptFixture()
	terminal, err := broadcastJobHistoryTerminalFromReceipt(want)
	if err != nil {
		t.Fatal("terminal_encode_failed")
	}
	got, err := broadcastJobHistoryReceiptFromTerminal(want.SourceIdentifier, terminal)
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
		if _, err = broadcastJobHistoryReceiptFromTerminal(want.SourceIdentifier, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe_terminal_accepted")
		}
	}
	for _, mutate := range []func(*outboundport.BroadcastJobHistoryReceipt){
		func(value *outboundport.BroadcastJobHistoryReceipt) { value.SourceIdentifier = "not-a-source-key" },
		func(value *outboundport.BroadcastJobHistoryReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
		func(value *outboundport.BroadcastJobHistoryReceipt) { value.TargetID = 0 },
		func(value *outboundport.BroadcastJobHistoryReceipt) { value.TargetDigest = [sha256.Size]byte{} },
		func(value *outboundport.BroadcastJobHistoryReceipt) { value.Replayed = true },
	} {
		bad := want
		mutate(&bad)
		if _, err = broadcastJobHistoryTerminalFromReceipt(bad); !errors.Is(err, ErrInvalidScope) {
			t.Fatal("unsafe_receipt_accepted")
		}
	}
}

func TestBroadcastJobHistoryJournalRejectsWorkOutsideCallerTransaction(t *testing.T) {
	journal, err := NewJournal(broadcastJobHistoryScopeFixture())
	if err != nil {
		t.Fatal("create_journal_failed")
	}
	if _, _, err = journal.LoadBroadcastJobHistory(context.Background(), broadcastJobHistoryReceiptFixture().SourceIdentifier); err == nil {
		t.Fatal("missing_transaction_accepted")
	}
}

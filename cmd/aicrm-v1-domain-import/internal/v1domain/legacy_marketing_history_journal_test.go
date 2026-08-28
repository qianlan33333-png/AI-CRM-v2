package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func legacyMarketingHistoryScope(tableID, targetTable string) Scope {
	return Scope{ImportVersion: legacyMarketingHistoryImportVersion, ArchiveRunID: "legacy-marketing-archive", AdapterID: v1archive.DefaultAdapterID,
		TableID: tableID, TargetDomain: legacyMarketingHistoryDomain, TargetTable: targetTable}
}

func legacyMarketingHistoryJournalFixture(t *testing.T) *LegacyMarketingHistoryJournal {
	t.Helper()
	state, err := NewJournal(legacyMarketingHistoryScope(legacyMarketingStateTable, legacyMarketingStateTarget))
	if err != nil {
		t.Fatal(err)
	}
	value, err := NewJournal(legacyMarketingHistoryScope(legacyMarketingValueTable, legacyMarketingValueTarget))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := NewLegacyMarketingHistoryJournal(state, value)
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func TestLegacyMarketingHistoryJournalPinsTwoExactScopes(t *testing.T) {
	journal := legacyMarketingHistoryJournalFixture(t)
	if err := journal.ValidateLegacyMarketingHistoryImportScope("legacy-marketing-archive"); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Scope){
		"version": func(scope *Scope) { scope.ImportVersion = "v1-legacy-marketing-history-a2" },
		"run":     func(scope *Scope) { scope.ArchiveRunID = "other-archive" },
		"adapter": func(scope *Scope) { scope.AdapterID = "other-adapter" },
		"table":   func(scope *Scope) { scope.TableID = legacyMarketingValueTable },
		"domain":  func(scope *Scope) { scope.TargetDomain = "contact" },
		"target":  func(scope *Scope) { scope.TargetTable = legacyMarketingValueTarget },
	} {
		t.Run(name, func(t *testing.T) {
			stateScope := legacyMarketingHistoryScope(legacyMarketingStateTable, legacyMarketingStateTarget)
			mutate(&stateScope)
			state, err := NewJournal(stateScope)
			if err != nil {
				t.Fatal(err)
			}
			value, err := NewJournal(legacyMarketingHistoryScope(legacyMarketingValueTable, legacyMarketingValueTarget))
			if err != nil {
				t.Fatal(err)
			}
			if candidate, err := NewLegacyMarketingHistoryJournal(state, value); err == nil || candidate != nil {
				t.Fatal("cross_scope_accepted")
			}
		})
	}
	if err := journal.ValidateLegacyMarketingHistoryImportScope("other-archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("run drift error = %v", err)
	}
	var missing *LegacyMarketingHistoryJournal
	if err := missing.ValidateLegacyMarketingHistoryImportScope("legacy-marketing-archive"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("nil journal error = %v", err)
	}
}

func TestLegacyMarketingHistoryJournalReceiptPinsKindAndDigests(t *testing.T) {
	for _, kind := range []string{legacyMarketingStateKind, legacyMarketingValueKind} {
		t.Run(kind, func(t *testing.T) {
			receipt := segmentport.LegacyMarketingHistoryReceipt{Kind: kind, SourceIdentifier: SourceIdentifier(sha256.Sum256([]byte(kind + "/source"))),
				PayloadDigest: sha256.Sum256([]byte(kind + "/payload")), TargetID: 17, TargetDigest: sha256.Sum256([]byte(kind + "/target"))}
			terminal, err := legacyMarketingHistoryTerminalFromReceipt(receipt)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := legacyMarketingHistoryReceiptFromTerminal(kind, receipt.SourceIdentifier, terminal)
			if err != nil || actual != receipt {
				t.Fatalf("roundtrip=%#v err=%v", actual, err)
			}
			for name, mutate := range map[string]func(*TerminalReceipt){
				"source_key":   func(value *TerminalReceipt) { value.SourceKeyDigest[0]++ },
				"payload_zero": func(value *TerminalReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
				"target_digest_zero": func(value *TerminalReceipt) {
					value.TargetDigest = [sha256.Size]byte{}
				},
				"target_id":   func(value *TerminalReceipt) { value.TargetID = "017" },
				"disposition": func(value *TerminalReceipt) { value.Disposition = "archive" },
				"metadata":    func(value *TerminalReceipt) { value.Metadata = map[string]any{"field_digest": "unexpected"} },
			} {
				t.Run(name, func(t *testing.T) {
					bad := terminal
					mutate(&bad)
					if _, err := legacyMarketingHistoryReceiptFromTerminal(kind, receipt.SourceIdentifier, bad); !errors.Is(err, ErrConflict) {
						t.Fatalf("unsafe terminal error = %v", err)
					}
				})
			}
			for name, mutate := range map[string]func(*segmentport.LegacyMarketingHistoryReceipt){
				"kind":          func(value *segmentport.LegacyMarketingHistoryReceipt) { value.Kind = "other" },
				"source":        func(value *segmentport.LegacyMarketingHistoryReceipt) { value.SourceIdentifier = "bad" },
				"payload":       func(value *segmentport.LegacyMarketingHistoryReceipt) { value.PayloadDigest = [sha256.Size]byte{} },
				"target_id":     func(value *segmentport.LegacyMarketingHistoryReceipt) { value.TargetID = 0 },
				"target_digest": func(value *segmentport.LegacyMarketingHistoryReceipt) { value.TargetDigest = [sha256.Size]byte{} },
				"replay":        func(value *segmentport.LegacyMarketingHistoryReceipt) { value.Replayed = true },
			} {
				t.Run(name, func(t *testing.T) {
					bad := receipt
					mutate(&bad)
					if _, err := legacyMarketingHistoryTerminalFromReceipt(bad); !errors.Is(err, ErrInvalidScope) {
						t.Fatalf("unsafe receipt error = %v", err)
					}
				})
			}
		})
	}
}

func TestLegacyMarketingHistoryJournalRequiresCallerTransaction(t *testing.T) {
	journal := legacyMarketingHistoryJournalFixture(t)
	if _, _, err := journal.LoadLegacyMarketingHistory(context.Background(), legacyMarketingStateKind, SourceIdentifier(sha256.Sum256([]byte("source")))); err == nil {
		t.Fatal("missing_transaction_accepted")
	}
}

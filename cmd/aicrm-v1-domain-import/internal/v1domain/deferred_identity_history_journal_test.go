package v1domain

import (
	"crypto/sha256"
	"testing"

	v1deferredidentityhistory "github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1deferredidentityhistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestNewDeferredIdentityHistoryJournalPinsThreeScopes(t *testing.T) {
	values := make([]*Journal, 0, len(deferredIdentityHistoryScopes))
	for _, scope := range deferredIdentityHistoryScopes {
		journal, err := NewJournal(Scope{ImportVersion: DeferredIdentityHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.table, TargetDomain: DeferredIdentityHistoryDomain, TargetTable: scope.target})
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, journal)
	}
	journal, err := NewDeferredIdentityHistoryJournal(values[0], values[1], values[2])
	if err != nil || journal.ValidateDeferredIdentityHistoryImportScope("archive-run") != nil {
		t.Fatalf("journal=%v err=%v", journal, err)
	}
	if journal.ValidateDeferredIdentityHistoryImportScope("other-run") == nil {
		t.Fatal("cross-run scope accepted")
	}

	wrong, err := NewJournal(Scope{ImportVersion: DeferredIdentityHistoryImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: v1deferredidentityhistory.PeopleTableID, TargetDomain: DeferredIdentityHistoryDomain, TargetTable: MissingRootIdentityTarget})
	if err != nil {
		t.Fatal(err)
	}
	if value, err := NewDeferredIdentityHistoryJournal(wrong, values[1], values[2]); err == nil || value != nil {
		t.Fatalf("crossed scope accepted: journal=%v err=%v", value, err)
	}
}

func TestDeferredIdentityHistoryTerminalRoundTripRejectsReplay(t *testing.T) {
	key := sha256.Sum256([]byte("source"))
	digest := sha256.Sum256([]byte("target"))
	receipt := contactport.DeferredIdentityHistoryReceipt{Kind: DeferredPersonHistoryKind, SourceIdentifier: SourceIdentifier(key), PayloadDigest: sha256.Sum256([]byte("payload")), TargetID: 7, TargetDigest: digest}
	terminal, err := deferredIdentityHistoryTerminal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := deferredIdentityHistoryReceipt(receipt.Kind, receipt.SourceIdentifier, terminal)
	if err != nil || got != receipt {
		t.Fatalf("round trip receipt=%+v err=%v", got, err)
	}
	receipt.Replayed = true
	if _, err := deferredIdentityHistoryTerminal(receipt); err == nil {
		t.Fatal("replay receipt was made into a new terminal receipt")
	}
}

package v1domain

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestContactTagJournalRejectsWrongScopedJournal(t *testing.T) {
	groups := contactJournal(t, contactTagGroupsTable, "tag_groups")
	tags := contactJournal(t, contactTagsTable, "tags")
	bindings := contactJournal(t, contactBindingsTable, "customer_tags")
	if _, err := NewContactTagJournal(groups, tags, bindings); err != nil {
		t.Fatal(err)
	}
	wrong := contactJournal(t, contactBindingsTable, "tags")
	if _, err := NewContactTagJournal(groups, tags, wrong); err == nil {
		t.Fatal("wrong target scope accepted")
	}
	differentRun, err := NewJournal(Scope{ImportVersion: "v1-a", ArchiveRunID: "other-run", AdapterID: v1archive.DefaultAdapterID, TableID: contactBindingsTable, TargetDomain: "contact", TargetTable: "customer_tags"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewContactTagJournal(groups, tags, differentRun); err == nil {
		t.Fatal("different archive run accepted")
	}
}

func TestContactTagLineageFromTerminalRequiresExactDigests(t *testing.T) {
	payload := sha256.Sum256([]byte("payload"))
	field := sha256.Sum256([]byte("field"))
	target := sha256.Sum256([]byte("target"))
	receipt := TerminalReceipt{PayloadDigest: payload, Disposition: "import", TargetID: "17", TargetDigest: target,
		Metadata: map[string]any{"payload_digest": hex.EncodeToString(payload[:]), "field_digest": hex.EncodeToString(field[:])}}
	lineage, err := contactTagLineageFromTerminal(contactport.HistoricalTagGroupSource, receipt)
	if err != nil || lineage.TargetID != 17 || lineage.CustomerID != 0 || lineage.PayloadDigest != payload || lineage.FieldDigest != field || lineage.TargetDigest != target {
		t.Fatalf("lineage=%+v err=%v", lineage, err)
	}
	receipt.Metadata["payload_digest"] = hex.EncodeToString(field[:])
	if _, err = contactTagLineageFromTerminal(contactport.HistoricalTagGroupSource, receipt); err == nil {
		t.Fatal("payload mismatch accepted")
	}
}

func TestContactTagLineageFromTerminalRequiresCustomerAddressForBinding(t *testing.T) {
	payload := sha256.Sum256([]byte("payload"))
	field := sha256.Sum256([]byte("field"))
	target := sha256.Sum256([]byte("target"))
	receipt := TerminalReceipt{PayloadDigest: payload, Disposition: "import", TargetID: "17", TargetDigest: target,
		Metadata: map[string]any{"payload_digest": hex.EncodeToString(payload[:]), "field_digest": hex.EncodeToString(field[:]), "customer_id": "19"}}
	lineage, err := contactTagLineageFromTerminal(contactport.HistoricalCustomerTagSource, receipt)
	if err != nil || lineage.CustomerID != 19 || lineage.TargetID != 17 {
		t.Fatalf("lineage=%+v err=%v", lineage, err)
	}
	delete(receipt.Metadata, "customer_id")
	if _, err = contactTagLineageFromTerminal(contactport.HistoricalCustomerTagSource, receipt); err == nil {
		t.Fatal("binding without customer address accepted")
	}
}

func contactJournal(t *testing.T, table, target string) *Journal {
	t.Helper()
	journal, err := NewJournal(Scope{ImportVersion: "v1-a", ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: "contact", TargetTable: target})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

var _ contactport.HistoricalTagJournal = (*ContactTagJournal)(nil)

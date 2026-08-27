package v1domain

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
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
}

func TestContactTagLineageFromTerminalRequiresExactDigests(t *testing.T) {
	payload := sha256.Sum256([]byte("payload"))
	field := sha256.Sum256([]byte("field"))
	target := sha256.Sum256([]byte("target"))
	receipt := TerminalReceipt{PayloadDigest: payload, Disposition: "import", TargetID: "17", TargetDigest: target,
		Metadata: map[string]any{"payload_digest": hex.EncodeToString(payload[:]), "field_digest": hex.EncodeToString(field[:])}}
	lineage, err := contactTagLineageFromTerminal(receipt)
	if err != nil || lineage.TargetID != 17 || lineage.PayloadDigest != payload || lineage.FieldDigest != field || lineage.TargetDigest != target {
		t.Fatalf("lineage=%+v err=%v", lineage, err)
	}
	receipt.Metadata["payload_digest"] = hex.EncodeToString(field[:])
	if _, err = contactTagLineageFromTerminal(receipt); err == nil {
		t.Fatal("payload mismatch accepted")
	}
}

func contactJournal(t *testing.T, table, target string) *Journal {
	t.Helper()
	journal, err := NewJournal(Scope{ImportVersion: "v1-a", ArchiveRunID: "run", AdapterID: "adapter", TableID: table, TargetDomain: "contact", TargetTable: target})
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

var _ contactport.HistoricalTagJournal = (*ContactTagJournal)(nil)

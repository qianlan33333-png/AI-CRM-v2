package v1domain

import (
	"crypto/sha256"
	"testing"
)

func TestSourceIdentifierRoundTrip(t *testing.T) {
	digest := sha256.Sum256([]byte("source"))
	encoded := SourceIdentifier(digest)
	decoded, err := ParseSourceIdentifier(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != digest {
		t.Fatalf("decoded digest = %x, want %x", decoded, digest)
	}
}

func TestNewJournalRejectsUnsafeScope(t *testing.T) {
	for _, scope := range []Scope{
		{},
		{ImportVersion: "v1", ArchiveRunID: "run", AdapterID: "adapter", TableID: "public/campaigns", TargetDomain: "campaign", TargetTable: "cloud-campaigns"},
		{ImportVersion: "../v1", ArchiveRunID: "run", AdapterID: "adapter", TableID: "public/campaigns", TargetDomain: "campaign", TargetTable: "cloud_campaigns"},
	} {
		if _, err := NewJournal(scope); err == nil {
			t.Fatalf("scope %#v unexpectedly accepted", scope)
		}
	}
}

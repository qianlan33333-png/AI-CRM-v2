package v1domain

import (
	"crypto/sha256"
	"encoding/json"
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

func TestNilReceiptMetadataIsJSONObject(t *testing.T) {
	encoded, err := marshalReceiptMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value.(map[string]any); !ok {
		t.Fatalf("metadata = %s, want JSON object", encoded)
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

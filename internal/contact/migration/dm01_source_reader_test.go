package migration

import (
	"errors"
	"testing"
)

func TestCanonicalSchemaDigestFailsClosed(t *testing.T) {
	columns := []SourceColumn{
		{Ordinal: 1, Name: "id", DataType: "bigint", NotNull: true},
		{Ordinal: 2, Name: "updated_at", DataType: "timestamp with time zone", NotNull: true},
	}
	digest, err := CanonicalSchemaDigest(columns)
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest = %q, err = %v", digest, err)
	}
	changed := append([]SourceColumn(nil), columns...)
	changed[1].NotNull = false
	changedDigest, err := CanonicalSchemaDigest(changed)
	if err != nil || changedDigest == digest {
		t.Fatal("column drift did not change canonical digest")
	}
	if _, err := CanonicalSchemaDigest([]SourceColumn{{Ordinal: 2, Name: "id", DataType: "bigint", NotNull: true}, {Ordinal: 1, Name: "name", DataType: "text", NotNull: true}}); !errors.Is(err, ErrSourceSchemaDrift) {
		t.Fatalf("invalid ordinal error = %v", err)
	}
}

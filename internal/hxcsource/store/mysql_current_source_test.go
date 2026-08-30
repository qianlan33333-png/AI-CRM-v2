package store

import (
	"math"
	"testing"
	"time"
)

func TestCurrentSourceRejectsWritableDSNSurface(t *testing.T) {
	if _, err := NewMySQLCurrentSource("readonly:secret@tcp(hxc.internal:3306)/hxc?multiStatements=true"); err == nil {
		t.Fatal("multi-statement source DSN accepted")
	}
}

func TestSourceValueConversions(t *testing.T) {
	text, err := sourceNullString([]byte("member"))
	if err != nil || !text.Valid || text.String != "member" {
		t.Fatalf("source string = %#v, %v", text, err)
	}
	parsed, err := sourceNullTime([]byte("2026-08-30 12:34:56"))
	if err != nil || !parsed.Valid || !parsed.Time.Equal(time.Date(2026, 8, 30, 12, 34, 56, 0, time.UTC)) {
		t.Fatalf("source time = %#v, %v", parsed, err)
	}
	if _, err := sourceInt32(math.MaxInt32 + 1); err == nil {
		t.Fatal("overflowing source integer accepted")
	}
}

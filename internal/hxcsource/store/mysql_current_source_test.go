package store

import (
	"testing"
)

func TestCurrentSourceRejectsWritableDSNSurface(t *testing.T) {
	if _, err := NewMySQLCurrentSource("readonly:secret@tcp(hxc.internal:3306)/hxc?multiStatements=true"); err == nil {
		t.Fatal("multi-statement source DSN accepted")
	}
}

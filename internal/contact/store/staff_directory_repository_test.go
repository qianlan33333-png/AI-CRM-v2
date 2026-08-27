package store

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFiniteStaffTimestamp(t *testing.T) {
	finite := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if !finiteStaffTimestamp(finite) {
		t.Fatal("finite timestamp must be accepted")
	}
	for _, value := range []pgtype.Timestamptz{
		{},
		{Valid: true, InfinityModifier: pgtype.Infinity},
		{Valid: true, InfinityModifier: pgtype.NegativeInfinity},
	} {
		if finiteStaffTimestamp(value) {
			t.Fatalf("non-finite timestamp must be rejected: %+v", value)
		}
	}
}

package store

import (
	"os"
	"strings"
	"testing"
)

func TestPaidOrderProjectionLocksEligibleOrderBeforeEntitlementWrite(t *testing.T) {
	contents, err := os.ReadFile("queries/orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := string(contents)
	start := strings.Index(query, "-- name: GetPaidOrderProjection :one")
	if start < 0 {
		t.Fatal("GetPaidOrderProjection query is missing")
	}
	end := strings.Index(query[start:], "\n\n-- name:")
	if end < 0 || !strings.Contains(query[start:start+end], "FOR UPDATE;") {
		t.Fatal("GetPaidOrderProjection must lock the paid order projection in the entitlement transaction")
	}
}

package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestAdminReadRepositoryRequiresDatabaseAndReadTransaction(t *testing.T) {
	repository := NewAdminReadRepository(nil)
	if _, err := repository.Read(context.Background(), ""); err == nil {
		t.Fatal("nil database unexpectedly succeeded")
	}
	contents, err := os.ReadFile("queries/admin_read.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := string(contents)
	for _, allowed := range []string{"id", "event_type", "occurred_at", "dispatched", "event_id", "consumer", "status", "attempt_count", "completed_at"} {
		if !strings.Contains(query, allowed) {
			t.Fatalf("query does not contain allowed field %q", allowed)
		}
	}
	for _, forbidden := range []string{"payload", "customer_id", "idempotency_key", "lease_owner", "lease_expires_at", "last_error_code", "river_job_id", "FOR UPDATE", "INSERT", "UPDATE", "DELETE"} {
		if strings.Contains(strings.ToUpper(query), strings.ToUpper(forbidden)) {
			t.Fatalf("query contains forbidden token %q", forbidden)
		}
	}
	contents, err = os.ReadFile("admin_read_repository.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "AccessMode: pgx.ReadOnly") {
		t.Fatal("repository does not begin a read-only transaction")
	}
}

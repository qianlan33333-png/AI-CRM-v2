package store

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

func TestMergeReviewHistoryQueryIsClosedStatusPartitionedAndTransactionBound(t *testing.T) {
	query := mergeReviewHistoryQuerySource(t)
	lower := strings.ToLower(query)
	for _, required := range []string{
		"pending.state = sqlc.arg(review_status)::text",
		"pending.id > sqlc.arg(after_id)::bigint",
		"order by pending.id",
		"limit sqlc.arg(page_limit)::int",
		"cardinality(pending.identity_ids) = 1",
		"pending.candidate_customer_ids",
		"pending.resolved_at",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("history query missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"normalized_value",
		"payload",
		"external_userid",
		"external_user_id",
		"review_fingerprint",
		"fingerprint_key_version",
		"policy_version",
		"operated_by",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("history query reads forbidden field %q", forbidden)
		}
	}

	repository := NewRepository()
	if _, err := repository.ListMergeReviewsByStatus(context.Background(), identityport.MergeReviewPending, 0, 10); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("plain context error=%v, want ErrTransactionRequired", err)
	}
	if _, err := repository.ListMergeReviewsByStatus(context.Background(), identityport.MergeReviewStatus("other"), 0, 10); !errors.Is(err, identityapp.ErrMergeReviewInvalid) {
		t.Fatalf("invalid status error=%v", err)
	}
}

func mergeReviewHistoryQuerySource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("queries/identities.sql")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "-- name: ListMergeReviewsByStatus :many"
	start := strings.Index(string(source), marker)
	if start < 0 {
		t.Fatalf("canonical query %q is missing", marker)
	}
	query := string(source[start:])
	if next := strings.Index(query[len(marker):], "-- name:"); next >= 0 {
		query = query[:len(marker)+next]
	}
	return query
}

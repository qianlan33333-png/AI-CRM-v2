package store

import (
	"context"
	"errors"
	"testing"
	"time"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestRefreshRepositoryRequiresTransactionContext(t *testing.T) {
	repository := NewRefreshRepository()
	if _, err := repository.LockDefinition(context.Background(), 1); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("LockDefinition() error = %v, want transaction requirement", err)
	}
	if _, err := repository.QuerySet(context.Background()); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("QuerySet() error = %v, want transaction requirement", err)
	}
	if err := repository.ReplaceMembers(context.Background(), 1, []int64{1}, time.Now().UTC()); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ReplaceMembers() error = %v, want transaction requirement", err)
	}
}

func TestRefreshRepositoryRejectsUnsafeReplacementInputs(t *testing.T) {
	repository := NewRefreshRepository()
	reference := time.Date(2026, 8, 13, 2, 3, 4, 0, time.UTC)
	tests := []struct {
		name        string
		segmentID   int64
		customerIDs []int64
		instant     time.Time
	}{
		{name: "invalid segment", segmentID: 0, instant: reference},
		{name: "zero customer", segmentID: 1, customerIDs: []int64{0}, instant: reference},
		{name: "unsorted customers", segmentID: 1, customerIDs: []int64{2, 1}, instant: reference},
		{name: "duplicate customers", segmentID: 1, customerIDs: []int64{1, 1}, instant: reference},
		{name: "non UTC instant", segmentID: 1, instant: reference.In(time.FixedZone("offset", 3600))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := repository.ReplaceMembers(context.Background(), segmentport.SegmentID(test.segmentID), test.customerIDs, test.instant)
			if !errors.Is(err, segmentapp.ErrInvalidSegmentRefresh) {
				t.Fatalf("ReplaceMembers() error = %v, want invalid refresh", err)
			}
		})
	}
	if !canonicalCustomerIDs(nil) {
		t.Fatal("empty customer set must be a valid replacement")
	}
}

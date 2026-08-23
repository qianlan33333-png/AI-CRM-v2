package store

import (
	"context"
	"errors"
	"testing"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

func TestTouchPlanSnapshotRepositoryRequiresCallerTransaction(t *testing.T) {
	repository := NewTouchPlanSnapshotRepository()
	if _, err := repository.ReadSegmentTouchPlanSnapshot(context.Background(), 1); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ReadSegmentTouchPlanSnapshot() error = %v, want transaction requirement", err)
	}
	if _, err := repository.ReadAudiencePackageTouchPlanSnapshot(context.Background(), 1); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("ReadAudiencePackageTouchPlanSnapshot() error = %v, want transaction requirement", err)
	}
	if _, err := repository.ReadSegmentTouchPlanSnapshot(context.Background(), segmentport.SegmentID(0)); !errors.Is(err, segmentport.ErrTouchPlanSnapshotUnavailable) {
		t.Fatalf("invalid SegmentID error = %v", err)
	}
}

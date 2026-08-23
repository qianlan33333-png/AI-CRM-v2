package port

import (
	"testing"
	"time"
)

func TestTouchPlanSnapshotsAreClosedAndDigestBound(t *testing.T) {
	watermark := time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC)
	segment := SegmentTouchPlanSnapshot{SegmentID: 7, RefreshedAt: watermark, CustomerIDs: []CustomerID{2, 9}}
	segment.Digest = CanonicalSegmentTouchPlanSnapshotDigest(segment.SegmentID, segment.RefreshedAt, segment.CustomerIDs)
	if !segment.Valid() {
		t.Fatalf("segment snapshot must be valid: %#v", segment)
	}
	segment.CustomerIDs[1] = 8
	if segment.Valid() {
		t.Fatal("mutated member snapshot must not retain a valid digest")
	}

	audience := AudiencePackageTouchPlanSnapshot{PackageID: 11, PackageVersion: 6, RefreshedAt: watermark, CustomerIDs: []CustomerID{2, 9}}
	audience.Digest = CanonicalAudiencePackageTouchPlanSnapshotDigest(audience.PackageID, audience.PackageVersion, audience.RefreshedAt, audience.CustomerIDs)
	if !audience.Valid() {
		t.Fatalf("audience snapshot must be valid: %#v", audience)
	}
	audience.PackageVersion++
	if audience.Valid() {
		t.Fatal("mutated package version must not retain a valid digest")
	}
}

func TestTouchPlanSnapshotRejectsOversizedOrUnsortedMembers(t *testing.T) {
	watermark := time.Date(2026, time.August, 23, 2, 3, 4, 0, time.UTC)
	unsorted := SegmentTouchPlanSnapshot{SegmentID: 7, RefreshedAt: watermark, CustomerIDs: []CustomerID{9, 2}}
	unsorted.Digest = CanonicalSegmentTouchPlanSnapshotDigest(unsorted.SegmentID, unsorted.RefreshedAt, unsorted.CustomerIDs)
	if unsorted.Valid() {
		t.Fatal("unsorted members must be rejected")
	}
	oversizedIDs := make([]CustomerID, MaximumTouchPlanSnapshotMembers+1)
	for index := range oversizedIDs {
		oversizedIDs[index] = CustomerID(index + 1)
	}
	oversized := SegmentTouchPlanSnapshot{SegmentID: 7, RefreshedAt: watermark, CustomerIDs: oversizedIDs}
	oversized.Digest = CanonicalSegmentTouchPlanSnapshotDigest(oversized.SegmentID, oversized.RefreshedAt, oversized.CustomerIDs)
	if oversized.Valid() {
		t.Fatal("oversized source must be rejected before Campaign policy evaluation")
	}
}

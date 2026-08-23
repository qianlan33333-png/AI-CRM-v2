package port

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

var ErrTouchPlanSnapshotUnavailable = errors.New("segment touch-plan snapshot unavailable")

// MaximumTouchPlanSnapshotMembers is the shared, closed Campaign initiation
// source limit. A source larger than this is rejected before Contact policy
// evaluation or Campaign materialization.
const MaximumTouchPlanSnapshotMembers = 1000

// TouchPlanSnapshotReader is the only cross-domain read seam for Campaign
// initiation. Implementations must use the caller's UoW transaction context
// and lock the authoritative Segment/Audience rows before returning a member
// snapshot. It intentionally has no write or paging surface.
type TouchPlanSnapshotReader interface {
	ReadSegmentTouchPlanSnapshot(context.Context, SegmentID) (SegmentTouchPlanSnapshot, error)
	ReadAudiencePackageTouchPlanSnapshot(context.Context, SegmentID) (AudiencePackageTouchPlanSnapshot, error)
}

// SegmentTouchPlanSnapshot freezes a current Segment member set. Digest is
// authored by Segment from the locked, sorted customer IDs; it is not a caller
// input and never contains a phone, unionid, or provider identifier.
type SegmentTouchPlanSnapshot struct {
	SegmentID   SegmentID
	RefreshedAt time.Time
	CustomerIDs []CustomerID
	Digest      string
}

func (snapshot SegmentTouchPlanSnapshot) Valid() bool {
	return snapshot.SegmentID > 0 && validTouchPlanWatermark(snapshot.RefreshedAt) &&
		validTouchPlanCustomerIDs(snapshot.CustomerIDs) &&
		snapshot.Digest == CanonicalSegmentTouchPlanSnapshotDigest(snapshot.SegmentID, snapshot.RefreshedAt, snapshot.CustomerIDs)
}

// AudiencePackageTouchPlanSnapshot adds the package version to the same
// authoritative Segment member snapshot. Package ID is the Segment-backed
// local package ID on current main.
type AudiencePackageTouchPlanSnapshot struct {
	PackageID      SegmentID
	PackageVersion int64
	RefreshedAt    time.Time
	CustomerIDs    []CustomerID
	Digest         string
}

func (snapshot AudiencePackageTouchPlanSnapshot) Valid() bool {
	return snapshot.PackageID > 0 && snapshot.PackageVersion > 0 && validTouchPlanWatermark(snapshot.RefreshedAt) &&
		validTouchPlanCustomerIDs(snapshot.CustomerIDs) &&
		snapshot.Digest == CanonicalAudiencePackageTouchPlanSnapshotDigest(snapshot.PackageID, snapshot.PackageVersion, snapshot.RefreshedAt, snapshot.CustomerIDs)
}

func CanonicalSegmentTouchPlanSnapshotDigest(segmentID SegmentID, refreshedAt time.Time, customerIDs []CustomerID) string {
	return canonicalTouchPlanSnapshotDigest("segment.touch_plan_members.v1", int64(segmentID), 0, refreshedAt, customerIDs)
}

func CanonicalAudiencePackageTouchPlanSnapshotDigest(packageID SegmentID, packageVersion int64, refreshedAt time.Time, customerIDs []CustomerID) string {
	return canonicalTouchPlanSnapshotDigest("segment.ai_audience_package_touch_plan_members.v1", int64(packageID), packageVersion, refreshedAt, customerIDs)
}

func canonicalTouchPlanSnapshotDigest(namespace string, sourceID, version int64, refreshedAt time.Time, customerIDs []CustomerID) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(namespace))
	for _, value := range []string{strconv.FormatInt(sourceID, 10), strconv.FormatInt(version, 10), refreshedAt.UTC().Format(time.RFC3339Nano)} {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(value))
	}
	for _, customerID := range customerIDs {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(int64(customerID), 10)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validTouchPlanWatermark(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.UTC().Format(time.RFC3339Nano) == value.Format(time.RFC3339Nano)
}

func validTouchPlanCustomerIDs(customerIDs []CustomerID) bool {
	if len(customerIDs) > MaximumTouchPlanSnapshotMembers {
		return false
	}
	for index, customerID := range customerIDs {
		if customerID < 1 || index > 0 && customerIDs[index-1] >= customerID {
			return false
		}
	}
	return true
}

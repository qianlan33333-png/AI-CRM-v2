// Package store binds the closed Segment query family to sqlc-generated code.
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	segmentdb "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store/generated"
)

// QuerySet contains no caller-provided SQL surface. Its DBTX is normally a
// transaction supplied by the future S04 refresh unit of work.
type QuerySet struct{ queries *segmentdb.Queries }

func NewQuerySet(db segmentdb.DBTX) *QuerySet {
	return &QuerySet{queries: segmentdb.New(db)}
}

func (set *QuerySet) Universe(ctx context.Context) ([]int64, error) {
	return set.queries.SelectSegmentUniverse(ctx)
}
func (set *QuerySet) StageEqual(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentStageEqual(ctx, value)
}
func (set *QuerySet) StageAny(ctx context.Context, values []int64) ([]int64, error) {
	return set.queries.SelectSegmentStageAny(ctx, values)
}
func (set *QuerySet) OwnerEqual(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentOwnerEqual(ctx, value)
}
func (set *QuerySet) OwnerAny(ctx context.Context, values []int64) ([]int64, error) {
	return set.queries.SelectSegmentOwnerAny(ctx, values)
}
func (set *QuerySet) ChannelEqual(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentChannelEqual(ctx, value)
}
func (set *QuerySet) ChannelAny(ctx context.Context, values []int64) ([]int64, error) {
	return set.queries.SelectSegmentChannelAny(ctx, values)
}
func (set *QuerySet) TagAny(ctx context.Context, values []int64) ([]int64, error) {
	return set.queries.SelectSegmentTagAny(ctx, values)
}
func (set *QuerySet) AddedBefore(ctx context.Context, instant time.Time) ([]int64, error) {
	return set.queries.SelectSegmentAddedBefore(ctx, timestamp(instant))
}
func (set *QuerySet) AddedAfter(ctx context.Context, instant time.Time) ([]int64, error) {
	return set.queries.SelectSegmentAddedAfter(ctx, timestamp(instant))
}
func (set *QuerySet) LastInteractBefore(ctx context.Context, instant time.Time) ([]int64, error) {
	return set.queries.SelectSegmentLastInteractBefore(ctx, timestamp(instant))
}
func (set *QuerySet) LastInteractAfter(ctx context.Context, instant time.Time) ([]int64, error) {
	return set.queries.SelectSegmentLastInteractAfter(ctx, timestamp(instant))
}
func (set *QuerySet) DeletedEqual(ctx context.Context, value bool) ([]int64, error) {
	return set.queries.SelectSegmentDeletedEqual(ctx, value)
}
func (set *QuerySet) LegacyAudiencePackageSnapshot(ctx context.Context, sourceID int64) ([]int64, error) {
	return set.queries.SelectLegacyAudiencePackageSnapshot(ctx, sourceID)
}
func (set *QuerySet) HXCSubscriptionTierEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCSubscriptionTierEqual(ctx, value)
}
func (set *QuerySet) HXCSubscriptionActiveEqual(ctx context.Context, value bool) ([]int64, error) {
	return set.queries.SelectSegmentHXCSubscriptionActiveEqual(ctx, value)
}
func (set *QuerySet) HXCDaysRemainingGTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCDaysRemainingGTE(ctx, value)
}
func (set *QuerySet) HXCDaysRemainingLTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCDaysRemainingLTE(ctx, value)
}
func (set *QuerySet) HXCUserMessages7DGTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCUserMessages7DGTE(ctx, value)
}
func (set *QuerySet) HXCUserMessages7DLTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCUserMessages7DLTE(ctx, value)
}
func (set *QuerySet) HXCUserMessages30DGTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCUserMessages30DGTE(ctx, value)
}
func (set *QuerySet) HXCUserMessages30DLTE(ctx context.Context, value int64) ([]int64, error) {
	return set.queries.SelectSegmentHXCUserMessages30DLTE(ctx, value)
}
func (set *QuerySet) HXCLastCapabilityEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCLastCapabilityEqual(ctx, value)
}
func (set *QuerySet) HXCBusinessStageEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCBusinessStageEqual(ctx, value)
}
func (set *QuerySet) HXCMainLineTypeEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCMainLineTypeEqual(ctx, value)
}
func (set *QuerySet) HXCUserSegmentEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCUserSegmentEqual(ctx, value)
}
func (set *QuerySet) HXCFocusTopicAny(ctx context.Context, values []string) ([]int64, error) {
	return set.queries.SelectSegmentHXCFocusTopicAny(ctx, values)
}
func (set *QuerySet) HXCPainTagEqual(ctx context.Context, value string) ([]int64, error) {
	return set.queries.SelectSegmentHXCPainTagEqual(ctx, value)
}

func timestamp(instant time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: instant, Valid: true}
}

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

func timestamp(instant time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: instant, Valid: true}
}

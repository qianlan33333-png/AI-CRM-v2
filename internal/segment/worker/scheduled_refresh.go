// Package worker owns River execution for Segment background work.
package worker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"github.com/riverqueue/river"
)

const (
	ScheduledRefreshJobKind = "segment_refresh_scheduled"
	scheduledRefreshTimeout = 5 * time.Minute
)

var ErrInvalidScheduledRefreshWorker = errors.New("invalid scheduled segment refresh worker")

type ScheduledRefreshArgs struct{}

func (ScheduledRefreshArgs) Kind() string { return ScheduledRefreshJobKind }

// ScheduledRefreshCandidate is an internal worker projection and deliberately
// does not extend the cross-domain Segment port.
type ScheduledRefreshCandidate struct {
	SegmentID   segmentport.SegmentID
	RefreshCron string
}

type ScheduledRefreshFinder interface {
	ListScheduledRefreshCandidates(context.Context) ([]ScheduledRefreshCandidate, error)
}

// RefreshInvoker is deliberately limited to the already-closed S04A unit of
// work. It has no write API of its own.
type RefreshInvoker interface {
	RefreshOnce(context.Context, segmentport.SegmentID, time.Time) (segmentapp.RefreshResult, error)
}

type ScheduledRefreshWorker struct {
	river.WorkerDefaults[ScheduledRefreshArgs]
	finder  ScheduledRefreshFinder
	refresh RefreshInvoker
	clock   func() time.Time
}

func NewScheduledRefreshWorker(
	finder ScheduledRefreshFinder,
	refresh RefreshInvoker,
	clock func() time.Time,
) (*ScheduledRefreshWorker, error) {
	if isNil(finder) || isNil(refresh) || clock == nil {
		return nil, ErrInvalidScheduledRefreshWorker
	}
	return &ScheduledRefreshWorker{finder: finder, refresh: refresh, clock: clock}, nil
}

func (worker *ScheduledRefreshWorker) Work(ctx context.Context, job *river.Job[ScheduledRefreshArgs]) error {
	if worker == nil || isNil(worker.finder) || isNil(worker.refresh) || worker.clock == nil || ctx == nil || job == nil {
		return ErrInvalidScheduledRefreshWorker
	}
	reference := worker.clock()
	if reference.IsZero() {
		return ErrInvalidScheduledRefreshWorker
	}
	reference = reference.UTC().Truncate(time.Minute)
	candidates, err := worker.finder.ListScheduledRefreshCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list scheduled segment refreshes: %w", err)
	}
	for _, candidate := range candidates {
		if candidate.SegmentID <= 0 {
			return ErrInvalidScheduledRefreshWorker
		}
		matches, err := segmentapp.RefreshCronMatches(candidate.RefreshCron, reference)
		if err != nil {
			return fmt.Errorf("validate scheduled segment %d: %w", candidate.SegmentID, err)
		}
		if !matches {
			continue
		}
		if _, err := worker.refresh.RefreshOnce(ctx, candidate.SegmentID, reference); err != nil {
			return fmt.Errorf("refresh scheduled segment %d: %w", candidate.SegmentID, err)
		}
	}
	return nil
}

func (*ScheduledRefreshWorker) Timeout(*river.Job[ScheduledRefreshArgs]) time.Duration {
	return scheduledRefreshTimeout
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	return !reflected.IsValid() ||
		((reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func ||
			reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Map ||
			reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil())
}

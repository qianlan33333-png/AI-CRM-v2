package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	"github.com/riverqueue/river"
)

type recordingScheduledFinder struct {
	candidates []ScheduledRefreshCandidate
	err        error
}

func (finder *recordingScheduledFinder) ListScheduledRefreshCandidates(context.Context) ([]ScheduledRefreshCandidate, error) {
	return append([]ScheduledRefreshCandidate(nil), finder.candidates...), finder.err
}

type recordingRefreshInvoker struct {
	calls []segmentport.SegmentID
	refs  []time.Time
	err   error
}

func (invoker *recordingRefreshInvoker) RefreshOnce(_ context.Context, id segmentport.SegmentID, reference time.Time) (segmentapp.RefreshResult, error) {
	invoker.calls = append(invoker.calls, id)
	invoker.refs = append(invoker.refs, reference)
	return segmentapp.RefreshResult{SegmentID: id, RefreshedAt: reference}, invoker.err
}

func TestScheduledRefreshWorkerRunsOnlyDueCandidatesAtStableUTCMinute(t *testing.T) {
	clock := time.Date(2026, time.August, 13, 9, 15, 31, 0, time.FixedZone("UTC+8", 8*60*60))
	finder := &recordingScheduledFinder{candidates: []ScheduledRefreshCandidate{
		{SegmentID: 1, RefreshCron: "15 1 * * *"},
		{SegmentID: 2, RefreshCron: "16 1 * * *"},
	}}
	invoker := &recordingRefreshInvoker{}
	worker, err := NewScheduledRefreshWorker(finder, invoker, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(context.Background(), &river.Job[ScheduledRefreshArgs]{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(invoker.calls, []segmentport.SegmentID{1}) {
		t.Fatalf("RefreshOnce() segment IDs = %v, want [1]", invoker.calls)
	}
	wantReference := clock.UTC().Truncate(time.Minute)
	if len(invoker.refs) != 1 || !invoker.refs[0].Equal(wantReference) || invoker.refs[0].Location() != time.UTC {
		t.Fatalf("RefreshOnce() references = %v, want [%v UTC]", invoker.refs, wantReference)
	}
}

func TestScheduledRefreshWorkerFailsClosed(t *testing.T) {
	validFinder := &recordingScheduledFinder{}
	validInvoker := &recordingRefreshInvoker{}
	var typedNilFinder *recordingScheduledFinder
	var typedNilInvoker *recordingRefreshInvoker
	for _, test := range []struct {
		name    string
		finder  ScheduledRefreshFinder
		invoker RefreshInvoker
		clock   func() time.Time
	}{
		{name: "nil finder", invoker: validInvoker, clock: time.Now},
		{name: "typed nil finder", finder: typedNilFinder, invoker: validInvoker, clock: time.Now},
		{name: "nil invoker", finder: validFinder, clock: time.Now},
		{name: "typed nil invoker", finder: validFinder, invoker: typedNilInvoker, clock: time.Now},
		{name: "nil clock", finder: validFinder, invoker: validInvoker},
	} {
		t.Run(test.name, func(t *testing.T) {
			worker, err := NewScheduledRefreshWorker(test.finder, test.invoker, test.clock)
			if worker != nil || !errors.Is(err, ErrInvalidScheduledRefreshWorker) {
				t.Fatalf("NewScheduledRefreshWorker() = %v, %v", worker, err)
			}
		})
	}

	worker, err := NewScheduledRefreshWorker(&recordingScheduledFinder{candidates: []ScheduledRefreshCandidate{{SegmentID: 1, RefreshCron: "@daily"}}}, validInvoker, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(context.Background(), &river.Job[ScheduledRefreshArgs]{}); !errors.Is(err, segmentapp.ErrInvalidRefreshSchedule) {
		t.Fatalf("Work(invalid cron) error = %v, want invalid schedule", err)
	}
}

func TestScheduledRefreshArgsAndTimeoutAreStable(t *testing.T) {
	if got := (ScheduledRefreshArgs{}).Kind(); got != "segment_refresh_scheduled" {
		t.Fatalf("ScheduledRefreshArgs.Kind() = %q", got)
	}
	worker, err := NewScheduledRefreshWorker(&recordingScheduledFinder{}, &recordingRefreshInvoker{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if got := worker.Timeout(nil); got != 5*time.Minute {
		t.Fatalf("Timeout() = %s, want 5m", got)
	}
}

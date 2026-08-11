package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

type recordingEventPartitionEnsurer struct {
	calls        int
	ctx          context.Context
	anchor       time.Time
	futureMonths int32
	err          error
}

func (ensurer *recordingEventPartitionEnsurer) EnsureEventPartitions(
	ctx context.Context,
	anchor time.Time,
	futureMonths int32,
) error {
	ensurer.calls++
	ensurer.ctx = ctx
	ensurer.anchor = anchor
	ensurer.futureMonths = futureMonths
	return ensurer.err
}

func TestEventPartitionMaintenanceArgsKindIsStable(t *testing.T) {
	if got := (EventPartitionMaintenanceArgs{}).Kind(); got != "contact_event_partitions.ensure" {
		t.Fatalf("EventPartitionMaintenanceArgs.Kind() = %q, want %q", got, "contact_event_partitions.ensure")
	}
}

func TestNewEventPartitionMaintenanceWorkerFailsClosedForInvalidDependencies(t *testing.T) {
	validEnsurer := &recordingEventPartitionEnsurer{}
	var typedNilEnsurer *recordingEventPartitionEnsurer

	tests := []struct {
		name    string
		ensurer EventPartitionEnsurer
		clock   func() time.Time
	}{
		{name: "nil ensurer", clock: time.Now},
		{name: "typed nil ensurer", ensurer: typedNilEnsurer, clock: time.Now},
		{name: "nil clock", ensurer: validEnsurer},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worker, err := NewEventPartitionMaintenanceWorker(test.ensurer, test.clock)
			if worker != nil || err == nil {
				t.Fatalf("NewEventPartitionMaintenanceWorker() = %v, %v; want nil, non-nil error", worker, err)
			}
		})
	}
}

func TestEventPartitionMaintenanceWorkerUsesUTCAnchorAndFixedHorizon(t *testing.T) {
	ensurer := &recordingEventPartitionEnsurer{}
	clockValue := time.Date(2026, time.March, 8, 23, 45, 0, 123456789, time.FixedZone("UTC-7", -7*60*60))
	clockCalls := 0
	worker, err := NewEventPartitionMaintenanceWorker(ensurer, func() time.Time {
		clockCalls++
		return clockValue
	})
	if err != nil {
		t.Fatalf("NewEventPartitionMaintenanceWorker() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), "event-partitions", "request-context")
	if err := worker.Work(ctx, &river.Job[EventPartitionMaintenanceArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	if ensurer.calls != 1 {
		t.Fatalf("EnsureEventPartitions() calls = %d, want 1", ensurer.calls)
	}
	if clockCalls != 1 {
		t.Fatalf("clock() calls = %d, want 1", clockCalls)
	}
	if ensurer.ctx != ctx {
		t.Fatalf("EnsureEventPartitions() context = %v, want original context", ensurer.ctx)
	}
	wantAnchor := clockValue.UTC()
	if !ensurer.anchor.Equal(wantAnchor) || ensurer.anchor.Location() != time.UTC {
		t.Fatalf("EnsureEventPartitions() anchor = %v (%v), want %v (UTC)", ensurer.anchor, ensurer.anchor.Location(), wantAnchor)
	}
	if ensurer.futureMonths != 3 {
		t.Fatalf("EnsureEventPartitions() futureMonths = %d, want 3", ensurer.futureMonths)
	}
}

func TestEventPartitionMaintenanceWorkerPropagatesEnsurerErrorUnchanged(t *testing.T) {
	wantErr := errors.New("ensure partitions failed")
	ensurer := &recordingEventPartitionEnsurer{err: wantErr}
	worker, err := NewEventPartitionMaintenanceWorker(ensurer, func() time.Time {
		return time.Date(2026, time.March, 9, 6, 45, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("NewEventPartitionMaintenanceWorker() error = %v", err)
	}

	if got := worker.Work(context.Background(), &river.Job[EventPartitionMaintenanceArgs]{}); got != wantErr {
		t.Fatalf("Work() error = %v, want original error %v", got, wantErr)
	}
	if ensurer.calls != 1 {
		t.Fatalf("EnsureEventPartitions() calls = %d, want 1", ensurer.calls)
	}
}

func TestEventPartitionMaintenanceWorkerTimeoutIsFixed(t *testing.T) {
	worker, err := NewEventPartitionMaintenanceWorker(&recordingEventPartitionEnsurer{}, time.Now)
	if err != nil {
		t.Fatalf("NewEventPartitionMaintenanceWorker() error = %v", err)
	}

	if got := worker.Timeout(nil); got != 2*time.Minute {
		t.Fatalf("Timeout() = %s, want 2m", got)
	}
}

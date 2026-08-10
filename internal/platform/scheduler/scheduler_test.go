package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	jobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	queueriver "github.com/riverqueue/river"
)

type schedulerArgs struct{}

func (schedulerArgs) Kind() string { return "p2s05_scheduler" }

type schedulerUnknownArgs struct{}

func (schedulerUnknownArgs) Kind() string { return "p2s05_unknown" }

type schedulerPointerArgs struct{}

func (*schedulerPointerArgs) Kind() string { return "p2s05_pointer" }

type schedulerWorker struct {
	queueriver.WorkerDefaults[schedulerArgs]
}

func (*schedulerWorker) Work(context.Context, *queueriver.Job[schedulerArgs]) error { return nil }

type pointerSchedule struct{}

func (*pointerSchedule) Next(current time.Time) time.Time { return current.Add(time.Minute) }

func TestBuildValidatesTheSinglePeriodicCatalog(t *testing.T) {
	workers := jobqueue.NewWorkerRegistry()
	if err := jobqueue.AddWorker(workers, jobqueue.QueueEvent, &schedulerWorker{}); err != nil {
		t.Fatal(err)
	}
	valid := Definition{ID: "p2s05.event", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}}
	typedNilSchedule := (*pointerSchedule)(nil)
	var typedNilArgs *schedulerPointerArgs

	tests := []struct {
		name        string
		workers     *jobqueue.WorkerRegistry
		definitions []Definition
		wantErr     error
		wantJobs    int
	}{
		{name: "empty catalog", workers: workers},
		{name: "valid explicit event job", workers: workers, definitions: []Definition{valid}, wantJobs: 1},
		{name: "nil registry", definitions: []Definition{valid}, wantErr: ErrInvalidRegistry},
		{name: "empty ID", workers: workers, definitions: []Definition{{Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "one character ID", workers: workers, definitions: []Definition{{ID: "x", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "invalid ID", workers: workers, definitions: []Definition{{ID: "has space", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "duplicate ID", workers: workers, definitions: []Definition{valid, valid}, wantErr: ErrDuplicateID},
		{name: "nil schedule", workers: workers, definitions: []Definition{{ID: "p2s05.nil", Queue: jobqueue.QueueEvent, Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "typed nil schedule", workers: workers, definitions: []Definition{{ID: "p2s05.typed_nil", Queue: jobqueue.QueueEvent, Schedule: typedNilSchedule, Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "nil args", workers: workers, definitions: []Definition{{ID: "p2s05.nil_args", Queue: jobqueue.QueueEvent, Schedule: Never()}}, wantErr: ErrInvalidDefinition},
		{name: "typed nil args", workers: workers, definitions: []Definition{{ID: "p2s05.typed_nil_args", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: typedNilArgs}}, wantErr: ErrInvalidDefinition},
		{name: "unregistered kind", workers: workers, definitions: []Definition{{ID: "p2s05.unknown", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerUnknownArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "wrong queue", workers: workers, definitions: []Definition{{ID: "p2s05.wrong_queue", Queue: jobqueue.QueueHeavy, Schedule: Never(), Args: schedulerArgs{}}}, wantErr: ErrInvalidDefinition},
		{name: "default queue override", workers: workers, definitions: []Definition{{ID: "p2s05.default", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}, Options: &queueriver.InsertOpts{Queue: "default"}}}, wantErr: ErrInvalidDefinition},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := Build(test.workers, test.definitions)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Build() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := len(plan.Jobs()); got != test.wantJobs {
				t.Fatalf("Jobs() count = %d, want %d", got, test.wantJobs)
			}
		})
	}
}

func TestPlanAndOptionsAreDefensiveCopies(t *testing.T) {
	workers := jobqueue.NewWorkerRegistry()
	if err := jobqueue.AddWorker(workers, jobqueue.QueueEvent, &schedulerWorker{}); err != nil {
		t.Fatal(err)
	}
	inputOptions := &queueriver.InsertOpts{Priority: 2}
	plan, err := Build(workers, []Definition{{
		ID: "p2s05.copy", Queue: jobqueue.QueueEvent, Schedule: Never(), Args: schedulerArgs{}, Options: inputOptions,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if inputOptions.Queue != "" {
		t.Fatalf("Build() mutated caller options queue to %q", inputOptions.Queue)
	}
	first := plan.Jobs()
	first[0] = nil
	if second := plan.Jobs(); len(second) != 1 || second[0] == nil {
		t.Fatalf("Jobs() exposed mutable plan state: %#v", second)
	}
	if jobs := (*Plan)(nil).Jobs(); jobs != nil {
		t.Fatalf("nil Plan Jobs() = %#v, want nil", jobs)
	}
}

func TestEveryRejectsSubSecondIntervals(t *testing.T) {
	for _, interval := range []time.Duration{-1, 0, time.Second - 1} {
		if _, err := Every(interval); !errors.Is(err, ErrInvalidInterval) {
			t.Fatalf("Every(%s) error = %v, want %v", interval, err, ErrInvalidInterval)
		}
	}
	schedule, err := Every(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(10, 0)
	if got := schedule.Next(now); !got.Equal(now.Add(time.Second)) {
		t.Fatalf("Every().Next() = %s", got)
	}
	if got := Never().Next(now); !got.After(now.Add(100 * 365 * 24 * time.Hour)) {
		t.Fatalf("Never().Next() = %s, want far future", got)
	}
}

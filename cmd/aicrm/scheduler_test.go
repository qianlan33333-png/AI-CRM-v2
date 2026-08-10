package main

import (
	"context"
	"testing"

	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

type schedulerDispatchWorker struct {
	river.WorkerDefaults[eventdispatcher.DispatchArgs]
}

func (*schedulerDispatchWorker) Work(context.Context, *river.Job[eventdispatcher.DispatchArgs]) error {
	return nil
}

func TestSchedulerPlanRegistersEventDispatcher(t *testing.T) {
	workers := platformjobqueue.NewWorkerRegistry()
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, &schedulerDispatchWorker{}); err != nil {
		t.Fatal(err)
	}
	plan, err := schedulerPlan(workers)
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 1 {
		t.Fatalf("schedulerPlan() jobs = %d, want 1", len(jobs))
	}
}

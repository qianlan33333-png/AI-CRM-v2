package main

import (
	"context"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactworker "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/worker"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	"github.com/riverqueue/river"
)

type schedulerDispatchWorker struct {
	river.WorkerDefaults[eventdispatcher.DispatchArgs]
}

type schedulerPartitionWorker struct {
	river.WorkerDefaults[contactworker.EventPartitionMaintenanceArgs]
}

type schedulerSegmentRefreshWorker struct {
	river.WorkerDefaults[segmentworker.ScheduledRefreshArgs]
}

type schedulerExternalContactSyncWorker struct {
	river.WorkerDefaults[wecomapp.ExternalContactSyncJobArgs]
}

func (*schedulerSegmentRefreshWorker) Work(context.Context, *river.Job[segmentworker.ScheduledRefreshArgs]) error {
	return nil
}

func (*schedulerPartitionWorker) Work(
	context.Context,
	*river.Job[contactworker.EventPartitionMaintenanceArgs],
) error {
	return nil
}

func (*schedulerDispatchWorker) Work(context.Context, *river.Job[eventdispatcher.DispatchArgs]) error {
	return nil
}

func (*schedulerExternalContactSyncWorker) Work(context.Context, *river.Job[wecomapp.ExternalContactSyncJobArgs]) error {
	return nil
}

func TestSchedulerPlanRegistersEventDispatcher(t *testing.T) {
	workers := platformjobqueue.NewWorkerRegistry()
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, &schedulerDispatchWorker{}); err != nil {
		t.Fatal(err)
	}
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, &schedulerPartitionWorker{}); err != nil {
		t.Fatal(err)
	}
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, &schedulerSegmentRefreshWorker{}); err != nil {
		t.Fatal(err)
	}
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 3 {
		t.Fatalf("schedulerPlan() jobs = %d, want 3", len(jobs))
	}
}

func TestSchedulerPlanAddsOnlyExplicitDirectorySyncStaff(t *testing.T) {
	workers := platformjobqueue.NewWorkerRegistry()
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, &schedulerDispatchWorker{}); err != nil {
		t.Fatal(err)
	}
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, &schedulerPartitionWorker{}); err != nil {
		t.Fatal(err)
	}
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueHeavy, &schedulerSegmentRefreshWorker{}); err != nil {
		t.Fatal(err)
	}
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueSync, &schedulerExternalContactSyncWorker{}); err != nil {
		t.Fatal(err)
	}
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{Enabled: true, StaffUserIDs: []string{"staff-1", "staff-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 5 {
		t.Fatalf("schedulerPlan() jobs = %d, want 5", len(jobs))
	}
}

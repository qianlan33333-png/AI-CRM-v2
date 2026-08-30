package main

import (
	"context"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactworker "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/worker"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	hxcworker "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/worker"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomarchive "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/archive"
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

type schedulerAcquisitionRecoveryWorker struct {
	river.WorkerDefaults[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]
}

type schedulerHXCCurrentSyncWorker struct {
	river.WorkerDefaults[hxcworker.CurrentSyncJobArgs]
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

func (*schedulerAcquisitionRecoveryWorker) Work(context.Context, *river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]) error {
	return nil
}

func (*schedulerHXCCurrentSyncWorker) Work(context.Context, *river.Job[hxcworker.CurrentSyncJobArgs]) error {
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
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{}, appconfig.WeComCustomerAcquisition{}, appconfig.HXC{})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 3 {
		t.Fatalf("schedulerPlan() jobs = %d, want 3", len(jobs))
	}
}

func TestSchedulerPlanAddsHXCCurrentSyncOnlyWhenEnabled(t *testing.T) {
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
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueSync, &schedulerHXCCurrentSyncWorker{}); err != nil {
		t.Fatal(err)
	}
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{}, appconfig.WeComCustomerAcquisition{}, appconfig.HXC{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 4 {
		t.Fatalf("enabled scheduler jobs=%d, want 4", len(jobs))
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
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{Enabled: true, StaffUserIDs: []string{"staff-1", "staff-2"}}, appconfig.WeComCustomerAcquisition{}, appconfig.HXC{})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 5 {
		t.Fatalf("schedulerPlan() jobs = %d, want 5", len(jobs))
	}
}

func TestSchedulerPlanAddsCH02RecoveryOnlyWhenProviderEnabled(t *testing.T) {
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
	if err := platformjobqueue.AddWorker(workers, platformjobqueue.QueueCritical, &schedulerAcquisitionRecoveryWorker{}); err != nil {
		t.Fatal(err)
	}
	plan, err := schedulerPlan(workers, appconfig.WeComDirectorySync{}, appconfig.WeComCustomerAcquisition{Enabled: true}, appconfig.HXC{})
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 4 {
		t.Fatalf("enabled scheduler jobs=%d, want 4", len(jobs))
	}
}

func TestWhitelistMessageArchivePlanHasOneRegisteredSyncJob(t *testing.T) {
	workers := platformjobqueue.NewWorkerRegistry()
	if err := wecomarchive.RegisterWorker(workers, &wecomarchive.Service{}); err != nil {
		t.Fatal(err)
	}
	plan, err := whitelistMessageArchivePeriodicPlan(workers)
	if err != nil {
		t.Fatal(err)
	}
	if jobs := plan.Jobs(); len(jobs) != 1 {
		t.Fatalf("archive periodic jobs=%d, want 1", len(jobs))
	}
}

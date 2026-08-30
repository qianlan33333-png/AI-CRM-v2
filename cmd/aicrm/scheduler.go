package main

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactworker "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/worker"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	hxcworker "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/worker"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformscheduler "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/scheduler"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomarchive "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/archive"
)

// schedulerPlan is the sole production catalog for periodic jobs. Functional
// slices add definitions here only after their worker and queue are frozen.
func schedulerPlan(workers *platformjobqueue.WorkerRegistry, directorySync appconfig.WeComDirectorySync, customerAcquisition appconfig.WeComCustomerAcquisition, hxc appconfig.HXC) (*platformscheduler.Plan, error) {
	dispatchSchedule, err := platformscheduler.Every(time.Second)
	if err != nil {
		return nil, err
	}
	partitionSchedule, err := platformscheduler.Every(24 * time.Hour)
	if err != nil {
		return nil, err
	}
	segmentRefreshSchedule, err := platformscheduler.Every(time.Minute)
	if err != nil {
		return nil, err
	}
	directorySyncSchedule, err := platformscheduler.Every(wecomapp.ExternalContactSyncPeriod)
	if err != nil {
		return nil, err
	}
	definitions := []platformscheduler.Definition{
		{
			ID:       "events.dispatcher",
			Queue:    platformjobqueue.QueueEvent,
			Schedule: dispatchSchedule,
			Args:     eventdispatcher.DispatchArgs{},
		},
		{
			ID:         "segment.refresh.scheduled",
			Queue:      platformjobqueue.QueueHeavy,
			Schedule:   segmentRefreshSchedule,
			Args:       segmentworker.ScheduledRefreshArgs{},
			RunOnStart: true,
		},
		{
			ID:         "contact.customer_events.partitions",
			Queue:      platformjobqueue.QueueHeavy,
			Schedule:   partitionSchedule,
			Args:       contactworker.EventPartitionMaintenanceArgs{},
			RunOnStart: true,
		},
	}
	if directorySync.Enabled {
		for _, staffUserID := range directorySync.StaffUserIDs {
			definitions = append(definitions, platformscheduler.Definition{
				ID:         directorySyncPeriodicID(staffUserID),
				Queue:      platformjobqueue.QueueSync,
				Schedule:   directorySyncSchedule,
				Args:       wecomapp.ExternalContactSyncJobArgs{StaffUserID: staffUserID},
				RunOnStart: true,
			})
		}
	}
	if customerAcquisition.Enabled {
		recoverySchedule, err := platformscheduler.Every(contactapp.ChannelAcquisitionAssetRecoveryPeriod)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, platformscheduler.Definition{
			ID: "contact.acquisition_asset.attempted_recovery", Queue: platformjobqueue.QueueCritical,
			Schedule: recoverySchedule, Args: contactapp.ChannelAcquisitionAssetRecoveryJobArgs{}, RunOnStart: true,
		})
	}
	if hxc.Enabled {
		hxcSchedule, err := platformscheduler.Every(6 * time.Hour)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, platformscheduler.Definition{
			ID: "hxc.current.sync", Queue: platformjobqueue.QueueSync,
			Schedule: hxcSchedule, Args: hxcworker.CurrentSyncJobArgs{}, RunOnStart: true,
		})
	}
	return platformscheduler.Build(workers, definitions)
}

func whitelistMessageArchivePeriodicPlan(workers *platformjobqueue.WorkerRegistry) (*platformscheduler.Plan, error) {
	schedule, err := platformscheduler.Every(wecomarchive.SyncPeriod)
	if err != nil {
		return nil, err
	}
	plan, err := platformscheduler.Build(workers, []platformscheduler.Definition{{
		ID: "wecom.message_archive.sync", Queue: platformjobqueue.QueueSync, Schedule: schedule, Args: wecomarchive.JobArgs{}, RunOnStart: true,
	}})
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func newWhitelistJobQueueClient(pool *pgxpool.Pool, concurrency platformjobqueue.QueueConcurrency, workers *platformjobqueue.WorkerRegistry, archiveEnabled bool) (*platformjobqueue.Client, error) {
	if !archiveEnabled {
		return platformjobqueue.NewClient(pool, concurrency, workers)
	}
	plan, err := whitelistMessageArchivePeriodicPlan(workers)
	if err != nil {
		return nil, err
	}
	return platformjobqueue.NewClient(pool, concurrency, workers, plan.Jobs()...)
}

func directorySyncPeriodicID(staffUserID string) string {
	digest := sha256.Sum256([]byte(staffUserID))
	return "wecom.external_contact_directory_sync." + hex.EncodeToString(digest[:])
}

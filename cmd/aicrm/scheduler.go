package main

import (
	"time"

	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformscheduler "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/scheduler"
)

// schedulerPlan is the sole production catalog for periodic jobs. Functional
// slices add definitions here only after their worker and queue are frozen.
func schedulerPlan(workers *platformjobqueue.WorkerRegistry) (*platformscheduler.Plan, error) {
	schedule, err := platformscheduler.Every(time.Second)
	if err != nil {
		return nil, err
	}
	return platformscheduler.Build(workers, []platformscheduler.Definition{{
		ID:       "events.dispatcher",
		Queue:    platformjobqueue.QueueEvent,
		Schedule: schedule,
		Args:     eventdispatcher.DispatchArgs{},
	}})
}

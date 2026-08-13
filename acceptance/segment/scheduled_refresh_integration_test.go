package segment_acceptance

import (
	"context"
	"testing"
	"time"

	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformscheduler "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/scheduler"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	segmentworker "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/worker"
)

func TestScheduledRefreshPG16RiverPeriodicHeavyWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := openRefreshPool(t, ctx)
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
ALTER TABLE segments
  ADD COLUMN refresh_mode TEXT NOT NULL DEFAULT 'manual',
  ADD COLUMN refresh_cron TEXT
`); err != nil {
		t.Fatal(err)
	}
	if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("Migrate(up) error = %v", err)
	}

	segmentID := insertRefreshSegment(t, ctx, pool, `{"field":"is_deleted","op":"eq","value":false}`)
	if _, err := pool.Exec(ctx, `UPDATE segments SET refresh_mode='scheduled', refresh_cron='* * * * *' WHERE id=$1`, segmentID); err != nil {
		t.Fatal(err)
	}
	reference := time.Now().UTC().Truncate(time.Minute)
	service := segmentapp.NewRefreshService(
		platformstore.NewUnitOfWork(pool), segmentstore.NewRefreshRepository(), eventstore.NewAppender(),
	)
	finder, err := segmentstore.NewScheduledRefreshRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	refreshWorker, err := segmentworker.NewScheduledRefreshWorker(finder, service, func() time.Time { return reference })
	if err != nil {
		t.Fatal(err)
	}
	registry := platformjobqueue.NewWorkerRegistry()
	if err = platformjobqueue.AddWorker(registry, platformjobqueue.QueueHeavy, refreshWorker); err != nil {
		t.Fatal(err)
	}
	schedule, err := platformscheduler.Every(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := platformscheduler.Build(registry, []platformscheduler.Definition{{
		ID: "segment.refresh.scheduled.acceptance", Queue: platformjobqueue.QueueHeavy,
		Schedule: schedule, Args: segmentworker.ScheduledRefreshArgs{}, RunOnStart: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: 1, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, registry, plan.Jobs()...)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- platformriver.NewRuntime(client).Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("River runtime stop error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("River runtime did not stop")
		}
	})

	deadline := time.Now().Add(8 * time.Second)
	for {
		var events, completedHeavy int
		err = pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM event_log WHERE event_type = 'segment.refreshed'),
  (SELECT count(*) FROM river_job WHERE kind = 'segment_refresh_scheduled' AND queue = 'heavy' AND state = 'completed')
`).Scan(&events, &completedHeavy)
		if err != nil {
			t.Fatal(err)
		}
		if events == 1 && completedHeavy >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("scheduled River chain events/completed-heavy = %d/%d, want 1/>=1", events, completedHeavy)
		}
		time.Sleep(25 * time.Millisecond)
	}
	assertRefreshProjection(t, ctx, pool, segmentID, []int64{1, 2, 3, 4, 5, 6}, 6, reference, "idle")
	assertRefreshEvent(t, ctx, pool, segmentID, 6, reference)
}

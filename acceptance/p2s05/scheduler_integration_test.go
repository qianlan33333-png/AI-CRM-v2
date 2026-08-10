package p2s05_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	jobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformscheduler "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/scheduler"
	queueriver "github.com/riverqueue/river"
)

func TestTwoWorkersEnqueueOneRunOnStartPeriodicJob(t *testing.T) {
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatalf("OpenPostgreSQL() error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})

	firstPool := openWorkerPool(t, ctx, databaseURL)
	if err = platformriver.Migrate(ctx, firstPool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("Migrate(up) error = %v", err)
	}
	secondPool := openWorkerPool(t, ctx, databaseURL)

	var worked atomic.Int32
	workedSignal := make(chan struct{}, 2)
	firstClient := newScheduledClient(t, firstPool, &worked, workedSignal)
	secondClient := newScheduledClient(t, secondPool, &worked, workedSignal)

	workerCtx, stopWorkers := context.WithCancel(ctx)
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() { firstDone <- platformriver.NewRuntime(firstClient).Run(workerCtx) }()
	go func() { secondDone <- platformriver.NewRuntime(secondClient).Run(workerCtx) }()

	select {
	case <-workedSignal:
	case <-time.After(15 * time.Second):
		t.Fatal("periodic RunOnStart job was not processed")
	}
	time.Sleep(500 * time.Millisecond)

	var total, eventJobs, defaultJobs, completed int
	err = firstPool.QueryRow(ctx, `
SELECT
  count(*) FILTER (WHERE kind = 'p2s05_periodic'),
  count(*) FILTER (WHERE kind = 'p2s05_periodic' AND queue = 'event'),
  count(*) FILTER (WHERE kind = 'p2s05_periodic' AND queue = 'default'),
  count(*) FILTER (WHERE kind = 'p2s05_periodic' AND state = 'completed')
FROM river_job`).Scan(&total, &eventJobs, &defaultJobs, &completed)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || eventJobs != 1 || defaultJobs != 0 || completed != 1 || worked.Load() != 1 {
		t.Fatalf("periodic total/event/default/completed/worked = %d/%d/%d/%d/%d, want 1/1/0/1/1",
			total, eventJobs, defaultJobs, completed, worked.Load())
	}

	stopWorkers()
	waitRuntime(t, firstDone)
	waitRuntime(t, secondDone)
}

func openWorkerPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.MaxConns = 9
	poolConfig.ConnConfig.RuntimeParams["search_path"] = acceptancefixtures.Schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newScheduledClient(t *testing.T, pool *pgxpool.Pool, worked *atomic.Int32, signal chan<- struct{}) *jobqueue.Client {
	t.Helper()
	workers := jobqueue.NewWorkerRegistry()
	if err := jobqueue.AddWorker(workers, jobqueue.QueueEvent, &periodicWorker{worked: worked, signal: signal}); err != nil {
		t.Fatal(err)
	}
	plan, err := platformscheduler.Build(workers, []platformscheduler.Definition{{
		ID:         "p2s05.periodic",
		Queue:      jobqueue.QueueEvent,
		Schedule:   platformscheduler.Never(),
		Args:       periodicArgs{},
		RunOnStart: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := jobqueue.NewClient(pool, jobqueue.QueueConcurrency{
		Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, workers, plan.Jobs()...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func waitRuntime(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("worker runtime stop error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker runtime did not stop")
	}
}

type periodicArgs struct{}

func (periodicArgs) Kind() string { return "p2s05_periodic" }

type periodicWorker struct {
	queueriver.WorkerDefaults[periodicArgs]
	worked *atomic.Int32
	signal chan<- struct{}
}

func (worker *periodicWorker) Work(context.Context, *queueriver.Job[periodicArgs]) error {
	worker.worked.Add(1)
	worker.signal <- struct{}{}
	return nil
}

package p2s04_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	jobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	queueriver "github.com/riverqueue/river"
)

func TestSQueueIsolationKeepsCriticalAndOutboundMovingWhileHeavyIsSaturated(t *testing.T) {
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
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("Migrate(up) error = %v", err)
	}

	heavyStarted := make(chan struct{}, 1)
	releaseHeavy := make(chan struct{})
	criticalWorked := make(chan struct{}, 1)
	outboundWorked := make(chan struct{}, 1)
	registry := jobqueue.NewWorkerRegistry()
	if err = jobqueue.AddWorker(registry, jobqueue.QueueHeavy, &heavyWorker{started: heavyStarted, release: releaseHeavy}); err != nil {
		t.Fatal(err)
	}
	if err = jobqueue.AddWorker(registry, jobqueue.QueueCritical, &signalWorker[criticalArgs]{worked: criticalWorked}); err != nil {
		t.Fatal(err)
	}
	if err = jobqueue.AddWorker(registry, jobqueue.QueueOutbound, &signalWorker[outboundArgs]{worked: outboundWorked}); err != nil {
		t.Fatal(err)
	}
	client, err := jobqueue.NewClient(pool, jobqueue.QueueConcurrency{
		Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []queueriver.JobArgs{heavyArgs{ID: 1}, heavyArgs{ID: 2}} {
		if _, err = client.Enqueue(ctx, jobqueue.QueueHeavy, args, nil); err != nil {
			t.Fatalf("enqueue heavy: %v", err)
		}
	}
	if _, err = client.Enqueue(ctx, jobqueue.QueueCritical, criticalArgs{}, nil); err != nil {
		t.Fatalf("enqueue critical: %v", err)
	}
	if _, err = client.Enqueue(ctx, jobqueue.QueueOutbound, outboundArgs{}, nil); err != nil {
		t.Fatalf("enqueue outbound: %v", err)
	}

	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- platformriver.NewRuntime(client).Run(workerCtx) }()
	waitSignal(t, heavyStarted, "heavy worker did not saturate its only slot")
	waitSignal(t, criticalWorked, "critical job was blocked behind heavy")
	waitSignal(t, outboundWorked, "outbound job was blocked behind heavy")

	var runningHeavy, availableHeavy, completedCritical, completedOutbound int
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = pool.QueryRow(ctx, `
SELECT
  count(*) FILTER (WHERE queue = 'heavy' AND state IN ('running', 'available')),
  count(*) FILTER (WHERE queue = 'heavy' AND state = 'available'),
  count(*) FILTER (WHERE queue = 'critical' AND state = 'completed'),
  count(*) FILTER (WHERE queue = 'outbound' AND state = 'completed')
FROM river_job`).Scan(&runningHeavy, &availableHeavy, &completedCritical, &completedOutbound)
		if err != nil {
			t.Fatal(err)
		}
		if runningHeavy == 2 && availableHeavy == 1 && completedCritical == 1 && completedOutbound == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("heavy/available/critical/outbound = %d/%d/%d/%d, want 2/1/1/1", runningHeavy, availableHeavy, completedCritical, completedOutbound)
		}
		time.Sleep(20 * time.Millisecond)
	}

	close(releaseHeavy)
	stopWorker()
	select {
	case runErr := <-workerDone:
		if runErr != nil {
			t.Fatalf("worker runtime stop error = %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker runtime did not stop")
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
	}
}

type heavyArgs struct {
	ID int `json:"id"`
}

func (heavyArgs) Kind() string { return "p2s04_heavy" }

type heavyWorker struct {
	queueriver.WorkerDefaults[heavyArgs]
	started chan<- struct{}
	release <-chan struct{}
}

func (worker *heavyWorker) Work(ctx context.Context, _ *queueriver.Job[heavyArgs]) error {
	select {
	case worker.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-worker.release:
		return nil
	}
}

type criticalArgs struct{}

func (criticalArgs) Kind() string { return "p2s04_critical" }

type outboundArgs struct{}

func (outboundArgs) Kind() string { return "p2s04_outbound" }

type signalWorker[T queueriver.JobArgs] struct {
	queueriver.WorkerDefaults[T]
	worked chan<- struct{}
}

func (worker *signalWorker[T]) Work(context.Context, *queueriver.Job[T]) error {
	worker.worked <- struct{}{}
	return nil
}

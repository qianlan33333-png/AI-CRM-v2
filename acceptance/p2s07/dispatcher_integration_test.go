package p2s07_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const crashLockKey int64 = 0x5032533037435248

func TestSIGKILLBetweenEnqueueAndCommitRecoversWithoutLossOrDuplicate(t *testing.T) {
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
	pool := openPool(t, ctx, databaseURL, "p2s07-parent")
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("Migrate(up) error = %v", err)
	}
	createFixtures(t, ctx, pool)

	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lockConn.Release()
	if _, err = lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, crashLockKey); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, crashLockKey) }()

	command := exec.Command(os.Args[0], "-test.run=^TestDispatcherCrashHelper$", "-test.v")
	command.Env = append(os.Environ(),
		"P2S07_CRASH_HELPER=1",
		"P2S07_DATABASE_URL="+databaseURL,
	)
	var helperOutput bytes.Buffer
	command.Stdout = &helperOutput
	command.Stderr = &helperOutput
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	waitForAdvisoryBlock(t, ctx, pool)
	if err = command.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	if waitErr := command.Wait(); waitErr == nil {
		t.Fatalf("helper exited without SIGKILL: %s", helperOutput.String())
	}
	waitForHelperExit(t, ctx, pool)
	if _, err = lockConn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, crashLockKey); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DROP TRIGGER block_dispatch_update ON event_log; DROP FUNCTION block_dispatch_update()`); err != nil {
		t.Fatal(err)
	}

	assertCounts(t, ctx, pool, false, 0, 0, 0)
	client, dispatcher := newHarness(t, pool, &auditSubscriber{pool: pool})
	dispatched, err := dispatcher.Dispatch(ctx)
	if err != nil || dispatched != 1 {
		t.Fatalf("restart Dispatch() count/error = %d/%v, want 1/nil", dispatched, err)
	}
	if dispatched, err = dispatcher.Dispatch(ctx); err != nil || dispatched != 0 {
		t.Fatalf("second Dispatch() count/error = %d/%v, want 0/nil", dispatched, err)
	}

	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- platformriver.NewRuntime(client).Run(workerCtx) }()
	waitForDelivery(t, ctx, pool)
	stopWorker()
	waitRuntime(t, workerDone)
	assertCounts(t, ctx, pool, true, 1, 1, 1)
}

func TestDispatcherCrashHelper(t *testing.T) {
	if os.Getenv("P2S07_CRASH_HELPER") != "1" {
		return
	}
	databaseURL := os.Getenv("P2S07_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openPool(t, ctx, databaseURL, "p2s07-crash-helper")
	_, dispatcher := newHarness(t, pool)
	if _, err := dispatcher.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	t.Fatal("crash helper passed the blocked commit boundary")
}

func openPool(t *testing.T, ctx context.Context, databaseURL, applicationName string) *pgxpool.Pool {
	t.Helper()
	if err := acceptancefixtures.ValidateDatabaseURL(databaseURL); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 9
	config.ConnConfig.RuntimeParams["search_path"] = acceptancefixtures.Schema
	config.ConnConfig.RuntimeParams["application_name"] = applicationName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func createFixtures(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
CREATE TABLE event_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type text NOT NULL,
  customer_id bigint,
  payload jsonb NOT NULL DEFAULT '{}',
  occurred_at timestamptz NOT NULL DEFAULT now(),
  idempotency_key text NOT NULL UNIQUE,
  dispatched boolean NOT NULL DEFAULT false
);
CREATE INDEX idx_el_undispatched ON event_log (id) WHERE NOT dispatched;
CREATE TABLE subscriber_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id bigint NOT NULL
);
INSERT INTO event_log (event_type, customer_id, payload, idempotency_key)
VALUES ('test.changed', 42, '{"value":"kept"}', 'test.changed:42');
CREATE FUNCTION block_dispatch_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM pg_advisory_xact_lock(`+"5778772738420462152"+`);
  RETURN NEW;
END
$$;
CREATE TRIGGER block_dispatch_update
BEFORE UPDATE OF dispatched ON event_log
FOR EACH ROW EXECUTE FUNCTION block_dispatch_update();`)
	if err != nil {
		t.Fatal(err)
	}
}

type clientReference struct {
	client *platformjobqueue.Client
}

func (reference *clientReference) EnqueueTx(ctx context.Context, tx pgx.Tx, queue platformjobqueue.Queue, args river.JobArgs, options *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if reference == nil || reference.client == nil {
		return nil, platformjobqueue.ErrClientUnavailable
	}
	return reference.client.EnqueueTx(ctx, tx, queue, args, options)
}

func newHarness(t *testing.T, pool *pgxpool.Pool, subscribers ...eventport.Subscriber) (*platformjobqueue.Client, *eventdispatcher.Dispatcher) {
	t.Helper()
	router, err := eventdispatcher.NewRouter(subscribers...)
	if err != nil {
		t.Fatal(err)
	}
	deliveryWorker, err := eventdispatcher.NewDeliveryWorker(pool, router)
	if err != nil {
		t.Fatal(err)
	}
	workers := platformjobqueue.NewWorkerRegistry()
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, deliveryWorker); err != nil {
		t.Fatal(err)
	}
	reference := &clientReference{}
	dispatcher, err := eventdispatcher.New(platformstore.NewUnitOfWork(pool), reference, eventdispatcher.DefaultBatchSize)
	if err != nil {
		t.Fatal(err)
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: 2, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, workers)
	if err != nil {
		t.Fatal(err)
	}
	reference.client = client
	return client, dispatcher
}

type auditSubscriber struct {
	pool *pgxpool.Pool
}

func (*auditSubscriber) EventTypes() []string { return []string{"test.changed"} }

func (subscriber *auditSubscriber) Consume(ctx context.Context, event eventport.Record) error {
	_, err := subscriber.pool.Exec(ctx, `INSERT INTO subscriber_audit (event_id) VALUES ($1)`, event.ID)
	return err
}

func waitForAdvisoryBlock(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	waitFor(t, ctx, func() (bool, error) {
		var blocked bool
		err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_stat_activity
  WHERE application_name = 'p2s07-crash-helper'
    AND wait_event_type = 'Lock'
    AND wait_event = 'advisory'
)`).Scan(&blocked)
		return blocked, err
	}, "crash helper never reached the enqueue-before-commit boundary")
}

func waitForHelperExit(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	waitFor(t, ctx, func() (bool, error) {
		var active bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = 'p2s07-crash-helper')`).Scan(&active)
		return !active, err
	}, "crash helper database session did not close")
}

func waitForDelivery(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	waitFor(t, ctx, func() (bool, error) {
		var audits, completed int
		err := pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM subscriber_audit),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver' AND state = 'completed')`).Scan(&audits, &completed)
		return audits == 1 && completed == 1, err
	}, "delivery job did not complete exactly once")
}

func waitFor(t *testing.T, ctx context.Context, condition func() (bool, error), failure string) {
	t.Helper()
	for {
		ok, err := condition()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(failure)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func waitRuntime(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("worker runtime stop error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("worker runtime did not stop")
	}
}

func assertCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantDispatched bool, wantJobs, wantCompleted, wantAudits int) {
	t.Helper()
	var dispatched bool
	var jobs, completed, audits, defaultJobs int
	err := pool.QueryRow(ctx, `
SELECT
  (SELECT dispatched FROM event_log WHERE id = 1),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver'),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver' AND state = 'completed'),
  (SELECT count(*) FROM subscriber_audit),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver' AND queue = 'default')`).Scan(
		&dispatched, &jobs, &completed, &audits, &defaultJobs)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched != wantDispatched || jobs != wantJobs || completed != wantCompleted || audits != wantAudits || defaultJobs != 0 {
		t.Fatalf("dispatched/jobs/completed/audits/default = %v/%d/%d/%d/%d, want %v/%d/%d/%d/0",
			dispatched, jobs, completed, audits, defaultJobs, wantDispatched, wantJobs, wantCompleted, wantAudits)
	}
}

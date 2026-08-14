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
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

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

	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyReader.Close()

	command := exec.Command(os.Args[0], "-test.run=^TestDispatcherCrashHelper$", "-test.v")
	command.ExtraFiles = []*os.File{readyWriter}
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
	if err = readyWriter.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	waitForCrashBoundary(t, ctx, readyReader)
	if err = command.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	if waitErr := command.Wait(); waitErr == nil {
		t.Fatalf("helper exited without SIGKILL: %s", helperOutput.String())
	}
	waitForHelperExit(t, ctx, pool)

	assertCounts(t, ctx, pool, false, 0, 0, 0)
	client, dispatcher := newHarness(t, pool, nil, &auditSubscriber{pool: pool})
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

func TestConcurrentDispatchersClaimDisjointBatches(t *testing.T) {
	databaseURL := os.Getenv("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ACCEPTANCE_FIXTURES_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture, err := acceptancefixtures.OpenPostgreSQL(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := fixture.Cleanup(cleanupCtx); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	})
	pool := openPool(t, ctx, databaseURL, "p2s07-concurrent")
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatal(err)
	}
	createFixtures(t, ctx, pool)
	if _, err = pool.Exec(ctx, `
INSERT INTO acceptance_fixtures.event_log (event_type, payload, idempotency_key)
SELECT 'test.changed', jsonb_build_object('sequence', value), 'test.concurrent:' || value
FROM generate_series(2, 200) AS value`); err != nil {
		t.Fatal(err)
	}
	_, dispatcher := newHarness(t, pool, nil)
	type result struct {
		count int
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			count, dispatchErr := dispatcher.Dispatch(ctx)
			results <- result{count: count, err: dispatchErr}
		}()
	}
	total := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		total += result.count
	}
	var dispatched, jobs, distinctArgs, defaultJobs int
	if err = pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM event_log WHERE dispatched),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver'),
  (SELECT count(DISTINCT args) FROM river_job WHERE kind = 'events_deliver'),
  (SELECT count(*) FROM river_job WHERE kind = 'events_deliver' AND queue = 'default')`).Scan(
		&dispatched, &jobs, &distinctArgs, &defaultJobs); err != nil {
		t.Fatal(err)
	}
	if total != 200 || dispatched != 200 || jobs != 200 || distinctArgs != 200 || defaultJobs != 0 {
		t.Fatalf("claimed/dispatched/jobs/distinct/default = %d/%d/%d/%d/%d, want 200/200/200/200/0",
			total, dispatched, jobs, distinctArgs, defaultJobs)
	}
}

func TestDispatcherCrashHelper(t *testing.T) {
	if os.Getenv("P2S07_CRASH_HELPER") != "1" {
		return
	}
	databaseURL := os.Getenv("P2S07_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openPool(t, ctx, databaseURL, "p2s07-crash-helper")
	ready := os.NewFile(3, "p2s07-crash-ready")
	if ready == nil {
		t.Fatal("crash helper readiness pipe is missing")
	}
	defer ready.Close()
	_, dispatcher := newHarness(t, pool, func() {
		if _, err := ready.Write([]byte{1}); err != nil {
			panic(err)
		}
		select {}
	})
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
CREATE TABLE acceptance_fixtures.event_log (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_type text NOT NULL,
  customer_id bigint,
  payload jsonb NOT NULL DEFAULT '{}',
  occurred_at timestamptz NOT NULL DEFAULT now(),
  idempotency_key text NOT NULL UNIQUE,
  dispatched boolean NOT NULL DEFAULT false
);
CREATE INDEX idx_el_undispatched ON event_log (id) WHERE NOT dispatched;
CREATE TABLE acceptance_fixtures.subscriber_audit (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id bigint NOT NULL
);
INSERT INTO acceptance_fixtures.event_log (event_type, customer_id, payload, idempotency_key)
VALUES ('test.changed', 42, '{"value":"kept"}', 'test.changed:42');`)
	if err != nil {
		t.Fatal(err)
	}
}

type clientReference struct {
	client       *platformjobqueue.Client
	afterEnqueue func()
}

func (reference *clientReference) EnqueueTx(ctx context.Context, tx pgx.Tx, queue platformjobqueue.Queue, args river.JobArgs, options *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if reference == nil || reference.client == nil {
		return nil, platformjobqueue.ErrClientUnavailable
	}
	result, err := reference.client.EnqueueTx(ctx, tx, queue, args, options)
	if err == nil && reference.afterEnqueue != nil {
		reference.afterEnqueue()
	}
	return result, err
}

func newHarness(t *testing.T, pool *pgxpool.Pool, afterEnqueue func(), subscribers ...eventport.Subscriber) (*platformjobqueue.Client, *eventdispatcher.Dispatcher) {
	t.Helper()
	router, err := eventdispatcher.NewRouter(subscribers...)
	if err != nil {
		t.Fatal(err)
	}
	reference := &clientReference{afterEnqueue: afterEnqueue}
	deliveries, err := eventstore.NewRuntimeDeliveryRepository(pool, reference, eventdispatcher.DefaultBatchSize, nil)
	if err != nil {
		t.Fatal(err)
	}
	deliveryWorker, err := eventdispatcher.NewDeliveryWorker(deliveries, router)
	if err != nil {
		t.Fatal(err)
	}
	workers := platformjobqueue.NewWorkerRegistry()
	if err = platformjobqueue.AddWorker(workers, platformjobqueue.QueueEvent, deliveryWorker); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := eventdispatcher.New(deliveries)
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
	_, err := subscriber.pool.Exec(ctx, `INSERT INTO acceptance_fixtures.subscriber_audit (event_id) VALUES ($1)`, event.ID)
	return err
}

func waitForCrashBoundary(t *testing.T, ctx context.Context, ready *os.File) {
	t.Helper()
	result := make(chan error, 1)
	go func() {
		var signal [1]byte
		_, err := ready.Read(signal[:])
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("read crash boundary signal: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("crash helper never reached the enqueue-before-commit boundary")
	}
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

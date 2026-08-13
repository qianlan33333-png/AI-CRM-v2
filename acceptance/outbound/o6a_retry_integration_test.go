package outbound_acceptance

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundworker "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/worker"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

var outboundO6ARealRiver = flag.Bool("o6a-real-river", false, "run isolated O6A real River retry acceptance")

func TestOutboundRetryableFailureIsRetriedByRealRiver(t *testing.T) {
	if !*outboundO6ARealRiver {
		t.Skip("o6a-real-river is not enabled")
	}
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6ARealRiverFixture(t, ctx, pool)

	provider := &retryThenSuccessProvider{}
	sender := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
	)
	workers := platformjobqueue.NewWorkerRegistry()
	if err := outboundworker.RegisterSenderWorkers(workers, sender); err != nil {
		t.Fatal(err)
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: 1, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, workers)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = client.Stop(stopCtx)
	})

	enqueued := enqueueOneFixture(
		t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6a-real-river-retry", "retry_then_success",
	)
	waitForRetriedTask(t, ctx, pool, enqueued.TaskID, enqueued.RiverJobID, provider)
	assertRetryHistoryAndStableAcceptance(t, ctx, pool, enqueued.TaskID, enqueued.RiverJobID)
}

func TestOutboundOutcomeUnknownIsNotRetriedByRealRiver(t *testing.T) {
	if !*outboundO6ARealRiver {
		t.Skip("o6a-real-river is not enabled")
	}
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6ARealRiverFixture(t, ctx, pool)

	provider := &fixtureProvider{}
	sender := outboundapp.NewSenderService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
	)
	workers := platformjobqueue.NewWorkerRegistry()
	if err := outboundworker.RegisterSenderWorkers(workers, sender); err != nil {
		t.Fatal(err)
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: 1, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, workers)
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = client.Stop(stopCtx)
	})

	enqueued := enqueueOneFixture(
		t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6a-real-river-unknown", "timeout",
	)
	waitForTerminalAttempt(t, ctx, pool, enqueued.TaskID, enqueued.RiverJobID, outboundapp.TaskStatusOutcomeUnknown, 1, "completed", 1)
	timer := time.NewTimer(750 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	waitForTerminalAttempt(t, ctx, pool, enqueued.TaskID, enqueued.RiverJobID, outboundapp.TaskStatusOutcomeUnknown, 1, "completed", 1)
	if calls := provider.Calls(enqueued.TaskID); calls != 1 {
		t.Fatalf("outcome_unknown provider calls=%d, want exactly one and no automatic retry", calls)
	}
	var markers, history, tasks, acceptedEvents, resultEvents int
	if err = pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM outbound_send_attempts WHERE river_job_id=$2),
  (SELECT count(*) FROM outbound_send_attempt_history AS history
     JOIN outbound_send_attempts AS marker ON marker.id=history.send_attempt_id
    WHERE marker.river_job_id=$2),
  (SELECT count(*) FROM outbound_tasks WHERE id=$1),
  (SELECT count(*) FROM event_log WHERE idempotency_key='outbound.accepted:' || $1::text),
  (SELECT count(*) FROM event_log WHERE idempotency_key='outbound.send-result:' ||
    (SELECT id::text FROM outbound_send_attempts WHERE river_job_id=$2))`, enqueued.TaskID, enqueued.RiverJobID).Scan(
		&markers, &history, &tasks, &acceptedEvents, &resultEvents,
	); err != nil || markers != 1 || history != 1 || tasks != 1 || acceptedEvents != 1 || resultEvents != 1 {
		t.Fatalf("outcome_unknown marker/history/task/accepted/result=%d/%d/%d/%d/%d err=%v, want 1/1/1/1/1",
			markers, history, tasks, acceptedEvents, resultEvents, err)
	}
}

func resetO6ARealRiverFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `
SELECT id
FROM river_job
WHERE queue = 'outbound'
	  AND kind IN ($1, $2)`, outboundapp.OutboundEnqueueOneJobKind, outboundapp.OutboundEnqueueBatchJobKind)
	if err != nil {
		t.Fatalf("list O6A real River jobs: %v", err)
	}
	var jobIDs []int64
	for rows.Next() {
		var jobID int64
		if err = rows.Scan(&jobID); err != nil {
			rows.Close()
			t.Fatalf("scan O6A real River job: %v", err)
		}
		jobIDs = append(jobIDs, jobID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("list O6A real River jobs: %v", err)
	}
	rows.Close()

	cleanupClient, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{SkipUnknownJobCheck: true})
	if err != nil {
		t.Fatalf("create O6A River fixture client: %v", err)
	}
	for _, jobID := range jobIDs {
		if _, err = cleanupClient.JobDelete(ctx, jobID); err != nil {
			t.Fatalf("delete O6A River fixture job %d: %v", jobID, err)
		}
	}
	resetOutboundFixture(t, pool)
}

func waitForRetriedTask(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	taskID outboundapp.TaskID,
	jobID int64,
	provider *retryThenSuccessProvider,
) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, attemptCount, jobState, riverAttempt, err := loadTaskAndJob(ctx, pool, taskID, jobID)
		if err == nil && status == string(outboundapp.TaskStatusSent) && attemptCount == 2 && jobState == "completed" && riverAttempt == 2 {
			if calls := provider.Calls(taskID); calls != 2 {
				t.Fatalf("provider calls=%d, want two real River attempts", calls)
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("real River retry did not close task/job: status=%q attempts=%d job=%q/%d provider_calls=%d query_err=%v context_err=%v",
				status, attemptCount, jobState, riverAttempt, provider.Calls(taskID), err, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForTerminalAttempt(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	taskID outboundapp.TaskID,
	jobID int64,
	wantStatus outboundapp.TaskStatus,
	wantAttemptCount int,
	wantJobState string,
	wantRiverAttempt int,
) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, attemptCount, jobState, riverAttempt, err := loadTaskAndJob(ctx, pool, taskID, jobID)
		if err == nil && status == string(wantStatus) && attemptCount == wantAttemptCount && jobState == wantJobState && riverAttempt == wantRiverAttempt {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("task/job did not become %s/%d/%s/%d: got %q/%d/%q/%d query_err=%v context_err=%v",
				wantStatus, wantAttemptCount, wantJobState, wantRiverAttempt,
				status, attemptCount, jobState, riverAttempt, err, ctx.Err())
		case <-ticker.C:
		}
	}
}

func loadTaskAndJob(
	ctx context.Context,
	pool *pgxpool.Pool,
	taskID outboundapp.TaskID,
	jobID int64,
) (string, int, string, int, error) {
	var status, jobState string
	var attemptCount, riverAttempt int
	err := pool.QueryRow(ctx, `
SELECT task.status, task.attempt_count, job.state, job.attempt
FROM outbound_tasks AS task
JOIN river_job AS job ON job.id = $2
WHERE task.id = $1`, taskID, jobID).Scan(&status, &attemptCount, &jobState, &riverAttempt)
	return status, attemptCount, jobState, riverAttempt, err
}

func assertRetryHistoryAndStableAcceptance(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	taskID outboundapp.TaskID,
	jobID int64,
) {
	t.Helper()
	var markerID int64
	var markers, tasks, acceptedEvents, resultEvents int
	if err := pool.QueryRow(ctx, `
SELECT marker.id,
  (SELECT count(*) FROM outbound_send_attempts WHERE river_job_id=$2),
  (SELECT count(*) FROM outbound_tasks WHERE id=$1),
  (SELECT count(*) FROM event_log WHERE idempotency_key='outbound.accepted:' || $1::text),
  (SELECT count(*) FROM event_log WHERE idempotency_key LIKE 'outbound.send-result:' || marker.id::text || '%')
FROM outbound_send_attempts AS marker
WHERE marker.river_job_id=$2`, taskID, jobID).Scan(&markerID, &markers, &tasks, &acceptedEvents, &resultEvents); err != nil ||
		markers != 1 || tasks != 1 || acceptedEvents != 1 || resultEvents != 2 {
		t.Fatalf("retry marker/id/count task/accepted/result=%d/%d/%d/%d/%d err=%v, want positive/1/1/1/2",
			markerID, markers, tasks, acceptedEvents, resultEvents, err)
	}
	type historyFact struct {
		riverAttempt, riverMaxAttempts                      int
		state, failureKind, providerCode, providerMessageID *string
	}
	rows, err := pool.Query(ctx, `
SELECT river_attempt, river_max_attempts, state, failure_kind, provider_code, provider_message_id
FROM outbound_send_attempt_history
WHERE send_attempt_id=$1
ORDER BY river_attempt`, markerID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var facts []historyFact
	for rows.Next() {
		var fact historyFact
		if err = rows.Scan(&fact.riverAttempt, &fact.riverMaxAttempts, &fact.state, &fact.failureKind, &fact.providerCode, &fact.providerMessageID); err != nil {
			t.Fatal(err)
		}
		facts = append(facts, fact)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(facts) != 2 || facts[0].riverAttempt != 1 || facts[1].riverAttempt != 2 ||
		facts[0].riverMaxAttempts != facts[1].riverMaxAttempts || facts[0].riverMaxAttempts < 2 ||
		nullableString(facts[0].state) != string(outboundapp.SendAttemptRetryableFailed) ||
		nullableString(facts[0].failureKind) != string(outboundapp.ProviderFailureRateLimited) ||
		nullableString(facts[0].providerCode) != "fixture-429" || facts[0].providerMessageID != nil ||
		nullableString(facts[1].state) != string(outboundapp.SendAttemptSucceeded) || facts[1].failureKind != nil ||
		facts[1].providerCode != nil || nullableString(facts[1].providerMessageID) == "" {
		t.Fatalf("retry history=%+v, want attempt 1 retryable_failed then attempt 2 succeeded with one max-attempt contract", facts)
	}
}

type retryThenSuccessProvider struct {
	mu    sync.Mutex
	calls map[outboundapp.TaskID]int
}

func (provider *retryThenSuccessProvider) Send(_ context.Context, request outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls == nil {
		provider.calls = make(map[outboundapp.TaskID]int)
	}
	provider.calls[request.TaskID]++
	if provider.calls[request.TaskID] == 1 {
		return outboundapp.ProviderResult{FailureKind: outboundapp.ProviderFailureRateLimited, Code: "fixture-429"}, nil
	}
	return outboundapp.ProviderResult{MessageID: fmt.Sprintf("fixture-retry-message-%d", request.TaskID)}, nil
}

func (provider *retryThenSuccessProvider) Calls(taskID outboundapp.TaskID) int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls[taskID]
}

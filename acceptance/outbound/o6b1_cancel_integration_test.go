package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	outboundworker "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/worker"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestCancelPendingTaskCommitsJobReceiptEventAndStableReplay(t *testing.T) {
	pool := openOutboundPool(t)
	secondPool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b1-cancel-one", "success")
	command := outboundapp.CancelCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-0000001"}
	first := newCancelService(t, pool, eventstore.NewAppender())
	second := newCancelService(t, secondPool, eventstore.NewAppender())

	type cancelResult struct {
		cancelled outboundapp.CancelledTask
		err       error
	}
	start := make(chan struct{})
	results := make(chan cancelResult, 2)
	var callers sync.WaitGroup
	for _, service := range []*outboundapp.CancelService{first, second} {
		callers.Add(1)
		go func(service *outboundapp.CancelService) {
			defer callers.Done()
			<-start
			cancelled, err := service.Cancel(ctx, command)
			results <- cancelResult{cancelled, err}
		}(service)
	}
	close(start)
	callers.Wait()
	close(results)
	var original outboundapp.CancelledTask
	for result := range results {
		if result.err != nil || result.cancelled.ReceiptID <= 0 || result.cancelled.EventID <= 0 ||
			result.cancelled.TaskID != enqueued.TaskID || result.cancelled.Status != outboundapp.TaskStatusCancelled ||
			result.cancelled.Job.RiverJobID != enqueued.RiverJobID || result.cancelled.Job.Generation != 1 {
			t.Fatalf("Cancel()=%+v err=%v", result.cancelled, result.err)
		}
		if original.ReceiptID == 0 {
			original = result.cancelled
		} else if result.cancelled != original {
			t.Fatalf("cancel replay=%+v, want original %+v", result.cancelled, original)
		}
	}

	assertCancelFacts(t, ctx, pool, original, 1, 1, 1, 0)
	replayed, err := first.Cancel(ctx, command)
	if err != nil || replayed != original {
		t.Fatalf("lost-response replay=%+v err=%v, want original %+v", replayed, err, original)
	}
	assertCancelFacts(t, ctx, pool, original, 1, 1, 1, 0)

	conflicting := command
	conflicting.TaskID++
	if _, err = second.Cancel(ctx, conflicting); !errors.Is(err, outboundapp.ErrCancelCommandConflict) {
		t.Fatalf("payload conflict error=%v", err)
	}
}

func TestCancelPendingBatchTaskUsesTypedRiverCompatibilityFallback(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	batch, err := newBatchService(t, pool).Enqueue(ctx, outboundapp.EnqueueBatchCommand{
		IdempotencyScope: "operator:7", IdempotencyKey: "outbound-o6b1-batch-fallback",
		Tier: outboundapp.BatchTierS, CustomerIDs: createOutboundCustomers(t, ctx, pool, 2),
		TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"cancel one batch task"}`),
	})
	if err != nil || batch.TaskCount != 2 {
		t.Fatalf("Enqueue batch=%+v err=%v", batch, err)
	}
	var taskID, jobID int64
	if err = pool.QueryRow(ctx, `
SELECT task.id, link.river_job_id
FROM outbound_tasks AS task
JOIN outbound_task_job_links AS link ON link.task_id=task.id AND link.generation=1
WHERE task.batch_id=$1 ORDER BY task.id LIMIT 1`, batch.BatchID).Scan(&taskID, &jobID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM outbound_task_job_links WHERE task_id=$1`, taskID); err != nil {
		t.Fatal(err)
	}
	command := outboundapp.CancelCommand{TaskID: outboundapp.TaskID(taskID), IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-0000002"}
	got, err := newCancelService(t, pool, eventstore.NewAppender()).Cancel(ctx, command)
	if err != nil || got.Job.RiverJobID != jobID || got.Job.JobKind != outboundapp.OutboundEnqueueBatchJobKind {
		t.Fatalf("fallback Cancel()=%+v err=%v", got, err)
	}
	assertCancelFacts(t, ctx, pool, got, 1, 1, 1, 0)
}

func TestCancelRejectsWorkerWinnerAndIllegalTaskStates(t *testing.T) {
	for _, test := range []struct {
		name   string
		status outboundapp.TaskStatus
	}{
		{"sending", outboundapp.TaskStatusSending},
		{"sent", outboundapp.TaskStatusSent},
		{"retryable_failed", outboundapp.TaskStatusRetryableFailed},
		{"final_failed", outboundapp.TaskStatusFinalFailed},
		{"outcome_unknown", outboundapp.TaskStatusOutcomeUnknown},
		{"cancelled", outboundapp.TaskStatusCancelled},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := openOutboundPool(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ensureOutboundRiverCatalog(t, ctx, pool)
			resetO6B1CancelFixture(t, ctx, pool)
			enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b1-state-"+test.name, "success")
			prepareCancelTaskStatus(t, ctx, pool, enqueued, test.status)
			command := outboundapp.CancelCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-state-" + test.name}
			_, err := newCancelService(t, pool, eventstore.NewAppender()).Cancel(ctx, command)
			if !errors.Is(err, outboundapp.ErrCancelTransitionConflict) {
				t.Fatalf("Cancel(%s) error=%v", test.status, err)
			}
			assertNoCompletedCancel(t, ctx, pool, command.IdempotencyKey)
		})
	}
}

func TestCancelAndWorkerProviderBoundaryBothWinnerOrders(t *testing.T) {
	t.Run("cancel wins before provider", func(t *testing.T) {
		pool := openOutboundPool(t)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		ensureOutboundRiverCatalog(t, ctx, pool)
		resetO6B1CancelFixture(t, ctx, pool)

		enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b1-cancel-before-provider", "success")
		command := outboundapp.CancelCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-0000005"}
		if _, err := newCancelService(t, pool, eventstore.NewAppender()).Cancel(ctx, command); err != nil {
			t.Fatal(err)
		}
		provider := &blockingCancelProvider{entered: make(chan struct{}), release: make(chan struct{})}
		sender := outboundapp.NewSenderService(
			platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
		)
		startCancelRaceClient(t, pool, sender)
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-provider.entered:
			t.Fatal("provider was called after cancellation deleted the River job")
		case <-timer.C:
		}
		if provider.Calls() != 0 {
			t.Fatalf("provider calls=%d after cancel, want zero", provider.Calls())
		}
	})

	t.Run("dispatch wins before cancel", func(t *testing.T) {
		pool := openOutboundPool(t)
		secondPool := openOutboundPool(t)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		ensureOutboundRiverCatalog(t, ctx, pool)
		resetO6B1CancelFixture(t, ctx, pool)

		enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b1-dispatch-before-cancel", "success")
		provider := &blockingCancelProvider{entered: make(chan struct{}), release: make(chan struct{})}
		sender := outboundapp.NewSenderService(
			platformstore.NewUnitOfWork(pool), outboundstore.NewSenderRepository(), eventstore.NewAppender(), provider, fixtureRateGate{},
		)
		startCancelRaceClient(t, pool, sender)
		select {
		case <-provider.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		command := outboundapp.CancelCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-0000006"}
		_, cancelErr := newCancelService(t, secondPool, eventstore.NewAppender()).Cancel(ctx, command)
		if !errors.Is(cancelErr, outboundapp.ErrCancelTransitionConflict) {
			t.Fatalf("cancel after dispatch error=%v", cancelErr)
		}
		close(provider.release)
		waitForTerminalAttempt(t, ctx, pool, enqueued.TaskID, enqueued.RiverJobID, outboundapp.TaskStatusSent, 1, "completed", 1)
		if provider.Calls() != 1 {
			t.Fatalf("dispatch winner provider calls=%d, want 1", provider.Calls())
		}
		assertNoCompletedCancel(t, ctx, pool, command.IdempotencyKey)
	})
}

func startCancelRaceClient(t *testing.T, pool *pgxpool.Pool, sender *outboundapp.SenderService) *platformjobqueue.Client {
	t.Helper()
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
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.Stop(stopCtx)
	})
	return client
}

func TestCancelEventFailureRollsBackJobReceiptAndTask(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b1-rollback", "success")
	command := outboundapp.CancelCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-cancel-0000004"}
	_, err := newCancelService(t, pool, failingCancelAppender{}).Cancel(ctx, command)
	if !errors.Is(err, errCancelEventRollback) {
		t.Fatalf("Cancel rollback error=%v", err)
	}
	var taskStatus, riverState string
	var receipts, cancelledEvents int
	if err = pool.QueryRow(ctx, `
SELECT task.status, job.state,
  (SELECT count(*) FROM outbound_control_receipts WHERE idempotency_key=$3),
  (SELECT count(*) FROM event_log WHERE event_type='outbound.cancelled' AND payload->>'task_id'=($1::bigint)::text)
FROM outbound_tasks AS task
JOIN river_job AS job ON job.id=$2
WHERE task.id=$1`, enqueued.TaskID, enqueued.RiverJobID, command.IdempotencyKey).Scan(&taskStatus, &riverState, &receipts, &cancelledEvents); err != nil || taskStatus != "pending" || riverState != "available" || receipts != 0 || cancelledEvents != 0 {
		t.Fatalf("rollback task/job/receipt/event=%s/%s/%d/%d err=%v", taskStatus, riverState, receipts, cancelledEvents, err)
	}
}

func newCancelService(t *testing.T, pool *pgxpool.Pool, events eventport.Appender) *outboundapp.CancelService {
	t.Helper()
	repository, err := outboundstore.NewControlRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	return outboundapp.NewCancelService(platformstore.NewUnitOfWork(pool), repository, events)
}

func resetO6B1CancelFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	client, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{SkipUnknownJobCheck: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.JobList(ctx, queueriver.NewJobListParams().First(10000).Queues("outbound").Kinds(
		outboundapp.OutboundEnqueueOneJobKind, outboundapp.OutboundEnqueueBatchJobKind,
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range result.Jobs {
		if _, err = client.JobDelete(ctx, job.ID); err != nil && !errors.Is(err, queueriver.ErrNotFound) {
			t.Fatalf("delete fixture job %d: %v", job.ID, err)
		}
	}
	resetOutboundFixture(t, pool)
}

func prepareCancelTaskStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, enqueued outboundapp.EnqueuedTask, status outboundapp.TaskStatus) {
	t.Helper()
	if status == outboundapp.TaskStatusPending {
		return
	}
	var currentAttemptID any
	attemptCount := 0
	var failureKind, lastError, providerMessage any
	var sentAt any
	if status != outboundapp.TaskStatusCancelled {
		var attemptID int64
		if err := pool.QueryRow(ctx, `
INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind, state, dispatch_started_at, completed_at, failure_kind, provider_code, provider_message_id)
VALUES ($1,$2,$3,$4,now(),CASE WHEN $4='dispatching' THEN NULL ELSE now() END,
  CASE WHEN $4 IN ('retryable_failed','final_failed','outcome_unknown') THEN 'temporary' ELSE NULL END,
  CASE WHEN $4 IN ('retryable_failed','final_failed','outcome_unknown') THEN 'fixture-state' ELSE NULL END,
  CASE WHEN $4='succeeded' THEN 'fixture-message' ELSE NULL END)
RETURNING id`, enqueued.RiverJobID, enqueued.TaskID, outboundapp.OutboundEnqueueOneJobKind, attemptStateForTask(status)).Scan(&attemptID); err != nil {
			t.Fatal(err)
		}
		currentAttemptID = attemptID
		attemptCount = 1
	}
	switch status {
	case outboundapp.TaskStatusSending:
	case outboundapp.TaskStatusSent:
		providerMessage = "fixture-message"
		sentAt = time.Now().UTC()
	case outboundapp.TaskStatusRetryableFailed, outboundapp.TaskStatusFinalFailed, outboundapp.TaskStatusOutcomeUnknown:
		failureKind = "temporary"
		lastError = "fixture-state"
	case outboundapp.TaskStatusCancelled:
	}
	if _, err := pool.Exec(ctx, `
UPDATE outbound_tasks
SET status=$2, attempt_count=$3, current_attempt_id=$4, last_failure_kind=$5,
    last_error=$6, provider_message_id=$7, sent_at=$8, status_updated_at=now()
WHERE id=$1`, enqueued.TaskID, status, attemptCount, currentAttemptID, failureKind, lastError, providerMessage, sentAt); err != nil {
		t.Fatal(err)
	}
}

func attemptStateForTask(status outboundapp.TaskStatus) string {
	switch status {
	case outboundapp.TaskStatusSending:
		return "dispatching"
	case outboundapp.TaskStatusSent:
		return "succeeded"
	case outboundapp.TaskStatusRetryableFailed:
		return "retryable_failed"
	case outboundapp.TaskStatusFinalFailed:
		return "final_failed"
	case outboundapp.TaskStatusOutcomeUnknown:
		return "outcome_unknown"
	default:
		return ""
	}
}

func assertCancelFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cancelled outboundapp.CancelledTask, receipts, links, events, jobs int) {
	t.Helper()
	var taskStatus string
	var gotReceipts, gotLinks, gotEvents, gotJobs int
	err := pool.QueryRow(ctx, `
SELECT task.status,
  (SELECT count(*) FROM outbound_control_receipts WHERE id=$2 AND state='completed' AND event_id=$3),
  (SELECT count(*) FROM outbound_task_job_links WHERE task_id=$1 AND generation=$4 AND river_job_id=$5 AND cancelled_at IS NOT NULL),
  (SELECT count(*) FROM event_log WHERE id=$3 AND event_type='outbound.cancelled'),
  (SELECT count(*) FROM river_job WHERE id=$5)
FROM outbound_tasks AS task WHERE task.id=$1`, cancelled.TaskID, cancelled.ReceiptID, cancelled.EventID, cancelled.Job.Generation, cancelled.Job.RiverJobID).Scan(
		&taskStatus, &gotReceipts, &gotLinks, &gotEvents, &gotJobs,
	)
	if err != nil || taskStatus != "cancelled" || gotReceipts != receipts || gotLinks != links || gotEvents != events || gotJobs != jobs {
		t.Fatalf("cancel facts task/receipt/link/event/job=%s/%d/%d/%d/%d err=%v", taskStatus, gotReceipts, gotLinks, gotEvents, gotJobs, err)
	}
}

func assertNoCompletedCancel(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key string) {
	t.Helper()
	var completed, events int
	if err := pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM outbound_control_receipts WHERE idempotency_key=$1 AND state='completed'),
	(SELECT count(*)
	   FROM event_log AS event
	   JOIN outbound_control_receipts AS receipt
	     ON event.idempotency_key='outbound.cancelled:' || receipt.id::text
	  WHERE receipt.idempotency_key=$1 AND event.event_type='outbound.cancelled')`, key).Scan(&completed, &events); err != nil || completed != 0 || events != 0 {
		t.Fatalf("completed receipt/events=%d/%d err=%v", completed, events, err)
	}
}

var errCancelEventRollback = errors.New("fixture cancel event rollback")

type failingCancelAppender struct{}

func (failingCancelAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errCancelEventRollback
}

type blockingCancelProvider struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan struct{}
}

func (provider *blockingCancelProvider) Send(ctx context.Context, _ outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	provider.mu.Lock()
	provider.calls++
	if provider.calls == 1 {
		close(provider.entered)
	}
	provider.mu.Unlock()
	select {
	case <-provider.release:
		return outboundapp.ProviderResult{MessageID: "fixture-cancel-race-message"}, nil
	case <-ctx.Done():
		return outboundapp.ProviderResult{}, ctx.Err()
	}
}

func (provider *blockingCancelProvider) Calls() int {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

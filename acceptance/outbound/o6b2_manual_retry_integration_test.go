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
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	queueriver "github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

func TestManualRetryFinalFailedCommitsOneNextGenerationAndStableReplay(t *testing.T) {
	pool := openOutboundPool(t)
	secondPool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b2-final", "success")
	deleteRiverJobFixture(t, ctx, pool, enqueued.RiverJobID)
	prepareCancelTaskStatus(t, ctx, pool, enqueued, outboundapp.TaskStatusFinalFailed)
	command := outboundapp.ManualRetryCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-manual-retry-0001"}
	services := []*outboundapp.ManualRetryService{newManualRetryService(t, pool, eventstore.NewAppender()), newManualRetryService(t, secondPool, eventstore.NewAppender())}

	type retryResult struct {
		result outboundapp.ManualRetryResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan retryResult, 2)
	var callers sync.WaitGroup
	for _, service := range services {
		callers.Add(1)
		go func(service *outboundapp.ManualRetryService) {
			defer callers.Done()
			<-start
			result, err := service.Retry(ctx, command)
			results <- retryResult{result: result, err: err}
		}(service)
	}
	close(start)
	callers.Wait()
	close(results)

	var original outboundapp.ManualRetryResult
	for got := range results {
		if got.err != nil || got.result.Status != outboundapp.TaskStatusPending || got.result.Job.Generation != 2 || got.result.Job.RiverJobID <= 0 {
			t.Fatalf("Retry()=%+v err=%v", got.result, got.err)
		}
		if original.ReceiptID == 0 {
			original = got.result
		} else if original != got.result {
			t.Fatalf("concurrent replay=%+v want %+v", got.result, original)
		}
	}
	assertManualRetryFacts(t, ctx, pool, original, 1, 2, 1, 1)
	replayed, err := services[0].Retry(ctx, command)
	if err != nil || replayed != original {
		t.Fatalf("lost-response replay=%+v err=%v", replayed, err)
	}
	assertManualRetryFacts(t, ctx, pool, original, 1, 2, 1, 1)

	conflicting := command
	conflicting.TaskID++
	if _, err = services[1].Retry(ctx, conflicting); !errors.Is(err, outboundapp.ErrManualRetryCommandConflict) {
		t.Fatalf("payload conflict err=%v", err)
	}
}

func TestManualRetryConcurrentDifferentKeysOnlyOneControlResultAndJob(t *testing.T) {
	pool := openOutboundPool(t)
	secondPool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b2-generation-race", "success")
	deleteRiverJobFixture(t, ctx, pool, enqueued.RiverJobID)
	prepareCancelTaskStatus(t, ctx, pool, enqueued, outboundapp.TaskStatusFinalFailed)
	commands := []outboundapp.ManualRetryCommand{
		{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-manual-retry-race-0001"},
		{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-manual-retry-race-0002"},
	}
	services := []*outboundapp.ManualRetryService{newManualRetryService(t, pool, eventstore.NewAppender()), newManualRetryService(t, secondPool, eventstore.NewAppender())}

	type retryResult struct {
		result outboundapp.ManualRetryResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan retryResult, 2)
	var callers sync.WaitGroup
	for index := range services {
		callers.Add(1)
		go func(index int) {
			defer callers.Done()
			<-start
			result, err := services[index].Retry(ctx, commands[index])
			results <- retryResult{result: result, err: err}
		}(index)
	}
	close(start)
	callers.Wait()
	close(results)

	var succeeded outboundapp.ManualRetryResult
	var successes, conflicts int
	for got := range results {
		switch {
		case got.err == nil:
			successes++
			succeeded = got.result
		case errors.Is(got.err, outboundapp.ErrManualRetryTransitionConflict):
			conflicts++
		default:
			t.Fatalf("concurrent different-key Retry()=%+v err=%v", got.result, got.err)
		}
	}
	if successes != 1 || conflicts != 1 || succeeded.Job.Generation != 2 {
		t.Fatalf("successes/conflicts/result=%d/%d/%+v", successes, conflicts, succeeded)
	}
	assertManualRetryFacts(t, ctx, pool, succeeded, 1, 2, 1, 1)
	var reserved int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbound_control_receipts WHERE idempotency_key LIKE 'outbound-manual-retry-race-%'`).Scan(&reserved); err != nil || reserved != 1 {
		t.Fatalf("committed control results=%d err=%v", reserved, err)
	}
}

func TestManualRetryCancelledBatchUsesTypedBatchArgs(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	batch, err := newBatchService(t, pool).Enqueue(ctx, outboundapp.EnqueueBatchCommand{
		IdempotencyScope: "operator:7", IdempotencyKey: "outbound-o6b2-batch-retry",
		Tier: outboundapp.BatchTierS, CustomerIDs: createOutboundCustomers(t, ctx, pool, 2),
		TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"manual retry batch"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID, batchID int64
	var chunkIndex int32
	if err = pool.QueryRow(ctx, `SELECT id,batch_id,batch_chunk_index FROM outbound_tasks WHERE batch_id=$1 ORDER BY id LIMIT 1`, batch.BatchID).Scan(&taskID, &batchID, &chunkIndex); err != nil {
		t.Fatal(err)
	}
	_, err = newCancelService(t, pool, eventstore.NewAppender()).Cancel(ctx, outboundapp.CancelCommand{
		TaskID: outboundapp.TaskID(taskID), IdempotencyScope: "operator:7", IdempotencyKey: "outbound-batch-cancel-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	retried, err := newManualRetryService(t, pool, eventstore.NewAppender()).Retry(ctx, outboundapp.ManualRetryCommand{
		TaskID: outboundapp.TaskID(taskID), IdempotencyScope: "operator:7", IdempotencyKey: "outbound-batch-manual-retry-0001",
	})
	if err != nil || retried.Job.JobKind != outboundapp.OutboundEnqueueBatchJobKind || retried.Job.Generation != 2 {
		t.Fatalf("Retry()=%+v err=%v", retried, err)
	}
	var encoded []byte
	if err = pool.QueryRow(ctx, `SELECT args FROM river_job WHERE id=$1`, retried.Job.RiverJobID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var args outboundapp.EnqueueBatchTaskArgs
	if err = json.Unmarshal(encoded, &args); err != nil || args.TaskID != outboundapp.TaskID(taskID) || args.BatchID != batchID || args.ChunkIndex != int(chunkIndex) {
		t.Fatalf("typed batch args=%+v err=%v", args, err)
	}
	assertManualRetryFacts(t, ctx, pool, retried, 1, 2, 1, 1)
}

func TestManualRetryRejectsFrozenStates(t *testing.T) {
	for _, status := range []outboundapp.TaskStatus{
		outboundapp.TaskStatusPending, outboundapp.TaskStatusSending, outboundapp.TaskStatusSent,
		outboundapp.TaskStatusRetryableFailed, outboundapp.TaskStatusOutcomeUnknown,
	} {
		t.Run(string(status), func(t *testing.T) {
			pool := openOutboundPool(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ensureOutboundRiverCatalog(t, ctx, pool)
			resetO6B1CancelFixture(t, ctx, pool)
			enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b2-state-"+string(status), "success")
			prepareCancelTaskStatus(t, ctx, pool, enqueued, status)
			command := outboundapp.ManualRetryCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-manual-retry-state-" + string(status)}
			_, err := newManualRetryService(t, pool, eventstore.NewAppender()).Retry(ctx, command)
			if !errors.Is(err, outboundapp.ErrManualRetryTransitionConflict) {
				t.Fatalf("Retry(%s) err=%v", status, err)
			}
			assertNoCompletedManualRetry(t, ctx, pool, command.IdempotencyKey)
		})
	}
}

func TestManualRetryEventFailureRollsBackJobReceiptLinkAndTask(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetO6B1CancelFixture(t, ctx, pool)

	enqueued := enqueueOneFixture(t, ctx, pool, createOutboundCustomer(t, ctx, pool), "outbound-o6b2-rollback", "success")
	deleteRiverJobFixture(t, ctx, pool, enqueued.RiverJobID)
	prepareCancelTaskStatus(t, ctx, pool, enqueued, outboundapp.TaskStatusFinalFailed)
	command := outboundapp.ManualRetryCommand{TaskID: enqueued.TaskID, IdempotencyScope: "operator:7", IdempotencyKey: "outbound-manual-retry-rollback"}
	_, err := newManualRetryService(t, pool, failingCancelAppender{}).Retry(ctx, command)
	if !errors.Is(err, errCancelEventRollback) {
		t.Fatalf("Retry rollback err=%v", err)
	}
	var status string
	var receipts, links, events, jobs int
	if err = pool.QueryRow(ctx, `
SELECT task.status,
 (SELECT count(*) FROM outbound_control_receipts WHERE idempotency_key=$2),
 (SELECT count(*) FROM outbound_task_job_links WHERE task_id=$1),
 (SELECT count(*) FROM event_log WHERE event_type='outbound.manual_retry_requested' AND payload->>'task_id'=($1::bigint)::text),
 (SELECT count(*) FROM river_job WHERE args->>'task_id'=($1::bigint)::text)
FROM outbound_tasks AS task WHERE task.id=$1`, enqueued.TaskID, command.IdempotencyKey).Scan(&status, &receipts, &links, &events, &jobs); err != nil || status != "final_failed" || receipts != 0 || links != 1 || events != 0 || jobs != 0 {
		t.Fatalf("rollback status/receipts/links/events/jobs=%s/%d/%d/%d/%d err=%v", status, receipts, links, events, jobs, err)
	}
}

func newManualRetryService(t *testing.T, pool *pgxpool.Pool, events eventport.Appender) *outboundapp.ManualRetryService {
	t.Helper()
	repository, err := outboundstore.NewControlRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	return outboundapp.NewManualRetryService(platformstore.NewUnitOfWork(pool), repository, events)
}

func deleteRiverJobFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	client, err := queueriver.NewClient(riverpgxv5.New(pool), &queueriver.Config{SkipUnknownJobCheck: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.JobDelete(ctx, jobID); err != nil {
		t.Fatal(err)
	}
}

func assertManualRetryFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, retried outboundapp.ManualRetryResult, receipts, links, events, jobs int) {
	t.Helper()
	var status string
	var gotReceipts, gotLinks, gotEvents, gotJobs int
	err := pool.QueryRow(ctx, `
SELECT task.status,
 (SELECT count(*) FROM outbound_control_receipts WHERE id=$2 AND operation='manual_retry' AND state='completed' AND event_id=$3),
 (SELECT count(*) FROM outbound_task_job_links WHERE task_id=$1),
 (SELECT count(*) FROM event_log WHERE id=$3 AND event_type='outbound.manual_retry_requested'),
 (SELECT count(*) FROM river_job WHERE id=$4)
FROM outbound_tasks AS task WHERE task.id=$1`, retried.TaskID, retried.ReceiptID, retried.EventID, retried.Job.RiverJobID).Scan(&status, &gotReceipts, &gotLinks, &gotEvents, &gotJobs)
	if err != nil || status != "pending" || gotReceipts != receipts || gotLinks != links || gotEvents != events || gotJobs != jobs {
		t.Fatalf("retry facts=%s/%d/%d/%d/%d err=%v", status, gotReceipts, gotLinks, gotEvents, gotJobs, err)
	}
}

func assertNoCompletedManualRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, key string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbound_control_receipts WHERE idempotency_key=$1 AND state='completed'`, key).Scan(&count); err != nil || count != 0 {
		t.Fatalf("completed manual retries=%d err=%v", count, err)
	}
}

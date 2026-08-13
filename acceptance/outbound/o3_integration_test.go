package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestEnqueueBatchTwoConnectionsReplayOriginalAcceptedFacts(t *testing.T) {
	pool := openOutboundPool(t)
	secondPool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundBatchFixture(t, pool)
	command := outboundBatchCommand(createOutboundCustomers(t, ctx, pool, 3))
	first := newBatchService(t, pool)
	second := newBatchService(t, secondPool)

	type result struct {
		batch outboundapp.EnqueuedBatch
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var callers sync.WaitGroup
	for _, service := range []*outboundapp.EnqueueBatchService{first, second} {
		callers.Add(1)
		go func(service *outboundapp.EnqueueBatchService) {
			defer callers.Done()
			<-start
			batch, err := service.Enqueue(ctx, command)
			results <- result{batch, err}
		}(service)
	}
	close(start)
	callers.Wait()
	close(results)
	var accepted outboundapp.EnqueuedBatch
	for got := range results {
		if got.err != nil || got.batch.BatchID <= 0 || got.batch.AcceptedEventID <= 0 || got.batch.TaskCount != 3 || got.batch.ChunkCount != 1 {
			t.Fatalf("Enqueue()=%+v err=%v", got.batch, got.err)
		}
		if accepted.BatchID == 0 {
			accepted = got.batch
		} else if got.batch != accepted {
			t.Fatalf("replay=%+v, want original %+v", got.batch, accepted)
		}
	}

	assertBatchCounts(t, pool, accepted.BatchID, 1, 3, 3, 3)
	var eventType, eventKey string
	var eventPayload []byte
	if err := pool.QueryRow(ctx, `SELECT event_type, payload, idempotency_key FROM event_log WHERE id=$1`, accepted.AcceptedEventID).Scan(&eventType, &eventPayload, &eventKey); err != nil {
		t.Fatal(err)
	}
	wantPayload := fmt.Sprintf(`{"batch_id":%d,"recipient_count":3,"tier":"S"}`, accepted.BatchID)
	if eventType != "outbound.batch.accepted" || eventKey != fmt.Sprintf("outbound.batch.accepted:%d", accepted.BatchID) || !equalJSON(eventPayload, []byte(wantPayload)) {
		t.Fatalf("batch event=%q/%s/%q", eventType, eventPayload, eventKey)
	}

	conflict := command
	conflict.Payload = json.RawMessage(`{"text":"different"}`)
	if _, err := second.Enqueue(ctx, conflict); !errors.Is(err, outboundapp.ErrEnqueueBatchConflict) {
		t.Fatalf("conflicting Enqueue() error=%v", err)
	}
	assertBatchCounts(t, pool, accepted.BatchID, 1, 3, 3, 3)
}

func TestEnqueueBatchResponseLossResumesAfterCommittedChunk(t *testing.T) {
	pool := openOutboundPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundBatchFixture(t, pool)
	command := outboundBatchCommand(createOutboundCustomers(t, ctx, pool, 1001))
	repository, err := outboundstore.NewEnqueueBatchRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	lossy := outboundapp.NewEnqueueBatchService(&cancelAfterCommitUoW{delegate: platformstore.NewUnitOfWork(pool), cancel: cancel, after: 2}, eventstore.NewAppender(), repository)
	if _, err = lossy.Enqueue(ctx, command); !errors.Is(err, context.Canceled) {
		t.Fatalf("lossy Enqueue() error=%v, want context cancellation after first chunk commit", err)
	}
	var batchID int64
	if err = pool.QueryRow(context.Background(), `SELECT id FROM outbound_batches WHERE idempotency_scope=$1 AND idempotency_key=$2`, command.IdempotencyScope, command.IdempotencyKey).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	assertBatchCounts(t, pool, batchID, 1, 1000, 1000, 1000)

	resumed, err := newBatchService(t, pool).Enqueue(context.Background(), command)
	if err != nil || resumed.BatchID != batchID || resumed.TaskCount != 1001 || resumed.ChunkCount != 2 {
		t.Fatalf("resumed Enqueue()=%+v err=%v", resumed, err)
	}
	assertBatchCounts(t, pool, batchID, 2, 1001, 1001, 1001)
	secondReplay, err := newBatchService(t, pool).Enqueue(context.Background(), command)
	if err != nil || secondReplay != resumed {
		t.Fatalf("completed replay=%+v err=%v, want %+v", secondReplay, err, resumed)
	}
	assertBatchCounts(t, pool, batchID, 2, 1001, 1001, 1001)
}

func TestEnqueueBatchRollsBackWholeChunkWhenRiverInsertFails(t *testing.T) {
	pool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundBatchFixture(t, pool)
	command := outboundBatchCommand(createOutboundCustomers(t, ctx, pool, 2))
	repository, err := outboundstore.NewEnqueueBatchRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	failing := &failingBatchRepository{EnqueueBatchRepository: repository}
	service := outboundapp.NewEnqueueBatchService(platformstore.NewUnitOfWork(pool), eventstore.NewAppender(), failing)
	if _, err = service.Enqueue(ctx, command); !errors.Is(err, errEnqueueRejected) {
		t.Fatalf("Enqueue() error=%v, want %v", err, errEnqueueRejected)
	}
	var batchID int64
	if err = pool.QueryRow(ctx, `SELECT id FROM outbound_batches`).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	assertBatchCounts(t, pool, batchID, 0, 0, 0, 0)
	resumed, err := newBatchService(t, pool).Enqueue(ctx, command)
	if err != nil || resumed.BatchID != batchID {
		t.Fatalf("retry=%+v err=%v", resumed, err)
	}
	assertBatchCounts(t, pool, batchID, 1, 2, 2, 2)
}

type cancelAfterCommitUoW struct {
	delegate platformport.UnitOfWork
	cancel   context.CancelFunc
	after    int
	calls    int
}

func (uow *cancelAfterCommitUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	err := uow.delegate.Within(ctx, callback)
	if err == nil {
		uow.calls++
		if uow.calls == uow.after {
			uow.cancel()
		}
	}
	return err
}

type failingBatchRepository struct {
	outboundapp.EnqueueBatchRepository
}

func (*failingBatchRepository) EnqueueBatchTask(context.Context, outboundapp.EnqueueBatchTaskArgs) (int64, error) {
	return 0, errEnqueueRejected
}

func newBatchService(t *testing.T, pool *pgxpool.Pool) *outboundapp.EnqueueBatchService {
	t.Helper()
	repository, err := outboundstore.NewEnqueueBatchRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	return outboundapp.NewEnqueueBatchService(platformstore.NewUnitOfWork(pool), eventstore.NewAppender(), repository)
}

func outboundBatchCommand(customerIDs []int64) outboundapp.EnqueueBatchCommand {
	return outboundapp.EnqueueBatchCommand{
		IdempotencyScope: "operator:7", IdempotencyKey: "outbound-enqueue-batch-acceptance",
		Tier: outboundapp.BatchTierS, CustomerIDs: customerIDs,
		TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"batch"}`),
	}
}

func createOutboundCustomers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, count int) []int64 {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ids := make([]int64, count)
	for index := range ids {
		ids[index], err = contactfixture.CreateCustomer(ctx, tx)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return ids
}

func resetOutboundBatchFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `TRUNCATE outbound_send_attempt_history, outbound_send_attempts, outbound_batch_chunks, outbound_enqueue_receipts, outbound_tasks, outbound_batches`)
	if err != nil {
		t.Fatalf("reset outbound batch fixture: %v", err)
	}
}

func assertBatchCounts(t *testing.T, pool *pgxpool.Pool, batchID int64, chunks, tasks, events, jobs int) {
	t.Helper()
	var gotChunks, gotTasks, gotEvents, gotJobs int
	err := pool.QueryRow(context.Background(), `
SELECT
  (SELECT count(*) FROM outbound_batch_chunks WHERE batch_id=$1 AND state='expanded'),
  (SELECT count(*) FROM outbound_tasks WHERE batch_id=$1),
  (SELECT count(*) FROM event_log WHERE (payload->>'batch_id')::bigint=$1 AND event_type='outbound.accepted'),
  (SELECT count(*) FROM river_job WHERE kind=$2 AND (args->>'batch_id')::bigint=$1)`, batchID, outboundapp.OutboundEnqueueBatchJobKind).Scan(&gotChunks, &gotTasks, &gotEvents, &gotJobs)
	if err != nil || gotChunks != chunks || gotTasks != tasks || gotEvents != events || gotJobs != jobs {
		t.Fatalf("batch %d chunk/task/event/job=%d/%d/%d/%d err=%v, want %d/%d/%d/%d", batchID, gotChunks, gotTasks, gotEvents, gotJobs, err, chunks, tasks, events, jobs)
	}
}

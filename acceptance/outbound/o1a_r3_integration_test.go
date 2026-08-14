package outbound_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var outboundDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 outbound database")

func TestOutboundStorageCatalogWaterlineAndIdentity(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()

	var waterline int
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&waterline); err != nil || waterline != 28 {
		t.Fatalf("migration waterline=%d err=%v, want 28", waterline, err)
	}

	for _, table := range []string{"outbound_tasks", "outbound_send_attempts", "outbound_send_attempt_history", "outbound_control_receipts"} {
		var identity, generation string
		if err := pool.QueryRow(ctx, `
SELECT is_identity, identity_generation
FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = 'id'`, table).Scan(&identity, &generation); err != nil || identity != "YES" || generation != "ALWAYS" {
			t.Fatalf("%s.id identity=%q generation=%q err=%v, want YES/ALWAYS", table, identity, generation, err)
		}
	}

	for _, forbidden := range []string{"accepted_event_id", "river_job_id"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema = 'public' AND table_name = 'outbound_tasks' AND column_name = $1::text
)`, forbidden).Scan(&exists); err != nil || exists {
			t.Fatalf("outbound_tasks forbidden column=%q exists=%t err=%v", forbidden, exists, err)
		}
	}

	for _, column := range []string{"status", "attempt_count", "current_attempt_id", "last_failure_kind", "last_error", "provider_message_id", "sent_at", "status_updated_at"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM information_schema.columns
  WHERE table_schema='public' AND table_name='outbound_tasks' AND column_name=$1
)`, column).Scan(&exists); err != nil || !exists {
			t.Fatalf("outbound_tasks status column=%q exists=%t err=%v", column, exists, err)
		}
	}
}

func TestAcceptOneCommitsDBGeneratedTaskAndAcceptedEvent(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()
	customerID := createOutboundCustomer(t, ctx, pool)
	sequenceStart := time.Now().UnixNano()
	if _, err := pool.Exec(ctx, `SELECT setval(pg_get_serial_sequence('outbound_tasks', 'id'), $1::bigint, true)`, sequenceStart); err != nil {
		t.Fatal(err)
	}

	repository := outboundstore.NewRepository()
	service := outboundapp.NewAcceptOneService(platformstore.NewUnitOfWork(pool), repository, eventstore.NewAppender())
	accepted, err := service.Accept(ctx, outboundapp.OneCommand{
		CustomerID:  customerID,
		TemplateKey: outboundapp.TemplateTextNoticeV1,
		Payload:     json.RawMessage(`{"text":"accepted only"}`),
	})
	if err != nil || accepted.TaskID != outboundapp.TaskID(sequenceStart+1) || accepted.EventID < 1 {
		t.Fatalf("Accept()=%+v err=%v, want database-generated task %d and event", accepted, err, sequenceStart+1)
	}

	var taskCustomerID int64
	var templateKey string
	var payload []byte
	if err = pool.QueryRow(ctx, `
SELECT customer_id, template_key, payload
FROM outbound_tasks
WHERE id = $1::bigint`, accepted.TaskID).Scan(&taskCustomerID, &templateKey, &payload); err != nil {
		t.Fatal(err)
	}
	if taskCustomerID != customerID || templateKey != outboundapp.TemplateTextNoticeV1 || !equalJSON(payload, []byte(`{"text":"accepted only"}`)) {
		t.Fatalf("task=%d/%q/%s, want persisted task", taskCustomerID, templateKey, payload)
	}

	var eventType, eventKey string
	var eventCustomerID *int64
	var eventPayload []byte
	if err = pool.QueryRow(ctx, `
SELECT event_type, customer_id, payload, idempotency_key
FROM event_log
WHERE id = $1::bigint`, accepted.EventID).Scan(&eventType, &eventCustomerID, &eventPayload, &eventKey); err != nil {
		t.Fatal(err)
	}
	wantPayload := fmt.Sprintf(`{"task_id":%d}`, accepted.TaskID)
	if eventType != eventport.EvOutboundAccepted || eventCustomerID == nil || *eventCustomerID != customerID || !equalJSON(eventPayload, []byte(wantPayload)) || eventKey != fmt.Sprintf("outbound.accepted:%d", accepted.TaskID) {
		t.Fatalf("event type=%q customer=%v payload=%s key=%q, want accepted event for task", eventType, eventCustomerID, eventPayload, eventKey)
	}
}

func TestAcceptOneRollsBackTaskWhenAcceptedEventCannotAppend(t *testing.T) {
	pool := openOutboundPool(t)
	resetOutboundFixture(t, pool)
	ctx := context.Background()
	var eventsBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_log`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	service := outboundapp.NewAcceptOneService(platformstore.NewUnitOfWork(pool), outboundstore.NewRepository(), failingAppender{})
	_, err := service.Accept(ctx, outboundapp.OneCommand{
		CustomerID:  createOutboundCustomer(t, ctx, pool),
		TemplateKey: outboundapp.TemplateTextNoticeV1,
		Payload:     json.RawMessage(`{"text":"rollback"}`),
	})
	if !errors.Is(err, errAppendRejected) {
		t.Fatalf("Accept() error=%v, want %v", err, errAppendRejected)
	}

	var tasks, eventsAfter int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbound_tasks), (SELECT count(*) FROM event_log)`).Scan(&tasks, &eventsAfter); err != nil || tasks != 0 || eventsAfter != eventsBefore {
		t.Fatalf("rollback facts tasks/events=%d/%d err=%v, want 0/%d", tasks, eventsAfter, err, eventsBefore)
	}
}

func TestEnqueueOneCommitsStableOutboundJobAndReplaysOriginalFacts(t *testing.T) {
	pool := openOutboundPool(t)
	secondPool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundEnqueueFixture(t, pool)
	customerID := createOutboundCustomer(t, ctx, pool)
	var firstPID, secondPID int
	if err := pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := secondPool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&secondPID); err != nil || firstPID == secondPID {
		t.Fatalf("independent PostgreSQL connections=%d/%d err=%v", firstPID, secondPID, err)
	}
	repository, err := outboundstore.NewEnqueueOneRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	secondRepository, err := outboundstore.NewEnqueueOneRepository(secondPool)
	if err != nil {
		t.Fatal(err)
	}
	service := outboundapp.NewEnqueueOneService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewRepository(), eventstore.NewAppender(), repository, repository,
	)
	secondService := outboundapp.NewEnqueueOneService(
		platformstore.NewUnitOfWork(secondPool), outboundstore.NewRepository(), eventstore.NewAppender(), secondRepository, secondRepository,
	)
	command := outboundapp.EnqueueOneCommand{
		OneCommand:       outboundapp.OneCommand{CustomerID: customerID, TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"enqueue once"}`)},
		IdempotencyScope: "operator:7",
		IdempotencyKey:   "outbound-enqueue-one-acceptance",
	}
	type enqueueResult struct {
		result outboundapp.EnqueuedTask
		err    error
	}
	start := make(chan struct{})
	results := make(chan enqueueResult, 2)
	var callers sync.WaitGroup
	for _, caller := range []*outboundapp.EnqueueOneService{service, secondService} {
		callers.Add(1)
		go func(caller *outboundapp.EnqueueOneService) {
			defer callers.Done()
			<-start
			result, callErr := caller.Enqueue(ctx, command)
			results <- enqueueResult{result: result, err: callErr}
		}(caller)
	}
	close(start)
	callers.Wait()
	close(results)
	var first outboundapp.EnqueuedTask
	for result := range results {
		if result.err != nil || result.result.TaskID <= 0 || result.result.EventID <= 0 || result.result.RiverJobID <= 0 {
			t.Fatalf("concurrent Enqueue() = %#v, %v", result.result, result.err)
		}
		if first.TaskID == 0 {
			first = result.result
			continue
		}
		if result.result != first {
			t.Fatalf("concurrent replay=%#v, want original %#v", result.result, first)
		}
	}

	var receiptTaskID, receiptEventID, receiptJobID int64
	var state string
	if err = pool.QueryRow(ctx, `
SELECT state, task_id, event_id, river_job_id
FROM outbound_enqueue_receipts
WHERE idempotency_scope = $1::text AND idempotency_key = $2::text`, command.IdempotencyScope, command.IdempotencyKey).
		Scan(&state, &receiptTaskID, &receiptEventID, &receiptJobID); err != nil {
		t.Fatal(err)
	}
	if state != "accepted" || receiptTaskID != int64(first.TaskID) || receiptEventID != int64(first.EventID) || receiptJobID != first.RiverJobID {
		t.Fatalf("receipt state/task/event/job=%q/%d/%d/%d, want accepted original facts", state, receiptTaskID, receiptEventID, receiptJobID)
	}
	var queue, kind string
	var args outboundapp.EnqueueOneArgs
	var encodedArgs []byte
	if err = pool.QueryRow(ctx, `SELECT queue, kind, args FROM river_job WHERE id = $1::bigint`, first.RiverJobID).Scan(&queue, &kind, &encodedArgs); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encodedArgs, &args); err != nil {
		t.Fatal(err)
	}
	if queue != "outbound" || kind != outboundapp.OutboundEnqueueOneJobKind || args.TaskID != first.TaskID || args.ReceiptID <= 0 {
		t.Fatalf("River job queue=%q kind=%q args=%#v, want stable outbound command", queue, kind, args)
	}

	conflicting := command
	conflicting.Payload = json.RawMessage(`{"text":"different"}`)
	if _, err = secondService.Enqueue(ctx, conflicting); !errors.Is(err, outboundapp.ErrEnqueueOneConflict) {
		t.Fatalf("conflicting Enqueue() error=%v, want %v", err, outboundapp.ErrEnqueueOneConflict)
	}
	var receipts, tasks, jobs int
	if err = pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM outbound_enqueue_receipts WHERE idempotency_scope = $1::text AND idempotency_key = $2::text),
  (SELECT count(*) FROM outbound_tasks),
  (SELECT count(*) FROM river_job WHERE id = $3::bigint AND kind = $4::text)`, command.IdempotencyScope, command.IdempotencyKey, first.RiverJobID, outboundapp.OutboundEnqueueOneJobKind).
		Scan(&receipts, &tasks, &jobs); err != nil || receipts != 1 || tasks != 1 || jobs != 1 {
		t.Fatalf("receipt/task/job counts=%d/%d/%d err=%v, want 1/1/1", receipts, tasks, jobs, err)
	}
}

func TestEnqueueOneRollsBackReceiptTaskAndEventWhenJobCannotInsert(t *testing.T) {
	pool := openOutboundPool(t)
	ctx := context.Background()
	ensureOutboundRiverCatalog(t, ctx, pool)
	resetOutboundEnqueueFixture(t, pool)
	var eventsBefore, jobsBefore int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM event_log), (SELECT count(*) FROM river_job WHERE kind = $1::text)`, outboundapp.OutboundEnqueueOneJobKind).Scan(&eventsBefore, &jobsBefore); err != nil {
		t.Fatal(err)
	}
	repository, err := outboundstore.NewEnqueueOneRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := outboundapp.NewEnqueueOneService(
		platformstore.NewUnitOfWork(pool), outboundstore.NewRepository(), eventstore.NewAppender(), repository, failingEnqueueOne{},
	)
	_, err = service.Enqueue(ctx, outboundapp.EnqueueOneCommand{
		OneCommand:       outboundapp.OneCommand{CustomerID: createOutboundCustomer(t, ctx, pool), TemplateKey: outboundapp.TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"rollback job"}`)},
		IdempotencyScope: "operator:7",
		IdempotencyKey:   "outbound-enqueue-one-rollback",
	})
	if !errors.Is(err, errEnqueueRejected) {
		t.Fatalf("Enqueue() error=%v, want %v", err, errEnqueueRejected)
	}
	var receipts, tasks, eventsAfter, jobs int
	if err = pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM outbound_enqueue_receipts),
  (SELECT count(*) FROM outbound_tasks),
  (SELECT count(*) FROM event_log),
	  (SELECT count(*) FROM river_job WHERE kind = $1::text)`, outboundapp.OutboundEnqueueOneJobKind).
		Scan(&receipts, &tasks, &eventsAfter, &jobs); err != nil || receipts != 0 || tasks != 0 || eventsAfter != eventsBefore || jobs != jobsBefore {
		t.Fatalf("rollback receipt/task/events/job=%d/%d/%d/%d err=%v, want 0/0/%d/%d", receipts, tasks, eventsAfter, jobs, err, eventsBefore, jobsBefore)
	}
}

var errAppendRejected = errors.New("accepted event append rejected")

type failingAppender struct{}

func (failingAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errAppendRejected
}

var errEnqueueRejected = errors.New("outbound River enqueue rejected")

type failingEnqueueOne struct{}

func (failingEnqueueOne) EnqueueOne(context.Context, outboundapp.EnqueueOneArgs) (int64, error) {
	return 0, errEnqueueRejected
}

func openOutboundPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *outboundDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*outboundDatabaseURL); err != nil {
		t.Fatalf("unsafe outbound database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *outboundDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v, want 160014", version, err)
	}
	return pool
}

func resetOutboundFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE outbound_control_receipts, outbound_task_job_links, outbound_send_attempt_history, outbound_send_attempts, outbound_batch_chunks, outbound_enqueue_receipts, outbound_tasks, outbound_batches`); err != nil {
		t.Fatalf("reset outbound fixture: %v", err)
	}
}

func resetOutboundEnqueueFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `TRUNCATE outbound_control_receipts, outbound_task_job_links, outbound_send_attempt_history, outbound_send_attempts, outbound_batch_chunks, outbound_enqueue_receipts, outbound_tasks, outbound_batches`); err != nil {
		t.Fatalf("reset outbound enqueue fixture: %v", err)
	}
}

func ensureOutboundRiverCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var tableCount int
	if err := pool.QueryRow(ctx, `
SELECT count(*)
FROM information_schema.tables
WHERE table_schema = current_schema() AND table_name LIKE 'river_%'`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount == 0 {
		if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
			t.Fatal(err)
		}
		return
	}
	if tableCount != 6 {
		t.Fatalf("River catalog tables=%d, want 0 or 6", tableCount)
	}
}

func createOutboundCustomer(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	customerID, err := contactfixture.CreateCustomer(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID
}

func equalJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

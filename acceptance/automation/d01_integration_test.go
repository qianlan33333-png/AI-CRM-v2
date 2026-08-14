package automation_acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

var d01DatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 D01 database")

func TestD01ContactProducerAndAutomationConsumerCloseOneObservableLoop(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	producer, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	uow := platformstore.NewUnitOfWork(pool)
	customerID, tagID := createContactFacts(t, ctx, pool)
	service := contactapp.NewCustomerMutationService(uow, contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(), producer)
	if err = service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: "admin:d01"}); err != nil {
		t.Fatal(err)
	}

	eventID, jobID := assertProducerFacts(t, ctx, pool, customerID, tagID)
	worker := realAutomationWorker(t, pool, producer)
	job := loadDeliveryJob(t, ctx, pool, jobID, 1, 25)
	if err = worker.Work(ctx, job); err != nil {
		t.Fatal(err)
	}
	assertCompletedFacts(t, ctx, pool, eventID, customerID, tagID)
	if err = worker.Work(ctx, loadDeliveryJob(t, ctx, pool, jobID, 2, 25)); err != nil {
		t.Fatal(err)
	}
	assertCompletedFacts(t, ctx, pool, eventID, customerID, tagID)

	if err = service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: "admin:d01"}); err != nil {
		t.Fatal(err)
	}
	assertCompletedFacts(t, ctx, pool, eventID, customerID, tagID)
}

func TestD01RetryPoisonOutcomeUnknownLeaseAndBackfill(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	customerID, tagID := createContactFacts(t, ctx, pool)

	transientID, transientJob := appendAndAccept(t, ctx, pool, deliveries, customerID, tagID, true)
	transient := errors.New("temporary dependency")
	firstSubscriber := &scriptedSubscriber{err: transient}
	worker := scriptedWorker(t, deliveries, firstSubscriber)
	if err = worker.Work(ctx, loadDeliveryJob(t, ctx, pool, transientJob, 1, 3)); !errors.Is(err, transient) {
		t.Fatalf("transient Work() error = %v", err)
	}
	assertDeliveryStatus(t, ctx, pool, transientID, eventport.DeliveryPending)
	if err = realAutomationWorker(t, pool, deliveries).Work(ctx, loadDeliveryJob(t, ctx, pool, transientJob, 2, 3)); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, ctx, pool, transientID, eventport.DeliveryCompleted)

	poisonID, poisonJob := appendAndAccept(t, ctx, pool, deliveries, customerID, tagID, false)
	poison := &scriptedSubscriber{err: eventport.PoisonDelivery(errors.New("bad payload"))}
	if err = scriptedWorker(t, deliveries, poison).Work(ctx, loadDeliveryJob(t, ctx, pool, poisonJob, 1, 3)); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, ctx, pool, poisonID, eventport.DeliveryFinalFailed)

	unknownID, unknownJob := appendAndAccept(t, ctx, pool, deliveries, customerID, tagID, true)
	unknown := &scriptedSubscriber{err: eventport.UnknownDeliveryOutcome(errors.New("ambiguous effect"))}
	unknownWorker := scriptedWorker(t, deliveries, unknown)
	if err = unknownWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, unknownJob, 1, 3)); err != nil {
		t.Fatal(err)
	}
	if err = unknownWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, unknownJob, 2, 3)); err != nil || unknown.calls != 1 {
		t.Fatalf("outcome_unknown replay error/calls = %v/%d", err, unknown.calls)
	}
	assertDeliveryStatus(t, ctx, pool, unknownID, eventport.DeliveryOutcomeUnknown)

	leaseID, _ := appendAndAccept(t, ctx, pool, deliveries, customerID, tagID, true)
	if _, err = deliveries.Claim(ctx, leaseID, eventport.ConsumerAutomationTagTrigger, "river:lease-1", time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	claim, err := deliveries.Claim(ctx, leaseID, eventport.ConsumerAutomationTagTrigger, "river:lease-2", time.Minute)
	if err != nil || claim.Owner != "river:lease-2" || claim.Attempt != 2 {
		t.Fatalf("expired lease claim = %+v, %v", claim, err)
	}
	if err = deliveries.FinalFail(ctx, leaseID, claim.Consumer, claim.Owner, "test_cleanup"); err != nil {
		t.Fatal(err)
	}

	replayID, replayJob := appendAndAccept(t, ctx, pool, deliveries, customerID, tagID, true)
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		return deliveries.Accept(txCtx, replayID, eventport.ConsumerAutomationTagTrigger)
	})
	if err != nil {
		t.Fatal(err)
	}
	var deliveriesCount, jobsCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_deliveries WHERE event_id=$1`, replayID).Scan(&deliveriesCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE id=$1`, replayJob).Scan(&jobsCount); err != nil || deliveriesCount != 1 || jobsCount != 1 {
		t.Fatalf("producer replay delivery/jobs = %d/%d err=%v", deliveriesCount, jobsCount, err)
	}

	historyID := appendHistoricalTagApplied(t, ctx, pool, customerID, tagID)
	generic := newDeliveryRuntime(t, pool, nil)
	if dispatched := drainDispatch(t, ctx, generic); dispatched < 1 {
		t.Fatal("legacy generic Dispatch() did not mark the historical event dispatched")
	}
	var historicalDispatched bool
	if err = pool.QueryRow(ctx, `SELECT dispatched FROM event_log WHERE id=$1`, historyID).Scan(&historicalDispatched); err != nil || !historicalDispatched {
		t.Fatalf("historical dispatched=%v err=%v", historicalDispatched, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_deliveries WHERE event_id=$1`, historyID).Scan(&deliveriesCount); err != nil || deliveriesCount != 0 {
		t.Fatalf("pre-backfill deliveries=%d err=%v", deliveriesCount, err)
	}
	backfill := newBackfillRuntime(t, pool)
	dispatched := drainDispatch(t, ctx, backfill)
	if dispatched < 1 {
		t.Fatal("historical Dispatch() did not backfill any event")
	}
	if count, dispatchErr := backfill.Dispatch(ctx); dispatchErr != nil || count != 0 {
		t.Fatalf("replayed Dispatch() count/error=%d/%v", count, dispatchErr)
	}
	var backfillJob int64
	if err = pool.QueryRow(ctx, `SELECT min(river_job_id),count(*) FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, historyID, eventport.ConsumerAutomationTagTrigger).Scan(&backfillJob, &deliveriesCount); err != nil || deliveriesCount != 1 {
		t.Fatalf("backfill job/delivery=%d/%d err=%v", backfillJob, deliveriesCount, err)
	}
	if err = realAutomationWorker(t, pool, backfill).Work(ctx, loadDeliveryJob(t, ctx, pool, backfillJob, 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertCompletedFacts(t, ctx, pool, historyID, customerID, tagID)
}

func TestD01ProducerRollsBackAllFiveFactsWhenAcceptanceFails(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	producer, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	customerID, tagID := createContactFacts(t, ctx, pool)
	service := contactapp.NewCustomerMutationService(
		platformstore.NewUnitOfWork(pool), contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(),
		failingAfterAccept{inner: producer},
	)
	err = service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: "admin:d01-rollback"})
	if !errors.Is(err, errAcceptanceFailure) {
		t.Fatalf("AddTag() error = %v", err)
	}
	var tags, timeline, events, deliveriesCount, jobs int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_tags WHERE customer_id=$1 AND tag_id=$2`, customerID, tagID).Scan(&tags); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM customer_events WHERE customer_id=$1 AND payload->>'tag_id'=$2`, customerID, strconv.FormatInt(tagID, 10)).Scan(&timeline); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_log WHERE customer_id=$1 AND payload->>'tag_id'=$2`, customerID, strconv.FormatInt(tagID, 10)).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM event_deliveries d JOIN event_log e ON e.id=d.event_id WHERE e.customer_id=$1 AND e.payload->>'tag_id'=$2`, customerID, strconv.FormatInt(tagID, 10)).Scan(&deliveriesCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE args->>'event_id' IN (SELECT id::text FROM event_log WHERE customer_id=$1 AND payload->>'tag_id'=$2)`, customerID, strconv.FormatInt(tagID, 10)).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if tags+timeline+events+deliveriesCount+jobs != 0 {
		t.Fatalf("rolled-back facts tag/timeline/event/delivery/job = %d/%d/%d/%d/%d", tags, timeline, events, deliveriesCount, jobs)
	}
}

func TestD01S200KReceiptPlanHasNoIllegalSequentialScan(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO automation_trigger_receipts
      (event_id,consumer,customer_id,tag_id,actor,state,triggered_event_id,triggered_at,completed_at)
      SELECT 100000000+g,'automation.tag-trigger.v1',g,g,'perf','triggered',200000000+g,
             now()-(g::text||' microseconds')::interval,now()
        FROM generate_series(1,200000) AS g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE automation_trigger_receipts`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, `EXPLAIN (COSTS OFF)
      SELECT id,event_id,customer_id,tag_id,triggered_at
        FROM automation_trigger_receipts
       WHERE state='triggered'
       ORDER BY triggered_at DESC,id DESC LIMIT 50`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if strings.Contains(plan.String(), "Seq Scan on automation_trigger_receipts") || !strings.Contains(plan.String(), "automation_trigger_receipts_list_idx") {
		t.Fatalf("illegal S plan:\n%s", plan.String())
	}
}

func TestD01StorageCatalogIsValidatedAndHasNoRiverOrAutomationCrossDomainFK(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	var waterline, constraints, invalidConstraints, indexes, invalidIndexes, eventLogFKs, receiptFKs, riverFKs int
	var deliveryPersistence, receiptPersistence string
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('event_deliveries'::regclass,'automation_trigger_receipts'::regclass)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('event_deliveries'::regclass,'automation_trigger_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('event_deliveries'::regclass,'automation_trigger_receipts'::regclass)),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('event_deliveries'::regclass,'automation_trigger_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid='event_deliveries'::regclass AND contype='f' AND confrelid='event_log'::regclass),
      (SELECT count(*) FROM pg_constraint WHERE conrelid='automation_trigger_receipts'::regclass AND contype='f'),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('event_deliveries'::regclass,'automation_trigger_receipts'::regclass) AND contype='f' AND confrelid='river_job'::regclass),
      (SELECT relpersistence::text FROM pg_class WHERE oid='event_deliveries'::regclass),
      (SELECT relpersistence::text FROM pg_class WHERE oid='automation_trigger_receipts'::regclass)`).Scan(
		&waterline, &constraints, &invalidConstraints, &indexes, &invalidIndexes,
		&eventLogFKs, &receiptFKs, &riverFKs, &deliveryPersistence, &receiptPersistence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if waterline != 25 || constraints != 16 || invalidConstraints != 0 || indexes != 7 || invalidIndexes != 0 ||
		eventLogFKs != 1 || receiptFKs != 0 || riverFKs != 0 || deliveryPersistence != "p" || receiptPersistence != "p" {
		t.Fatalf("catalog waterline/constraints/invalid/indexes/invalid/fks/persistence=%d/%d/%d/%d/%d/%d/%d/%d/%s/%s",
			waterline, constraints, invalidConstraints, indexes, invalidIndexes, eventLogFKs, receiptFKs, riverFKs, deliveryPersistence, receiptPersistence)
	}
}

type scriptedSubscriber struct {
	err   error
	calls int
}

func (*scriptedSubscriber) Consumer() string     { return eventport.ConsumerAutomationTagTrigger }
func (*scriptedSubscriber) EventTypes() []string { return []string{eventport.EvTagApplied} }
func (subscriber *scriptedSubscriber) ConsumeDelivery(context.Context, eventport.DeliveryClaim) error {
	subscriber.calls++
	return subscriber.err
}

var errAcceptanceFailure = errors.New("acceptance failed after River insert")

type failingAfterAccept struct{ inner eventport.DeliveryAcceptor }

func (acceptor failingAfterAccept) Accept(ctx context.Context, eventID eventport.EventID, consumer string) error {
	if err := acceptor.inner.Accept(ctx, eventID, consumer); err != nil {
		return err
	}
	return errAcceptanceFailure
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *d01DatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*d01DatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *d01DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}
	return pool, ctx
}

func ensureRiver(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('river_job') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func createContactFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (int64, int64) {
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
	tagID, err := contactfixture.CreateTag(ctx, tx, "d01-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID, tagID
}

func assertProducerFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID, tagID int64) (eventport.EventID, int64) {
	t.Helper()
	var eventID, jobID int64
	var eventType, consumer, status, kind, queue string
	var customerEvents, tags int
	err := pool.QueryRow(ctx, `SELECT e.id,d.river_job_id,e.event_type,d.consumer,d.status,j.kind,j.queue,
      (SELECT count(*) FROM customer_events WHERE customer_id=$1 AND payload->>'tag_id'=$2),
      (SELECT count(*) FROM customer_tags WHERE customer_id=$1 AND tag_id=$3)
      FROM event_log e JOIN event_deliveries d ON d.event_id=e.id JOIN river_job j ON j.id=d.river_job_id
      WHERE e.customer_id=$1 AND e.payload->>'tag_id'=$2 ORDER BY e.id DESC LIMIT 1`,
		customerID, strconv.FormatInt(tagID, 10), tagID).Scan(&eventID, &jobID, &eventType, &consumer, &status, &kind, &queue, &customerEvents, &tags)
	if err != nil || eventType != eventport.EvTagApplied || consumer != eventport.ConsumerAutomationTagTrigger || status != "pending" ||
		kind != eventport.DeliveryJobKind || queue != "event" || customerEvents != 1 || tags != 1 {
		t.Fatalf("producer facts=%d/%d %s/%s/%s %s/%s timeline=%d tags=%d err=%v", eventID, jobID, eventType, consumer, status, kind, queue, customerEvents, tags, err)
	}
	return eventport.EventID(eventID), jobID
}

func realAutomationWorker(t *testing.T, pool *pgxpool.Pool, deliveries *eventstore.DeliveryRepository) *eventdispatcher.DeliveryWorker {
	t.Helper()
	consumer, err := automationstore.NewTagTriggerConsumer(platformstore.NewUnitOfWork(pool), automationstore.NewRepository(pool), eventstore.NewAppender(), deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return scriptedWorker(t, deliveries, consumer)
}

func scriptedWorker(t *testing.T, deliveries eventport.DeliveryRuntime, subscriber eventport.DeliverySubscriber) *eventdispatcher.DeliveryWorker {
	t.Helper()
	router, err := eventdispatcher.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	if err = router.RegisterDelivery(subscriber); err != nil {
		t.Fatal(err)
	}
	worker, err := eventdispatcher.NewDeliveryWorker(deliveries, router)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func loadDeliveryJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64, attempt, maxAttempts int) *river.Job[eventport.DeliveryJobArgs] {
	t.Helper()
	var encoded []byte
	if err := pool.QueryRow(ctx, `SELECT args FROM river_job WHERE id=$1`, jobID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var args eventport.DeliveryJobArgs
	if err := json.Unmarshal(encoded, &args); err != nil {
		t.Fatal(err)
	}
	return &river.Job[eventport.DeliveryJobArgs]{JobRow: &rivertype.JobRow{ID: jobID, Attempt: attempt, MaxAttempts: maxAttempts}, Args: args}
}

func appendAndAccept(t *testing.T, ctx context.Context, pool *pgxpool.Pool, deliveries *eventstore.DeliveryRepository, customerID, tagID int64, valid bool) (eventport.EventID, int64) {
	t.Helper()
	key := fmt.Sprintf("customer.tag_applied:d01:%d", time.Now().UnixNano())
	payload := json.RawMessage(fmt.Sprintf(`{"customer_id":%d,"tag_id":%d,"actor":"admin:d01"}`, customerID, tagID))
	if !valid {
		payload = json.RawMessage(`{"customer_id":0}`)
	}
	var eventID eventport.EventID
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		var appendErr error
		eventID, appendErr = eventstore.NewAppender().Append(txCtx, eventport.Event{
			Type: eventport.EvTagApplied, CustomerID: eventport.CustomerID(customerID), Payload: payload,
			OccurredAt: time.Now().UTC(), IdempotencyKey: key,
		})
		if appendErr != nil {
			return appendErr
		}
		return deliveries.Accept(txCtx, eventID, eventport.ConsumerAutomationTagTrigger)
	})
	if err != nil {
		t.Fatal(err)
	}
	var jobID int64
	if err = pool.QueryRow(ctx, `SELECT river_job_id FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerAutomationTagTrigger).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	return eventID, jobID
}

func appendHistoricalTagApplied(t *testing.T, ctx context.Context, pool *pgxpool.Pool, customerID, tagID int64) eventport.EventID {
	t.Helper()
	var eventID eventport.EventID
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		var appendErr error
		eventID, appendErr = eventstore.NewAppender().Append(txCtx, eventport.Event{
			Type: eventport.EvTagApplied, CustomerID: eventport.CustomerID(customerID),
			Payload:    json.RawMessage(fmt.Sprintf(`{"customer_id":%d,"tag_id":%d,"actor":"admin:d01-backfill"}`, customerID, tagID)),
			OccurredAt: time.Now().UTC(), IdempotencyKey: fmt.Sprintf("customer.tag_applied:d01-backfill:%d", time.Now().UnixNano()),
		})
		return appendErr
	})
	if err != nil {
		t.Fatal(err)
	}
	return eventID
}

func newBackfillRuntime(t *testing.T, pool *pgxpool.Pool) *eventstore.DeliveryRepository {
	t.Helper()
	return newDeliveryRuntime(t, pool, []eventport.DeliveryBinding{{
		EventType: eventport.EvTagApplied, Consumer: eventport.ConsumerAutomationTagTrigger,
	}})
}

func newDeliveryRuntime(t *testing.T, pool *pgxpool.Pool, bindings []eventport.DeliveryBinding) *eventstore.DeliveryRepository {
	t.Helper()
	deferred := eventdispatcher.NewDeferredEnqueuer()
	deliveries, err := eventstore.NewRuntimeDeliveryRepository(pool, deferred, 100, bindings)
	if err != nil {
		t.Fatal(err)
	}
	router, err := eventdispatcher.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	worker, err := eventdispatcher.NewDeliveryWorker(deliveries, router)
	if err != nil {
		t.Fatal(err)
	}
	registry := platformjobqueue.NewWorkerRegistry()
	if err = platformjobqueue.AddWorker(registry, platformjobqueue.QueueEvent, worker); err != nil {
		t.Fatal(err)
	}
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{
		Critical: 1, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1,
	}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err = deferred.Bind(client); err != nil {
		t.Fatal(err)
	}
	return deliveries
}

func drainDispatch(t *testing.T, ctx context.Context, deliveries *eventstore.DeliveryRepository) int {
	t.Helper()
	total := 0
	for batch := 0; batch < 100; batch++ {
		count, err := deliveries.Dispatch(ctx)
		if err != nil {
			t.Fatalf("Dispatch() batch/count/error=%d/%d/%v", batch, count, err)
		}
		total += count
		if count == 0 {
			return total
		}
	}
	t.Fatal("Dispatch() did not drain within 100 batches")
	return 0
}

func assertDeliveryStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID, want eventport.DeliveryStatus) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerAutomationTagTrigger).Scan(&got); err != nil || got != string(want) {
		t.Fatalf("delivery status=%q err=%v want=%q", got, err, want)
	}
}

func assertCompletedFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID, customerID, tagID int64) {
	t.Helper()
	var receipts, triggered, completed int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM automation_trigger_receipts WHERE event_id=$1 AND customer_id=$2 AND tag_id=$3 AND state='triggered'),
      (SELECT count(*) FROM event_log WHERE event_type='automation.triggered' AND payload->>'source_event_id'=$4),
      (SELECT count(*) FROM event_deliveries WHERE event_id=$1 AND consumer=$5 AND status='completed')`,
		eventID, customerID, tagID, strconv.FormatInt(int64(eventID), 10), eventport.ConsumerAutomationTagTrigger).Scan(&receipts, &triggered, &completed)
	if err != nil || receipts != 1 || triggered != 1 || completed != 1 {
		t.Fatalf("completed receipt/event/delivery=%d/%d/%d err=%v", receipts, triggered, completed, err)
	}
}

func TestD01MigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	customerID, tagID := createContactFacts(t, ctx, pool)
	var eventID eventport.EventID
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		var appendErr error
		eventID, appendErr = eventstore.NewAppender().Append(txCtx, eventport.Event{
			Type: eventport.EvTagApplied, CustomerID: eventport.CustomerID(customerID),
			Payload:    json.RawMessage(fmt.Sprintf(`{"customer_id":%d,"tag_id":%d,"actor":"migration"}`, customerID, tagID)),
			OccurredAt: time.Now().UTC(), IdempotencyKey: fmt.Sprintf("d01-migration-history:%d", time.Now().UnixNano()),
		})
		return appendErr
	})
	if err != nil || eventID <= 0 {
		t.Fatalf("history fixture event=%d err=%v", eventID, err)
	}
	t.Logf("D01_HISTORY_EVENT_ID=%d", eventID)
}

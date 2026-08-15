package stats_acceptance

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

	"github.com/jackc/pgx/v5"
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
	statsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/stats/store"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

var l01DatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 L01 database")

func TestL01SameEventCompletesAutomationAndStatsIndependently(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries := producerDeliveries(t, pool)
	eventID, occurredAt, jobs, customerID, tagID := produceTag(t, ctx, pool, deliveries, "admin:l01-normal")
	if len(jobs) != 2 || jobs[eventport.ConsumerAutomationTagTrigger] <= 0 || jobs[eventport.ConsumerStatsTagApplied] <= 0 {
		t.Fatalf("named jobs=%v", jobs)
	}
	worker := realDeliveryWorker(t, pool, deliveries)
	if err := worker.Work(ctx, loadDeliveryJob(t, ctx, pool, jobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(ctx, loadDeliveryJob(t, ctx, pool, jobs[eventport.ConsumerStatsTagApplied], 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertAutomationCompleted(t, ctx, pool, eventID)
	assertStatsCompleted(t, ctx, pool, eventID, occurredAt, tagID, 1)

	if err := worker.Work(ctx, loadDeliveryJob(t, ctx, pool, jobs[eventport.ConsumerStatsTagApplied], 2, 25)); err != nil {
		t.Fatal(err)
	}
	assertStatsCompleted(t, ctx, pool, eventID, occurredAt, tagID, 1)

	service := contactapp.NewCustomerMutationService(
		platformstore.NewUnitOfWork(pool), contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(), deliveries,
	)
	if err := service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: "admin:l01-normal"}); err != nil {
		t.Fatal(err)
	}
	assertStatsCompleted(t, ctx, pool, eventID, occurredAt, tagID, 1)
}

func TestL01StatsRetryPoisonOutcomeUnknownAndLeaseDoNotAffectAutomation(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries := producerDeliveries(t, pool)

	transientID, transientAt, transientJobs, _, transientTag := produceTag(t, ctx, pool, deliveries, "admin:l01-transient")
	realWorker := realDeliveryWorker(t, pool, deliveries)
	if err := realWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, transientJobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	realStats := realStatsConsumer(t, pool, deliveries)
	temporary := errors.New("temporary stats store failure")
	fault := &faultInjectingSubscriber{inner: realStats, err: temporary}
	if err := workerFor(t, deliveries, fault).Work(ctx, loadDeliveryJob(t, ctx, pool, transientJobs[eventport.ConsumerStatsTagApplied], 1, 3)); !errors.Is(err, temporary) {
		t.Fatalf("transient Work() error=%v", err)
	}
	assertDeliveryStatus(t, ctx, pool, transientID, eventport.ConsumerAutomationTagTrigger, eventport.DeliveryCompleted)
	assertDeliveryStatus(t, ctx, pool, transientID, eventport.ConsumerStatsTagApplied, eventport.DeliveryPending)
	if err := realWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, transientJobs[eventport.ConsumerStatsTagApplied], 2, 3)); err != nil {
		t.Fatal(err)
	}
	assertStatsCompleted(t, ctx, pool, transientID, transientAt, transientTag, 1)

	poisonID, _, poisonJobs, _, _ := produceTag(t, ctx, pool, deliveries, "admin:l01-poison")
	if err := realWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, poisonJobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	poison := &faultInjectingSubscriber{inner: realStats, err: eventport.PoisonDelivery(errors.New("invalid stats dimensions"))}
	if err := workerFor(t, deliveries, poison).Work(ctx, loadDeliveryJob(t, ctx, pool, poisonJobs[eventport.ConsumerStatsTagApplied], 1, 3)); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, ctx, pool, poisonID, eventport.ConsumerAutomationTagTrigger, eventport.DeliveryCompleted)
	assertDeliveryStatus(t, ctx, pool, poisonID, eventport.ConsumerStatsTagApplied, eventport.DeliveryFinalFailed)

	unknownID, _, unknownJobs, _, _ := produceTag(t, ctx, pool, deliveries, "admin:l01-unknown")
	if err := realWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, unknownJobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	unknown := &faultInjectingSubscriber{inner: realStats, err: eventport.UnknownDeliveryOutcome(errors.New("ambiguous stats result"))}
	unknownWorker := workerFor(t, deliveries, unknown)
	if err := unknownWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, unknownJobs[eventport.ConsumerStatsTagApplied], 1, 3)); err != nil {
		t.Fatal(err)
	}
	if err := unknownWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, unknownJobs[eventport.ConsumerStatsTagApplied], 2, 3)); err != nil || unknown.calls != 1 {
		t.Fatalf("outcome_unknown replay error/calls=%v/%d", err, unknown.calls)
	}
	assertDeliveryStatus(t, ctx, pool, unknownID, eventport.ConsumerAutomationTagTrigger, eventport.DeliveryCompleted)
	assertDeliveryStatus(t, ctx, pool, unknownID, eventport.ConsumerStatsTagApplied, eventport.DeliveryOutcomeUnknown)

	leaseID, _, leaseJobs, _, _ := produceTag(t, ctx, pool, deliveries, "admin:l01-lease")
	if err := realWorker.Work(ctx, loadDeliveryJob(t, ctx, pool, leaseJobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	if _, err := deliveries.Claim(ctx, leaseID, eventport.ConsumerStatsTagApplied, "river:l01-lease-1", time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	claim, err := deliveries.Claim(ctx, leaseID, eventport.ConsumerStatsTagApplied, "river:l01-lease-2", time.Minute)
	if err != nil || claim.Owner != "river:l01-lease-2" || claim.Attempt != 2 {
		t.Fatalf("expired Stats lease claim=%+v err=%v", claim, err)
	}
	if err = deliveries.FinalFail(ctx, leaseID, claim.Consumer, claim.Owner, "test_cleanup"); err != nil {
		t.Fatal(err)
	}
	assertDeliveryStatus(t, ctx, pool, leaseID, eventport.ConsumerAutomationTagTrigger, eventport.DeliveryCompleted)
}

func TestL01HistoricalMissingStatsDeliveryBackfillsOnce(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries := producerDeliveries(t, pool)
	customerID, tagID := createContactFacts(t, ctx, pool)
	occurredAt := time.Now().UTC()
	var eventID eventport.EventID
	err := platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		var appendErr error
		eventID, appendErr = eventstore.NewAppender().Append(txCtx, eventport.Event{
			Type: eventport.EvTagApplied, CustomerID: eventport.CustomerID(customerID),
			Payload:    json.RawMessage(fmt.Sprintf(`{"customer_id":%d,"tag_id":%d,"actor":"admin:l01-backfill"}`, customerID, tagID)),
			OccurredAt: occurredAt, IdempotencyKey: fmt.Sprintf("customer.tag_applied:l01-backfill:%d", time.Now().UnixNano()),
		})
		if appendErr != nil {
			return appendErr
		}
		return deliveries.Accept(txCtx, eventID, eventport.ConsumerAutomationTagTrigger)
	})
	if err != nil {
		t.Fatal(err)
	}
	var automationJob int64
	if err = pool.QueryRow(ctx, `SELECT river_job_id FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerAutomationTagTrigger).Scan(&automationJob); err != nil {
		t.Fatal(err)
	}
	if err = realDeliveryWorker(t, pool, deliveries).Work(ctx, loadDeliveryJob(t, ctx, pool, automationJob, 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertNoStatsDelivery(t, ctx, pool, eventID)

	backfill := runtimeDeliveries(t, pool)
	if count, dispatchErr := backfill.Dispatch(ctx); dispatchErr != nil || count < 1 {
		t.Fatalf("Stats backfill count/error=%d/%v", count, dispatchErr)
	}
	if count, dispatchErr := backfill.Dispatch(ctx); dispatchErr != nil || count != 0 {
		t.Fatalf("Stats replay count/error=%d/%v", count, dispatchErr)
	}
	var statsJob int64
	if err = pool.QueryRow(ctx, `SELECT river_job_id FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerStatsTagApplied).Scan(&statsJob); err != nil {
		t.Fatal(err)
	}
	if err = realDeliveryWorker(t, pool, backfill).Work(ctx, loadDeliveryJob(t, ctx, pool, statsJob, 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertAutomationCompleted(t, ctx, pool, eventID)
	assertStatsCompleted(t, ctx, pool, eventID, occurredAt, tagID, 1)
}

func TestL01ProducerRollsBackBothNamedAcceptances(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries := producerDeliveries(t, pool)
	customerID, tagID := createContactFacts(t, ctx, pool)
	service := contactapp.NewCustomerMutationService(
		platformstore.NewUnitOfWork(pool), contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(),
		failStatsAfterAccept{inner: deliveries},
	)
	err := service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: "admin:l01-rollback"})
	if !errors.Is(err, errStatsAcceptanceFailure) {
		t.Fatalf("AddTag() error=%v", err)
	}
	var tags, timeline, events, deliveryCount, jobs int
	if err = pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM customer_tags WHERE customer_id=$1 AND tag_id=$2),
      (SELECT count(*) FROM customer_events WHERE customer_id=$1 AND payload->>'tag_id'=$3),
      (SELECT count(*) FROM event_log WHERE customer_id=$1 AND payload->>'tag_id'=$3),
      (SELECT count(*) FROM event_deliveries d JOIN event_log e ON e.id=d.event_id WHERE e.customer_id=$1 AND e.payload->>'tag_id'=$3),
      (SELECT count(*) FROM river_job WHERE args->>'event_id' IN (SELECT id::text FROM event_log WHERE customer_id=$1 AND payload->>'tag_id'=$3))`,
		customerID, tagID, strconv.FormatInt(tagID, 10)).Scan(&tags, &timeline, &events, &deliveryCount, &jobs); err != nil {
		t.Fatal(err)
	}
	if tags+timeline+events+deliveryCount+jobs != 0 {
		t.Fatalf("rolled-back facts tag/timeline/event/deliveries/jobs=%d/%d/%d/%d/%d", tags, timeline, events, deliveryCount, jobs)
	}
}

func TestL01S200KPlansUseStatsIndexes(t *testing.T) {
	pool, ctx := openPool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO stats_daily (stat_date,metric_key,dims,value)
      SELECT DATE '2026-08-14'-(g % 365)::int,'customer.tag_applied',jsonb_build_object('tag_id',900000000+g),1
        FROM generate_series(1,200000) AS g`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO stats_event_receipts
      (event_id,consumer,stat_date,metric_key,dims,value_delta,applied_at)
      SELECT 900000000+g,'stats.tag-applied.v1',DATE '2026-08-14'-(g % 365)::int,
             'customer.tag_applied',jsonb_build_object('tag_id',900000000+g),1,
             now()-(g::text||' microseconds')::interval
        FROM generate_series(1,200000) AS g`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE stats_daily`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `ANALYZE stats_event_receipts`); err != nil {
		t.Fatal(err)
	}
	metricPlan := explain(t, ctx, tx, `EXPLAIN (COSTS OFF)
      SELECT stat_date,metric_key,dims,value FROM stats_daily
       WHERE metric_key='customer.tag_applied' ORDER BY stat_date DESC LIMIT 50`)
	if strings.Contains(metricPlan, "Seq Scan on stats_daily") || !strings.Contains(metricPlan, "stats_daily_metric_date_idx") {
		t.Fatalf("illegal stats_daily S plan:\n%s", metricPlan)
	}
	receiptPlan := explain(t, ctx, tx, `EXPLAIN (COSTS OFF)
      SELECT event_id,consumer FROM stats_event_receipts
       WHERE event_id=900100000 AND consumer='stats.tag-applied.v1'`)
	if strings.Contains(receiptPlan, "Seq Scan on stats_event_receipts") || !strings.Contains(receiptPlan, "stats_event_receipts_pkey") {
		t.Fatalf("illegal receipt S plan:\n%s", receiptPlan)
	}
}

func TestL01StorageCatalogHasValidatedStatsOwnedTables(t *testing.T) {
	pool, ctx := openPool(t)
	var waterline, constraints, invalidConstraints, indexes, invalidIndexes, foreignKeys int
	var dailyPersistence, receiptPersistence string
	err := pool.QueryRow(ctx, `SELECT
      (SELECT max(version_id) FROM goose_db_version WHERE is_applied),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('stats_daily'::regclass,'stats_event_receipts'::regclass)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('stats_daily'::regclass,'stats_event_receipts'::regclass) AND NOT convalidated),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('stats_daily'::regclass,'stats_event_receipts'::regclass)),
      (SELECT count(*) FROM pg_index WHERE indrelid IN ('stats_daily'::regclass,'stats_event_receipts'::regclass) AND (NOT indisvalid OR NOT indisready OR NOT indislive)),
      (SELECT count(*) FROM pg_constraint WHERE conrelid IN ('stats_daily'::regclass,'stats_event_receipts'::regclass) AND contype='f'),
      (SELECT relpersistence::text FROM pg_class WHERE oid='stats_daily'::regclass),
      (SELECT relpersistence::text FROM pg_class WHERE oid='stats_event_receipts'::regclass)`).Scan(
		&waterline, &constraints, &invalidConstraints, &indexes, &invalidIndexes, &foreignKeys, &dailyPersistence, &receiptPersistence,
	)
	if err != nil {
		t.Fatal(err)
	}
	if waterline != 41 || constraints != 10 || invalidConstraints != 0 || indexes != 3 || invalidIndexes != 0 ||
		foreignKeys != 0 || dailyPersistence != "p" || receiptPersistence != "p" {
		t.Fatalf("catalog waterline/constraints/invalid/indexes/invalid/fks/persistence=%d/%d/%d/%d/%d/%d/%s/%s",
			waterline, constraints, invalidConstraints, indexes, invalidIndexes, foreignKeys, dailyPersistence, receiptPersistence)
	}
}

func TestL01MigrationHistoryFixture(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	deliveries := producerDeliveries(t, pool)
	eventID, _, jobs, _, _ := produceTag(t, ctx, pool, deliveries, "admin:l01-migration")
	if err := realDeliveryWorker(t, pool, deliveries).Work(ctx, loadDeliveryJob(t, ctx, pool, jobs[eventport.ConsumerAutomationTagTrigger], 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertAutomationCompleted(t, ctx, pool, eventID)
	assertDeliveryStatus(t, ctx, pool, eventID, eventport.ConsumerStatsTagApplied, eventport.DeliveryPending)
}

func TestL01ConsumeMigrationHistory(t *testing.T) {
	pool, ctx := openPool(t)
	ensureRiver(t, ctx, pool)
	var eventID, tagID, statsJob int64
	var occurredAt time.Time
	err := pool.QueryRow(ctx, `SELECT e.id,(e.payload->>'tag_id')::bigint,e.occurred_at,d.river_job_id
      FROM event_log e JOIN event_deliveries d ON d.event_id=e.id AND d.consumer=$1
      WHERE e.payload->>'actor'='admin:l01-migration' ORDER BY e.id DESC LIMIT 1`, eventport.ConsumerStatsTagApplied).
		Scan(&eventID, &tagID, &occurredAt, &statsJob)
	if err != nil {
		t.Fatal(err)
	}
	deliveries := producerDeliveries(t, pool)
	if err = realDeliveryWorker(t, pool, deliveries).Work(ctx, loadDeliveryJob(t, ctx, pool, statsJob, 1, 25)); err != nil {
		t.Fatal(err)
	}
	assertStatsCompleted(t, ctx, pool, eventport.EventID(eventID), occurredAt, tagID, 1)
}

type faultInjectingSubscriber struct {
	inner eventport.DeliverySubscriber
	err   error
	calls int
}

func (subscriber *faultInjectingSubscriber) Consumer() string { return subscriber.inner.Consumer() }
func (subscriber *faultInjectingSubscriber) EventTypes() []string {
	return subscriber.inner.EventTypes()
}
func (subscriber *faultInjectingSubscriber) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	subscriber.calls++
	if subscriber.err != nil {
		return subscriber.err
	}
	return subscriber.inner.ConsumeDelivery(ctx, claim)
}

var errStatsAcceptanceFailure = errors.New("Stats acceptance failed after River insert")

type failStatsAfterAccept struct{ inner eventport.DeliveryAcceptor }

func (acceptor failStatsAfterAccept) Accept(ctx context.Context, eventID eventport.EventID, consumer string) error {
	if err := acceptor.inner.Accept(ctx, eventID, consumer); err != nil {
		return err
	}
	if consumer == eventport.ConsumerStatsTagApplied {
		return errStatsAcceptanceFailure
	}
	return nil
}

func openPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *l01DatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*l01DatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *l01DatabaseURL)
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

func producerDeliveries(t *testing.T, pool *pgxpool.Pool) *eventstore.DeliveryRepository {
	t.Helper()
	repository, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	return repository
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
	tagID, err := contactfixture.CreateTag(ctx, tx, "l01-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return customerID, tagID
}

func produceTag(t *testing.T, ctx context.Context, pool *pgxpool.Pool, deliveries *eventstore.DeliveryRepository, actor string) (eventport.EventID, time.Time, map[string]int64, int64, int64) {
	t.Helper()
	customerID, tagID := createContactFacts(t, ctx, pool)
	service := contactapp.NewCustomerMutationService(
		platformstore.NewUnitOfWork(pool), contactstore.NewCustomerMutationRepository(), eventstore.NewAppender(), deliveries,
	)
	if err := service.AddTag(ctx, contactapp.CustomerTagCommand{ID: contactport.CustomerID(customerID), TagID: tagID, Actor: contactport.Actor(actor)}); err != nil {
		t.Fatal(err)
	}
	var eventID int64
	var occurredAt time.Time
	var deliveriesCount, jobsCount int
	err := pool.QueryRow(ctx, `SELECT e.id,e.occurred_at,count(DISTINCT d.consumer),count(DISTINCT d.river_job_id)
      FROM event_log e JOIN event_deliveries d ON d.event_id=e.id
      WHERE e.customer_id=$1 AND e.payload->>'tag_id'=$2
      GROUP BY e.id,e.occurred_at ORDER BY e.id DESC LIMIT 1`, customerID, strconv.FormatInt(tagID, 10)).
		Scan(&eventID, &occurredAt, &deliveriesCount, &jobsCount)
	if err != nil || deliveriesCount != 2 || jobsCount != 2 {
		t.Fatalf("producer event/deliveries/jobs=%d/%d/%d err=%v", eventID, deliveriesCount, jobsCount, err)
	}
	jobs := make(map[string]int64, 2)
	rows, err := pool.Query(ctx, `SELECT consumer,river_job_id FROM event_deliveries WHERE event_id=$1`, eventID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var consumer string
		var jobID int64
		if err = rows.Scan(&consumer, &jobID); err != nil {
			t.Fatal(err)
		}
		jobs[consumer] = jobID
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return eventport.EventID(eventID), occurredAt, jobs, customerID, tagID
}

func realStatsConsumer(t *testing.T, pool *pgxpool.Pool, deliveries eventport.DeliveryCompleter) eventport.DeliverySubscriber {
	t.Helper()
	consumer, err := statsstore.NewTagAppliedConsumer(platformstore.NewUnitOfWork(pool), statsstore.NewRepository(pool), deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return consumer
}

func realDeliveryWorker(t *testing.T, pool *pgxpool.Pool, deliveries *eventstore.DeliveryRepository) *eventdispatcher.DeliveryWorker {
	t.Helper()
	automation, err := automationstore.NewTagTriggerConsumer(
		platformstore.NewUnitOfWork(pool), automationstore.NewRepository(pool), eventstore.NewAppender(), deliveries,
	)
	if err != nil {
		t.Fatal(err)
	}
	router, err := eventdispatcher.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	if err = router.RegisterDelivery(automation); err != nil {
		t.Fatal(err)
	}
	if err = router.RegisterDelivery(realStatsConsumer(t, pool, deliveries)); err != nil {
		t.Fatal(err)
	}
	worker, err := eventdispatcher.NewDeliveryWorker(deliveries, router)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func workerFor(t *testing.T, deliveries eventport.DeliveryRuntime, subscriber eventport.DeliverySubscriber) *eventdispatcher.DeliveryWorker {
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

func runtimeDeliveries(t *testing.T, pool *pgxpool.Pool) *eventstore.DeliveryRepository {
	t.Helper()
	deferred := eventdispatcher.NewDeferredEnqueuer()
	deliveries, err := eventstore.NewRuntimeDeliveryRepository(pool, deferred, 100, []eventport.DeliveryBinding{
		{EventType: eventport.EvTagApplied, Consumer: eventport.ConsumerAutomationTagTrigger},
		{EventType: eventport.EvTagApplied, Consumer: eventport.ConsumerStatsTagApplied},
	})
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

func assertDeliveryStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID, consumer string, want eventport.DeliveryStatus) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, consumer).Scan(&got); err != nil || got != string(want) {
		t.Fatalf("delivery %s status=%q err=%v want=%q", consumer, got, err, want)
	}
}

func assertAutomationCompleted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID) {
	t.Helper()
	var receipts, triggered int
	err := pool.QueryRow(ctx, `SELECT
      (SELECT count(*) FROM automation_trigger_receipts WHERE event_id=$1 AND state='triggered'),
      (SELECT count(*) FROM event_log WHERE event_type='automation.triggered' AND payload->>'source_event_id'=$2)`,
		eventID, strconv.FormatInt(int64(eventID), 10)).Scan(&receipts, &triggered)
	if err != nil || receipts != 1 || triggered != 1 {
		t.Fatalf("Automation receipt/triggered=%d/%d err=%v", receipts, triggered, err)
	}
	assertDeliveryStatus(t, ctx, pool, eventID, eventport.ConsumerAutomationTagTrigger, eventport.DeliveryCompleted)
}

func assertStatsCompleted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID, occurredAt time.Time, tagID, want int64) {
	t.Helper()
	projection, err := statsstore.NewRepository(pool).GetTagApplied(ctx, occurredAt, tagID)
	if err != nil || projection.TagID != tagID || projection.Value != want {
		t.Fatalf("Stats projection=%+v err=%v want=%d", projection, err, want)
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM stats_event_receipts WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerStatsTagApplied).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("Stats receipts=%d err=%v", receipts, err)
	}
	assertDeliveryStatus(t, ctx, pool, eventID, eventport.ConsumerStatsTagApplied, eventport.DeliveryCompleted)
}

func assertNoStatsDelivery(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID eventport.EventID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM event_deliveries WHERE event_id=$1 AND consumer=$2`, eventID, eventport.ConsumerStatsTagApplied).Scan(&count); err != nil || count != 0 {
		t.Fatalf("pre-backfill Stats deliveries=%d err=%v", count, err)
	}
}

func explain(t *testing.T, ctx context.Context, tx pgx.Tx, statement string) string {
	t.Helper()
	rows, err := tx.Query(ctx, statement)
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
	return plan.String()
}

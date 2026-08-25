package outbound_acceptance

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventdispatcher "github.com/qianlan33333-png/AI-CRM-v2/internal/events/dispatcher"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	eventsfixture "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/acceptancefixture"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var outboundCampaignHandoffDatabaseURL = flag.String("campaign-handoff-database-url", "", "dedicated PostgreSQL 16.14 outbound campaign handoff database")

type approvedCampaignHandoffSource struct {
	snapshot outboundport.ApprovedCampaignHandoffSnapshot
	calls    atomic.Int32
}

func (source *approvedCampaignHandoffSource) LockApprovedCampaignHandoff(ctx context.Context, campaignCode, planID string) (outboundport.ApprovedCampaignHandoffSnapshot, error) {
	if _, err := platformstore.TxFromContext(ctx); err != nil {
		return outboundport.ApprovedCampaignHandoffSnapshot{}, err
	}
	source.calls.Add(1)
	if campaignCode != source.snapshot.CampaignCode || planID != source.snapshot.PlanID {
		return outboundport.ApprovedCampaignHandoffSnapshot{}, outbound.ErrCampaignHandoffNotFound
	}
	result := source.snapshot
	result.CustomerIDs = append([]int64(nil), result.CustomerIDs...)
	result.Steps = append([]outbound.CampaignHandoffStep(nil), result.Steps...)
	return result, nil
}

func TestCampaignHandoffAcceptPostgreSQLConcurrencyReplayAndLocalEventDelivery(t *testing.T) {
	pool := openOutboundCampaignHandoffPool(t)
	secondPool := openOutboundCampaignHandoffPool(t)
	ctx := context.Background()
	assertOutboundCampaignHandoffWaterline(t, ctx, pool)
	resetOutboundCampaignHandoffFixture(t, ctx, pool, nil)
	var eventIDs []int64
	t.Cleanup(func() { resetOutboundCampaignHandoffFixture(t, context.Background(), pool, eventIDs) })
	ensureOutboundRiverCatalog(t, ctx, pool)

	var firstPID, secondPID int
	if err := pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := secondPool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&secondPID); err != nil || firstPID == secondPID {
		t.Fatalf("physical PostgreSQL connections=%d/%d err=%v", firstPID, secondPID, err)
	}

	planID := outboundCampaignHandoffPlanID('a')
	source := &approvedCampaignHandoffSource{snapshot: outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: "outbound-accept", PlanID: planID, ReviewVersion: 3,
		SourceDigest: strings.Repeat("11", 32), TargetDigest: strings.Repeat("22", 32), ContentDigest: strings.Repeat("33", 32),
		CustomerIDs: []int64{202, 101}, Steps: []outbound.CampaignHandoffStep{{Index: 1, Content: "immutable local content"}},
		ApprovedAt: time.Now().UTC().Truncate(time.Microsecond),
	}}
	services := []*outboundapp.CampaignHandoffService{
		newCampaignHandoffService(t, pool, source),
		newCampaignHandoffService(t, secondPool, source),
	}
	commands := []outboundapp.AcceptCampaignHandoffCommand{
		{CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ExpectedReviewVersion: 3, ActorID: 71, IdempotencyKey: "outbound-accept-race-key-one"},
		{CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ExpectedReviewVersion: 3, ActorID: 72, IdempotencyKey: "outbound-accept-race-key-two"},
	}
	type result struct {
		index   int
		summary outbound.CampaignHandoffSummary
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var callers sync.WaitGroup
	for index := range services {
		callers.Add(1)
		go func(index int) {
			defer callers.Done()
			<-start
			summary, err := services[index].Accept(ctx, commands[index])
			results <- result{index: index, summary: summary, err: err}
		}(index)
	}
	close(start)
	callers.Wait()
	close(results)

	winner := result{index: -1}
	losers := 0
	for candidate := range results {
		if candidate.err == nil {
			if winner.index >= 0 || !outbound.ValidCampaignHandoffSummary(candidate.summary) {
				t.Fatalf("unexpected concurrent success: %#v after %#v", candidate, winner)
			}
			winner = candidate
			continue
		}
		if !errors.Is(candidate.err, outbound.ErrCampaignHandoffConflict) {
			t.Fatalf("concurrent loser error=%v, want conflict", candidate.err)
		}
		losers++
	}
	if winner.index < 0 || losers != 1 {
		t.Fatalf("concurrent winner=%#v losers=%d, want one each", winner, losers)
	}
	eventIDs = append(eventIDs, campaignHandoffReceiptEventID(t, ctx, pool, planID))
	assertCampaignHandoffFactCounts(t, ctx, pool, planID, 1, 1, 1, 2, 1, 0, 0)

	callsBeforeReplay := source.calls.Load()
	replayed, err := services[winner.index].Accept(ctx, commands[winner.index])
	if err != nil || !reflect.DeepEqual(replayed, winner.summary) || source.calls.Load() != callsBeforeReplay {
		t.Fatalf("durable replay=%#v err=%v source calls=%d/%d", replayed, err, source.calls.Load(), callsBeforeReplay)
	}
	assertCampaignHandoffFactCounts(t, ctx, pool, planID, 1, 1, 1, 2, 1, 0, 0)

	wrongScope := commands[winner.index]
	wrongScope.CampaignCode = "outbound-other"
	wrongScope.IdempotencyKey = "outbound-accept-cross-scope"
	if _, err = services[winner.index].Accept(ctx, wrongScope); !errors.Is(err, outbound.ErrCampaignHandoffNotFound) {
		t.Fatalf("cross-scope accept error=%v, want not found", err)
	}
	var wrongReceipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM public.outbound_campaign_handoff_receipts WHERE campaign_code=$1`, wrongScope.CampaignCode).Scan(&wrongReceipts); err != nil || wrongReceipts != 0 {
		t.Fatalf("cross-scope receipts=%d err=%v, want zero", wrongReceipts, err)
	}

	deliveries := newCampaignHandoffDeliveryRepository(t, pool)
	dispatched, err := deliveries.Dispatch(ctx)
	if err != nil || dispatched != 1 {
		t.Fatalf("Events dispatch count=%d err=%v, want one internal delivery job", dispatched, err)
	}
	assertCampaignHandoffFactCounts(t, ctx, pool, planID, 1, 1, 1, 2, 1, 1, 0)
	var queue, kind, consumer string
	if err = pool.QueryRow(ctx, `SELECT job.queue, job.kind, delivery.consumer
FROM public.event_deliveries AS delivery
JOIN public.event_log AS event ON event.id=delivery.event_id
JOIN public.river_job AS job ON job.id=delivery.river_job_id
WHERE event.event_type=$1 AND event.payload ->> 'plan_id'=$2`, eventport.EvOutboundCampaignHandoffFact, planID).Scan(&queue, &kind, &consumer); err != nil {
		t.Fatal(err)
	}
	if queue != string(platformjobqueue.QueueEvent) || kind != eventport.DeliveryJobKind || consumer != eventport.ConsumerOutboundCampaignHandoffFact {
		t.Fatalf("delivery queue/kind/consumer=%q/%q/%q", queue, kind, consumer)
	}
}

func TestCampaignHandoffMigrationRejectsIncompleteAndTamperedFacts(t *testing.T) {
	pool := openOutboundCampaignHandoffPool(t)
	ctx := context.Background()
	assertOutboundCampaignHandoffWaterline(t, ctx, pool)
	resetOutboundCampaignHandoffFixture(t, ctx, pool, nil)
	t.Cleanup(func() { resetOutboundCampaignHandoffFixture(t, context.Background(), pool, nil) })

	t.Run("reserved receipt", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `INSERT INTO public.outbound_campaign_handoff_receipts
  (actor_id,key_digest,payload_digest,campaign_code,plan_id,created_at)
VALUES (81,decode(repeat('81',32),'hex'),decode(repeat('82',32),'hex'),'reserved-guard',$1,now())`, outboundCampaignHandoffPlanID('b'))
		if err != nil {
			t.Fatal(err)
		}
		assertOutboundCampaignHandoffSQLState(t, tx.Commit(ctx), "23514")
	})

	t.Run("header and children without receipt", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		planID := outboundCampaignHandoffPlanID('c')
		handoffID := insertRawCampaignHandoffHeader(t, ctx, tx, "missing-receipt", planID, 82)
		insertRawCampaignHandoffChildren(t, ctx, tx, handoffID)
		assertOutboundCampaignHandoffSQLState(t, tx.Commit(ctx), "23514")
		assertNoCampaignHandoffPlan(t, ctx, pool, planID)
	})

	t.Run("event plan and handoff mismatch", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		planID := outboundCampaignHandoffPlanID('d')
		handoffID := insertRawCampaignHandoffHeader(t, ctx, tx, "mismatched-event", planID, 83)
		insertRawCampaignHandoffChildren(t, ctx, tx, handoffID)
		for _, appendFact := range []func(context.Context, pgx.Tx, int64, string) (int64, error){
			eventsfixture.AppendCampaignHandoffAcceptedFact,
			eventsfixture.AppendCampaignHandoffAcceptedFactWithForbiddenExtraKey,
		} {
			if _, err = appendFact(ctx, tx, handoffID, outboundCampaignHandoffPlanID('e')); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("mismatched event fact error=%v, want pgx.ErrNoRows", err)
			}
		}
	})

	for _, tamper := range []string{"event-extra-key", "result-count"} {
		tamper := tamper
		t.Run(tamper, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			planID := outboundCampaignHandoffPlanID(map[string]rune{"event-extra-key": 'f', "result-count": '1'}[tamper])
			insertRawCompletedCampaignHandoff(t, ctx, tx, "tampered-"+tamper, planID, 83, tamper)
			assertOutboundCampaignHandoffSQLState(t, tx.Commit(ctx), "23514")
			assertNoCampaignHandoffPlan(t, ctx, pool, planID)
		})
	}
}

func TestCampaignHandoffMigrationFactsGuardAndEmptyDownUp(t *testing.T) {
	pool := openOutboundCampaignHandoffPool(t)
	ctx := context.Background()
	assertOutboundCampaignHandoffWaterline(t, ctx, pool)
	resetOutboundCampaignHandoffFixture(t, ctx, pool, nil)
	repositoryRoot := outboundCampaignHandoffRepositoryRoot(t)
	var eventIDs []int64
	t.Cleanup(func() {
		if outboundCampaignHandoffMigrationWaterline(t, context.Background(), pool) < 68 {
			runOutboundCampaignHandoffGoose(t, context.Background(), repositoryRoot, "up-to", "68")
		}
		resetOutboundCampaignHandoffFixture(t, context.Background(), pool, eventIDs)
	})

	planID := outboundCampaignHandoffPlanID('f')
	source := &approvedCampaignHandoffSource{snapshot: outboundport.ApprovedCampaignHandoffSnapshot{
		CampaignCode: "outbound-down-guard", PlanID: planID, ReviewVersion: 3,
		SourceDigest: strings.Repeat("11", 32), TargetDigest: strings.Repeat("22", 32), ContentDigest: strings.Repeat("33", 32),
		CustomerIDs: []int64{101}, Steps: []outbound.CampaignHandoffStep{{Index: 1, Content: "immutable local content"}},
		ApprovedAt: time.Now().UTC().Truncate(time.Microsecond),
	}}
	if _, err := newCampaignHandoffService(t, pool, source).Accept(ctx, outboundapp.AcceptCampaignHandoffCommand{
		CampaignCode: source.snapshot.CampaignCode, PlanID: planID, ExpectedReviewVersion: 3,
		ActorID: 84, IdempotencyKey: "outbound-campaign-handoff-down-guard",
	}); err != nil {
		t.Fatal(err)
	}
	eventIDs = append(eventIDs, campaignHandoffReceiptEventID(t, ctx, pool, planID))
	err := outboundCampaignHandoffGoose(ctx, repositoryRoot, "down-to", "67")
	if err == nil || !strings.Contains(err.Error(), "55000") {
		t.Fatalf("facts rollback error=%v, want SQLSTATE 55000", err)
	}
	if got := outboundCampaignHandoffMigrationWaterline(t, ctx, pool); got != 68 {
		t.Fatalf("migration waterline after rejected rollback=%d, want 68", got)
	}
	assertCampaignHandoffFactCounts(t, ctx, pool, planID, 1, 1, 1, 1, 1, 0, 0)

	resetOutboundCampaignHandoffFixture(t, ctx, pool, eventIDs)
	eventIDs = nil
	runOutboundCampaignHandoffGoose(t, ctx, repositoryRoot, "down-to", "67")
	if got := outboundCampaignHandoffMigrationWaterline(t, ctx, pool); got != 67 {
		t.Fatalf("migration waterline after empty rollback=%d, want 67", got)
	}
	for _, table := range []string{"outbound_campaign_handoffs", "outbound_campaign_handoff_steps", "outbound_campaign_handoff_customer_tasks", "outbound_campaign_handoff_receipts"} {
		var relation *string
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1)::text`, table).Scan(&relation); err != nil {
			t.Fatal(err)
		}
		if relation != nil {
			t.Fatalf("%s remains after empty rollback: %q", table, *relation)
		}
	}
	runOutboundCampaignHandoffGoose(t, ctx, repositoryRoot, "up-to", "68")
	assertOutboundCampaignHandoffWaterline(t, ctx, pool)
}

func newCampaignHandoffService(t *testing.T, pool *pgxpool.Pool, source outboundport.ApprovedCampaignHandoffSource) *outboundapp.CampaignHandoffService {
	t.Helper()
	events, err := outboundstore.NewCampaignHandoffEventLogAdapter(eventstore.NewAppender())
	if err != nil {
		t.Fatal(err)
	}
	service, err := outboundapp.NewCampaignHandoffService(platformstore.NewUnitOfWork(pool), source, outboundstore.NewCampaignHandoffRepository(), events)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func openOutboundCampaignHandoffPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if *outboundCampaignHandoffDatabaseURL == "" {
		t.Skip("campaign-handoff-database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURLForDatabase(*outboundCampaignHandoffDatabaseURL, acceptancefixtures.OutboundCampaignHandoffDatabaseName); err != nil {
		t.Fatalf("unsafe outbound campaign handoff database URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *outboundCampaignHandoffDatabaseURL)
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

func newCampaignHandoffDeliveryRepository(t *testing.T, pool *pgxpool.Pool) *eventstore.DeliveryRepository {
	t.Helper()
	deferred := eventdispatcher.NewDeferredEnqueuer()
	deliveries, err := eventstore.NewRuntimeDeliveryRepository(pool, deferred, 1, []eventport.DeliveryBinding{{
		EventType: eventport.EvOutboundCampaignHandoffFact, Consumer: eventport.ConsumerOutboundCampaignHandoffFact,
	}})
	if err != nil {
		t.Fatal(err)
	}
	router, err := eventdispatcher.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := outbound.NewCampaignHandoffFactDeliveryConsumer(platformstore.NewUnitOfWork(pool), deliveries)
	if err != nil {
		t.Fatal(err)
	}
	if err = router.RegisterDelivery(consumer); err != nil {
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
	client, err := platformjobqueue.NewClient(pool, platformjobqueue.QueueConcurrency{Critical: 1, Event: 1, Outbound: 1, Sync: 1, Heavy: 1, AI: 1}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if err = deferred.Bind(client); err != nil {
		t.Fatal(err)
	}
	return deliveries
}

func assertOutboundCampaignHandoffWaterline(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var version int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`).Scan(&version); err != nil || version < 68 {
		t.Fatalf("migration waterline=%d err=%v, want at least 68", version, err)
	}
}

func outboundCampaignHandoffMigrationWaterline(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int64 {
	t.Helper()
	var version int64
	if err := pool.QueryRow(ctx, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func outboundCampaignHandoffRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(directory, "migrations")); statErr == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root with migrations directory not found")
		}
		directory = parent
	}
}

func runOutboundCampaignHandoffGoose(t *testing.T, ctx context.Context, repositoryRoot, operation, version string) {
	t.Helper()
	if err := outboundCampaignHandoffGoose(ctx, repositoryRoot, operation, version); err != nil {
		t.Fatal(err)
	}
}

func outboundCampaignHandoffGoose(ctx context.Context, repositoryRoot, operation, version string) error {
	command := exec.CommandContext(ctx, "go", "tool", "-modfile=tools/go.mod", "goose", "-dir", "migrations", "postgres", *outboundCampaignHandoffDatabaseURL, operation, version)
	command.Dir = repositoryRoot
	command.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("goose %s %s: %w: %s", operation, version, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resetOutboundCampaignHandoffFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventIDs []int64) {
	t.Helper()
	truncateTables := `public.outbound_campaign_handoff_receipts,
  public.outbound_campaign_handoff_customer_tasks,
  public.outbound_campaign_handoff_steps,
  public.outbound_campaign_handoffs,
  public.outbound_control_receipts,
  public.outbound_task_job_links,
  public.outbound_send_attempt_history,
  public.outbound_send_attempts,
  public.outbound_batch_chunks,
  public.outbound_enqueue_receipts,
  public.outbound_tasks,
		public.outbound_batches`
	var dispatchTablesExist bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.outbound_campaign_dispatches') IS NOT NULL`).Scan(&dispatchTablesExist); err != nil {
		t.Fatal(err)
	}
	if dispatchTablesExist {
		truncateTables = `public.outbound_campaign_provider_attempt_receipts,
  public.outbound_campaign_dispatch_receipts,
  public.outbound_campaign_dispatches,
  ` + truncateTables
	}
	if _, err := pool.Exec(ctx, `TRUNCATE `+truncateTables); err != nil {
		t.Fatal(err)
	}
	if err := eventsfixture.DeleteCampaignHandoffFacts(ctx, pool, eventIDs); err != nil {
		t.Fatal(err)
	}
}

func campaignHandoffReceiptEventID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, planID string) int64 {
	t.Helper()
	var eventID int64
	if err := pool.QueryRow(ctx, `SELECT event_id FROM public.outbound_campaign_handoff_receipts WHERE plan_id=$1`, planID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	return eventID
}

func assertCampaignHandoffFactCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, planID string, handoffs, receipts, events, links, steps, deliveries, outboundJobs int) {
	t.Helper()
	var gotHandoffs, gotReceipts, gotEvents, gotLinks, gotSteps, gotDeliveries, gotOutboundJobs, gotOutboundTasks int
	err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.outbound_campaign_handoffs WHERE plan_id=$1),
  (SELECT count(*) FROM public.outbound_campaign_handoff_receipts WHERE plan_id=$1),
  (SELECT count(*) FROM public.event_log WHERE event_type=$2 AND payload ->> 'plan_id'=$1),
  (SELECT count(*) FROM public.outbound_campaign_handoff_customer_tasks AS link JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=link.handoff_id WHERE handoff.plan_id=$1 AND link.state='held' AND link.eligibility='not_evaluated' AND link.outbound_task_id IS NULL),
  (SELECT count(*) FROM public.outbound_campaign_handoff_steps AS step JOIN public.outbound_campaign_handoffs AS handoff ON handoff.id=step.handoff_id WHERE handoff.plan_id=$1),
  (SELECT count(*) FROM public.event_deliveries AS delivery JOIN public.event_log AS event ON event.id=delivery.event_id WHERE event.event_type=$2 AND event.payload ->> 'plan_id'=$1),
  (SELECT count(*) FROM public.river_job WHERE queue='outbound'),
  (SELECT count(*) FROM public.outbound_tasks)`, planID, eventport.EvOutboundCampaignHandoffFact).Scan(
		&gotHandoffs, &gotReceipts, &gotEvents, &gotLinks, &gotSteps, &gotDeliveries, &gotOutboundJobs, &gotOutboundTasks)
	if err != nil || gotHandoffs != handoffs || gotReceipts != receipts || gotEvents != events || gotLinks != links || gotSteps != steps || gotDeliveries != deliveries || gotOutboundJobs != outboundJobs || gotOutboundTasks != 0 {
		t.Fatalf("handoff/receipt/event/link/step/delivery/outbound-job/outbound-task=%d/%d/%d/%d/%d/%d/%d/%d err=%v, want %d/%d/%d/%d/%d/%d/%d/0", gotHandoffs, gotReceipts, gotEvents, gotLinks, gotSteps, gotDeliveries, gotOutboundJobs, gotOutboundTasks, err, handoffs, receipts, events, links, steps, deliveries, outboundJobs)
	}
}

func outboundCampaignHandoffPlanID(character rune) string {
	return "ctp_" + strings.Repeat(string(character), 64)
}

func assertOutboundCampaignHandoffSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var postgresError *pgconn.PgError
	if err == nil || !errors.As(err, &postgresError) || postgresError.Code != want {
		t.Fatalf("error=%v, want SQLSTATE %s", err, want)
	}
}

func insertRawCampaignHandoffHeader(t *testing.T, ctx context.Context, tx pgx.Tx, campaignCode, planID string, actorID int64) int64 {
	t.Helper()
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO public.outbound_campaign_handoffs (
  campaign_code,plan_id,review_version,source_digest,target_digest,content_digest,target_count,step_count,status,accepted_by_actor_id,accepted_at
) VALUES ($1,$2,3,decode(repeat('11',32),'hex'),decode(repeat('22',32),'hex'),decode(repeat('33',32),'hex'),1,1,'held',$3,now()) RETURNING id`, campaignCode, planID, actorID).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func insertRawCampaignHandoffChildren(t *testing.T, ctx context.Context, tx pgx.Tx, handoffID int64) {
	t.Helper()
	if _, err := tx.Exec(ctx, `INSERT INTO public.outbound_campaign_handoff_steps (handoff_id,step_index,delay_minutes,content) VALUES ($1,1,0,'immutable local content')`, handoffID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.outbound_campaign_handoff_customer_tasks (handoff_id,customer_id,state,eligibility,outbound_task_id) VALUES ($1,101,'held','not_evaluated',NULL)`, handoffID); err != nil {
		t.Fatal(err)
	}
}

func insertRawCompletedCampaignHandoff(t *testing.T, ctx context.Context, tx pgx.Tx, campaignCode, planID string, actorID int64, tamper string) {
	t.Helper()
	var receiptID int64
	if err := tx.QueryRow(ctx, `INSERT INTO public.outbound_campaign_handoff_receipts
  (actor_id,key_digest,payload_digest,campaign_code,plan_id,created_at)
VALUES ($1,decode(repeat('84',32),'hex'),decode(repeat('85',32),'hex'),$2,$3,now()) RETURNING id`, actorID, campaignCode, planID).Scan(&receiptID); err != nil {
		t.Fatal(err)
	}
	handoffID := insertRawCampaignHandoffHeader(t, ctx, tx, campaignCode, planID, actorID)
	insertRawCampaignHandoffChildren(t, ctx, tx, handoffID)
	var eventID int64
	var err error
	switch tamper {
	case "event-extra-key":
		eventID, err = eventsfixture.AppendCampaignHandoffAcceptedFactWithForbiddenExtraKey(ctx, tx, handoffID, planID)
	case "result-count":
		eventID, err = eventsfixture.AppendCampaignHandoffAcceptedFact(ctx, tx, handoffID, planID)
	default:
		t.Fatalf("unknown Campaign handoff tamper %q", tamper)
	}
	if err != nil {
		t.Fatal(err)
	}
	heldCount := 1
	if tamper == "result-count" {
		heldCount = 0
	}
	_, err = tx.Exec(ctx, `UPDATE public.outbound_campaign_handoff_receipts AS receipt SET
  state='completed',handoff_id=handoff.id,event_id=$2,completed_at=now(),result_snapshot=jsonb_build_object(
    'id',handoff.id,'campaign_code',handoff.campaign_code,'plan_id',handoff.plan_id,'review_version',handoff.review_version,'status',handoff.status,
    'target_count',handoff.target_count,'step_count',handoff.step_count,'held_count',$3::integer,'blocked_count',0,'pending_count',0,
    'not_evaluated_count',1,'eligible_count',0,'inactive_count',0,'contact_policy_count',0,
    'accepted_at_unix_micro',floor(extract(epoch FROM handoff.accepted_at)*1000000)::bigint,
    'local_only',handoff.local_only,'provider_execution_eligible',handoff.provider_execution_eligible,
    'real_external_call_executed',handoff.real_external_call_executed,'delivery_proven',handoff.delivery_proven)
FROM public.outbound_campaign_handoffs AS handoff WHERE receipt.id=$1 AND handoff.id=$4`, receiptID, eventID, heldCount, handoffID)
	if err != nil {
		t.Fatal(err)
	}
}

func assertNoCampaignHandoffPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, planID string) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.outbound_campaign_handoffs WHERE plan_id=$1`, planID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("handoff plan %s count=%d err=%v, want zero", planID, count, err)
	}
}

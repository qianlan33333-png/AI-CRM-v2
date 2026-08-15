package operationcycle_acceptance

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	acceptancefixtures "github.com/qianlan33333-png/AI-CRM-v2/acceptance/fixtures"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	operationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/app"
	operationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/operationcycle/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var operationCycleDatabaseURL = flag.String("database-url", "", "isolated PostgreSQL 16.14 operation-cycle database")
var operationCycleExpectedWaterline = flag.Int("expected-waterline", 42, "expected migration waterline for operation-cycle database")

func TestP4OperationCycleABNormalBoundaryAndOutcomeUnknownTerminal(t *testing.T) {
	pool, ctx := openOperationCyclePool(t)
	service := operationCycleService(t, pool)
	prefix := fmt.Sprintf("p4-operation-cycle-%d", time.Now().UnixNano())
	strategyKey, runKey := "growth-"+prefix, "run-"+prefix
	report := operationapp.ReportCommand{IdempotencyKey: "report-" + prefix, ReporterID: "runner-principal", ClientID: "campaign-agent", Snapshot: map[string]any{
		"schema_version": "operation_cycle_snapshot.v1", "strategy_key": strategyKey, "run_key": runKey, "revision": 1, "strategy_version": 1, "title": "增长运营",
	}}
	accepted, err := service.Report(ctx, report)
	if err != nil || accepted["accepted"] != true || accepted["projection_updated"] != true {
		t.Fatalf("report=%#v err=%v", accepted, err)
	}
	if replay, replayErr := service.Report(ctx, report); replayErr != nil || replay["accepted_revision"] != float64(1) && replay["accepted_revision"] != int32(1) {
		t.Fatalf("report replay=%#v err=%v", replay, replayErr)
	}

	runnerID := "runner-" + prefix
	if _, err = service.Heartbeat(ctx, operationapp.RunnerHeartbeatCommand{RunnerID: runnerID, PrincipalID: "runner-principal", ConnectorVersion: "1.0.0", CodexVersion: "1.0.0", AppServerProtocol: "v1", CompatibilityStatus: "ready", BindingKeys: []string{strategyKey}}); err != nil {
		t.Fatal(err)
	}
	started, err := service.Start(ctx, operationapp.StartCommand{StrategyKey: strategyKey, RunKey: runKey, ActionKey: "refresh", IdempotencyKey: "start-" + prefix, ActorID: "7"})
	if err != nil || started["status"] != "queued" {
		t.Fatalf("start=%#v err=%v", started, err)
	}
	requestID, _ := started["request_id"].(string)
	claimed, err := service.Claim(ctx, runnerID, "runner-principal")
	if err != nil || claimed["claimed"] != true {
		t.Fatalf("claim=%#v err=%v", claimed, err)
	}
	leaseToken, _ := claimed["lease_token"].(string)
	for _, event := range []operationapp.ActionEventCommand{
		{RequestID: requestID, EventID: "bound-" + prefix, EventType: "thread_bound", LeaseToken: leaseToken, ThreadID: "thread-" + prefix},
		{RequestID: requestID, EventID: "turn-" + prefix, EventType: "turn_started", LeaseToken: leaseToken, ThreadID: "thread-" + prefix, TurnID: "turn-" + prefix},
		{RequestID: requestID, EventID: "complete-" + prefix, EventType: "completed", LeaseToken: leaseToken, Result: map[string]any{"outcome": "outcome_unknown"}},
	} {
		if _, err = service.RecordActionEvent(ctx, event); err != nil {
			t.Fatalf("%s: %v", event.EventType, err)
		}
	}
	if _, err = service.RecordActionEvent(ctx, operationapp.ActionEventCommand{RequestID: requestID, EventID: "complete-" + prefix, EventType: "completed", LeaseToken: leaseToken, Result: map[string]any{"outcome": "outcome_unknown"}}); err != nil {
		t.Fatalf("completed replay: %v", err)
	}
	if _, err = service.RecordActionEvent(ctx, operationapp.ActionEventCommand{RequestID: requestID, EventID: "after-terminal-" + prefix, EventType: "failed", LeaseToken: leaseToken, Result: map[string]any{"outcome": "outcome_unknown"}}); !errors.Is(err, operationapp.ErrConflict) {
		t.Fatalf("terminal outcome_unknown event error=%v, want conflict", err)
	}

	proposal, err := service.CreateProposal(ctx, operationapp.ProposalCommand{IdempotencyKey: "proposal-" + prefix, ActorID: "campaign-agent", Payload: map[string]any{
		"schema_version": "operation_cycle_strategy_change_proposal.v1", "strategy_key": strategyKey, "base_strategy_version": 1, "change": "manual-review",
	}})
	if err != nil || proposal["status"] != "pending" {
		t.Fatalf("proposal=%#v err=%v", proposal, err)
	}
	proposalID, _ := proposal["proposal_id"].(string)
	if decided, decideErr := service.DecideProposal(ctx, proposalID, "accept", "7"); decideErr != nil || decided["status"] != "accepted" {
		t.Fatalf("proposal decision=%#v err=%v", decided, decideErr)
	}
	if _, err = service.DecideProposal(ctx, proposalID, "reject", "7"); !errors.Is(err, operationapp.ErrConflict) {
		t.Fatalf("repeat proposal decision error=%v, want conflict", err)
	}

	var status, outcome string
	var actionEvents, proposalRows, factEvents, deliveries, forbiddenScopeColumns int
	err = pool.QueryRow(ctx, `SELECT
  (SELECT status FROM operation_cycle_action_requests WHERE request_id=$1),
  (SELECT final_result->>'outcome' FROM operation_cycle_action_requests WHERE request_id=$1),
  (SELECT count(*) FROM operation_cycle_action_request_events WHERE request_id=$1),
  (SELECT count(*) FROM operation_cycle_strategy_proposals WHERE proposal_id=$2),
  (SELECT count(*) FROM event_log WHERE event_type='operation_cycle.fact_recorded' AND occurred_at >= now()-interval '5 minutes'),
  (SELECT count(*) FROM event_deliveries delivery JOIN event_log event ON event.id=delivery.event_id WHERE event.event_type='operation_cycle.fact_recorded' AND event.occurred_at >= now()-interval '5 minutes'),
	  (SELECT count(*) FROM pg_attribute attribute JOIN pg_class class ON class.oid=attribute.attrelid WHERE class.relname LIKE 'operation_cycle_%' AND attribute.attnum>0 AND NOT attribute.attisdropped AND attribute.attname ILIKE ('%' || 'ten' || 'ant%'))`, requestID, proposalID).Scan(&status, &outcome, &actionEvents, &proposalRows, &factEvents, &deliveries, &forbiddenScopeColumns)
	if err != nil || status != "completed" || outcome != "outcome_unknown" || actionEvents != 3 || proposalRows != 1 || factEvents < 9 || deliveries < 9 || forbiddenScopeColumns != 0 {
		t.Fatalf("status/outcome/events/proposals/facts/deliveries/forbidden_scope/error=%s/%s/%d/%d/%d/%d/%d/%v", status, outcome, actionEvents, proposalRows, factEvents, deliveries, forbiddenScopeColumns, err)
	}
}

func TestP4OperationCycleABConcurrentSameKeyCreatesOneActionFact(t *testing.T) {
	pool, ctx := openOperationCyclePool(t)
	service := operationCycleService(t, pool)
	prefix := fmt.Sprintf("p4-operation-cycle-race-%d", time.Now().UnixNano())
	strategyKey, runKey := "race-"+prefix, "run-"+prefix
	if _, err := service.Report(ctx, operationapp.ReportCommand{IdempotencyKey: "report-" + prefix, ReporterID: "runner-principal", ClientID: "campaign-agent", Snapshot: map[string]any{"schema_version": "operation_cycle_snapshot.v1", "strategy_key": strategyKey, "run_key": runKey, "revision": 1}}); err != nil {
		t.Fatal(err)
	}
	runnerID := "runner-" + prefix
	if _, err := service.Heartbeat(ctx, operationapp.RunnerHeartbeatCommand{RunnerID: runnerID, PrincipalID: "runner-principal", ConnectorVersion: "1", CodexVersion: "1", AppServerProtocol: "v1", CompatibilityStatus: "ready", BindingKeys: []string{strategyKey}}); err != nil {
		t.Fatal(err)
	}
	command := operationapp.StartCommand{StrategyKey: strategyKey, RunKey: runKey, ActionKey: "refresh", IdempotencyKey: "start-" + prefix, ActorID: "7"}
	const workers = 8
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, callErr := service.Start(ctx, command)
			errorsByWorker <- callErr
		}()
	}
	close(start)
	group.Wait()
	close(errorsByWorker)
	for callErr := range errorsByWorker {
		if callErr != nil {
			t.Fatalf("same-key race error=%v", callErr)
		}
	}
	var actions, queuedFacts, actionDeliveries int
	err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM operation_cycle_action_requests WHERE strategy_key=$1),
  (SELECT count(*) FROM event_log WHERE event_type='operation_cycle.fact_recorded' AND idempotency_key=$2),
  (SELECT count(*) FROM event_deliveries delivery JOIN event_log event ON event.id=delivery.event_id WHERE event.idempotency_key=$2)`, strategyKey, "operation_cycle:"+command.IdempotencyKey).Scan(&actions, &queuedFacts, &actionDeliveries)
	if err != nil || actions != 1 || queuedFacts != 1 || actionDeliveries != 1 {
		t.Fatalf("actions/facts/deliveries/error=%d/%d/%d/%v", actions, queuedFacts, actionDeliveries, err)
	}
}

func operationCycleService(t *testing.T, pool *pgxpool.Pool) *operationapp.Service {
	t.Helper()
	deliveries, err := eventstore.NewProducerDeliveryRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	return operationapp.NewService(platformstore.NewUnitOfWork(pool), operationstore.NewRepository(), eventstore.NewAppender(), deliveries)
}

func openOperationCyclePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	if *operationCycleDatabaseURL == "" {
		t.Skip("database-url is not set")
	}
	if err := acceptancefixtures.ValidateDatabaseURL(*operationCycleDatabaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, *operationCycleDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil || version != "160014" {
		t.Fatalf("postgres version=%q err=%v", version, err)
	}
	var waterline int
	if err = pool.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&waterline); err != nil || waterline != *operationCycleExpectedWaterline {
		t.Fatalf("waterline=%d err=%v", waterline, err)
	}
	if err = platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("river migration=%v", err)
	}
	return pool, ctx
}

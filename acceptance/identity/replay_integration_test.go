package identity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitystore "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestPendingReplayAttributesOriginalFactAndCompletesAtomically(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	pending, command := createReplayPending(t, pool, "complete")
	customerID := createBindCustomer(t, pool)
	bindReplayPendingIdentities(t, pool, pending, customerID)
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newPendingReplayService(pool, recorder)

	result, err := service.ReplayOnce(ctx)
	if err != nil || result.Status != identityapp.PendingReplayCompleted || result.PendingEventID != pending ||
		result.CustomerID != contactport.CustomerID(customerID) || result.EventID <= 0 {
		t.Fatalf("ReplayOnce()=%+v err=%v", result, err)
	}
	assertReplayedPending(t, pool, pending, 2)
	assertReplayTimeline(t, pool, command, customerID, result.EventID, 1)
	if countIngestRecordedEvents(recorder.Events(), command.EventType) != 1 {
		t.Fatalf("recorded events=%+v", recorder.Events())
	}

	again, err := service.ReplayOnce(ctx)
	if err != nil || again.Status != identityapp.PendingReplayIdle {
		t.Fatalf("second ReplayOnce()=%+v err=%v", again, err)
	}
	assertReplayTimeline(t, pool, command, customerID, result.EventID, 1)
}

func TestPendingReplayUnresolvedFactStaysRetryableThenCompletes(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	pending, command := createReplayPending(t, pool, "retryable")
	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	service := newPendingReplayService(pool, recorder)

	first, err := service.ReplayOnce(ctx)
	if err != nil || first.Status != identityapp.PendingReplayRetryable || first.PendingEventID != pending {
		t.Fatalf("unresolved ReplayOnce()=%+v err=%v", first, err)
	}
	assertPendingState(t, pool, pending, "pending", 2, false)
	assertReplayTimeline(t, pool, command, 0, 0, 0)

	customerID := createBindCustomer(t, pool)
	bindReplayPendingIdentities(t, pool, pending, customerID)
	second, err := service.ReplayOnce(ctx)
	if err != nil || second.Status != identityapp.PendingReplayCompleted || second.CustomerID != contactport.CustomerID(customerID) {
		t.Fatalf("resolved ReplayOnce()=%+v err=%v", second, err)
	}
	assertReplayedPending(t, pool, pending, 3)
	assertReplayTimeline(t, pool, command, customerID, second.EventID, 1)
}

func TestPendingReplayDefersUnresolvedFactWithoutStarvingReadyFact(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	unresolved, unresolvedCommand := createReplayPending(t, pool, "fairness-unresolved")
	ready, readyCommand := createReplayPending(t, pool, "fairness-ready")
	if unresolved >= ready {
		t.Fatalf("pending order unresolved=%d ready=%d", unresolved, ready)
	}
	customerID := createBindCustomer(t, pool)
	bindReplayPendingIdentities(t, pool, ready, customerID)
	service := newPendingReplayService(pool, &recordingEventAppender{delegate: eventstore.NewAppender()})

	first, err := service.ReplayOnce(ctx)
	if err != nil || first.Status != identityapp.PendingReplayRetryable || first.PendingEventID != unresolved {
		t.Fatalf("first ReplayOnce()=%+v err=%v", first, err)
	}
	second, err := service.ReplayOnce(ctx)
	if err != nil || second.Status != identityapp.PendingReplayCompleted || second.PendingEventID != ready ||
		second.CustomerID != contactport.CustomerID(customerID) {
		t.Fatalf("second ReplayOnce()=%+v err=%v", second, err)
	}
	assertPendingState(t, pool, unresolved, "pending", 2, false)
	assertReplayTimeline(t, pool, unresolvedCommand, 0, 0, 0)
	assertReplayedPending(t, pool, ready, 2)
	assertReplayTimeline(t, pool, readyCommand, customerID, second.EventID, 1)
}

func TestPendingReplayEventFailureRollsBackTimelineAndRemainsRetryable(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	pending, command := createReplayPending(t, pool, "rollback")
	customerID := createBindCustomer(t, pool)
	bindReplayPendingIdentities(t, pool, pending, customerID)

	if _, err := newPendingReplayService(pool, failingEventAppender{}).ReplayOnce(ctx); err == nil {
		t.Fatal("ReplayOnce succeeded while event append failed")
	}
	assertPendingState(t, pool, pending, "pending", 1, false)
	assertReplayTimeline(t, pool, command, 0, 0, 0)

	recorder := &recordingEventAppender{delegate: eventstore.NewAppender()}
	result, err := newPendingReplayService(pool, recorder).ReplayOnce(ctx)
	if err != nil || result.Status != identityapp.PendingReplayCompleted {
		t.Fatalf("retry ReplayOnce()=%+v err=%v", result, err)
	}
	assertReplayedPending(t, pool, pending, 2)
	assertReplayTimeline(t, pool, command, customerID, result.EventID, 1)
}

func TestPendingReplayConcurrentClaimProducesOneTimelineAndEvent(t *testing.T) {
	pool := openIdentityPool(t)
	resetIdentityUpsert(t, pool)
	ctx := context.Background()
	pending, command := createReplayPending(t, pool, "concurrent")
	customerID := createBindCustomer(t, pool)
	bindReplayPendingIdentities(t, pool, pending, customerID)
	secondPool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondPool.Close)
	recorders := []*recordingEventAppender{{delegate: eventstore.NewAppender()}, {delegate: eventstore.NewAppender()}}
	services := []*identityapp.PendingReplayService{
		newPendingReplayService(pool, recorders[0]), newPendingReplayService(secondPool, recorders[1]),
	}
	results := make([]identityapp.PendingReplayResult, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range services {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errs[index] = services[index].ReplayOnce(ctx)
		}(index)
	}
	close(start)
	wait.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("concurrent errors=%v/%v", errs[0], errs[1])
	}
	completed, idle := 0, 0
	var eventID contactport.EventID
	for _, result := range results {
		switch result.Status {
		case identityapp.PendingReplayCompleted:
			completed++
			eventID = result.EventID
		case identityapp.PendingReplayIdle:
			idle++
		default:
			t.Fatalf("unexpected concurrent result=%+v", result)
		}
	}
	if completed != 1 || idle != 1 {
		t.Fatalf("concurrent results=%+v", results)
	}
	assertReplayedPending(t, pool, pending, 2)
	assertReplayTimeline(t, pool, command, customerID, eventID, 1)
	if events := countIngestRecordedEvents(recorders[0].Events(), command.EventType) + countIngestRecordedEvents(recorders[1].Events(), command.EventType); events != 1 {
		t.Fatalf("recorded event count=%d", events)
	}
}

func newPendingReplayService(pool *pgxpool.Pool, events eventport.Appender) *identityapp.PendingReplayService {
	uow := platformstore.NewUnitOfWork(pool)
	repository := identitystore.NewRepository()
	ingest := identityapp.NewIngestService(uow, repository, contactstore.NewMergePortRepository(), events, ingestReceiptKey)
	return identityapp.NewPendingReplayService(uow, repository, ingest)
}

func createReplayPending(t *testing.T, pool *pgxpool.Pool, name string) (int64, identityport.IngestCommand) {
	t.Helper()
	suffix := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	ref := identityport.IDRef{
		Kind: identityport.KindExtension, Scope: "ext:replay-" + suffix, Value: "fact", Assurance: identityport.AssuranceVerified,
		Source: "acceptance.replay",
	}
	command := identityport.IngestCommand{
		Refs: []identityport.IDRef{ref}, EventType: "identity.replay." + suffix,
		Payload: json.RawMessage(`{"answer":42,"source_fact":"preserved"}`), Source: ref.Source,
		OccurredAt:     time.Date(2026, 8, 13, 2, 3, 4, 567890123, time.FixedZone("CST", 8*60*60)),
		IdempotencyKey: "identity-replay-" + suffix,
	}
	result, err := newIdentityIngestService(pool, eventstore.NewAppender()).Ingest(context.Background(), command)
	if err != nil || result.Status != identityport.IngestPending || result.PendingEventID <= 0 {
		t.Fatalf("create pending result=%+v err=%v", result, err)
	}
	return result.PendingEventID, command
}

func bindReplayPendingIdentities(t *testing.T, pool *pgxpool.Pool, pendingID, customerID int64) {
	t.Helper()
	result, err := pool.Exec(context.Background(), `
UPDATE identities SET customer_id=$1::bigint, bound_at=now()
WHERE id = ANY(ARRAY(SELECT unnest(identity_ids) FROM pending_events WHERE id=$2::bigint))
  AND customer_id IS NULL`, customerID, pendingID)
	if err != nil || result.RowsAffected() < 1 {
		t.Fatalf("bind pending identities affected=%d err=%v", result.RowsAffected(), err)
	}
}

func assertReplayedPending(t *testing.T, pool *pgxpool.Pool, pendingID, version int64) {
	t.Helper()
	assertPendingState(t, pool, pendingID, "replayed", version, true)
}

func assertPendingState(t *testing.T, pool *pgxpool.Pool, pendingID int64, state string, version int64, resolved bool) {
	t.Helper()
	var gotState string
	var gotVersion int64
	var hasResolved bool
	err := pool.QueryRow(context.Background(), `
SELECT state,version,resolved_at IS NOT NULL FROM pending_events WHERE id=$1::bigint`, pendingID).
		Scan(&gotState, &gotVersion, &hasResolved)
	if err != nil || gotState != state || gotVersion != version || hasResolved != resolved {
		t.Fatalf("pending state=%q version=%d resolved=%v err=%v", gotState, gotVersion, hasResolved, err)
	}
}

func assertReplayTimeline(
	t *testing.T,
	pool *pgxpool.Pool,
	command identityport.IngestCommand,
	customerID int64,
	eventID contactport.EventID,
	want int,
) {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `
SELECT count(*) FROM customer_events
WHERE event_type=$1::text
  AND ($2::bigint=0 OR customer_id=$2::bigint)
  AND ($3::bigint=0 OR id=$3::bigint)
  AND payload=$4::jsonb
  AND actor=$5::text
  AND occurred_at=$6::timestamptz`, command.EventType, customerID, eventID, command.Payload, command.Source,
		command.OccurredAt.UTC().Truncate(time.Microsecond)).Scan(&count)
	if err != nil || count != want {
		t.Fatalf("timeline count=%d err=%v, want %d", count, err, want)
	}
}

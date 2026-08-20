package contact_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformriver "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/river"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestB02ABTagExecutionQueuesExactlyOnceWithoutProviderCall(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("river migration: %v", err)
	}
	repository, err := contactstore.NewLegacyTagExecutionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	uow := platformstore.NewUnitOfWork(pool)
	syncService := contactapp.NewLegacyTagSyncService(uow, repository, eventstore.NewAppender(), repository)
	liveService := contactapp.NewLegacyTagLiveMutationService(uow, repository, eventstore.NewAppender(), repository)
	statusService := contactapp.NewLegacyTagExecutionStatusService(uow, repository)
	key := fmt.Sprintf("b02ab-sync-%d", time.Now().UnixNano())
	command := contactapp.LegacyTagSyncCommand{Actor: 711, IdempotencyKey: key, TraceID: "tag-sync", Kind: contactapp.LegacyTagSyncManual}
	accepted, err := syncService.Request(ctx, command)
	if err != nil || accepted.State != contactapp.LegacyTagSyncQueued || accepted.ReceiptID <= 0 || accepted.EventID <= 0 || accepted.RiverJobID <= 0 {
		t.Fatalf("sync accepted=%#v err=%v", accepted, err)
	}
	replayed, err := syncService.Request(ctx, command)
	if err != nil || replayed != accepted {
		t.Fatalf("sync replay=%#v err=%v", replayed, err)
	}
	if _, err = syncService.Request(ctx, contactapp.LegacyTagSyncCommand{Actor: command.Actor, IdempotencyKey: key, TraceID: command.TraceID, Kind: contactapp.LegacyTagSyncDue}); !errors.Is(err, contactapp.ErrLegacyTagSyncConflict) {
		t.Fatalf("sync conflict=%v", err)
	}

	liveKey := fmt.Sprintf("b02ab-live-%d", time.Now().UnixNano())
	liveCommand := contactapp.LegacyTagLiveMutationCommand{Actor: 712, IdempotencyKey: liveKey, TraceID: "tag-live", Operation: contactapp.LegacyTagLiveMutationMark, Payload: json.RawMessage(`{"tag_id":2,"external_userid":"u-1","score":1}`)}
	liveAccepted, err := liveService.Request(ctx, liveCommand)
	if err != nil || liveAccepted.State != contactapp.LegacyTagLiveMutationQueued || liveAccepted.ReceiptID <= 0 || liveAccepted.EventID <= 0 || liveAccepted.RiverJobID <= 0 {
		t.Fatalf("live accepted=%#v err=%v", liveAccepted, err)
	}
	liveReplay := liveCommand
	liveReplay.Payload = json.RawMessage(`{"score":1.0,"external_userid":"u-1","tag_id":2}`)
	replayedLive, err := liveService.Request(ctx, liveReplay)
	if err != nil || replayedLive != liveAccepted {
		t.Fatalf("live replay=%#v err=%v", replayedLive, err)
	}
	if _, err = liveService.Request(ctx, contactapp.LegacyTagLiveMutationCommand{Actor: liveCommand.Actor, IdempotencyKey: liveKey, TraceID: liveCommand.TraceID, Operation: contactapp.LegacyTagLiveMutationUnmark, Payload: liveCommand.Payload}); !errors.Is(err, contactapp.ErrLegacyTagLiveMutationConflict) {
		t.Fatalf("live conflict=%v", err)
	}

	status, err := statusService.Get(ctx)
	if err != nil || status.ProviderExecutionEligible || !status.LocalCommandAcceptanceAvailable || !status.LocalQueueAvailable || status.SyncExecuted || status.RealExternalCallExecuted || status.ObservedAt.IsZero() {
		t.Fatalf("safe gate=%#v err=%v", status, err)
	}
	if contactapp.LegacyTagSyncCanAutoRetry(contactapp.LegacyTagSyncOutcomeUnknown) || contactapp.LegacyTagLiveMutationCanAutoRetry(contactapp.LegacyTagLiveMutationOutcomeUnknown) {
		t.Fatal("outcome_unknown must not auto-retry")
	}

	var syncState, liveState, syncKind, liveKind, syncQueue, liveQueue string
	var eventCount, syncReceipts, liveReceipts int
	err = pool.QueryRow(ctx, `SELECT
  (SELECT state FROM legacy_tag_sync_receipts WHERE id=$1),
  (SELECT state FROM legacy_tag_live_mutation_receipts WHERE id=$2),
  (SELECT kind FROM river_job WHERE id=$3),
  (SELECT kind FROM river_job WHERE id=$4),
  (SELECT queue FROM river_job WHERE id=$3),
  (SELECT queue FROM river_job WHERE id=$4),
  (SELECT count(*) FROM event_log WHERE id IN ($5,$6)),
  (SELECT count(*) FROM legacy_tag_sync_receipts WHERE actor_id=$7 AND idempotency_key=$8),
  (SELECT count(*) FROM legacy_tag_live_mutation_receipts WHERE actor_id=$9 AND idempotency_key=$10)`,
		accepted.ReceiptID, liveAccepted.ReceiptID, accepted.RiverJobID, liveAccepted.RiverJobID, accepted.EventID, liveAccepted.EventID, command.Actor, key, liveCommand.Actor, liveKey,
	).Scan(&syncState, &liveState, &syncKind, &liveKind, &syncQueue, &liveQueue, &eventCount, &syncReceipts, &liveReceipts)
	if err != nil || syncState != "queued" || liveState != "queued" || syncKind != contactapp.LegacyTagSyncJobKind || liveKind != contactapp.LegacyTagLiveMutationJobKind || syncQueue != "sync" || liveQueue != "sync" || eventCount != 2 || syncReceipts != 1 || liveReceipts != 1 {
		t.Fatalf("facts=%q/%q %q/%q %q/%q event=%d receipt=%d/%d err=%v", syncState, liveState, syncKind, liveKind, syncQueue, liveQueue, eventCount, syncReceipts, liveReceipts, err)
	}
}

func TestB02ABTagExecutionConcurrentReplayAndIncompleteReceiptRollback(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	if err := platformriver.Migrate(ctx, pool, platformriver.DirectionUp, nil); err != nil {
		t.Fatalf("river migration: %v", err)
	}
	repository, err := contactstore.NewLegacyTagExecutionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	service := contactapp.NewLegacyTagSyncService(platformstore.NewUnitOfWork(pool), repository, eventstore.NewAppender(), repository)
	key := fmt.Sprintf("b02ab-race-%d", time.Now().UnixNano())
	command := contactapp.LegacyTagSyncCommand{Actor: 713, IdempotencyKey: key, Kind: contactapp.LegacyTagSyncManual}
	const callers = 8
	start := make(chan struct{})
	results := make(chan contactapp.LegacyTagSyncAcceptance, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, requestErr := service.Request(ctx, command)
			results <- result
			errs <- requestErr
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errs)
	var first contactapp.LegacyTagSyncAcceptance
	for result := range results {
		if first.ReceiptID == 0 {
			first = result
		} else if result != first {
			t.Fatalf("race result=%#v want=%#v", result, first)
		}
	}
	for requestErr := range errs {
		if requestErr != nil {
			t.Fatalf("race error=%v", requestErr)
		}
	}
	var receipts, jobs, events int
	if err = pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM legacy_tag_sync_receipts WHERE actor_id=$1 AND idempotency_key=$2 AND state='queued'),
  (SELECT count(*) FROM river_job WHERE id=$3 AND kind=$4 AND queue='sync'),
  (SELECT count(*) FROM event_log WHERE id=$5)`, command.Actor, key, first.RiverJobID, contactapp.LegacyTagSyncJobKind, first.EventID).Scan(&receipts, &jobs, &events); err != nil || receipts != 1 || jobs != 1 || events != 1 {
		t.Fatalf("race facts=%d/%d/%d err=%v", receipts, jobs, events, err)
	}

	incompleteKey := fmt.Sprintf("b02ab-incomplete-%d", time.Now().UnixNano())
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO legacy_tag_sync_receipts(actor_id,idempotency_key,key_digest,kind,trace_id) VALUES ($1,$2,decode(repeat('ab',32),'hex'),'manual','')`, 714, incompleteKey)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err == nil {
		t.Fatal("incomplete receipt committed")
	}
	_ = tx.Rollback(ctx)
	var incomplete int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM legacy_tag_sync_receipts WHERE actor_id=714 AND idempotency_key=$1`, incompleteKey).Scan(&incomplete); err != nil || incomplete != 0 {
		t.Fatalf("incomplete receipt count=%d err=%v", incomplete, err)
	}

	immutableKey := fmt.Sprintf("b02ab-immutable-%d", time.Now().UnixNano())
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO legacy_tag_sync_receipts(actor_id,idempotency_key,key_digest,kind,trace_id) VALUES ($1,$2,decode(repeat('cd',32),'hex'),'manual','trace')`, 716, immutableKey)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE legacy_tag_sync_receipts SET kind='due',state='queued',event_id=1,river_job_id=1,accepted_at=now() WHERE actor_id=716 AND idempotency_key=$1`, immutableKey)
	}
	if err == nil {
		t.Fatal("receipt kind changed during acceptance")
	}
	_ = tx.Rollback(ctx)
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM legacy_tag_sync_receipts WHERE actor_id=716 AND idempotency_key=$1`, immutableKey).Scan(&incomplete); err != nil || incomplete != 0 {
		t.Fatalf("immutable receipt count=%d err=%v", incomplete, err)
	}
}

type b02ABFailingEvents struct{}

func (b02ABFailingEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, errors.New("event append blocked")
}

func TestB02ABTagExecutionEventFailureRollsBackAcceptance(t *testing.T) {
	pool, ctx := c01OpenPool(t)
	repository, err := contactstore.NewLegacyTagExecutionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("b02ab-event-rollback-%d", time.Now().UnixNano())
	command := contactapp.LegacyTagSyncCommand{Actor: 715, IdempotencyKey: key, Kind: contactapp.LegacyTagSyncManual}
	_, err = contactapp.NewLegacyTagSyncService(platformstore.NewUnitOfWork(pool), repository, b02ABFailingEvents{}, repository).Request(ctx, command)
	if !errors.Is(err, contactapp.ErrLegacyTagSyncFailed) {
		t.Fatalf("event failure=%v", err)
	}
	var receipts int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM legacy_tag_sync_receipts WHERE actor_id=$1 AND idempotency_key=$2`, command.Actor, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("rollback receipts=%d err=%v", receipts, err)
	}
}

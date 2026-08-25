package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type legacyTagSyncReceipts struct {
	u            *legacyTagUOW
	reserved     LegacyTagSyncReceipt
	accepted     LegacyTagSyncReceipt
	reserveCalls int
	acceptCalls  int
	reserveErr   error
	acceptErr    error
}

func (store *legacyTagSyncReceipts) ReserveLegacyTagSync(_ context.Context, command LegacyTagSyncCommand) (LegacyTagSyncReceipt, error) {
	if store.u == nil || !store.u.in {
		return LegacyTagSyncReceipt{}, errors.New("reserve outside uow")
	}
	store.reserveCalls++
	if store.reserveErr != nil {
		return LegacyTagSyncReceipt{}, store.reserveErr
	}
	return store.reserved, nil
}

func (store *legacyTagSyncReceipts) AcceptLegacyTagSync(_ context.Context, receiptID int64, eventID eventport.EventID, jobID int64) (LegacyTagSyncReceipt, error) {
	if store.u == nil || !store.u.in {
		return LegacyTagSyncReceipt{}, errors.New("accept outside uow")
	}
	store.acceptCalls++
	if store.acceptErr != nil {
		return LegacyTagSyncReceipt{}, store.acceptErr
	}
	if receiptID != store.reserved.ID || eventID != store.accepted.EventID || jobID != store.accepted.RiverJobID {
		return LegacyTagSyncReceipt{}, errors.New("unexpected sync acceptance facts")
	}
	return store.accepted, nil
}

type legacyTagSyncJobs struct {
	u     *legacyTagUOW
	job   LegacyTagSyncJob
	id    int64
	calls int
	err   error
}

func (jobs *legacyTagSyncJobs) EnqueueLegacyTagSync(_ context.Context, job LegacyTagSyncJob) (int64, error) {
	if jobs.u == nil || !jobs.u.in {
		return 0, errors.New("enqueue outside uow")
	}
	jobs.calls++
	jobs.job = job
	return jobs.id, jobs.err
}

func TestP4CustomerTagsAcceptsManualAndDueSyncInsideOneUOW(t *testing.T) {
	for _, command := range []LegacyTagSyncCommand{p4CustomerTagSyncCommand(LegacyTagSyncManual), p4CustomerTagSyncCommand(LegacyTagSyncDue)} {
		t.Run(string(command.Kind), func(t *testing.T) {
			u := &legacyTagUOW{}
			events := &legacyTagEvents{u: u}
			receipts := &legacyTagSyncReceipts{u: u, reserved: LegacyTagSyncReceipt{ID: 41, Command: command, State: LegacyTagSyncReceiptReserved}, accepted: LegacyTagSyncReceipt{ID: 41, Command: command, State: LegacyTagSyncReceiptQueued, EventID: 1, RiverJobID: 43}}
			jobs := &legacyTagSyncJobs{u: u, id: 43}
			service := NewLegacyTagSyncService(u, receipts, events, jobs)
			service.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }

			got, err := service.Request(context.Background(), command)
			if err != nil || got != (LegacyTagSyncAcceptance{ReceiptID: 41, EventID: 1, RiverJobID: 43, State: LegacyTagSyncQueued}) {
				t.Fatalf("Request() = %#v, %v", got, err)
			}
			if u.calls != 1 || receipts.reserveCalls != 1 || receipts.acceptCalls != 1 || jobs.calls != 1 || len(events.items) != 1 {
				t.Fatalf("uow/reserve/accept/jobs/events = %d/%d/%d/%d/%d", u.calls, receipts.reserveCalls, receipts.acceptCalls, jobs.calls, len(events.items))
			}
			if jobs.job != (LegacyTagSyncJob{ReceiptID: 41, SyncKind: command.Kind, TraceID: command.TraceID}) || events.items[0].Type != legacyTagSyncAcceptedEvent || events.items[0].CustomerID != 0 {
				t.Fatalf("job/event = %#v/%#v", jobs.job, events.items[0])
			}
		})
	}
}

func TestP4CustomerTagsSyncReplayReturnsQueuedFactsWithoutASecondJob(t *testing.T) {
	command := p4CustomerTagSyncCommand(LegacyTagSyncManual)
	u := &legacyTagUOW{}
	events := &legacyTagEvents{u: u}
	receipts := &legacyTagSyncReceipts{u: u, reserved: LegacyTagSyncReceipt{ID: 41, Command: command, State: LegacyTagSyncReceiptQueued, EventID: 42, RiverJobID: 43}}
	jobs := &legacyTagSyncJobs{u: u, id: 99}

	got, err := NewLegacyTagSyncService(u, receipts, events, jobs).Request(context.Background(), command)
	if err != nil || got != (LegacyTagSyncAcceptance{ReceiptID: 41, EventID: 42, RiverJobID: 43, State: LegacyTagSyncQueued}) {
		t.Fatalf("Request() = %#v, %v", got, err)
	}
	if receipts.acceptCalls != 0 || jobs.calls != 0 || len(events.items) != 0 {
		t.Fatalf("accept/jobs/events = %d/%d/%d, want 0/0/0", receipts.acceptCalls, jobs.calls, len(events.items))
	}
}

func TestP4CustomerTagsSyncCommitHookRunsInsideUOWAndPropagatesFailure(t *testing.T) {
	command := p4CustomerTagSyncCommand(LegacyTagSyncManual)
	u := &legacyTagUOW{}
	receipts := &legacyTagSyncReceipts{u: u, reserved: LegacyTagSyncReceipt{ID: 41, Command: command, State: LegacyTagSyncReceiptReserved}, accepted: LegacyTagSyncReceipt{ID: 41, Command: command, State: LegacyTagSyncReceiptQueued, EventID: 1, RiverJobID: 43}}
	hookErr := errors.New("typed queue failed")
	_, err := NewLegacyTagSyncService(u, receipts, &legacyTagEvents{u: u}, &legacyTagSyncJobs{u: u, id: 43}).RequestWithCommitHook(context.Background(), command, func(_ context.Context, acceptance LegacyTagSyncAcceptance, replay bool) error {
		if !u.in || replay || acceptance.ReceiptID != 41 || acceptance.State != LegacyTagSyncQueued {
			t.Fatalf("hook acceptance/uow = %#v/%v", acceptance, u.in)
		}
		return hookErr
	})
	if !errors.Is(err, hookErr) {
		t.Fatalf("RequestWithCommitHook() error = %v, want hook failure", err)
	}
}

func TestP4CustomerTagsSyncRejectsIdempotencyConflictAndInvalidInputBeforeQueueing(t *testing.T) {
	command := p4CustomerTagSyncCommand(LegacyTagSyncManual)
	stored := command
	stored.Kind = LegacyTagSyncDue
	u := &legacyTagUOW{}
	events := &legacyTagEvents{u: u}
	receipts := &legacyTagSyncReceipts{u: u, reserved: LegacyTagSyncReceipt{ID: 41, Command: stored, State: LegacyTagSyncReceiptQueued, EventID: 42, RiverJobID: 43}}
	jobs := &legacyTagSyncJobs{u: u, id: 43}
	service := NewLegacyTagSyncService(u, receipts, events, jobs)
	if _, err := service.Request(context.Background(), command); !errors.Is(err, ErrLegacyTagSyncConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	if jobs.calls != 0 || len(events.items) != 0 {
		t.Fatalf("conflict queued job/event = %d/%d", jobs.calls, len(events.items))
	}

	for _, invalid := range []LegacyTagSyncCommand{{}, {Actor: 7, IdempotencyKey: " key", Kind: LegacyTagSyncManual}, {Actor: 7, IdempotencyKey: "key", Kind: "unknown"}} {
		if _, err := service.Request(context.Background(), invalid); !errors.Is(err, ErrInvalidLegacyTagSync) {
			t.Fatalf("invalid command %#v error = %v", invalid, err)
		}
	}
}

func TestP4CustomerTagsSyncDoesNotRetryUnknownOutcomeAutomatically(t *testing.T) {
	for _, state := range []LegacyTagSyncState{LegacyTagSyncAttempted, LegacyTagSyncExecuted, LegacyTagSyncOutcomeUnknown, LegacyTagSyncReconciled} {
		if LegacyTagSyncCanAutoRetry(state) {
			t.Fatalf("state %q must not automatically retry", state)
		}
	}
	if !LegacyTagSyncCanAutoRetry(LegacyTagSyncQueued) {
		t.Fatal("queued job must remain eligible for first worker delivery")
	}
}

func p4CustomerTagSyncCommand(kind LegacyTagSyncKind) LegacyTagSyncCommand {
	return LegacyTagSyncCommand{Actor: 7, IdempotencyKey: "sync-request-001", TraceID: "trace-sync-001", Kind: kind}
}

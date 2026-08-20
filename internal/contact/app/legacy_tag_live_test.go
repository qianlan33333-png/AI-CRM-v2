package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type legacyTagExecutionStatusReader struct {
	u      *legacyTagUOW
	status LegacyTagExecutionStatus
	err    error
	calls  int
}

func (reader *legacyTagExecutionStatusReader) ReadLegacyTagExecutionStatus(context.Context) (LegacyTagExecutionStatus, error) {
	if reader.u == nil || !reader.u.in {
		return LegacyTagExecutionStatus{}, errors.New("status read outside uow")
	}
	reader.calls++
	return reader.status, reader.err
}

type legacyTagLiveMutationReceipts struct {
	u            *legacyTagUOW
	reserved     LegacyTagLiveMutationReceipt
	accepted     LegacyTagLiveMutationReceipt
	reserveCalls int
	acceptCalls  int
}

func (store *legacyTagLiveMutationReceipts) ReserveLegacyTagLiveMutation(_ context.Context, command LegacyTagLiveMutationCommand) (LegacyTagLiveMutationReceipt, error) {
	if store.u == nil || !store.u.in {
		return LegacyTagLiveMutationReceipt{}, errors.New("reserve outside uow")
	}
	store.reserveCalls++
	return store.reserved, nil
}

func (store *legacyTagLiveMutationReceipts) AcceptLegacyTagLiveMutation(_ context.Context, receiptID int64, eventID eventport.EventID, jobID int64) (LegacyTagLiveMutationReceipt, error) {
	if store.u == nil || !store.u.in {
		return LegacyTagLiveMutationReceipt{}, errors.New("accept outside uow")
	}
	store.acceptCalls++
	if receiptID != store.reserved.ID || eventID != store.accepted.EventID || jobID != store.accepted.RiverJobID {
		return LegacyTagLiveMutationReceipt{}, errors.New("unexpected mutation acceptance facts")
	}
	return store.accepted, nil
}

type legacyTagLiveMutationJobs struct {
	u     *legacyTagUOW
	id    int64
	job   LegacyTagLiveMutationJob
	calls int
}

func (jobs *legacyTagLiveMutationJobs) EnqueueLegacyTagLiveMutation(_ context.Context, job LegacyTagLiveMutationJob) (int64, error) {
	if jobs.u == nil || !jobs.u.in {
		return 0, errors.New("enqueue outside uow")
	}
	jobs.calls++
	jobs.job = job
	return jobs.id, nil
}

func TestP4CustomerTagsProjectsOnlyTheSafeExecutionGateWithoutProviderCall(t *testing.T) {
	u := &legacyTagUOW{}
	observedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	reader := &legacyTagExecutionStatusReader{u: u, status: LegacyTagExecutionStatus{ObservedAt: observedAt, Payload: []byte(`{"mode":"provider_execution_unavailable","accepted":true,"queued":true,"attempted":false,"executed":false,"outcome_unknown":false,"reconciled":false,"real_external_call_executed":false,"sync_executed":false,"sensitive_future_field":"discard"}`)}}
	got, err := NewLegacyTagExecutionStatusService(u, reader).Get(context.Background())
	if err != nil || got != (LegacyTagExecutionGate{ProviderExecutionEligible: false, LocalCommandAcceptanceAvailable: true, LocalQueueAvailable: true, SyncExecuted: false, ObservedAt: observedAt, RealExternalCallExecuted: false}) {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if u.calls != 1 || reader.calls != 1 {
		t.Fatalf("uow/reader calls = %d/%d, want 1/1", u.calls, reader.calls)
	}

	reader.status = LegacyTagExecutionStatus{ObservedAt: observedAt, Payload: []byte(`null`)}
	if _, err := NewLegacyTagExecutionStatusService(u, reader).Get(context.Background()); !errors.Is(err, ErrInvalidLegacyTagExecutionStatus) {
		t.Fatalf("invalid status error = %v", err)
	}
	reader.status = LegacyTagExecutionStatus{ObservedAt: observedAt, Payload: []byte(`{"mode":"provider_execution_unavailable","accepted":true,"queued":true,"attempted":true,"executed":false,"outcome_unknown":false,"reconciled":false,"real_external_call_executed":false,"sync_executed":false}`)}
	if _, err := NewLegacyTagExecutionStatusService(u, reader).Get(context.Background()); !errors.Is(err, ErrInvalidLegacyTagExecutionStatus) {
		t.Fatalf("attempted source must fail closed: %v", err)
	}
}

func TestP4CustomerTagsQueuesMarkAndUnmarkWithoutClaimingExecution(t *testing.T) {
	for _, command := range []LegacyTagLiveMutationCommand{p4CustomerTagLiveMutationCommand(LegacyTagLiveMutationMark, []byte(`{"tag_ids":[1]}`)), p4CustomerTagLiveMutationCommand(LegacyTagLiveMutationUnmark, []byte(`{"tag_ids":[1]}`))} {
		t.Run(string(command.Operation), func(t *testing.T) {
			u := &legacyTagUOW{}
			events := &legacyTagEvents{u: u}
			receipts := &legacyTagLiveMutationReceipts{u: u, reserved: LegacyTagLiveMutationReceipt{ID: 51, Command: command, State: LegacyTagLiveMutationReceiptReserved}, accepted: LegacyTagLiveMutationReceipt{ID: 51, Command: command, State: LegacyTagLiveMutationReceiptQueued, EventID: 1, RiverJobID: 53}}
			jobs := &legacyTagLiveMutationJobs{u: u, id: 53}
			service := NewLegacyTagLiveMutationService(u, receipts, events, jobs)
			service.now = func() time.Time { return time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC) }

			got, err := service.Request(context.Background(), command)
			if err != nil || got != (LegacyTagLiveMutationAcceptance{ReceiptID: 51, EventID: 1, RiverJobID: 53, State: LegacyTagLiveMutationQueued}) {
				t.Fatalf("Request() = %#v, %v", got, err)
			}
			if u.calls != 1 || receipts.reserveCalls != 1 || receipts.acceptCalls != 1 || jobs.calls != 1 || len(events.items) != 1 {
				t.Fatalf("uow/reserve/accept/jobs/events = %d/%d/%d/%d/%d", u.calls, receipts.reserveCalls, receipts.acceptCalls, jobs.calls, len(events.items))
			}
			if jobs.job.ReceiptID != 51 || jobs.job.Operation != command.Operation || !legacyTagLiveJSONEqual(jobs.job.Payload, command.Payload) || events.items[0].Type != legacyTagLiveMutationAcceptedEvent {
				t.Fatalf("job/event = %#v/%#v", jobs.job, events.items[0])
			}
		})
	}
}

func TestP4CustomerTagsLiveMutationReplayUsesSemanticJSONAndRejectsConflict(t *testing.T) {
	stored := p4CustomerTagLiveMutationCommand(LegacyTagLiveMutationMark, []byte(`{"tag_ids":[1]}`))
	replayed := stored
	replayed.Payload = []byte(`{"tag_ids":[1.0]}`)
	u := &legacyTagUOW{}
	events := &legacyTagEvents{u: u}
	receipts := &legacyTagLiveMutationReceipts{u: u, reserved: LegacyTagLiveMutationReceipt{ID: 51, Command: stored, State: LegacyTagLiveMutationReceiptQueued, EventID: 1, RiverJobID: 53}}
	jobs := &legacyTagLiveMutationJobs{u: u, id: 53}
	service := NewLegacyTagLiveMutationService(u, receipts, events, jobs)
	got, err := service.Request(context.Background(), replayed)
	if err != nil || got.State != LegacyTagLiveMutationQueued || jobs.calls != 0 || len(events.items) != 0 {
		t.Fatalf("semantic replay=%#v err=%v jobs/events=%d/%d", got, err, jobs.calls, len(events.items))
	}

	conflicting := stored
	conflicting.Operation = LegacyTagLiveMutationUnmark
	if _, err := service.Request(context.Background(), conflicting); !errors.Is(err, ErrLegacyTagLiveMutationConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	if jobs.calls != 0 || len(events.items) != 0 {
		t.Fatalf("conflict jobs/events=%d/%d", jobs.calls, len(events.items))
	}
}

func TestP4CustomerTagsLiveMutationRejectsInvalidPayloadAndNeverRetriesUnknown(t *testing.T) {
	u := &legacyTagUOW{}
	events := &legacyTagEvents{u: u}
	command := p4CustomerTagLiveMutationCommand(LegacyTagLiveMutationMark, []byte(`{"tag_ids":[1]}`))
	receipts := &legacyTagLiveMutationReceipts{u: u, reserved: LegacyTagLiveMutationReceipt{ID: 51, Command: command, State: LegacyTagLiveMutationReceiptReserved}, accepted: LegacyTagLiveMutationReceipt{ID: 51, Command: command, State: LegacyTagLiveMutationReceiptQueued, EventID: 1, RiverJobID: 53}}
	jobs := &legacyTagLiveMutationJobs{u: u, id: 53}
	service := NewLegacyTagLiveMutationService(u, receipts, events, jobs)
	invalid := command
	invalid.Payload = []byte(`{"tag_ids":`)
	if _, err := service.Request(context.Background(), invalid); !errors.Is(err, ErrInvalidLegacyTagLiveMutation) {
		t.Fatalf("invalid payload error = %v", err)
	}
	if u.calls != 0 || jobs.calls != 0 || len(events.items) != 0 {
		t.Fatalf("invalid uow/jobs/events=%d/%d/%d", u.calls, jobs.calls, len(events.items))
	}
	for _, state := range []LegacyTagLiveMutationState{LegacyTagLiveMutationAttempted, LegacyTagLiveMutationExecuted, LegacyTagLiveMutationOutcomeUnknown, LegacyTagLiveMutationReconciled} {
		if LegacyTagLiveMutationCanAutoRetry(state) {
			t.Fatalf("state %q must not retry automatically", state)
		}
	}
}

func p4CustomerTagLiveMutationCommand(operation LegacyTagLiveMutationOperation, payload []byte) LegacyTagLiveMutationCommand {
	return LegacyTagLiveMutationCommand{Actor: 7, IdempotencyKey: "live-test-1", TraceID: "trace-test-1", Operation: operation, Payload: payload}
}

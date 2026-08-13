package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type manualRetryTestRepository struct {
	receipt       ManualRetryReceipt
	target        ManualRetryTarget
	inserted      TaskJob
	reserveCalls  int
	lockCalls     int
	insertCalls   int
	completeCalls int
}

func (repository *manualRetryTestRepository) ReserveManualRetryReceipt(_ context.Context, command ManualRetryCommand) (ManualRetryReceipt, error) {
	repository.reserveCalls++
	if repository.receipt.ID == 0 {
		repository.receipt = ManualRetryReceipt{ID: 71, Command: command, State: ControlReceiptReserved}
	}
	return repository.receipt, nil
}

func (repository *manualRetryTestRepository) LockManualRetryTarget(context.Context, TaskID) (ManualRetryTarget, error) {
	repository.lockCalls++
	return repository.target, nil
}

func (repository *manualRetryTestRepository) InsertManualRetryJob(context.Context, int64, ManualRetryTarget) (TaskJob, error) {
	repository.insertCalls++
	return repository.inserted, nil
}

func (repository *manualRetryTestRepository) CompleteManualRetry(_ context.Context, id int64, target ManualRetryTarget, job TaskJob, eventID eventport.EventID) (ManualRetryReceipt, error) {
	repository.completeCalls++
	if id != repository.receipt.ID {
		return ManualRetryReceipt{}, ErrManualRetryFailed
	}
	repository.receipt.State = ControlReceiptCompleted
	repository.receipt.CustomerID = target.CustomerID
	repository.receipt.TaskStatus = TaskStatusPending
	repository.receipt.Job = job
	repository.receipt.EventID = eventID
	repository.receipt.CompletedAt = time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC)
	return repository.receipt, nil
}

func TestManualRetryCreatesNextGenerationAndStableEvent(t *testing.T) {
	command := manualRetryTestCommand()
	repository := &manualRetryTestRepository{
		target: ManualRetryTarget{TaskID: command.TaskID, CustomerID: 42, Status: TaskStatusFinalFailed,
			Job: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind}},
		inserted: TaskJob{TaskID: command.TaskID, Generation: 2, RiverJobID: 52, JobKind: OutboundEnqueueOneJobKind},
	}
	events := &cancelTestEvents{}
	service := NewManualRetryService(enqueueTestUoW{}, repository, events)
	service.clock = func() time.Time { return time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC) }

	got, err := service.Retry(context.Background(), command)
	if err != nil || got.ReceiptID != 71 || got.Status != TaskStatusPending || got.Job != repository.inserted || got.EventID != 1 {
		t.Fatalf("Retry()=%+v err=%v", got, err)
	}
	if repository.reserveCalls != 1 || repository.lockCalls != 1 || repository.insertCalls != 1 || repository.completeCalls != 1 || len(events.events) != 1 {
		t.Fatalf("reserve/lock/insert/complete/events=%d/%d/%d/%d/%d", repository.reserveCalls, repository.lockCalls, repository.insertCalls, repository.completeCalls, len(events.events))
	}
	if event := events.events[0]; event.Type != outboundManualRetryRequestedEvent || event.IdempotencyKey != "outbound.manual_retry_requested:71" || string(event.Payload) != `{"task_id":41,"previous_generation":1,"generation":2,"river_job_id":52}` {
		t.Fatalf("event=%+v", event)
	}
}

func TestManualRetryReplayHasNoNewSideEffects(t *testing.T) {
	command := manualRetryTestCommand()
	repository := &manualRetryTestRepository{receipt: ManualRetryReceipt{
		ID: 71, Command: command, State: ControlReceiptCompleted, CustomerID: 42, TaskStatus: TaskStatusPending,
		Job:     TaskJob{TaskID: command.TaskID, Generation: 2, RiverJobID: 52, JobKind: OutboundEnqueueOneJobKind},
		EventID: 61, CompletedAt: time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC),
	}}
	got, err := NewManualRetryService(enqueueTestUoW{}, repository, &cancelTestEvents{}).Retry(context.Background(), command)
	if err != nil || got.ReceiptID != 71 || got.Job.Generation != 2 || repository.lockCalls != 0 || repository.insertCalls != 0 || repository.completeCalls != 0 {
		t.Fatalf("Retry replay=%+v err=%v calls=%d/%d/%d", got, err, repository.lockCalls, repository.insertCalls, repository.completeCalls)
	}
}

func TestManualRetryRejectsFrozenStatesAndPayloadConflict(t *testing.T) {
	for _, status := range []TaskStatus{TaskStatusPending, TaskStatusSending, TaskStatusSent, TaskStatusRetryableFailed, TaskStatusOutcomeUnknown} {
		t.Run(string(status), func(t *testing.T) {
			command := manualRetryTestCommand()
			repository := &manualRetryTestRepository{target: ManualRetryTarget{
				TaskID: command.TaskID, CustomerID: 42, Status: status,
				Job: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind},
			}}
			_, err := NewManualRetryService(enqueueTestUoW{}, repository, &cancelTestEvents{}).Retry(context.Background(), command)
			if !errors.Is(err, ErrManualRetryTransitionConflict) || repository.insertCalls != 0 {
				t.Fatalf("Retry(%s) err=%v inserts=%d", status, err, repository.insertCalls)
			}
		})
	}
	command := manualRetryTestCommand()
	repository := &manualRetryTestRepository{receipt: ManualRetryReceipt{ID: 71, Command: command, State: ControlReceiptReserved}}
	command.TaskID++
	if _, err := NewManualRetryService(enqueueTestUoW{}, repository, &cancelTestEvents{}).Retry(context.Background(), command); !errors.Is(err, ErrManualRetryCommandConflict) {
		t.Fatalf("payload conflict err=%v", err)
	}
}

func manualRetryTestCommand() ManualRetryCommand {
	return ManualRetryCommand{TaskID: 41, IdempotencyScope: "operator:7", IdempotencyKey: "manual-retry-command-0001"}
}

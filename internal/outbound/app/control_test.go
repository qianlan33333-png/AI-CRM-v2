package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type cancelTestRepository struct {
	receipt       CancelReceipt
	target        CancelTarget
	deleted       TaskJob
	reserveCalls  int
	lockCalls     int
	deleteCalls   int
	completeCalls int
	deleteErr     error
}

func (repository *cancelTestRepository) ReserveCancelReceipt(_ context.Context, command CancelCommand) (CancelReceipt, error) {
	repository.reserveCalls++
	if repository.receipt.ID == 0 {
		repository.receipt = CancelReceipt{ID: 31, Command: command, State: ControlReceiptReserved}
	}
	return repository.receipt, nil
}

func (repository *cancelTestRepository) LockCancelTarget(context.Context, TaskID) (CancelTarget, error) {
	repository.lockCalls++
	return repository.target, nil
}

func (repository *cancelTestRepository) DeletePendingTaskJob(context.Context, CancelTarget) (TaskJob, error) {
	repository.deleteCalls++
	if repository.deleteErr != nil {
		return TaskJob{}, repository.deleteErr
	}
	return repository.deleted, nil
}

func (repository *cancelTestRepository) CompleteCancel(
	_ context.Context,
	receiptID int64,
	target CancelTarget,
	job TaskJob,
	eventID eventport.EventID,
) (CancelReceipt, error) {
	repository.completeCalls++
	repository.receipt.State = ControlReceiptCompleted
	repository.receipt.CustomerID = target.CustomerID
	repository.receipt.TaskStatus = TaskStatusCancelled
	repository.receipt.Job = job
	repository.receipt.EventID = eventID
	repository.receipt.CompletedAt = time.Date(2026, 8, 14, 4, 30, 0, 0, time.UTC)
	if repository.receipt.ID != receiptID {
		return CancelReceipt{}, ErrCancelFailed
	}
	return repository.receipt, nil
}

type cancelTestEvents struct {
	events []eventport.Event
	err    error
}

func (events *cancelTestEvents) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if events.err != nil {
		return 0, events.err
	}
	events.events = append(events.events, event)
	return eventport.EventID(len(events.events)), nil
}

func TestCancelPendingTaskCommitsOriginalFacts(t *testing.T) {
	command := cancelTestCommand()
	repository := &cancelTestRepository{
		target:  CancelTarget{TaskID: command.TaskID, CustomerID: 42, Status: TaskStatusPending, Job: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind}},
		deleted: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind},
	}
	events := &cancelTestEvents{}
	service := NewCancelService(enqueueTestUoW{}, repository, events)
	service.clock = func() time.Time { return time.Date(2026, 8, 14, 4, 30, 0, 0, time.UTC) }

	got, err := service.Cancel(context.Background(), command)
	if err != nil || got.ReceiptID != 31 || got.TaskID != command.TaskID || got.CustomerID != 42 || got.Status != TaskStatusCancelled || got.EventID != 1 || got.Job != repository.deleted {
		t.Fatalf("Cancel()=%+v err=%v", got, err)
	}
	if repository.reserveCalls != 1 || repository.lockCalls != 1 || repository.deleteCalls != 1 || repository.completeCalls != 1 || len(events.events) != 1 {
		t.Fatalf("reserve/lock/delete/complete/events=%d/%d/%d/%d/%d, want 1/1/1/1/1", repository.reserveCalls, repository.lockCalls, repository.deleteCalls, repository.completeCalls, len(events.events))
	}
	if event := events.events[0]; event.Type != outboundCancelledEvent || event.CustomerID != 42 || event.IdempotencyKey != "outbound.cancelled:31" || string(event.Payload) != `{"task_id":41,"river_job_id":51,"generation":1}` {
		t.Fatalf("cancel event=%+v", event)
	}
}

func TestCancelReplayReturnsOriginalFactsWithoutNewSideEffects(t *testing.T) {
	command := cancelTestCommand()
	repository := &cancelTestRepository{receipt: CancelReceipt{
		ID: 31, Command: command, State: ControlReceiptCompleted, CustomerID: 42, TaskStatus: TaskStatusCancelled,
		Job: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind}, EventID: 61,
		CompletedAt: time.Date(2026, 8, 14, 4, 30, 0, 0, time.UTC),
	}}
	events := &cancelTestEvents{}

	got, err := NewCancelService(enqueueTestUoW{}, repository, events).Cancel(context.Background(), command)
	if err != nil || got.ReceiptID != 31 || got.EventID != 61 || got.Job.RiverJobID != 51 || got.Status != TaskStatusCancelled {
		t.Fatalf("Cancel()=%+v err=%v", got, err)
	}
	if repository.lockCalls != 0 || repository.deleteCalls != 0 || repository.completeCalls != 0 || len(events.events) != 0 {
		t.Fatalf("replay lock/delete/complete/events=%d/%d/%d/%d, want zero", repository.lockCalls, repository.deleteCalls, repository.completeCalls, len(events.events))
	}
}

func TestCancelRejectsIdempotencyPayloadConflict(t *testing.T) {
	command := cancelTestCommand()
	repository := &cancelTestRepository{receipt: CancelReceipt{ID: 31, Command: command, State: ControlReceiptReserved}}
	command.TaskID++
	_, err := NewCancelService(enqueueTestUoW{}, repository, &cancelTestEvents{}).Cancel(context.Background(), command)
	if !errors.Is(err, ErrCancelCommandConflict) || repository.lockCalls != 0 || repository.deleteCalls != 0 {
		t.Fatalf("Cancel() error=%v lock/delete=%d/%d", err, repository.lockCalls, repository.deleteCalls)
	}
}

func TestCancelRejectsEveryNonPendingTransition(t *testing.T) {
	for _, status := range []TaskStatus{
		TaskStatusSending, TaskStatusSent, TaskStatusRetryableFailed, TaskStatusFinalFailed,
		TaskStatusOutcomeUnknown, TaskStatusCancelled,
	} {
		t.Run(string(status), func(t *testing.T) {
			command := cancelTestCommand()
			repository := &cancelTestRepository{target: CancelTarget{TaskID: command.TaskID, CustomerID: 42, Status: status}}
			_, err := NewCancelService(enqueueTestUoW{}, repository, &cancelTestEvents{}).Cancel(context.Background(), command)
			if !errors.Is(err, ErrCancelTransitionConflict) || repository.deleteCalls != 0 || repository.completeCalls != 0 {
				t.Fatalf("Cancel(%s) error=%v delete/complete=%d/%d", status, err, repository.deleteCalls, repository.completeCalls)
			}
		})
	}
}

func TestCancelReportsDispatchWinnerWithoutEvent(t *testing.T) {
	command := cancelTestCommand()
	repository := &cancelTestRepository{
		target:    CancelTarget{TaskID: command.TaskID, CustomerID: 42, Status: TaskStatusPending},
		deleteErr: ErrCancelWorkerWon,
	}
	events := &cancelTestEvents{}
	_, err := NewCancelService(enqueueTestUoW{}, repository, events).Cancel(context.Background(), command)
	if !errors.Is(err, ErrCancelWorkerWon) || repository.completeCalls != 0 || len(events.events) != 0 {
		t.Fatalf("Cancel() error=%v complete/events=%d/%d", err, repository.completeCalls, len(events.events))
	}
}

func TestCancelEventFailureNeverCompletesReceipt(t *testing.T) {
	command := cancelTestCommand()
	repository := &cancelTestRepository{
		target:  CancelTarget{TaskID: command.TaskID, CustomerID: 42, Status: TaskStatusPending},
		deleted: TaskJob{TaskID: command.TaskID, Generation: 1, RiverJobID: 51, JobKind: OutboundEnqueueOneJobKind},
	}
	events := &cancelTestEvents{err: errors.New("fixture event rollback")}
	_, err := NewCancelService(enqueueTestUoW{}, repository, events).Cancel(context.Background(), command)
	if !errors.Is(err, ErrCancelFailed) || repository.completeCalls != 0 {
		t.Fatalf("Cancel() error=%v complete=%d", err, repository.completeCalls)
	}
}

func TestCancelRejectsInvalidCommands(t *testing.T) {
	for _, command := range []CancelCommand{
		{},
		{TaskID: 1, IdempotencyScope: "", IdempotencyKey: "1234567890123456"},
		{TaskID: 1, IdempotencyScope: "operator:1", IdempotencyKey: "short"},
		{TaskID: 1, IdempotencyScope: " operator:1", IdempotencyKey: "1234567890123456"},
	} {
		if _, err := NewCancelService(enqueueTestUoW{}, &cancelTestRepository{}, &cancelTestEvents{}).Cancel(context.Background(), command); !errors.Is(err, ErrInvalidCancelCommand) {
			t.Fatalf("Cancel(%+v) error=%v", command, err)
		}
	}
}

func cancelTestCommand() CancelCommand {
	return CancelCommand{TaskID: 41, IdempotencyScope: "operator:7", IdempotencyKey: "cancel-command-0001"}
}

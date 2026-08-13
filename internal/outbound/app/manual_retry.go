package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const outboundManualRetryRequestedEvent = "outbound.manual_retry_requested"

var (
	ErrInvalidManualRetryCommand     = errors.New("invalid outbound manual retry command")
	ErrManualRetryCommandConflict    = errors.New("outbound manual retry idempotency command conflict")
	ErrManualRetryTaskNotFound       = errors.New("outbound manual retry task not found")
	ErrManualRetryTransitionConflict = errors.New("outbound manual retry task transition conflict")
	ErrManualRetryFailed             = errors.New("manual retry outbound task command")
)

type ManualRetryCommand struct {
	TaskID           TaskID
	IdempotencyScope string
	IdempotencyKey   string
}

type ManualRetryTarget struct {
	TaskID           TaskID
	CustomerID       int64
	Status           TaskStatus
	Job              TaskJob
	EnqueueReceiptID int64
	BatchID          int64
	BatchChunkIndex  int32
}

type ManualRetryReceipt struct {
	ID          int64
	Command     ManualRetryCommand
	State       ControlReceiptState
	CustomerID  int64
	TaskStatus  TaskStatus
	Job         TaskJob
	EventID     eventport.EventID
	CompletedAt time.Time
}

type ManualRetryResult struct {
	ReceiptID  int64
	TaskID     TaskID
	CustomerID int64
	Status     TaskStatus
	Job        TaskJob
	EventID    eventport.EventID
	RetriedAt  time.Time
}

type ManualRetryRepository interface {
	ReserveManualRetryReceipt(context.Context, ManualRetryCommand) (ManualRetryReceipt, error)
	LockManualRetryTarget(context.Context, TaskID) (ManualRetryTarget, error)
	InsertManualRetryJob(context.Context, int64, ManualRetryTarget) (TaskJob, error)
	CompleteManualRetry(context.Context, int64, ManualRetryTarget, TaskJob, eventport.EventID) (ManualRetryReceipt, error)
}

type ManualRetryService struct {
	uow        platformport.UnitOfWork
	repository ManualRetryRepository
	events     eventport.Appender
	clock      func() time.Time
}

func NewManualRetryService(uow platformport.UnitOfWork, repository ManualRetryRepository, events eventport.Appender) *ManualRetryService {
	return &ManualRetryService{uow: uow, repository: repository, events: events, clock: time.Now}
}

// Retry creates exactly one next-generation River job and returns the task to
// pending in the same transaction as its immutable receipt and event.
func (service *ManualRetryService) Retry(ctx context.Context, command ManualRetryCommand) (ManualRetryResult, error) {
	if ctx == nil || !validManualRetryCommand(command) || service == nil || service.uow == nil ||
		service.repository == nil || service.events == nil || service.clock == nil {
		return ManualRetryResult{}, ErrInvalidManualRetryCommand
	}

	var result ManualRetryResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.repository.ReserveManualRetryReceipt(txCtx, command)
		if err != nil {
			return err
		}
		if receipt.Command != command {
			return ErrManualRetryCommandConflict
		}
		switch receipt.State {
		case ControlReceiptCompleted:
			result, err = manualRetryResultFromReceipt(receipt)
			return err
		case ControlReceiptReserved:
			if receipt.ID <= 0 {
				return ErrManualRetryFailed
			}
		default:
			return ErrManualRetryFailed
		}

		target, err := service.repository.LockManualRetryTarget(txCtx, command.TaskID)
		if err != nil {
			return err
		}
		if target.TaskID != command.TaskID || target.CustomerID <= 0 || !validTaskJob(target.Job, command.TaskID) {
			return ErrManualRetryFailed
		}
		if target.Status != TaskStatusFinalFailed && target.Status != TaskStatusCancelled {
			return ErrManualRetryTransitionConflict
		}
		job, err := service.repository.InsertManualRetryJob(txCtx, receipt.ID, target)
		if err != nil {
			return err
		}
		if !validTaskJob(job, command.TaskID) || job.Generation != target.Job.Generation+1 || job.JobKind != target.Job.JobKind {
			return ErrManualRetryFailed
		}

		payload, err := json.Marshal(struct {
			TaskID             TaskID `json:"task_id"`
			PreviousGeneration int32  `json:"previous_generation"`
			Generation         int32  `json:"generation"`
			RiverJobID         int64  `json:"river_job_id"`
		}{command.TaskID, target.Job.Generation, job.Generation, job.RiverJobID})
		if err != nil {
			return errors.Join(ErrManualRetryFailed, err)
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type: outboundManualRetryRequestedEvent, CustomerID: eventport.CustomerID(target.CustomerID),
			Payload: payload, OccurredAt: service.clock().UTC(),
			IdempotencyKey: fmt.Sprintf("outbound.manual_retry_requested:%d", receipt.ID),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrManualRetryFailed, err)
		}
		completed, err := service.repository.CompleteManualRetry(txCtx, receipt.ID, target, job, eventID)
		if err != nil {
			return err
		}
		result, err = manualRetryResultFromReceipt(completed)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrInvalidManualRetryCommand) || errors.Is(err, ErrManualRetryCommandConflict) ||
			errors.Is(err, ErrManualRetryTaskNotFound) || errors.Is(err, ErrManualRetryTransitionConflict) {
			return ManualRetryResult{}, err
		}
		return ManualRetryResult{}, errors.Join(ErrManualRetryFailed, err)
	}
	return result, nil
}

func validManualRetryCommand(command ManualRetryCommand) bool {
	return command.TaskID > 0 && validCancelText(command.IdempotencyScope, 1, 200) &&
		validCancelText(command.IdempotencyKey, 16, 128)
}

func manualRetryResultFromReceipt(receipt ManualRetryReceipt) (ManualRetryResult, error) {
	if receipt.ID <= 0 || receipt.State != ControlReceiptCompleted || !validManualRetryCommand(receipt.Command) ||
		receipt.CustomerID <= 0 || receipt.TaskStatus != TaskStatusPending ||
		!validTaskJob(receipt.Job, receipt.Command.TaskID) || receipt.EventID <= 0 || receipt.CompletedAt.IsZero() {
		return ManualRetryResult{}, ErrManualRetryFailed
	}
	return ManualRetryResult{
		ReceiptID: receipt.ID, TaskID: receipt.Command.TaskID, CustomerID: receipt.CustomerID,
		Status: receipt.TaskStatus, Job: receipt.Job, EventID: receipt.EventID, RetriedAt: receipt.CompletedAt,
	}, nil
}

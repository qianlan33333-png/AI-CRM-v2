package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const outboundCancelledEvent = "outbound.cancelled"

var (
	ErrInvalidCancelCommand     = errors.New("invalid outbound cancel command")
	ErrCancelCommandConflict    = errors.New("outbound cancel idempotency command conflict")
	ErrCancelTaskNotFound       = errors.New("outbound cancel task not found")
	ErrCancelTransitionConflict = errors.New("outbound cancel task transition conflict")
	ErrCancelWorkerWon          = errors.New("outbound cancel worker dispatch won")
	ErrCancelFailed             = errors.New("cancel outbound task command")
)

type CancelCommand struct {
	TaskID           TaskID
	IdempotencyScope string
	IdempotencyKey   string
}

type ControlReceiptState string

const (
	ControlReceiptReserved  ControlReceiptState = "reserved"
	ControlReceiptCompleted ControlReceiptState = "completed"
)

// TaskJob is Outbound's durable link to a River job. The River catalog remains
// owned and mutated only by the platform jobqueue adapter.
type TaskJob struct {
	TaskID     TaskID
	Generation int32
	RiverJobID int64
	JobKind    string
}

type CancelTarget struct {
	TaskID     TaskID
	CustomerID int64
	Status     TaskStatus
	Job        TaskJob
}

type CancelReceipt struct {
	ID          int64
	Command     CancelCommand
	State       ControlReceiptState
	CustomerID  int64
	TaskStatus  TaskStatus
	Job         TaskJob
	EventID     eventport.EventID
	CompletedAt time.Time
}

type CancelledTask struct {
	ReceiptID   int64
	TaskID      TaskID
	CustomerID  int64
	Status      TaskStatus
	Job         TaskJob
	EventID     eventport.EventID
	CancelledAt time.Time
}

type CancelRepository interface {
	ReserveCancelReceipt(context.Context, CancelCommand) (CancelReceipt, error)
	LockCancelTarget(context.Context, TaskID) (CancelTarget, error)
	DeletePendingTaskJob(context.Context, CancelTarget) (TaskJob, error)
	CompleteCancel(context.Context, int64, CancelTarget, TaskJob, eventport.EventID) (CancelReceipt, error)
}

type CancelService struct {
	uow        platformport.UnitOfWork
	repository CancelRepository
	events     eventport.Appender
	clock      func() time.Time
}

func NewCancelService(uow platformport.UnitOfWork, repository CancelRepository, events eventport.Appender) *CancelService {
	return &CancelService{uow: uow, repository: repository, events: events, clock: time.Now}
}

// Cancel deletes a not-yet-running River job and commits the task projection,
// immutable event, control receipt, and job link in the same transaction. A
// worker that has already claimed or projected dispatch wins with a conflict.
func (service *CancelService) Cancel(ctx context.Context, command CancelCommand) (CancelledTask, error) {
	if ctx == nil || !validCancelCommand(command) || service == nil || service.uow == nil ||
		service.repository == nil || service.events == nil || service.clock == nil {
		return CancelledTask{}, ErrInvalidCancelCommand
	}

	var result CancelledTask
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.repository.ReserveCancelReceipt(txCtx, command)
		if err != nil {
			return err
		}
		if !sameCancelCommand(receipt.Command, command) {
			return ErrCancelCommandConflict
		}
		switch receipt.State {
		case ControlReceiptCompleted:
			completed, completedErr := cancelledTaskFromReceipt(receipt)
			if completedErr != nil {
				return completedErr
			}
			result = completed
			return nil
		case ControlReceiptReserved:
			if receipt.ID <= 0 {
				return ErrCancelFailed
			}
		default:
			return ErrCancelFailed
		}

		target, err := service.repository.LockCancelTarget(txCtx, command.TaskID)
		if err != nil {
			return err
		}
		if target.TaskID != command.TaskID || target.CustomerID <= 0 {
			return ErrCancelFailed
		}
		if target.Status != TaskStatusPending {
			return ErrCancelTransitionConflict
		}
		job, err := service.repository.DeletePendingTaskJob(txCtx, target)
		if err != nil {
			return err
		}
		if !validTaskJob(job, command.TaskID) {
			return ErrCancelFailed
		}

		payload, err := json.Marshal(struct {
			TaskID     TaskID `json:"task_id"`
			RiverJobID int64  `json:"river_job_id"`
			Generation int32  `json:"generation"`
		}{command.TaskID, job.RiverJobID, job.Generation})
		if err != nil {
			return errors.Join(ErrCancelFailed, err)
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type:           outboundCancelledEvent,
			CustomerID:     eventport.CustomerID(target.CustomerID),
			Payload:        payload,
			OccurredAt:     service.clock().UTC(),
			IdempotencyKey: fmt.Sprintf("outbound.cancelled:%d", receipt.ID),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrCancelFailed, err)
		}
		completed, err := service.repository.CompleteCancel(txCtx, receipt.ID, target, job, eventID)
		if err != nil {
			return err
		}
		result, err = cancelledTaskFromReceipt(completed)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCancelCommand) || errors.Is(err, ErrCancelCommandConflict) ||
			errors.Is(err, ErrCancelTaskNotFound) || errors.Is(err, ErrCancelTransitionConflict) ||
			errors.Is(err, ErrCancelWorkerWon) {
			return CancelledTask{}, err
		}
		return CancelledTask{}, errors.Join(ErrCancelFailed, err)
	}
	return result, nil
}

func validCancelCommand(command CancelCommand) bool {
	return command.TaskID > 0 && validCancelText(command.IdempotencyScope, 1, 200) &&
		validCancelText(command.IdempotencyKey, 16, 128)
}

func validCancelText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && value != "" && strings.TrimSpace(value) == value
}

func sameCancelCommand(left, right CancelCommand) bool {
	return left == right
}

func validTaskJob(job TaskJob, taskID TaskID) bool {
	return job.TaskID == taskID && job.Generation > 0 && job.RiverJobID > 0 &&
		(job.JobKind == OutboundEnqueueOneJobKind || job.JobKind == OutboundEnqueueBatchJobKind)
}

func cancelledTaskFromReceipt(receipt CancelReceipt) (CancelledTask, error) {
	if receipt.ID <= 0 || receipt.State != ControlReceiptCompleted || !validCancelCommand(receipt.Command) ||
		receipt.CustomerID <= 0 || receipt.TaskStatus != TaskStatusCancelled ||
		!validTaskJob(receipt.Job, receipt.Command.TaskID) || receipt.EventID <= 0 || receipt.CompletedAt.IsZero() {
		return CancelledTask{}, ErrCancelFailed
	}
	return CancelledTask{
		ReceiptID: receipt.ID, TaskID: receipt.Command.TaskID, CustomerID: receipt.CustomerID,
		Status: receipt.TaskStatus, Job: receipt.Job, EventID: receipt.EventID, CancelledAt: receipt.CompletedAt,
	}, nil
}

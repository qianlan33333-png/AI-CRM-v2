package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const OutboundEnqueueOneJobKind = "outbound_enqueue_one"

var (
	ErrInvalidEnqueueOneCommand = errors.New("invalid outbound enqueue one command")
	ErrEnqueueOneConflict       = errors.New("outbound enqueue one idempotency command conflict")
	ErrEnqueueOneFailed         = errors.New("enqueue outbound one command")
)

// EnqueueOneCommand carries the single-recipient command identity through the
// durable task, event, and River insertion boundary.
type EnqueueOneCommand struct {
	OneCommand
	IdempotencyScope string
	IdempotencyKey   string
}

// EnqueueOneArgs is the stable River command. Its worker belongs to a later
// slice; this slice only inserts the job transactionally.
type EnqueueOneArgs struct {
	TaskID    TaskID `json:"task_id"`
	ReceiptID int64  `json:"receipt_id"`
}

func (EnqueueOneArgs) Kind() string { return OutboundEnqueueOneJobKind }

type EnqueueReceiptState string

const (
	EnqueueReceiptReserved EnqueueReceiptState = "reserved"
	EnqueueReceiptAccepted EnqueueReceiptState = "accepted"
)

type EnqueueReceipt struct {
	ID         int64
	Command    EnqueueOneCommand
	State      EnqueueReceiptState
	TaskID     TaskID
	EventID    eventport.EventID
	RiverJobID int64
}

type EnqueuedTask struct {
	TaskID     TaskID
	EventID    eventport.EventID
	RiverJobID int64
}

type EnqueueReceiptStore interface {
	ReserveEnqueueReceipt(context.Context, EnqueueOneCommand) (EnqueueReceipt, error)
	AcceptEnqueueReceipt(context.Context, int64, TaskID, eventport.EventID, int64) (EnqueueReceipt, error)
}

type EnqueueOneEnqueuer interface {
	EnqueueOne(context.Context, EnqueueOneArgs) (int64, error)
}

type EnqueueOneService struct {
	uow      platformport.UnitOfWork
	tasks    TaskRepository
	events   eventport.Appender
	receipts EnqueueReceiptStore
	enqueuer EnqueueOneEnqueuer
	clock    func() time.Time
}

func NewEnqueueOneService(
	uow platformport.UnitOfWork,
	tasks TaskRepository,
	events eventport.Appender,
	receipts EnqueueReceiptStore,
	enqueuer EnqueueOneEnqueuer,
) *EnqueueOneService {
	return &EnqueueOneService{
		uow: uow, tasks: tasks, events: events, receipts: receipts, enqueuer: enqueuer, clock: time.Now,
	}
}

// Enqueue persists a fresh task, accepted event, and stable outbound River job
// in one UoW. A matching accepted receipt returns its original three facts;
// a different command at the same idempotency key conflicts.
func (service *EnqueueOneService) Enqueue(ctx context.Context, command EnqueueOneCommand) (EnqueuedTask, error) {
	if ctx == nil || !validEnqueueOneCommand(command) || service == nil || service.uow == nil || service.tasks == nil ||
		service.events == nil || service.receipts == nil || service.enqueuer == nil || service.clock == nil {
		return EnqueuedTask{}, ErrInvalidEnqueueOneCommand
	}

	var result EnqueuedTask
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.receipts.ReserveEnqueueReceipt(txCtx, command)
		if err != nil {
			return err
		}
		if !sameEnqueueOneCommand(receipt.Command, command) {
			return ErrEnqueueOneConflict
		}
		switch receipt.State {
		case EnqueueReceiptAccepted:
			if !validEnqueuedTask(receipt.TaskID, receipt.EventID, receipt.RiverJobID) {
				return ErrEnqueueOneFailed
			}
			result = EnqueuedTask{TaskID: receipt.TaskID, EventID: receipt.EventID, RiverJobID: receipt.RiverJobID}
			return nil
		case EnqueueReceiptReserved:
			if receipt.ID <= 0 {
				return ErrEnqueueOneFailed
			}
		default:
			return ErrEnqueueOneFailed
		}

		taskID, err := service.tasks.CreateAcceptedTask(txCtx, command.OneCommand)
		if err != nil || taskID <= 0 {
			return errors.Join(ErrEnqueueOneFailed, err)
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type:           eventport.EvOutboundAccepted,
			CustomerID:     eventport.CustomerID(command.CustomerID),
			Payload:        json.RawMessage(fmt.Sprintf(`{"task_id":%d}`, taskID)),
			OccurredAt:     service.clock().UTC(),
			IdempotencyKey: fmt.Sprintf("outbound.accepted:%d", taskID),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrEnqueueOneFailed, err)
		}
		jobID, err := service.enqueuer.EnqueueOne(txCtx, EnqueueOneArgs{TaskID: taskID, ReceiptID: receipt.ID})
		if err != nil || jobID <= 0 {
			return errors.Join(ErrEnqueueOneFailed, err)
		}
		accepted, err := service.receipts.AcceptEnqueueReceipt(txCtx, receipt.ID, taskID, eventID, jobID)
		if err != nil || accepted.ID != receipt.ID || !sameEnqueueOneCommand(accepted.Command, command) ||
			accepted.State != EnqueueReceiptAccepted || accepted.TaskID != taskID || accepted.EventID != eventID || accepted.RiverJobID != jobID {
			return errors.Join(ErrEnqueueOneFailed, err)
		}
		result = EnqueuedTask{TaskID: taskID, EventID: eventID, RiverJobID: jobID}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrEnqueueOneConflict) || errors.Is(err, ErrInvalidEnqueueOneCommand) {
			return EnqueuedTask{}, err
		}
		return EnqueuedTask{}, errors.Join(ErrEnqueueOneFailed, err)
	}
	return result, nil
}

func validEnqueueOneCommand(command EnqueueOneCommand) bool {
	return validOneCommand(command.OneCommand) && validEnqueueText(command.IdempotencyScope, 1, 200) &&
		validEnqueueText(command.IdempotencyKey, 16, 128)
}

func validEnqueueText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value && value != ""
}

func validEnqueuedTask(taskID TaskID, eventID eventport.EventID, riverJobID int64) bool {
	return taskID > 0 && eventID > 0 && riverJobID > 0
}

func sameEnqueueOneCommand(left, right EnqueueOneCommand) bool {
	return left.CustomerID == right.CustomerID && left.TemplateKey == right.TemplateKey &&
		left.IdempotencyScope == right.IdempotencyScope && left.IdempotencyKey == right.IdempotencyKey && equalEnqueueJSON(left.Payload, right.Payload)
}

func equalEnqueueJSON(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

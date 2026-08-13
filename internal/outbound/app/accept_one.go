// Package app coordinates transaction-bound outbound task acceptance.
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

const TemplateTextNoticeV1 = "text.notice.v1"

var (
	ErrInvalidOneCommand = errors.New("invalid outbound one command")
	ErrAcceptOneFailed   = errors.New("accept outbound one command")
)

type TaskID int64

type OneCommand struct {
	CustomerID  int64
	TemplateKey string
	Payload     json.RawMessage
}

type AcceptedTask struct {
	TaskID  TaskID
	EventID eventport.EventID
}

type TaskRepository interface {
	CreateAcceptedTask(context.Context, OneCommand) (TaskID, error)
}

type AcceptOneService struct {
	uow    platformport.UnitOfWork
	tasks  TaskRepository
	events eventport.Appender
	clock  func() time.Time
}

func NewAcceptOneService(uow platformport.UnitOfWork, tasks TaskRepository, events eventport.Appender) *AcceptOneService {
	return &AcceptOneService{uow: uow, tasks: tasks, events: events, clock: time.Now}
}

// Accept persists a database-generated outbound task and its accepted event in
// one transaction. It neither enqueues work nor invokes a provider.
func (service *AcceptOneService) Accept(ctx context.Context, command OneCommand) (AcceptedTask, error) {
	if ctx == nil || !validOneCommand(command) || service == nil || service.uow == nil || service.tasks == nil || service.events == nil || service.clock == nil {
		return AcceptedTask{}, ErrInvalidOneCommand
	}

	var accepted AcceptedTask
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		taskID, err := service.tasks.CreateAcceptedTask(txCtx, command)
		if err != nil || taskID <= 0 {
			return errors.Join(ErrAcceptOneFailed, err)
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type:           eventport.EvOutboundAccepted,
			CustomerID:     eventport.CustomerID(command.CustomerID),
			Payload:        json.RawMessage(fmt.Sprintf(`{"task_id":%d}`, taskID)),
			OccurredAt:     service.clock().UTC(),
			IdempotencyKey: fmt.Sprintf("outbound.accepted:%d", taskID),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrAcceptOneFailed, err)
		}
		accepted = AcceptedTask{TaskID: taskID, EventID: eventID}
		return nil
	})
	if err != nil {
		return AcceptedTask{}, errors.Join(ErrAcceptOneFailed, err)
	}
	return accepted, nil
}

func validOneCommand(command OneCommand) bool {
	if command.CustomerID <= 0 || command.TemplateKey != TemplateTextNoticeV1 || len(command.Payload) == 0 || !json.Valid(command.Payload) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(command.Payload, &object) == nil && object != nil
}

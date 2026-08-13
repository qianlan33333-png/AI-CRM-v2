// Package app coordinates outbound business use cases without binding a real provider.
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

var (
	ErrInvalidSendCommand  = errors.New("invalid outbound send command")
	ErrSendAttemptConflict = errors.New("outbound send attempt command conflict")
	ErrSendAttemptFailed   = errors.New("execute outbound send attempt")
)

type SendAttemptState string

const (
	SendAttemptReserved        SendAttemptState = "reserved"
	SendAttemptDispatching     SendAttemptState = "dispatching"
	SendAttemptSucceeded       SendAttemptState = "succeeded"
	SendAttemptRetryableFailed SendAttemptState = "retryable_failed"
	SendAttemptFinalFailed     SendAttemptState = "final_failed"
	SendAttemptOutcomeUnknown  SendAttemptState = "outcome_unknown"
)

type ProviderFailureKind string

const (
	ProviderFailureTimeout              ProviderFailureKind = "timeout"
	ProviderFailureConnection           ProviderFailureKind = "connection"
	ProviderFailureNoResponse5xx        ProviderFailureKind = "no_response_5xx"
	ProviderFailureRateLimited          ProviderFailureKind = "rate_limited"
	ProviderFailureTemporary            ProviderFailureKind = "temporary"
	ProviderFailureInvalidArgument      ProviderFailureKind = "invalid_argument"
	ProviderFailureRecipientUnavailable ProviderFailureKind = "recipient_unavailable"
	ProviderFailureAdapterError         ProviderFailureKind = "adapter_error"
	ProviderFailureInvalidResult        ProviderFailureKind = "invalid_provider_result"
	ProviderFailureInterruptedDispatch  ProviderFailureKind = "interrupted_dispatch"
)

type SendCommand struct {
	RiverJobID int64
	TaskID     TaskID
	JobKind    string
}

type SendRequest struct {
	TaskID      TaskID
	CustomerID  int64
	TemplateKey string
	Payload     json.RawMessage
}

type ProviderResult struct {
	MessageID   string
	FailureKind ProviderFailureKind
	Code        string
}

type SendAttempt struct {
	ID                int64
	RiverJobID        int64
	TaskID            TaskID
	JobKind           string
	State             SendAttemptState
	FailureKind       ProviderFailureKind
	ProviderCode      string
	ProviderMessageID string
	CompletedAt       time.Time
}

type TaskStatus string

const (
	TaskStatusPending         TaskStatus = "pending"
	TaskStatusSending         TaskStatus = "sending"
	TaskStatusSent            TaskStatus = "sent"
	TaskStatusRetryableFailed TaskStatus = "retryable_failed"
	TaskStatusFinalFailed     TaskStatus = "final_failed"
	TaskStatusOutcomeUnknown  TaskStatus = "outcome_unknown"
	TaskStatusCancelled       TaskStatus = "cancelled"
)

// TaskResultFact is the stable, database-projected terminal fact used to append
// one outbound result event. It contains no message content or recipient data.
type TaskResultFact struct {
	TaskID            TaskID
	CustomerID        int64
	AttemptID         int64
	RiverJobID        int64
	Status            TaskStatus
	AttemptCount      int32
	FailureKind       ProviderFailureKind
	ProviderCode      string
	ProviderMessageID string
	OccurredAt        time.Time
}

type CompleteSendAttempt struct {
	ID                int64
	State             SendAttemptState
	FailureKind       ProviderFailureKind
	ProviderCode      string
	ProviderMessageID string
}

type SendAttemptRepository interface {
	ReserveSendAttempt(context.Context, SendCommand) (SendAttempt, error)
	StartSendAttempt(context.Context, int64) (SendAttempt, bool, error)
	LoadSendRequest(context.Context, TaskID) (SendRequest, error)
	CompleteSendAttempt(context.Context, CompleteSendAttempt) (SendAttempt, error)
	MarkTaskSending(context.Context, SendAttempt) error
	ProjectTaskResult(context.Context, SendAttempt) (TaskResultFact, error)
}

// ProviderAdapter is deliberately injected. O4 ships no live-provider implementation.
type ProviderAdapter interface {
	Send(context.Context, SendRequest) (ProviderResult, error)
}

type RateGate interface {
	Wait(context.Context) error
}

type SenderService struct {
	uow        platformport.UnitOfWork
	repository SendAttemptRepository
	events     eventport.Appender
	provider   ProviderAdapter
	rate       RateGate
}

func NewSenderService(uow platformport.UnitOfWork, repository SendAttemptRepository, events eventport.Appender, provider ProviderAdapter, rate RateGate) *SenderService {
	return &SenderService{uow: uow, repository: repository, events: events, provider: provider, rate: rate}
}

// Execute reserves a durable attempt before crossing the provider boundary.
// A replay of dispatching is classified unknown and never invokes the provider
// again. Receipt, task status, and result event are committed atomically.
func (service *SenderService) Execute(ctx context.Context, command SendCommand) (SendAttempt, error) {
	if ctx == nil || service == nil || service.uow == nil || service.repository == nil || service.events == nil || service.provider == nil || service.rate == nil || !validSendCommand(command) {
		return SendAttempt{}, ErrInvalidSendCommand
	}

	var attempt SendAttempt
	var request SendRequest
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		reserved, reserveErr := service.repository.ReserveSendAttempt(txCtx, command)
		if reserveErr != nil {
			return reserveErr
		}
		if !sameSendCommand(reserved, command) {
			return ErrSendAttemptConflict
		}
		attempt = reserved
		if attempt.State != SendAttemptReserved {
			return nil
		}
		loaded, loadErr := service.repository.LoadSendRequest(txCtx, command.TaskID)
		if loadErr != nil {
			return loadErr
		}
		if !validSendRequest(loaded, command.TaskID) {
			return ErrInvalidSendCommand
		}
		request = loaded
		return nil
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	if terminalSendAttempt(attempt.State) {
		return service.project(ctx, attempt)
	}
	if attempt.State == SendAttemptDispatching {
		return service.complete(ctx, attempt, CompleteSendAttempt{
			ID: attempt.ID, State: SendAttemptOutcomeUnknown, FailureKind: ProviderFailureInterruptedDispatch,
			ProviderCode: "river_replay_after_dispatch",
		})
	}
	if attempt.State != SendAttemptReserved {
		return SendAttempt{}, ErrSendAttemptFailed
	}
	if waitErr := service.rate.Wait(ctx); waitErr != nil {
		return SendAttempt{}, errors.Join(ErrSendAttemptFailed, waitErr)
	}

	var started bool
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		current, didStart, startErr := service.repository.StartSendAttempt(txCtx, attempt.ID)
		if startErr != nil {
			return startErr
		}
		if !sameSendCommand(current, command) {
			return ErrSendAttemptConflict
		}
		if current.State == SendAttemptDispatching {
			if markErr := service.repository.MarkTaskSending(txCtx, current); markErr != nil {
				return markErr
			}
		}
		attempt, started = current, didStart
		return nil
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	if !started {
		if terminalSendAttempt(attempt.State) {
			return service.project(ctx, attempt)
		}
		if attempt.State == SendAttemptDispatching {
			return service.complete(ctx, attempt, CompleteSendAttempt{
				ID: attempt.ID, State: SendAttemptOutcomeUnknown, FailureKind: ProviderFailureInterruptedDispatch,
				ProviderCode: "concurrent_dispatch_detected",
			})
		}
		return SendAttempt{}, ErrSendAttemptFailed
	}

	result, providerErr := service.provider.Send(ctx, request)
	completion := classifyProviderResult(attempt.ID, result, providerErr)
	return service.complete(ctx, attempt, completion)
}

func (service *SenderService) complete(ctx context.Context, attempt SendAttempt, completion CompleteSendAttempt) (SendAttempt, error) {
	var completed SendAttempt
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		stored, completeErr := service.repository.CompleteSendAttempt(txCtx, completion)
		if completeErr != nil {
			return completeErr
		}
		if stored.ID != attempt.ID || stored.RiverJobID != attempt.RiverJobID || stored.TaskID != attempt.TaskID || stored.JobKind != attempt.JobKind || !terminalSendAttempt(stored.State) {
			return ErrSendAttemptFailed
		}
		if _, projectErr := service.projectWithin(txCtx, stored); projectErr != nil {
			return projectErr
		}
		completed = stored
		return nil
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	return completed, nil
}

func (service *SenderService) project(ctx context.Context, attempt SendAttempt) (SendAttempt, error) {
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		_, projectErr := service.projectWithin(txCtx, attempt)
		return projectErr
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	return attempt, nil
}

func (service *SenderService) projectWithin(ctx context.Context, attempt SendAttempt) (eventport.EventID, error) {
	fact, err := service.repository.ProjectTaskResult(ctx, attempt)
	if err != nil {
		return 0, err
	}
	if fact.TaskID != attempt.TaskID || fact.CustomerID <= 0 || fact.AttemptID != attempt.ID || fact.RiverJobID != attempt.RiverJobID || fact.AttemptCount < 1 || fact.Status != taskStatusForAttempt(attempt.State) || fact.OccurredAt.IsZero() {
		return 0, ErrSendAttemptFailed
	}
	payload, err := json.Marshal(struct {
		TaskID            TaskID              `json:"task_id"`
		AttemptID         int64               `json:"attempt_id"`
		RiverJobID        int64               `json:"river_job_id"`
		Status            TaskStatus          `json:"status"`
		AttemptCount      int32               `json:"attempt_count"`
		FailureKind       ProviderFailureKind `json:"failure_kind,omitempty"`
		ProviderCode      string              `json:"provider_code,omitempty"`
		ProviderMessageID string              `json:"provider_message_id,omitempty"`
	}{fact.TaskID, fact.AttemptID, fact.RiverJobID, fact.Status, fact.AttemptCount, fact.FailureKind, fact.ProviderCode, fact.ProviderMessageID})
	if err != nil {
		return 0, err
	}
	eventType := eventport.EvOutboundFailed
	if fact.Status == TaskStatusSent {
		eventType = eventport.EvOutboundSent
	}
	return service.events.Append(ctx, eventport.Event{
		Type: eventType, CustomerID: eventport.CustomerID(fact.CustomerID), Payload: payload,
		OccurredAt: fact.OccurredAt, IdempotencyKey: fmt.Sprintf("outbound.send-result:%d", fact.AttemptID),
	})
}

func taskStatusForAttempt(state SendAttemptState) TaskStatus {
	switch state {
	case SendAttemptSucceeded:
		return TaskStatusSent
	case SendAttemptRetryableFailed:
		return TaskStatusRetryableFailed
	case SendAttemptFinalFailed:
		return TaskStatusFinalFailed
	case SendAttemptOutcomeUnknown:
		return TaskStatusOutcomeUnknown
	default:
		return ""
	}
}

func classifyProviderResult(attemptID int64, result ProviderResult, providerErr error) CompleteSendAttempt {
	completion := CompleteSendAttempt{ID: attemptID, FailureKind: result.FailureKind, ProviderCode: strings.TrimSpace(result.Code)}
	if providerErr != nil {
		completion.State, completion.FailureKind, completion.ProviderCode = SendAttemptOutcomeUnknown, ProviderFailureAdapterError, "adapter_error"
		return completion
	}
	switch result.FailureKind {
	case "":
		if strings.TrimSpace(result.MessageID) != "" {
			completion.State, completion.ProviderMessageID = SendAttemptSucceeded, strings.TrimSpace(result.MessageID)
			return completion
		}
	case ProviderFailureTimeout, ProviderFailureConnection, ProviderFailureNoResponse5xx:
		completion.State = SendAttemptOutcomeUnknown
	case ProviderFailureRateLimited, ProviderFailureTemporary:
		completion.State = SendAttemptRetryableFailed
	case ProviderFailureInvalidArgument, ProviderFailureRecipientUnavailable:
		completion.State = SendAttemptFinalFailed
	default:
		completion.State, completion.FailureKind, completion.ProviderCode = SendAttemptOutcomeUnknown, ProviderFailureInvalidResult, "invalid_provider_result"
		return completion
	}
	if completion.ProviderCode == "" {
		completion.ProviderCode = string(completion.FailureKind)
	}
	return completion
}

func validSendCommand(command SendCommand) bool {
	return command.RiverJobID > 0 && command.TaskID > 0 && (command.JobKind == OutboundEnqueueOneJobKind || command.JobKind == OutboundEnqueueBatchJobKind)
}

func validSendRequest(request SendRequest, taskID TaskID) bool {
	return request.TaskID == taskID && request.CustomerID > 0 && validOneCommand(OneCommand{CustomerID: request.CustomerID, TemplateKey: request.TemplateKey, Payload: request.Payload})
}

func sameSendCommand(attempt SendAttempt, command SendCommand) bool {
	return attempt.ID > 0 && attempt.RiverJobID == command.RiverJobID && attempt.TaskID == command.TaskID && attempt.JobKind == command.JobKind
}

func terminalSendAttempt(state SendAttemptState) bool {
	return state == SendAttemptSucceeded || state == SendAttemptRetryableFailed || state == SendAttemptFinalFailed || state == SendAttemptOutcomeUnknown
}

func sendError(err error) error {
	if errors.Is(err, ErrInvalidSendCommand) || errors.Is(err, ErrSendAttemptConflict) {
		return err
	}
	return errors.Join(ErrSendAttemptFailed, err)
}

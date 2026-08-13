// Package app coordinates outbound business use cases without binding a real provider.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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
	provider   ProviderAdapter
	rate       RateGate
}

func NewSenderService(uow platformport.UnitOfWork, repository SendAttemptRepository, provider ProviderAdapter, rate RateGate) *SenderService {
	return &SenderService{uow: uow, repository: repository, provider: provider, rate: rate}
}

// Execute reserves a durable attempt before crossing the provider boundary.
// A replay of dispatching is classified unknown and never invokes the provider
// again; O5 owns future task status/event projection from this stable receipt.
func (service *SenderService) Execute(ctx context.Context, command SendCommand) (SendAttempt, error) {
	if ctx == nil || service == nil || service.uow == nil || service.repository == nil || service.provider == nil || service.rate == nil || !validSendCommand(command) {
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
		return attempt, nil
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
		attempt, started = current, didStart
		return nil
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	if !started {
		if terminalSendAttempt(attempt.State) {
			return attempt, nil
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
		completed = stored
		return nil
	})
	if err != nil {
		return SendAttempt{}, sendError(err)
	}
	return completed, nil
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

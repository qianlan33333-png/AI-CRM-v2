package app

import (
	"context"
	"errors"
	"testing"
)

type senderTestRepository struct {
	attempt       SendAttempt
	task          SendRequest
	completeErr   error
	reserveCalls  int
	startCalls    int
	loadCalls     int
	completeCalls int
}

func (repository *senderTestRepository) ReserveSendAttempt(_ context.Context, command SendCommand) (SendAttempt, error) {
	repository.reserveCalls++
	if repository.attempt.ID == 0 {
		repository.attempt = SendAttempt{ID: 31, RiverJobID: command.RiverJobID, TaskID: command.TaskID, JobKind: command.JobKind, State: SendAttemptReserved}
	}
	return repository.attempt, nil
}

func (repository *senderTestRepository) StartSendAttempt(context.Context, int64) (SendAttempt, bool, error) {
	repository.startCalls++
	if repository.attempt.State != SendAttemptReserved {
		return repository.attempt, false, nil
	}
	repository.attempt.State = SendAttemptDispatching
	return repository.attempt, true, nil
}

func (repository *senderTestRepository) LoadSendRequest(context.Context, TaskID) (SendRequest, error) {
	repository.loadCalls++
	return repository.task, nil
}

func (repository *senderTestRepository) CompleteSendAttempt(_ context.Context, command CompleteSendAttempt) (SendAttempt, error) {
	repository.completeCalls++
	if repository.completeErr != nil {
		return SendAttempt{}, repository.completeErr
	}
	repository.attempt.State = command.State
	repository.attempt.FailureKind = command.FailureKind
	repository.attempt.ProviderCode = command.ProviderCode
	repository.attempt.ProviderMessageID = command.ProviderMessageID
	return repository.attempt, nil
}

type senderTestProvider struct {
	result ProviderResult
	err    error
	calls  int
}

func (provider *senderTestProvider) Send(context.Context, SendRequest) (ProviderResult, error) {
	provider.calls++
	return provider.result, provider.err
}

type senderTestGate struct{ calls int }

func (gate *senderTestGate) Wait(context.Context) error {
	gate.calls++
	return nil
}

func TestSenderCompletesStableSuccessReceipt(t *testing.T) {
	repository := &senderTestRepository{task: senderTestRequest()}
	provider := &senderTestProvider{result: ProviderResult{MessageID: "fixture-message-41"}}
	gate := &senderTestGate{}
	service := NewSenderService(enqueueTestUoW{}, repository, provider, gate)

	got, err := service.Execute(context.Background(), senderTestCommand())
	if err != nil || got.State != SendAttemptSucceeded || got.ProviderMessageID != "fixture-message-41" {
		t.Fatalf("Execute()=%+v err=%v", got, err)
	}
	if provider.calls != 1 || gate.calls != 1 || repository.reserveCalls != 1 || repository.startCalls != 1 || repository.loadCalls != 1 || repository.completeCalls != 1 {
		t.Fatalf("provider/gate/reserve/start/load/complete=%d/%d/%d/%d/%d/%d", provider.calls, gate.calls, repository.reserveCalls, repository.startCalls, repository.loadCalls, repository.completeCalls)
	}
}

func TestSenderClassifiesProviderFailures(t *testing.T) {
	tests := []struct {
		name        string
		kind        ProviderFailureKind
		providerErr error
		want        SendAttemptState
		wantKind    ProviderFailureKind
	}{
		{"timeout", ProviderFailureTimeout, nil, SendAttemptOutcomeUnknown, ProviderFailureTimeout},
		{"connection", ProviderFailureConnection, nil, SendAttemptOutcomeUnknown, ProviderFailureConnection},
		{"no response 5xx", ProviderFailureNoResponse5xx, nil, SendAttemptOutcomeUnknown, ProviderFailureNoResponse5xx},
		{"rate limited", ProviderFailureRateLimited, nil, SendAttemptRetryableFailed, ProviderFailureRateLimited},
		{"temporary", ProviderFailureTemporary, nil, SendAttemptRetryableFailed, ProviderFailureTemporary},
		{"invalid argument", ProviderFailureInvalidArgument, nil, SendAttemptFinalFailed, ProviderFailureInvalidArgument},
		{"recipient unavailable", ProviderFailureRecipientUnavailable, nil, SendAttemptFinalFailed, ProviderFailureRecipientUnavailable},
		{"adapter error", "", errors.New("fixture adapter failure"), SendAttemptOutcomeUnknown, ProviderFailureAdapterError},
		{"invalid result", "unexpected", nil, SendAttemptOutcomeUnknown, ProviderFailureInvalidResult},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &senderTestRepository{task: senderTestRequest()}
			provider := &senderTestProvider{result: ProviderResult{FailureKind: test.kind, Code: "fixture-code"}, err: test.providerErr}
			got, err := NewSenderService(enqueueTestUoW{}, repository, provider, &senderTestGate{}).Execute(context.Background(), senderTestCommand())
			if err != nil || got.State != test.want || got.FailureKind != test.wantKind || got.ProviderCode == "" {
				t.Fatalf("Execute()=%+v err=%v, want state=%s kind=%s", got, err, test.want, test.wantKind)
			}
		})
	}
}

func TestSenderReplayAfterDispatchNeverCallsProviderAgain(t *testing.T) {
	command := senderTestCommand()
	repository := &senderTestRepository{attempt: SendAttempt{ID: 31, RiverJobID: command.RiverJobID, TaskID: command.TaskID, JobKind: command.JobKind, State: SendAttemptDispatching}}
	provider := &senderTestProvider{}
	got, err := NewSenderService(enqueueTestUoW{}, repository, provider, &senderTestGate{}).Execute(context.Background(), command)
	if err != nil || got.State != SendAttemptOutcomeUnknown || got.FailureKind != ProviderFailureInterruptedDispatch {
		t.Fatalf("Execute()=%+v err=%v", got, err)
	}
	if provider.calls != 0 || repository.startCalls != 0 || repository.loadCalls != 0 || repository.completeCalls != 1 {
		t.Fatalf("provider/start/load/complete=%d/%d/%d/%d, want 0/0/0/1", provider.calls, repository.startCalls, repository.loadCalls, repository.completeCalls)
	}
}

func TestSenderReturnsCompletedReceiptWithoutProviderCall(t *testing.T) {
	command := senderTestCommand()
	want := SendAttempt{ID: 31, RiverJobID: command.RiverJobID, TaskID: command.TaskID, JobKind: command.JobKind, State: SendAttemptSucceeded, ProviderMessageID: "fixture-original"}
	repository := &senderTestRepository{attempt: want}
	provider := &senderTestProvider{}
	got, err := NewSenderService(enqueueTestUoW{}, repository, provider, &senderTestGate{}).Execute(context.Background(), command)
	if err != nil || got != want || provider.calls != 0 || repository.completeCalls != 0 {
		t.Fatalf("Execute()=%+v err=%v provider=%d complete=%d", got, err, provider.calls, repository.completeCalls)
	}
}

func TestSenderPersistenceLossLeavesReplayConservativelyUnknown(t *testing.T) {
	command := senderTestCommand()
	repository := &senderTestRepository{task: senderTestRequest(), completeErr: errors.New("fixture commit lost")}
	provider := &senderTestProvider{result: ProviderResult{MessageID: "fixture-message-41"}}
	service := NewSenderService(enqueueTestUoW{}, repository, provider, &senderTestGate{})
	if _, err := service.Execute(context.Background(), command); !errors.Is(err, ErrSendAttemptFailed) {
		t.Fatalf("first Execute() error=%v, want %v", err, ErrSendAttemptFailed)
	}
	repository.completeErr = nil
	got, err := service.Execute(context.Background(), command)
	if err != nil || got.State != SendAttemptOutcomeUnknown || got.FailureKind != ProviderFailureInterruptedDispatch {
		t.Fatalf("replay Execute()=%+v err=%v", got, err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls=%d, want exactly one", provider.calls)
	}
}

func senderTestCommand() SendCommand {
	return SendCommand{RiverJobID: 21, TaskID: 22, JobKind: OutboundEnqueueOneJobKind}
}

func senderTestRequest() SendRequest {
	return SendRequest{TaskID: 22, CustomerID: 23, TemplateKey: TemplateTextNoticeV1, Payload: []byte(`{"text":"fixture"}`)}
}

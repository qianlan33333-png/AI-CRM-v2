package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestRegisterSenderWorkersFreezesBothKindsToOutboundQueue(t *testing.T) {
	registry := platformjobqueue.NewWorkerRegistry()
	service := outboundapp.NewSenderService(senderWorkerUoW{}, &senderWorkerRepository{}, senderWorkerEvents{}, senderWorkerProvider{}, senderWorkerGate{})
	if err := RegisterSenderWorkers(registry, service); err != nil {
		t.Fatal(err)
	}
	for _, args := range []river.JobArgs{outboundapp.EnqueueOneArgs{}, outboundapp.EnqueueBatchTaskArgs{}} {
		options, err := registry.ExplicitOptions(platformjobqueue.QueueOutbound, args, nil)
		if err != nil || options.Queue != string(platformjobqueue.QueueOutbound) {
			t.Fatalf("kind=%s options=%+v err=%v", args.Kind(), options, err)
		}
	}
}

func TestTokenBucketRejectsOutOfContractRate(t *testing.T) {
	for _, rate := range []int{0, 51} {
		if _, err := NewTokenBucket(rate); err == nil {
			t.Fatalf("NewTokenBucket(%d) succeeded", rate)
		}
	}
	bucket, err := NewTokenBucket(1)
	if err != nil {
		t.Fatal(err)
	}
	if err = bucket.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = bucket.Wait(ctx); err != context.DeadlineExceeded {
		t.Fatalf("second Wait() error=%v, want deadline", err)
	}
}

func TestSenderWorkerPassesRiverAttemptContractAndRetriesOnlyRetryable(t *testing.T) {
	for _, test := range []struct {
		name    string
		state   outboundapp.SendAttemptState
		wantErr error
	}{
		{"retryable failed", outboundapp.SendAttemptRetryableFailed, ErrRetryableSendAttempt},
		{"outcome unknown", outboundapp.SendAttemptOutcomeUnknown, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &senderWorkerRepository{state: test.state}
			service := outboundapp.NewSenderService(senderWorkerUoW{}, repository, senderWorkerEvents{}, senderWorkerProvider{}, senderWorkerGate{})
			worker, err := NewEnqueueOneSender(service)
			if err != nil {
				t.Fatal(err)
			}
			err = worker.Work(context.Background(), &river.Job[outboundapp.EnqueueOneArgs]{
				JobRow: &rivertype.JobRow{ID: 31, Attempt: 2, MaxAttempts: 5, State: rivertype.JobStateRunning},
				Args:   outboundapp.EnqueueOneArgs{TaskID: 32, ReceiptID: 33},
			})
			if !errors.Is(err, test.wantErr) || (test.wantErr == nil && err != nil) {
				t.Fatalf("Work() error=%v, want %v", err, test.wantErr)
			}
			if repository.command.RiverJobID != 31 || repository.command.TaskID != 32 || repository.command.RiverAttempt != 2 ||
				repository.command.RiverMaxAttempts != 5 || repository.command.RiverJobState != string(rivertype.JobStateRunning) {
				t.Fatalf("River command=%+v, want job/task/attempt/max/state 31/32/2/5/running", repository.command)
			}
		})
	}
}

type senderWorkerUoW struct{}

func (senderWorkerUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type senderWorkerRepository struct {
	command outboundapp.SendCommand
	state   outboundapp.SendAttemptState
}

func (repository *senderWorkerRepository) ReserveSendAttempt(_ context.Context, command outboundapp.SendCommand) (outboundapp.SendAttempt, error) {
	repository.command = command
	return outboundapp.SendAttempt{
		ID: 41, HistoryID: 42, RiverJobID: command.RiverJobID, TaskID: command.TaskID, JobKind: command.JobKind,
		RiverAttempt: command.RiverAttempt, RiverMaxAttempts: command.RiverMaxAttempts, State: repository.state,
		CompletedAt: time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
	}, nil
}
func (*senderWorkerRepository) StartSendAttempt(context.Context, outboundapp.SendAttempt) (outboundapp.SendAttempt, bool, error) {
	return outboundapp.SendAttempt{}, false, nil
}
func (*senderWorkerRepository) LoadSendRequest(context.Context, outboundapp.TaskID) (outboundapp.SendRequest, error) {
	return outboundapp.SendRequest{}, nil
}
func (*senderWorkerRepository) CompleteSendAttempt(context.Context, outboundapp.CompleteSendAttempt) (outboundapp.SendAttempt, error) {
	return outboundapp.SendAttempt{}, nil
}
func (*senderWorkerRepository) MarkTaskSending(context.Context, outboundapp.SendAttempt) error {
	return nil
}
func (*senderWorkerRepository) ProjectTaskResult(_ context.Context, attempt outboundapp.SendAttempt) (outboundapp.TaskResultFact, error) {
	return outboundapp.TaskResultFact{
		TaskID: attempt.TaskID, CustomerID: 43, AttemptID: attempt.ID, HistoryID: attempt.HistoryID,
		RiverJobID: attempt.RiverJobID, RiverAttempt: attempt.RiverAttempt, RiverMaxAttempts: attempt.RiverMaxAttempts,
		Status: workerTaskStatus(attempt.State), AttemptCount: attempt.RiverAttempt, OccurredAt: attempt.CompletedAt,
	}, nil
}

func workerTaskStatus(state outboundapp.SendAttemptState) outboundapp.TaskStatus {
	if state == outboundapp.SendAttemptRetryableFailed {
		return outboundapp.TaskStatusRetryableFailed
	}
	return outboundapp.TaskStatusOutcomeUnknown
}

type senderWorkerEvents struct{}

func (senderWorkerEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 1, nil
}

type senderWorkerProvider struct{}

func (senderWorkerProvider) Send(context.Context, outboundapp.SendRequest) (outboundapp.ProviderResult, error) {
	return outboundapp.ProviderResult{}, nil
}

type senderWorkerGate struct{}

func (senderWorkerGate) Wait(context.Context) error { return nil }

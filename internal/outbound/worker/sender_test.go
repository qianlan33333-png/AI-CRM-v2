package worker

import (
	"context"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
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

type senderWorkerUoW struct{}

func (senderWorkerUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type senderWorkerRepository struct{}

func (*senderWorkerRepository) ReserveSendAttempt(context.Context, outboundapp.SendCommand) (outboundapp.SendAttempt, error) {
	return outboundapp.SendAttempt{}, nil
}
func (*senderWorkerRepository) StartSendAttempt(context.Context, int64) (outboundapp.SendAttempt, bool, error) {
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
func (*senderWorkerRepository) ProjectTaskResult(context.Context, outboundapp.SendAttempt) (outboundapp.TaskResultFact, error) {
	return outboundapp.TaskResultFact{}, nil
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

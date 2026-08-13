// Package worker binds outbound application services to their River job kinds.
package worker

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var (
	ErrInvalidSenderWorker  = errors.New("invalid outbound sender worker")
	ErrRetryableSendAttempt = errors.New("outbound send attempt is retryable")
)

type EnqueueOneSender struct {
	river.WorkerDefaults[outboundapp.EnqueueOneArgs]
	sender *outboundapp.SenderService
}

type EnqueueBatchTaskSender struct {
	river.WorkerDefaults[outboundapp.EnqueueBatchTaskArgs]
	sender *outboundapp.SenderService
}

func RegisterSenderWorkers(registry *platformjobqueue.WorkerRegistry, sender *outboundapp.SenderService) error {
	one, err := NewEnqueueOneSender(sender)
	if err != nil {
		return err
	}
	batch, err := NewEnqueueBatchTaskSender(sender)
	if err != nil {
		return err
	}
	if err = platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, one); err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, batch)
}

func NewEnqueueOneSender(sender *outboundapp.SenderService) (*EnqueueOneSender, error) {
	if sender == nil {
		return nil, ErrInvalidSenderWorker
	}
	return &EnqueueOneSender{sender: sender}, nil
}

func NewEnqueueBatchTaskSender(sender *outboundapp.SenderService) (*EnqueueBatchTaskSender, error) {
	if sender == nil {
		return nil, ErrInvalidSenderWorker
	}
	return &EnqueueBatchTaskSender{sender: sender}, nil
}

func (worker *EnqueueOneSender) Work(ctx context.Context, job *river.Job[outboundapp.EnqueueOneArgs]) error {
	if worker == nil || worker.sender == nil || job == nil || job.JobRow == nil || job.ID <= 0 || job.Attempt <= 0 ||
		job.MaxAttempts < job.Attempt || job.Attempt > math.MaxInt32 || job.MaxAttempts > math.MaxInt32 ||
		job.Args.TaskID <= 0 || job.Args.ReceiptID <= 0 {
		return ErrInvalidSenderWorker
	}
	return executeSender(ctx, worker.sender, outboundapp.SendCommand{
		RiverJobID: job.ID, TaskID: job.Args.TaskID, JobKind: outboundapp.OutboundEnqueueOneJobKind,
		RiverAttempt: int32(job.Attempt), RiverMaxAttempts: int32(job.MaxAttempts), RiverJobState: string(job.State),
	})
}

func (worker *EnqueueBatchTaskSender) Work(ctx context.Context, job *river.Job[outboundapp.EnqueueBatchTaskArgs]) error {
	if worker == nil || worker.sender == nil || job == nil || job.JobRow == nil || job.ID <= 0 || job.Attempt <= 0 ||
		job.MaxAttempts < job.Attempt || job.Attempt > math.MaxInt32 || job.MaxAttempts > math.MaxInt32 ||
		job.Args.BatchID <= 0 || job.Args.ChunkIndex < 0 || job.Args.TaskID <= 0 {
		return ErrInvalidSenderWorker
	}
	return executeSender(ctx, worker.sender, outboundapp.SendCommand{
		RiverJobID: job.ID, TaskID: job.Args.TaskID, JobKind: outboundapp.OutboundEnqueueBatchJobKind,
		RiverAttempt: int32(job.Attempt), RiverMaxAttempts: int32(job.MaxAttempts), RiverJobState: string(job.State),
	})
}

func executeSender(ctx context.Context, sender *outboundapp.SenderService, command outboundapp.SendCommand) error {
	attempt, err := sender.Execute(ctx, command)
	if err != nil {
		return err
	}
	if attempt.State == outboundapp.SendAttemptRetryableFailed {
		return ErrRetryableSendAttempt
	}
	return nil
}

func (*EnqueueOneSender) Timeout(*river.Job[outboundapp.EnqueueOneArgs]) time.Duration {
	return 30 * time.Second
}

func (*EnqueueBatchTaskSender) Timeout(*river.Job[outboundapp.EnqueueBatchTaskArgs]) time.Duration {
	return 30 * time.Second
}

// TokenBucket is a process-local gate shared by both outbound job kinds. It
// deliberately contains no provider credentials or external side effects.
type TokenBucket struct {
	mu       sync.Mutex
	rate     float64
	capacity float64
	tokens   float64
	last     time.Time
}

func NewTokenBucket(ratePerSecond int) (*TokenBucket, error) {
	if ratePerSecond < 1 || ratePerSecond > 50 {
		return nil, ErrInvalidSenderWorker
	}
	now := time.Now()
	rate := float64(ratePerSecond)
	return &TokenBucket{rate: rate, capacity: rate, tokens: rate, last: now}, nil
}

func (bucket *TokenBucket) Wait(ctx context.Context) error {
	if ctx == nil || bucket == nil || bucket.rate <= 0 || bucket.capacity <= 0 {
		return ErrInvalidSenderWorker
	}
	for {
		bucket.mu.Lock()
		now := time.Now()
		bucket.tokens = math.Min(bucket.capacity, bucket.tokens+now.Sub(bucket.last).Seconds()*bucket.rate)
		bucket.last = now
		if bucket.tokens >= 1 {
			bucket.tokens--
			bucket.mu.Unlock()
			return nil
		}
		wait := time.Duration((1-bucket.tokens)/bucket.rate*float64(time.Second)) + time.Nanosecond
		bucket.mu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

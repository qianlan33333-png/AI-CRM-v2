// Package worker owns the critical River boundary for WeCom contact ingress.
// The worker only processes local inbox facts; it has no Provider client.
package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	"github.com/riverqueue/river"
)

var ErrInvalidInboundWorker = errors.New("invalid WeCom inbound worker")

type InboundWorker struct {
	river.WorkerDefaults[wecomapp.InboundJobArgs]
	service *wecomapp.InboundService
}

func NewInboundWorker(service *wecomapp.InboundService) (*InboundWorker, error) {
	if service == nil {
		return nil, ErrInvalidInboundWorker
	}
	return &InboundWorker{service: service}, nil
}

func RegisterInboundWorker(registry *platformjobqueue.WorkerRegistry, service *wecomapp.InboundService) error {
	worker, err := NewInboundWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, worker)
}

func (worker *InboundWorker) Work(ctx context.Context, job *river.Job[wecomapp.InboundJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.ID <= 0 || job.Args.InboxID <= 0 {
		return ErrInvalidInboundWorker
	}
	err := worker.service.Process(ctx, job.Args.InboxID, fmt.Sprintf("river:%d", job.ID))
	return inboundWorkResult(err)
}

func inboundWorkResult(err error) error {
	if errors.Is(err, wecomapp.ErrInboundIdentityPending) {
		return river.JobSnooze(wecomapp.InboundIdentityRetryPeriod)
	}
	return err
}

func (*InboundWorker) Timeout(*river.Job[wecomapp.InboundJobArgs]) time.Duration {
	return platformjobqueue.CriticalJobTimeout
}

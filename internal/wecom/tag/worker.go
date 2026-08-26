package tag

import (
	"context"
	"errors"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidWorker = errors.New("invalid WeCom tag effect worker")

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service  *Service
	provider Provider
}

// NewWorker keeps provider selection in explicit composition. It performs no
// provider I/O while being constructed.
func NewWorker(service *Service, provider Provider) (*Worker, error) {
	if service == nil || nilDependency(provider) {
		return nil, ErrInvalidWorker
	}
	return &Worker{service: service, provider: provider}, nil
}

func RegisterWorker(registry *platformjobqueue.WorkerRegistry, service *Service, provider Provider) error {
	worker, err := NewWorker(service, provider)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueSync, worker)
}

func NewDisabledWorker(service *Service) (*Worker, error) {
	return NewWorker(service, DisabledProvider{})
}

func RegisterDisabledWorker(registry *platformjobqueue.WorkerRegistry, service *Service) error {
	return RegisterWorker(registry, service, DisabledProvider{})
}

func (worker *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if worker == nil || worker.service == nil || nilDependency(worker.provider) || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Args.EffectID == "" {
		return ErrInvalidWorker
	}
	_, err := worker.service.Execute(ctx, job.Args.EffectID, digest("worker", job.Args.EffectID), worker.provider)
	return err
}

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 30 * time.Second }

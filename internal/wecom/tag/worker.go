package tag

import (
	"context"
	"errors"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidWorker = errors.New("invalid WeCom tag effect worker")

// Worker deliberately hard-wires DisabledProvider. Enabling a real WeCom
// adapter requires a future explicit composition change; this commit cannot
// make network calls in production.
type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *Service
}

func NewDisabledWorker(service *Service) (*Worker, error) {
	if service == nil {
		return nil, ErrInvalidWorker
	}
	return &Worker{service: service}, nil
}

func RegisterDisabledWorker(registry *platformjobqueue.WorkerRegistry, service *Service) error {
	worker, err := NewDisabledWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueSync, worker)
}

func (worker *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Args.EffectID == "" {
		return ErrInvalidWorker
	}
	_, err := worker.service.Execute(ctx, job.Args.EffectID, digest("worker", job.Args.EffectID), DisabledProvider{})
	return err
}

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 30 * time.Second }

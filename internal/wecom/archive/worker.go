package archive

import (
	"context"
	"errors"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidWorker = errors.New("invalid WeCom message archive worker")

type JobArgs struct{}

func (JobArgs) Kind() string { return JobKind }

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *Service
}

func RegisterWorker(registry *platformjobqueue.WorkerRegistry, service *Service) error {
	if registry == nil || service == nil {
		return ErrInvalidWorker
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueSync, &Worker{service: service})
}

func (worker *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 {
		return ErrInvalidWorker
	}
	_, err := worker.service.Sync(ctx)
	return err
}

func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 15 * time.Minute }

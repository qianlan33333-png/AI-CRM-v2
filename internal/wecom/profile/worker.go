package profile

import (
	"context"
	"errors"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	"github.com/riverqueue/river"
)

var ErrInvalidWorker = errors.New("invalid WeCom contact profile effect worker")

type Worker struct {
	river.WorkerDefaults[JobArgs]
	service *Service
	writer  wecomport.ContactProfileWriter
}

func NewWorker(service *Service, writer wecomport.ContactProfileWriter) (*Worker, error) {
	if service == nil || writer == nil {
		return nil, ErrInvalidWorker
	}
	return &Worker{service: service, writer: writer}, nil
}
func RegisterWorker(registry *platformjobqueue.WorkerRegistry, service *Service, writer wecomport.ContactProfileWriter) error {
	worker, err := NewWorker(service, writer)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueSync, worker)
}
func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	if w == nil || w.service == nil || w.writer == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Args.EffectID == "" {
		return ErrInvalidWorker
	}
	_, err := w.service.Execute(ctx, job.Args.EffectID, digest("worker", job.Args.EffectID), w.writer)
	return err
}
func (*Worker) Timeout(*river.Job[JobArgs]) time.Duration { return 30 * time.Second }

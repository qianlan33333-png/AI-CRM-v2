package groupopsworker

import (
	"context"
	"errors"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

const (
	materialContinuationSnooze         = 15 * time.Minute
	materialManualReviewSnooze         = 6 * time.Hour
	maximumMaterialContinuationSnoozes = 48
)

type materialContinuationService interface {
	ContinueMaterialIntent(context.Context, eer.Digest) (groupopsapp.MaterialContinuationResult, error)
}

type MaterialContinuationWorker struct {
	river.WorkerDefaults[groupopsapp.GroupOpsMaterialContinuationJobArgs]
	service materialContinuationService
}

func NewMaterialContinuationWorker(service materialContinuationService) (*MaterialContinuationWorker, error) {
	if service == nil {
		return nil, ErrInvalidRiverDispatchWorker
	}
	return &MaterialContinuationWorker{service: service}, nil
}

func RegisterMaterialContinuationWorker(registry *platformjobqueue.WorkerRegistry, service materialContinuationService) error {
	worker, err := NewMaterialContinuationWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, worker)
}

func (worker *MaterialContinuationWorker) Work(ctx context.Context, job *river.Job[groupopsapp.GroupOpsMaterialContinuationJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Args.ExecutionKeyDigest == "" {
		return ErrInvalidRiverDispatchWorker
	}
	result, err := worker.service.ContinueMaterialIntent(ctx, eer.Digest(job.Args.ExecutionKeyDigest))
	if errors.Is(err, groupopsapp.ErrMaterialPreparationOutcomeUnknown) || result.ManualBlocker {
		// This only rechecks Media's local reconciliation projection. It never
		// replays the ambiguous upload effect.
		return river.JobSnooze(materialManualReviewSnooze)
	}
	if errors.Is(err, groupopsapp.ErrMaterialPreparationPending) || result.Pending {
		if job.Attempt < maximumMaterialContinuationSnoozes {
			return river.JobSnooze(materialContinuationSnooze)
		}
		return groupopsapp.ErrMaterialPreparationPending
	}
	return err
}

func (*MaterialContinuationWorker) Timeout(*river.Job[groupopsapp.GroupOpsMaterialContinuationJobArgs]) time.Duration {
	return 30 * time.Second
}

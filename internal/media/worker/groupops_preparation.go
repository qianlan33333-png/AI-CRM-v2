package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrGroupOpsMaterialPreparationWorker = errors.New("invalid group ops material preparation worker")

type groupOpsMaterialPreparationService interface {
	RunUploadEffect(context.Context, string, eer.Digest) error
}

type GroupOpsMaterialPreparationWorker struct {
	river.WorkerDefaults[mediaapp.GroupOpsMaterialPreparationJobArgs]
	service groupOpsMaterialPreparationService
}

func NewGroupOpsMaterialPreparationWorker(service groupOpsMaterialPreparationService) (*GroupOpsMaterialPreparationWorker, error) {
	if service == nil {
		return nil, ErrGroupOpsMaterialPreparationWorker
	}
	return &GroupOpsMaterialPreparationWorker{service: service}, nil
}

func RegisterGroupOpsMaterialPreparationWorker(registry *platformjobqueue.WorkerRegistry, service groupOpsMaterialPreparationService) error {
	worker, err := NewGroupOpsMaterialPreparationWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, worker)
}

func (worker *GroupOpsMaterialPreparationWorker) Work(ctx context.Context, job *river.Job[mediaapp.GroupOpsMaterialPreparationJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.EffectID == "" {
		return ErrGroupOpsMaterialPreparationWorker
	}
	sum := sha256.Sum256([]byte("media.group_ops_preparation.worker.v1\x00" + job.Args.EffectID))
	err := worker.service.RunUploadEffect(ctx, job.Args.EffectID, eer.Digest("sha256:"+hex.EncodeToString(sum[:])))
	if errors.Is(err, mediaapp.ErrGroupOpsMaterialAttemptStillRunning) {
		return river.JobSnooze(5 * time.Second)
	}
	return err
}

func (*GroupOpsMaterialPreparationWorker) Timeout(*river.Job[mediaapp.GroupOpsMaterialPreparationJobArgs]) time.Duration {
	return 60 * time.Second
}

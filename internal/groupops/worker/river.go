package groupopsworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var ErrInvalidRiverDispatchWorker = errors.New("invalid group ops River dispatch worker")

type riverDispatchService interface {
	Dispatch(context.Context, string, eer.Digest) (any, error)
}

type RiverDispatchWorker struct {
	river.WorkerDefaults[groupopsapp.GroupOpsDispatchJobArgs]
	service riverDispatchService
}

func NewRiverDispatchWorker(service riverDispatchService) (*RiverDispatchWorker, error) {
	if service == nil {
		return nil, ErrInvalidRiverDispatchWorker
	}
	return &RiverDispatchWorker{service: service}, nil
}

func RegisterDispatchWorker(registry *platformjobqueue.WorkerRegistry, service *DispatchWorker) error {
	worker, err := NewRiverDispatchWorker(dispatchWorkerService{worker: service})
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueOutbound, worker)
}

func (worker *RiverDispatchWorker) Work(ctx context.Context, job *river.Job[groupopsapp.GroupOpsDispatchJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 || job.Args.EffectID == "" {
		return ErrInvalidRiverDispatchWorker
	}
	sum := sha256.Sum256([]byte("group-ops.dispatch.worker.v1\x00" + job.Args.EffectID))
	_, err := worker.service.Dispatch(ctx, job.Args.EffectID, eer.Digest("sha256:"+hex.EncodeToString(sum[:])))
	return err
}
func (*RiverDispatchWorker) Timeout(*river.Job[groupopsapp.GroupOpsDispatchJobArgs]) time.Duration {
	return 30 * time.Second
}

type dispatchWorkerService struct{ worker *DispatchWorker }

func (service dispatchWorkerService) Dispatch(ctx context.Context, effectID string, digest eer.Digest) (any, error) {
	if service.worker == nil {
		return nil, ErrInvalidRiverDispatchWorker
	}
	return service.worker.Dispatch(ctx, effectID, digest)
}

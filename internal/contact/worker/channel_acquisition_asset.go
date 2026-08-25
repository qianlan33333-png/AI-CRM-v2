package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

var (
	ErrInvalidChannelAcquisitionAssetWorker   = errors.New("invalid contact acquisition asset worker")
	ErrChannelAcquisitionAssetRecoveryPending = errors.New("contact acquisition asset attempted recovery pending")
)

type channelAcquisitionAssetApplication interface {
	Execute(context.Context, string, eer.Digest) (contactapp.ChannelAcquisitionAssetExecution, error)
}

// ChannelAcquisitionAssetWorker accepts effect_id only. The immutable provider
// request is reloaded from the Contact-owned binding by the application.
type ChannelAcquisitionAssetWorker struct {
	river.WorkerDefaults[contactapp.ChannelAcquisitionAssetJobArgs]
	service channelAcquisitionAssetApplication
}

func NewChannelAcquisitionAssetWorker(service channelAcquisitionAssetApplication) (*ChannelAcquisitionAssetWorker, error) {
	if channelAcquisitionAssetWorkerNil(service) {
		return nil, ErrInvalidChannelAcquisitionAssetWorker
	}
	return &ChannelAcquisitionAssetWorker{service: service}, nil
}

func RegisterChannelAcquisitionAssetWorker(registry *platformjobqueue.WorkerRegistry, service channelAcquisitionAssetApplication) error {
	worker, err := NewChannelAcquisitionAssetWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, worker)
}

func (worker *ChannelAcquisitionAssetWorker) Work(ctx context.Context, job *river.Job[contactapp.ChannelAcquisitionAssetJobArgs]) error {
	if worker == nil || channelAcquisitionAssetWorkerNil(worker.service) || ctx == nil || job == nil || job.JobRow == nil ||
		job.ID < 1 || job.Attempt < 1 || job.Args.EffectID == "" {
		return ErrInvalidChannelAcquisitionAssetWorker
	}
	sum := sha256.Sum256([]byte("contact.acquisition.asset.worker.v1\x00" + job.Args.EffectID))
	result, err := worker.service.Execute(ctx, job.Args.EffectID, eer.Digest("sha256:"+hex.EncodeToString(sum[:])))
	if err != nil {
		return err
	}
	switch result.State {
	case eer.StateExecuted, eer.StateFinalFailed, eer.StateOutcomeUnknown, eer.StateReconciled:
		return nil
	case eer.StateAttempted:
		// This retry performs no Provider I/O. Once the persisted lease expires,
		// Execute converges the stuck attempt to outcome_unknown and returns nil.
		return ErrChannelAcquisitionAssetRecoveryPending
	default:
		return ErrInvalidChannelAcquisitionAssetWorker
	}
}

func (*ChannelAcquisitionAssetWorker) Timeout(*river.Job[contactapp.ChannelAcquisitionAssetJobArgs]) time.Duration {
	return platformjobqueue.CriticalJobTimeout
}

func channelAcquisitionAssetWorkerNil(value any) bool {
	if value == nil {
		return true
	}
	typed := reflect.ValueOf(value)
	switch typed.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return typed.IsNil()
	default:
		return false
	}
}

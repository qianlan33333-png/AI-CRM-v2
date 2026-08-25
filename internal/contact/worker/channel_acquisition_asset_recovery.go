package worker

import (
	"context"
	"reflect"
	"time"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
)

type channelAcquisitionAssetRecoveryApplication interface {
	EnqueueExpired(context.Context) (int, error)
}

type ChannelAcquisitionAssetRecoveryWorker struct {
	river.WorkerDefaults[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]
	service channelAcquisitionAssetRecoveryApplication
}

func NewChannelAcquisitionAssetRecoveryWorker(service channelAcquisitionAssetRecoveryApplication) (*ChannelAcquisitionAssetRecoveryWorker, error) {
	if channelAcquisitionAssetRecoveryWorkerNil(service) {
		return nil, ErrInvalidChannelAcquisitionAssetWorker
	}
	return &ChannelAcquisitionAssetRecoveryWorker{service: service}, nil
}

func RegisterChannelAcquisitionAssetRecoveryWorker(registry *platformjobqueue.WorkerRegistry, service channelAcquisitionAssetRecoveryApplication) error {
	worker, err := NewChannelAcquisitionAssetRecoveryWorker(service)
	if err != nil {
		return err
	}
	return platformjobqueue.AddWorker(registry, platformjobqueue.QueueCritical, worker)
}

func (worker *ChannelAcquisitionAssetRecoveryWorker) Work(ctx context.Context, job *river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]) error {
	if worker == nil || channelAcquisitionAssetRecoveryWorkerNil(worker.service) || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || job.Attempt < 1 {
		return ErrInvalidChannelAcquisitionAssetWorker
	}
	_, err := worker.service.EnqueueExpired(ctx)
	return err
}

func (*ChannelAcquisitionAssetRecoveryWorker) Timeout(*river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]) time.Duration {
	return platformjobqueue.CriticalJobTimeout
}

func channelAcquisitionAssetRecoveryWorkerNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

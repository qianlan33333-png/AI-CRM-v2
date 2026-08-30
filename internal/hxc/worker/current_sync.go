package worker

import (
	"context"
	"errors"
	"time"

	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	"github.com/riverqueue/river"
)

const CurrentSyncJobKind = "hxc_current_sync"

var ErrInvalidCurrentSyncWorker = errors.New("invalid hxc current sync worker")

type CurrentSyncJobArgs struct{}

func (CurrentSyncJobArgs) Kind() string { return CurrentSyncJobKind }

type CurrentSyncWorker struct {
	river.WorkerDefaults[CurrentSyncJobArgs]
	service *hxcapp.CurrentSyncService
}

func NewCurrentSyncWorker(service *hxcapp.CurrentSyncService) (*CurrentSyncWorker, error) {
	if service == nil {
		return nil, ErrInvalidCurrentSyncWorker
	}
	return &CurrentSyncWorker{service: service}, nil
}

func (worker *CurrentSyncWorker) Work(ctx context.Context, job *river.Job[CurrentSyncJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil {
		return ErrInvalidCurrentSyncWorker
	}
	_, err := worker.service.Sync(ctx)
	return err
}

func (*CurrentSyncWorker) Timeout(*river.Job[CurrentSyncJobArgs]) time.Duration {
	return 20 * time.Minute
}

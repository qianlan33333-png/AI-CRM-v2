// Package worker owns River adapters for WeCom domain services.
package worker

import (
	"context"
	"errors"
	"strings"
	"time"

	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	"github.com/riverqueue/river"
)

var ErrInvalidExternalContactSyncWorker = errors.New("invalid WeCom external contact sync worker")

type externalContactSyncApplication interface {
	SyncNext(context.Context, string) (wecomclient.ExternalContactPage, error)
}

// ExternalContactSyncWorker advances at most one persisted provider cursor
// page. It never registers itself; central composition decides whether an
// enabled provider and a controlled periodic schedule are available.
type ExternalContactSyncWorker struct {
	river.WorkerDefaults[wecomapp.ExternalContactSyncJobArgs]
	service externalContactSyncApplication
}

func NewExternalContactSyncWorker(service externalContactSyncApplication) (*ExternalContactSyncWorker, error) {
	if service == nil {
		return nil, ErrInvalidExternalContactSyncWorker
	}
	return &ExternalContactSyncWorker{service: service}, nil
}

func (worker *ExternalContactSyncWorker) Work(ctx context.Context, job *river.Job[wecomapp.ExternalContactSyncJobArgs]) error {
	if worker == nil || worker.service == nil || ctx == nil || job == nil || job.JobRow == nil || job.ID < 1 || !validExternalContactSyncStaff(job.Args.StaffUserID) {
		return ErrInvalidExternalContactSyncWorker
	}
	_, err := worker.service.SyncNext(ctx, job.Args.StaffUserID)
	if errors.Is(err, wecomapp.ErrCursorSyncDone) {
		return nil
	}
	return err
}

func (*ExternalContactSyncWorker) Timeout(*river.Job[wecomapp.ExternalContactSyncJobArgs]) time.Duration {
	return platformjobqueue.CriticalJobTimeout
}

func validExternalContactSyncStaff(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

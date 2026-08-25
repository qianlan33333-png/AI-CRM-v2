package worker

import (
	"context"
	"errors"
	"testing"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type acquisitionRecoveryWorkerApp struct {
	calls int
	err   error
}

func (app *acquisitionRecoveryWorkerApp) EnqueueExpired(context.Context) (int, error) {
	app.calls++
	return 2, app.err
}

func TestCH02RecoveryWorkerIsExplicitCriticalPeriodicTarget(t *testing.T) {
	app := &acquisitionRecoveryWorkerApp{}
	worker, err := NewChannelAcquisitionAssetRecoveryWorker(app)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]{
		JobRow: &rivertype.JobRow{ID: 71, Attempt: 1, State: rivertype.JobStateRunning},
	}
	if err = worker.Work(context.Background(), job); err != nil || app.calls != 1 {
		t.Fatalf("Work() calls=%d err=%v", app.calls, err)
	}
	registry := platformjobqueue.NewWorkerRegistry()
	if err = RegisterChannelAcquisitionAssetRecoveryWorker(registry, app); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.ExplicitOptions(platformjobqueue.QueueCritical, contactapp.ChannelAcquisitionAssetRecoveryJobArgs{}, nil); err != nil {
		t.Fatalf("critical recovery registration err=%v", err)
	}
	if _, err = registry.ExplicitOptions(platformjobqueue.QueueSync, contactapp.ChannelAcquisitionAssetRecoveryJobArgs{}, nil); err == nil {
		t.Fatal("sync queue unexpectedly accepted recovery job")
	}
}

func TestCH02RecoveryWorkerRejectsInvalidJobAndPropagatesError(t *testing.T) {
	sentinel := errors.New("recovery unavailable")
	app := &acquisitionRecoveryWorkerApp{err: sentinel}
	worker, err := NewChannelAcquisitionAssetRecoveryWorker(app)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(context.Background(), &river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]{}); !errors.Is(err, ErrInvalidChannelAcquisitionAssetWorker) {
		t.Fatalf("invalid job err=%v", err)
	}
	job := &river.Job[contactapp.ChannelAcquisitionAssetRecoveryJobArgs]{JobRow: &rivertype.JobRow{ID: 72, Attempt: 1}}
	if err = worker.Work(context.Background(), job); !errors.Is(err, sentinel) {
		t.Fatalf("application err=%v", err)
	}
	var typedNil *acquisitionRecoveryWorkerApp
	if worker, err = NewChannelAcquisitionAssetRecoveryWorker(typedNil); worker != nil || !errors.Is(err, ErrInvalidChannelAcquisitionAssetWorker) {
		t.Fatalf("typed-nil worker=%v err=%v", worker, err)
	}
}

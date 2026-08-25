package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

type channelAcquisitionAssetWorkerApp struct {
	calls    int
	effectID string
	digest   eer.Digest
	result   contactapp.ChannelAcquisitionAssetExecution
	err      error
}

func (app *channelAcquisitionAssetWorkerApp) Execute(_ context.Context, effectID string, digest eer.Digest) (contactapp.ChannelAcquisitionAssetExecution, error) {
	app.calls++
	app.effectID, app.digest = effectID, digest
	return app.result, app.err
}

func TestCH02WorkerCarriesEffectIDOnlyAndUnknownCompletesRiver(t *testing.T) {
	args := contactapp.ChannelAcquisitionAssetJobArgs{EffectID: "eer_ch02_41"}
	payload, err := json.Marshal(args)
	if err != nil || string(payload) != `{"effect_id":"eer_ch02_41"}` {
		t.Fatalf("job payload=%s err=%v", payload, err)
	}
	application := &channelAcquisitionAssetWorkerApp{result: contactapp.ChannelAcquisitionAssetExecution{State: eer.StateOutcomeUnknown}}
	worker, err := NewChannelAcquisitionAssetWorker(application)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[contactapp.ChannelAcquisitionAssetJobArgs]{
		JobRow: &rivertype.JobRow{ID: 41, Attempt: 1, State: rivertype.JobStateRunning}, Args: args,
	}
	if err = worker.Work(context.Background(), job); err != nil {
		t.Fatalf("outcome_unknown Work() error=%v", err)
	}
	if application.calls != 1 || application.effectID != args.EffectID || application.digest == "" {
		t.Fatalf("calls=%d effect=%q digest=%q", application.calls, application.effectID, application.digest)
	}
}

func TestCH02WorkerRetriesOnlyLiveAttemptedRecoveryAndPropagatesApplicationError(t *testing.T) {
	application := &channelAcquisitionAssetWorkerApp{result: contactapp.ChannelAcquisitionAssetExecution{State: eer.StateAttempted}}
	worker, err := NewChannelAcquisitionAssetWorker(application)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[contactapp.ChannelAcquisitionAssetJobArgs]{
		JobRow: &rivertype.JobRow{ID: 42, Attempt: 2, State: rivertype.JobStateRunning},
		Args:   contactapp.ChannelAcquisitionAssetJobArgs{EffectID: "eer_ch02_42"},
	}
	if err = worker.Work(context.Background(), job); !errors.Is(err, ErrChannelAcquisitionAssetRecoveryPending) {
		t.Fatalf("live attempted Work() error=%v", err)
	}
	wantErr := errors.New("application unavailable")
	application.err = wantErr
	if err = worker.Work(context.Background(), job); !errors.Is(err, wantErr) {
		t.Fatalf("application error=%v", err)
	}
}

func TestCH02WorkerRejectsInvalidDependenciesJobsAndQueue(t *testing.T) {
	var typedNil *channelAcquisitionAssetWorkerApp
	for _, service := range []channelAcquisitionAssetApplication{nil, typedNil} {
		if worker, err := NewChannelAcquisitionAssetWorker(service); worker != nil || !errors.Is(err, ErrInvalidChannelAcquisitionAssetWorker) {
			t.Fatalf("worker=%v err=%v", worker, err)
		}
	}
	application := &channelAcquisitionAssetWorkerApp{result: contactapp.ChannelAcquisitionAssetExecution{State: eer.StateExecuted}}
	worker, err := NewChannelAcquisitionAssetWorker(application)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(context.Background(), &river.Job[contactapp.ChannelAcquisitionAssetJobArgs]{}); !errors.Is(err, ErrInvalidChannelAcquisitionAssetWorker) {
		t.Fatalf("invalid job error=%v", err)
	}
	registry := platformjobqueue.NewWorkerRegistry()
	if err = RegisterChannelAcquisitionAssetWorker(registry, application); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.ExplicitOptions(platformjobqueue.QueueCritical, contactapp.ChannelAcquisitionAssetJobArgs{}, nil); err != nil {
		t.Fatalf("critical registration error=%v", err)
	}
	if _, err = registry.ExplicitOptions(platformjobqueue.QueueOutbound, contactapp.ChannelAcquisitionAssetJobArgs{}, nil); err == nil {
		t.Fatal("outbound queue unexpectedly accepted CH02 job")
	}
}

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	mediaapp "github.com/qianlan33333-png/AI-CRM-v2/internal/media/app"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestGroupOpsMaterialPreparationWorkerUsesOnlyEffectID(t *testing.T) {
	spy := &preparationWorkerSpy{}
	worker, err := NewGroupOpsMaterialPreparationWorker(spy)
	if err != nil {
		t.Fatal(err)
	}
	err = worker.Work(context.Background(), &river.Job[mediaapp.GroupOpsMaterialPreparationJobArgs]{JobRow: &rivertype.JobRow{ID: 17, Attempt: 1}, Args: mediaapp.GroupOpsMaterialPreparationJobArgs{EffectID: "eer_7"}})
	if err != nil || spy.effectID != "eer_7" || spy.digest == "" {
		t.Fatalf("err=%v spy=%+v", err, spy)
	}
	if err = worker.Work(context.Background(), &river.Job[mediaapp.GroupOpsMaterialPreparationJobArgs]{}); !errors.Is(err, ErrGroupOpsMaterialPreparationWorker) {
		t.Fatalf("err=%v", err)
	}
}

func TestGroupOpsMaterialPreparationWorkerSnoozesLiveAttemptAndKeepsPersistenceBudget(t *testing.T) {
	spy := &preparationWorkerSpy{err: mediaapp.ErrGroupOpsMaterialAttemptStillRunning}
	worker, err := NewGroupOpsMaterialPreparationWorker(spy)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[mediaapp.GroupOpsMaterialPreparationJobArgs]{JobRow: &rivertype.JobRow{ID: 17, Attempt: 2}, Args: mediaapp.GroupOpsMaterialPreparationJobArgs{EffectID: "eer_7"}}
	err = worker.Work(context.Background(), job)
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != 5*time.Second || worker.Timeout(job) != 60*time.Second {
		t.Fatalf("err=%v timeout=%s", err, worker.Timeout(job))
	}
}

type preparationWorkerSpy struct {
	effectID string
	digest   eer.Digest
	err      error
}

func (spy *preparationWorkerSpy) RunUploadEffect(_ context.Context, effectID string, digest eer.Digest) error {
	spy.effectID, spy.digest = effectID, digest
	return spy.err
}

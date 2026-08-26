package groupopsworker

import (
	"context"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestMaterialContinuationKeepsManualUnknownRecoverableWithoutReplayingUpload(t *testing.T) {
	service := &materialContinuationStub{result: groupopsapp.MaterialContinuationResult{Pending: true, ManualBlocker: true}, err: groupopsapp.ErrMaterialPreparationOutcomeUnknown}
	worker, err := NewMaterialContinuationWorker(service)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[groupopsapp.GroupOpsMaterialContinuationJobArgs]{JobRow: &rivertype.JobRow{ID: 1, Attempt: 3}, Args: groupopsapp.GroupOpsMaterialContinuationJobArgs{ExecutionKeyDigest: string(materialWorkerDigest("intent"))}}
	err = worker.Work(context.Background(), job)
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != materialManualReviewSnooze || service.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, service.calls)
	}
}

func TestMaterialContinuationPendingSnoozeIsBounded(t *testing.T) {
	service := &materialContinuationStub{result: groupopsapp.MaterialContinuationResult{Pending: true}, err: groupopsapp.ErrMaterialPreparationPending}
	worker, _ := NewMaterialContinuationWorker(service)
	job := &river.Job[groupopsapp.GroupOpsMaterialContinuationJobArgs]{JobRow: &rivertype.JobRow{ID: 1, Attempt: maximumMaterialContinuationSnoozes}, Args: groupopsapp.GroupOpsMaterialContinuationJobArgs{ExecutionKeyDigest: string(materialWorkerDigest("intent"))}}
	err := worker.Work(context.Background(), job)
	var snooze *river.JobSnoozeError
	if !errors.Is(err, groupopsapp.ErrMaterialPreparationPending) || errors.As(err, &snooze) {
		t.Fatalf("err=%v", err)
	}
}

type materialContinuationStub struct {
	result groupopsapp.MaterialContinuationResult
	err    error
	calls  int
}

func (stub *materialContinuationStub) ContinueMaterialIntent(context.Context, eer.Digest) (groupopsapp.MaterialContinuationResult, error) {
	stub.calls++
	return stub.result, stub.err
}

func materialWorkerDigest(value string) eer.Digest { return digest("material-" + value) }

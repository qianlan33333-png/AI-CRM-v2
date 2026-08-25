package worker

import (
	"context"
	"errors"
	"testing"

	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomclient "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/client"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func TestExternalContactSyncWorkerProcessesOneStaffPageAndCompletesDone(t *testing.T) {
	service := &externalContactSyncWorkerStub{}
	worker, err := NewExternalContactSyncWorker(service)
	if err != nil {
		t.Fatal(err)
	}
	job := &river.Job[wecomapp.ExternalContactSyncJobArgs]{
		JobRow: &rivertype.JobRow{ID: 41}, Args: wecomapp.ExternalContactSyncJobArgs{StaffUserID: "staff-1"},
	}
	if err = worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work() error = %v", err)
	}
	if service.staffUserID != "staff-1" || service.calls != 1 {
		t.Fatalf("sync calls=%d staff=%q", service.calls, service.staffUserID)
	}

	service.err = wecomapp.ErrCursorSyncDone
	if err = worker.Work(context.Background(), job); err != nil {
		t.Fatalf("completed Work() error = %v", err)
	}
}

func TestExternalContactSyncWorkerRejectsInvalidJobsAndPropagatesFailures(t *testing.T) {
	if _, err := NewExternalContactSyncWorker(nil); !errors.Is(err, ErrInvalidExternalContactSyncWorker) {
		t.Fatalf("NewExternalContactSyncWorker(nil) error = %v", err)
	}
	service := &externalContactSyncWorkerStub{err: wecomapp.ErrCursorSyncDisabled}
	worker, err := NewExternalContactSyncWorker(service)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Work(context.Background(), &river.Job[wecomapp.ExternalContactSyncJobArgs]{}); !errors.Is(err, ErrInvalidExternalContactSyncWorker) {
		t.Fatalf("invalid Work() error = %v", err)
	}
	job := &river.Job[wecomapp.ExternalContactSyncJobArgs]{
		JobRow: &rivertype.JobRow{ID: 42}, Args: wecomapp.ExternalContactSyncJobArgs{StaffUserID: "staff-2"},
	}
	if err = worker.Work(context.Background(), job); !errors.Is(err, wecomapp.ErrCursorSyncDisabled) {
		t.Fatalf("disabled Work() error = %v", err)
	}
}

type externalContactSyncWorkerStub struct {
	staffUserID string
	calls       int
	err         error
}

func (stub *externalContactSyncWorkerStub) SyncNext(_ context.Context, staffUserID string) (wecomclient.ExternalContactPage, error) {
	stub.calls++
	stub.staffUserID = staffUserID
	return wecomclient.ExternalContactPage{}, stub.err
}

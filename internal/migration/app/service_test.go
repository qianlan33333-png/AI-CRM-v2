package app

import (
	"context"
	"errors"
	"testing"

	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
)

type fakeRunner struct{ result migration.RunResult }

func (runner fakeRunner) Run(context.Context, migration.RunRequest) (migration.RunResult, error) {
	return runner.result, nil
}

type fakeReconciler struct{ called bool }

func (reconciler *fakeReconciler) Reconcile(context.Context, migration.RunID) error {
	reconciler.called = true
	return nil
}

type fakeReadiness struct{ value migration.Readiness }

func (readiness fakeReadiness) Readiness(context.Context, migration.RunID) (migration.Readiness, error) {
	return readiness.value, nil
}

func TestServiceDelegatesModuleOperations(t *testing.T) {
	reconciler := &fakeReconciler{}
	wantRun := migration.RunResult{Imported: 2}
	wantReadiness := migration.Readiness{RunID: "run-1", Phase: migration.PhaseReconciled, Ready: true}
	service := NewService(fakeRunner{result: wantRun}, reconciler, fakeReadiness{value: wantReadiness})
	if got, err := service.Execute(context.Background(), migration.RunRequest{ID: "run-1", Adapter: "fixture"}); err != nil || got != wantRun {
		t.Fatalf("Execute() = %#v, %v", got, err)
	}
	if err := service.Reconcile(context.Background(), "run-1"); err != nil || !reconciler.called {
		t.Fatalf("Reconcile() err=%v called=%t", err, reconciler.called)
	}
	if got, err := service.Readiness(context.Background(), "run-1"); err != nil || got != wantReadiness {
		t.Fatalf("Readiness() = %#v, %v", got, err)
	}
}

func TestServiceFailsClosedWithoutDependencies(t *testing.T) {
	service := NewService(nil, nil, nil)
	if _, err := service.Execute(context.Background(), migration.RunRequest{}); !errors.Is(err, migration.ErrInvalidRun) {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := service.Reconcile(context.Background(), "run"); !errors.Is(err, migration.ErrInvalidRun) {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := service.Readiness(context.Background(), "run"); !errors.Is(err, migration.ErrInvalidRun) {
		t.Fatalf("Readiness() error = %v", err)
	}
}

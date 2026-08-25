package app

import (
	"context"

	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
)

type Runner interface {
	Run(context.Context, migration.RunRequest) (migration.RunResult, error)
}

type Reconciler interface {
	Reconcile(context.Context, migration.RunID) error
}

// Service is the module-local operator application surface. HTTP/OpenAPI
// exposure is intentionally left to the serial integration lane.
type Service struct {
	runner     Runner
	reconciler Reconciler
	readiness  migration.ReadinessStore
}

func NewService(runner Runner, reconciler Reconciler, readiness migration.ReadinessStore) *Service {
	return &Service{runner: runner, reconciler: reconciler, readiness: readiness}
}

func (service *Service) Execute(ctx context.Context, request migration.RunRequest) (migration.RunResult, error) {
	if service == nil || service.runner == nil {
		return migration.RunResult{}, migration.ErrInvalidRun
	}
	return service.runner.Run(ctx, request)
}

func (service *Service) Reconcile(ctx context.Context, runID migration.RunID) error {
	if service == nil || service.reconciler == nil {
		return migration.ErrInvalidRun
	}
	return service.reconciler.Reconcile(ctx, runID)
}

func (service *Service) Readiness(ctx context.Context, runID migration.RunID) (migration.Readiness, error) {
	if service == nil || service.readiness == nil || runID == "" {
		return migration.Readiness{}, migration.ErrInvalidRun
	}
	return service.readiness.Readiness(ctx, runID)
}

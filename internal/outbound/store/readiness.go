package store

import (
	"context"
	"errors"

	dbgen "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
)

// ReadinessRepository owns the bounded outbound aggregate used by public
// readiness. It never returns a task, receipt, payload, or provider result.
type ReadinessRepository struct{ querier dbgen.Querier }

func NewReadinessRepository(db dbgen.DBTX) *ReadinessRepository {
	return &ReadinessRepository{querier: dbgen.New(db)}
}

func (repository *ReadinessRepository) CountOutcomeUnknown(ctx context.Context) (uint64, error) {
	if repository == nil {
		return 0, errors.New("outbound readiness repository is required")
	}
	value, err := repository.querier.CountOutcomeUnknownTasks(ctx)
	if err != nil || value < 0 {
		return 0, errors.New("outbound readiness aggregate failed")
	}
	return uint64(value), nil
}

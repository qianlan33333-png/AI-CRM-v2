package opsstore

import (
	"context"
	"errors"

	dbgen "github.com/qianlan33333-png/AI-CRM-v2/internal/ops/store/generated"
)

type ReadinessSnapshot struct {
	CurrentSchemaCompatible bool
}

// ReadinessRepository owns the read-only database and schema observations for
// the public system-health capability.
type ReadinessRepository struct{ querier dbgen.Querier }

func NewReadinessRepository(db dbgen.DBTX) *ReadinessRepository {
	return &ReadinessRepository{querier: dbgen.New(db)}
}

func (repository *ReadinessRepository) Observe(ctx context.Context) (ReadinessSnapshot, error) {
	if repository == nil {
		return ReadinessSnapshot{}, errors.New("ops readiness repository is required")
	}
	value, err := repository.querier.Ping(ctx)
	if err != nil || value != 1 {
		return ReadinessSnapshot{}, errors.New("ops readiness database probe failed")
	}
	compatible, err := repository.querier.CurrentSchemaCompatible(ctx)
	if err != nil {
		return ReadinessSnapshot{}, errors.New("ops readiness schema probe failed")
	}
	return ReadinessSnapshot{CurrentSchemaCompatible: compatible.Valid && compatible.Bool}, nil
}

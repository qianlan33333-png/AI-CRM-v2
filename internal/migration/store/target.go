package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type ApplyOperation func(context.Context, pgx.Tx, migration.LeaseFence, []byte) error
type VerifyOperation func(context.Context, pgx.Tx, migration.ResultReceipt) error

// TargetRegistry is sealed at composition time. Operation names route only to
// typed Go callbacks and can never contain caller-supplied SQL.
type TargetRegistry struct {
	apply  map[string]ApplyOperation
	verify map[string]VerifyOperation
}

func NewTargetRegistry(apply map[string]ApplyOperation, verify map[string]VerifyOperation) (*TargetRegistry, error) {
	registry := &TargetRegistry{apply: make(map[string]ApplyOperation, len(apply)), verify: make(map[string]VerifyOperation, len(verify))}
	for name, operation := range apply {
		if name == "" || operation == nil || verify[name] == nil {
			return nil, migration.ErrInvalidManifest
		}
		registry.apply[name] = operation
		registry.verify[name] = verify[name]
	}
	if len(registry.apply) == 0 || len(registry.apply) != len(verify) {
		return nil, migration.ErrInvalidManifest
	}
	return registry, nil
}

type Target struct{ registry *TargetRegistry }

var (
	_ migration.TargetWriter   = (*Target)(nil)
	_ migration.TargetVerifier = (*Target)(nil)
)

func NewTarget(registry *TargetRegistry) *Target { return &Target{registry: registry} }

func (target *Target) Apply(ctx context.Context, fence migration.LeaseFence, row migration.MappedRow) error {
	if target == nil || target.registry == nil || row.Operation == "" || row.Digest == (migration.Digest{}) {
		return migration.ErrInvalidRun
	}
	operation := target.registry.apply[row.Operation]
	if operation == nil {
		return migration.ErrInvalidRun
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return migration.ErrInvalidRun
	}
	return operation(ctx, tx, fence, append([]byte(nil), row.Payload...))
}

func (target *Target) VerifyResultReceipt(ctx context.Context, receipt migration.ResultReceipt) error {
	if target == nil || target.registry == nil || receipt.Operation == "" {
		if receipt.Disposition == migration.DispositionImport || receipt.Disposition == migration.DispositionRebuild || receipt.Disposition == migration.DispositionReset {
			return migration.ErrTargetTampered
		}
		return nil
	}
	operation := target.registry.verify[receipt.Operation]
	if operation == nil {
		return migration.ErrTargetTampered
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return migration.ErrInvalidRun
	}
	return operation(ctx, tx, receipt)
}

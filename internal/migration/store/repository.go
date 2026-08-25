package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
	migrationdb "github.com/qianlan33333-png/AI-CRM-v2/internal/migration/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{ pool *pgxpool.Pool }

var (
	_ migration.RunStore                   = (*Repository)(nil)
	_ migration.RowReceiptStore            = (*Repository)(nil)
	_ migration.ResultReceiptStore         = (*Repository)(nil)
	_ migration.ReconciliationReceiptStore = (*Repository)(nil)
	_ migration.QuarantineWriter           = (*Repository)(nil)
	_ migration.ArchiveWriter              = (*Repository)(nil)
	_ migration.ReadinessStore             = (*Repository)(nil)
)

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Open(ctx context.Context, start migration.StartRun) (migration.RunState, error) {
	q, err := repository.transaction(ctx)
	if err != nil {
		return migration.RunState{}, err
	}
	if start.ID == "" || start.Adapter == "" || start.SourceIdentity == "" || start.SourceSchemaDigest == (migration.Digest{}) || start.ManifestDigest == (migration.Digest{}) || len(start.Bounds) == 0 {
		return migration.RunState{}, migration.ErrInvalidRun
	}
	_, err = q.InsertRun(ctx, migrationdb.InsertRunParams{RunID: string(start.ID), AdapterID: string(start.Adapter), SourceIdentity: start.SourceIdentity, SourceSchemaDigest: bytes(start.SourceSchemaDigest), ManifestDigest: bytes(start.ManifestDigest)})
	if err == nil {
		for ordinal, bound := range start.Bounds {
			if bound.Table == "" || bound.SourceIdentity == "" || bound.SchemaDigest == (migration.Digest{}) || !validBound(bound.UpperBound) {
				return migration.RunState{}, migration.ErrInvalidRun
			}
			if err = q.InsertRunTable(ctx, migrationdb.InsertRunTableParams{RunID: string(start.ID), TableID: string(bound.Table), Ordinal: int32(ordinal), SourceIdentity: bound.SourceIdentity, SchemaDigest: bytes(bound.SchemaDigest), UpperBound: nullableBound(bound.UpperBound), UpperBoundEmpty: bound.Empty}); err != nil {
				return migration.RunState{}, translate(err)
			}
		}
		return loadWith(ctx, q, start.ID, true)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return migration.RunState{}, translate(err)
	}
	state, err := loadWith(ctx, q, start.ID, true)
	if err != nil {
		return migration.RunState{}, err
	}
	if state.Adapter != start.Adapter || state.SourceIdentity != start.SourceIdentity || state.SourceSchemaDigest != start.SourceSchemaDigest || state.ManifestDigest != start.ManifestDigest || !sameBounds(state, start.Bounds) {
		return migration.RunState{}, migration.ErrSourceDrift
	}
	if state.Phase != migration.PhaseRunning {
		return migration.RunState{}, migration.ErrInvalidRun
	}
	return state, nil
}

func (repository *Repository) Load(ctx context.Context, runID migration.RunID) (migration.RunState, error) {
	q, err := repository.queries(ctx)
	if err != nil {
		return migration.RunState{}, err
	}
	return loadWith(ctx, q, runID, false)
}

func loadWith(ctx context.Context, q migrationdb.Querier, runID migration.RunID, lock bool) (migration.RunState, error) {
	if runID == "" {
		return migration.RunState{}, migration.ErrInvalidRun
	}
	var row migrationdb.DataMigrationRun
	var err error
	if lock {
		row, err = q.LockRun(ctx, string(runID))
	} else {
		row, err = q.GetRun(ctx, string(runID))
	}
	if err != nil {
		return migration.RunState{}, translate(err)
	}
	state, err := runState(row)
	if err != nil {
		return migration.RunState{}, err
	}
	tables, err := q.ListRunTables(ctx, string(runID))
	if err != nil {
		return migration.RunState{}, translate(err)
	}
	state.Tables = make(map[migration.TableID]migration.TableCheckpoint, len(tables))
	for _, table := range tables {
		checkpoint, err := tableCheckpoint(table)
		if err != nil {
			return migration.RunState{}, err
		}
		state.Tables[migration.TableID(table.TableID)] = checkpoint
	}
	if len(state.Tables) == 0 {
		return migration.RunState{}, migration.ErrTargetTampered
	}
	return state, nil
}

func (repository *Repository) AcquireLease(ctx context.Context, runID migration.RunID, now time.Time, ttl time.Duration) (migration.LeaseFence, error) {
	q, err := repository.transaction(ctx)
	if err != nil {
		return migration.LeaseFence{}, err
	}
	if runID == "" || now.IsZero() || ttl <= 0 {
		return migration.LeaseFence{}, migration.ErrInvalidRun
	}
	run, err := q.LockRunPhaseNow(ctx, string(runID))
	if err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	now = timeValue(run.DatabaseNow)
	if run.Phase != string(migration.PhaseRunning) && run.Phase != string(migration.PhaseCompleted) {
		return migration.LeaseFence{}, migration.ErrInvalidRun
	}
	active, err := q.LockActiveLease(ctx, string(runID))
	if err == nil {
		if timeValue(active.ExpiresAt).After(now) {
			return migration.LeaseFence{}, migration.ErrLeaseFenced
		}
		if err = q.RetireActiveLease(ctx, migrationdb.RetireActiveLeaseParams{RunID: string(runID), RetiredAt: timestamp(now), Generation: active.Generation}); err != nil {
			return migration.LeaseFence{}, translate(err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return migration.LeaseFence{}, translate(err)
	}
	generation, err := q.NextLeaseGeneration(ctx, string(runID))
	if err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	token := migration.Digest{}
	if _, err = rand.Read(token[:]); err != nil {
		return migration.LeaseFence{}, fmt.Errorf("data migration fence entropy: %w", err)
	}
	expires := now.Add(ttl)
	if err = q.InsertLease(ctx, migrationdb.InsertLeaseParams{RunID: string(runID), Generation: generation, Fence: bytes(token), AcquiredAt: timestamp(now), ExpiresAt: timestamp(expires)}); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	return migration.LeaseFence{RunID: runID, Generation: uint64(generation), Token: token, ExpiresAt: expires}, nil
}

func (repository *Repository) RenewLease(ctx context.Context, fence migration.LeaseFence, now time.Time, ttl time.Duration) (migration.LeaseFence, error) {
	q, err := repository.transaction(ctx)
	if err != nil {
		return migration.LeaseFence{}, err
	}
	if !validFence(fence) || now.IsZero() || ttl <= 0 {
		return migration.LeaseFence{}, migration.ErrLeaseFenced
	}
	expires, err := q.LockLeaseExpiry(ctx, migrationdb.LockLeaseExpiryParams{RunID: string(fence.RunID), Generation: int64(fence.Generation), Fence: bytes(fence.Token)})
	if err != nil {
		return migration.LeaseFence{}, migration.ErrLeaseFenced
	}
	databaseNow, err := q.DatabaseNow(ctx)
	if err != nil || !timeValue(expires).After(timeValue(databaseNow)) {
		return migration.LeaseFence{}, migration.ErrLeaseFenced
	}
	desired := timeValue(databaseNow).Add(ttl)
	if !desired.After(timeValue(expires)) {
		fence.ExpiresAt = timeValue(expires)
		return fence, nil
	}
	if err = q.RenewLeaseExpiry(ctx, migrationdb.RenewLeaseExpiryParams{RunID: string(fence.RunID), Generation: int64(fence.Generation), Fence: bytes(fence.Token), ExpiresAt: timestamp(desired)}); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	fence.ExpiresAt = desired
	return fence, nil
}

func (repository *Repository) Advance(ctx context.Context, fence migration.LeaseFence, table migration.TableID, checkpoint migration.TableCheckpoint) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	if !validFence(fence) || table == "" || !validBound(checkpoint.UpperBound) {
		return migration.ErrInvalidRun
	}
	changed, err := q.AdvanceCheckpoint(ctx, migrationdb.AdvanceCheckpointParams{RunID: string(fence.RunID), TableID: string(table), LastLeaseGeneration: int8Value(int64(fence.Generation)), LastLeaseFence: bytes(fence.Token), Cursor: optionalText(string(checkpoint.Cursor)), Processed: int64(checkpoint.Processed), Complete: checkpoint.Complete, UpperBound: nullableBound(checkpoint.UpperBound), UpperBoundEmpty: checkpoint.UpperBound.Empty, SchemaDigest: bytes(checkpoint.SchemaDigest), SourceIdentity: checkpoint.SourceIdentity})
	if err != nil {
		return translate(err)
	}
	if changed != 1 {
		return migration.ErrLeaseFenced
	}
	return nil
}

func (repository *Repository) Finish(ctx context.Context, fence migration.LeaseFence) error {
	return repository.finish(ctx, fence, migration.PhaseCompleted)
}

func (repository *Repository) MarkReconciled(ctx context.Context, fence migration.LeaseFence) error {
	return repository.finish(ctx, fence, migration.PhaseReconciled)
}

func (repository *Repository) finish(ctx context.Context, fence migration.LeaseFence, phase migration.RunPhase) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	if !validFence(fence) {
		return migration.ErrLeaseFenced
	}
	lease, err := q.LockLeaseStatusNow(ctx, migrationdb.LockLeaseStatusNowParams{RunID: string(fence.RunID), Generation: int64(fence.Generation), Fence: bytes(fence.Token)})
	if err != nil || !lease.Active || !timeValue(lease.ExpiresAt).After(timeValue(lease.DatabaseNow)) {
		return migration.ErrLeaseFenced
	}
	now := timeValue(lease.DatabaseNow)
	if err = q.RetireLease(ctx, migrationdb.RetireLeaseParams{RunID: string(fence.RunID), Generation: int64(fence.Generation), Fence: bytes(fence.Token), RetiredAt: timestamp(now)}); err != nil {
		return translate(err)
	}
	var changed int64
	if phase == migration.PhaseCompleted {
		changed, err = q.CompleteRun(ctx, migrationdb.CompleteRunParams{RunID: string(fence.RunID), CompletedAt: timestamp(now)})
	} else {
		changed, err = q.MarkRunReconciled(ctx, migrationdb.MarkRunReconciledParams{RunID: string(fence.RunID), ReconciledAt: timestamp(now)})
	}
	if err != nil {
		return translate(err)
	}
	if changed != 1 {
		return migration.ErrTargetTampered
	}
	return nil
}

func (repository *Repository) FindRowReceipt(ctx context.Context, adapter migration.AdapterID, table migration.TableID, sourceKey migration.Digest) (migration.RowReceipt, bool, error) {
	q, err := repository.queries(ctx)
	if err != nil {
		return migration.RowReceipt{}, false, err
	}
	if _, err = platformstore.TxFromContext(ctx); err == nil {
		key := string(adapter) + ":" + string(table) + ":" + hex.EncodeToString(sourceKey[:])
		if err = q.LockRowReceiptAdvisory(ctx, key); err != nil {
			return migration.RowReceipt{}, false, translate(err)
		}
	}
	row, err := q.GetRowReceipt(ctx, migrationdb.GetRowReceiptParams{AdapterID: string(adapter), TableID: string(table), SourceKeyDigest: bytes(sourceKey)})
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.RowReceipt{}, false, nil
	}
	if err != nil {
		return migration.RowReceipt{}, false, translate(err)
	}
	receipt, err := rowReceipt(row)
	return receipt, err == nil, translate(err)
}

func (repository *Repository) AppendRowReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.RowReceipt) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	err = q.InsertRowReceipt(ctx, migrationdb.InsertRowReceiptParams{AdapterID: string(receipt.Adapter), TableID: string(receipt.Table), SourceKeyDigest: bytes(receipt.SourceKey), PayloadDigest: bytes(receipt.Payload), FieldDigest: bytes(receipt.Field), Disposition: string(receipt.Disposition), MappingDigest: bytes(receipt.Mapping), PolicyDigest: bytes(receipt.Policy), Operation: receipt.Operation, MutationDigest: bytes(receipt.Mutation), RunID: string(fence.RunID), LeaseGeneration: int64(fence.Generation), LeaseFence: bytes(fence.Token)})
	return translate(err)
}

func (repository *Repository) Quarantine(ctx context.Context, fence migration.LeaseFence, value migration.Quarantine) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	err = q.InsertQuarantine(ctx, migrationdb.InsertQuarantineParams{RunID: string(fence.RunID), AdapterID: string(value.Adapter), TableID: string(value.Table), SourceKeyDigest: bytes(value.SourceKey), PayloadDigest: bytes(value.Payload), FieldDigest: bytes(value.Field), Reason: value.Reason, LeaseGeneration: int64(fence.Generation), LeaseFence: bytes(fence.Token)})
	return translate(err)
}

func (repository *Repository) Archive(ctx context.Context, fence migration.LeaseFence, value migration.Archive) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	err = q.InsertArchive(ctx, migrationdb.InsertArchiveParams{RunID: string(fence.RunID), AdapterID: string(value.Adapter), TableID: string(value.Table), SourceKeyDigest: bytes(value.SourceKey), PayloadDigest: bytes(value.Payload), FieldDigest: bytes(value.Field), LeaseGeneration: int64(fence.Generation), LeaseFence: bytes(fence.Token)})
	return translate(err)
}

func (repository *Repository) FindResultReceipt(ctx context.Context, runID migration.RunID, adapter migration.AdapterID, table migration.TableID, sourceKey migration.Digest) (migration.ResultReceipt, bool, error) {
	q, err := repository.queries(ctx)
	if err != nil {
		return migration.ResultReceipt{}, false, err
	}
	row, err := q.GetResultReceipt(ctx, migrationdb.GetResultReceiptParams{RunID: string(runID), AdapterID: string(adapter), TableID: string(table), SourceKeyDigest: bytes(sourceKey)})
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.ResultReceipt{}, false, nil
	}
	if err != nil {
		return migration.ResultReceipt{}, false, translate(err)
	}
	receipt, err := resultReceipt(row)
	return receipt, err == nil, translate(err)
}

func (repository *Repository) AppendResultReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.ResultReceipt) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	err = q.InsertResultReceipt(ctx, migrationdb.InsertResultReceiptParams{RunID: string(receipt.RunID), AdapterID: string(receipt.Adapter), TableID: string(receipt.Table), SourceKeyDigest: bytes(receipt.SourceKey), PayloadDigest: bytes(receipt.Payload), FieldDigest: bytes(receipt.Field), Disposition: string(receipt.Disposition), MappingDigest: bytes(receipt.Mapping), PolicyDigest: bytes(receipt.Policy), Operation: receipt.Operation, MutationDigest: bytes(receipt.Mutation), Outcome: string(receipt.Outcome), LeaseGeneration: int64(fence.Generation), LeaseFence: bytes(fence.Token)})
	return translate(err)
}

func (repository *Repository) ListResultReceipts(ctx context.Context, runID migration.RunID) ([]migration.ResultReceipt, error) {
	q, err := repository.queries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListResultReceipts(ctx, string(runID))
	if err != nil {
		return nil, translate(err)
	}
	values := make([]migration.ResultReceipt, 0, len(rows))
	for _, row := range rows {
		value, err := resultReceipt(row)
		if err != nil {
			return nil, translate(err)
		}
		values = append(values, value)
	}
	return values, nil
}

func (repository *Repository) AppendReconciliationReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.ReconciliationReceipt) error {
	q, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	err = q.InsertReconciliationReceipt(ctx, migrationdb.InsertReconciliationReceiptParams{RunID: string(receipt.RunID), SourceRowCount: int64(receipt.SourceRowCount), ResultRowCount: int64(receipt.ResultRowCount), TargetVerifiedCount: int64(receipt.TargetVerifiedCount), ComparisonDigest: bytes(receipt.ComparisonDigest), LeaseGeneration: int64(fence.Generation), LeaseFence: bytes(fence.Token)})
	return translate(err)
}

func (repository *Repository) Readiness(ctx context.Context, runID migration.RunID) (migration.Readiness, error) {
	q, err := repository.queries(ctx)
	if err != nil {
		return migration.Readiness{}, err
	}
	row, err := q.GetReadiness(ctx, string(runID))
	if err != nil {
		return migration.Readiness{}, translate(err)
	}
	if row.PendingTables < 0 || row.ProcessedRows < 0 || row.ResultRows < 0 || row.QuarantinedRows < 0 {
		return migration.Readiness{}, migration.ErrTargetTampered
	}
	value := migration.Readiness{RunID: migration.RunID(row.RunID), Phase: migration.RunPhase(row.Phase), PendingTables: uint64(row.PendingTables), ProcessedRows: uint64(row.ProcessedRows), ResultRows: uint64(row.ResultRows), QuarantinedRows: uint64(row.QuarantinedRows), Reconciled: row.Reconciled}
	value.Ready = value.Phase == migration.PhaseReconciled && value.PendingTables == 0 && value.ProcessedRows == value.ResultRows && value.QuarantinedRows == 0 && value.Reconciled
	return value, nil
}

func (repository *Repository) queries(ctx context.Context) (migrationdb.Querier, error) {
	if repository == nil || repository.pool == nil {
		return nil, migration.ErrInvalidRun
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return migrationdb.New(tx), nil
	}
	return migrationdb.New(repository.pool), nil
}

func (repository *Repository) transaction(ctx context.Context) (migrationdb.Querier, error) {
	if repository == nil {
		return nil, migration.ErrInvalidRun
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, migration.ErrInvalidRun
	}
	return migrationdb.New(tx), nil
}

func runState(row migrationdb.DataMigrationRun) (migration.RunState, error) {
	source, err := digest(row.SourceSchemaDigest)
	if err != nil {
		return migration.RunState{}, err
	}
	manifest, err := digest(row.ManifestDigest)
	if err != nil {
		return migration.RunState{}, err
	}
	return migration.RunState{ID: migration.RunID(row.RunID), Adapter: migration.AdapterID(row.AdapterID), SourceIdentity: row.SourceIdentity, SourceSchemaDigest: source, ManifestDigest: manifest, Phase: migration.RunPhase(row.Phase)}, nil
}

func tableCheckpoint(row migrationdb.DataMigrationRunTable) (migration.TableCheckpoint, error) {
	schema, err := digest(row.SchemaDigest)
	if err != nil || row.Processed < 0 {
		return migration.TableCheckpoint{}, migration.ErrTargetTampered
	}
	value := migration.TableCheckpoint{SourceIdentity: row.SourceIdentity, SchemaDigest: schema, UpperBound: migration.UpperBound{Value: append([]byte(nil), row.UpperBound...), Empty: row.UpperBoundEmpty}, Processed: uint64(row.Processed), Complete: row.Complete}
	if row.Cursor.Valid {
		value.Cursor = migration.Cursor(row.Cursor.String)
	}
	return value, nil
}

func rowReceipt(row migrationdb.DataMigrationRowReceipt) (migration.RowReceipt, error) {
	source, err := digest(row.SourceKeyDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	payload, err := digest(row.PayloadDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	field, err := digest(row.FieldDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	mapping, err := digest(row.MappingDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	policy, err := digest(row.PolicyDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	mutation, err := digest(row.MutationDigest)
	if err != nil {
		return migration.RowReceipt{}, err
	}
	return migration.RowReceipt{Adapter: migration.AdapterID(row.AdapterID), Table: migration.TableID(row.TableID), SourceKey: source, Payload: payload, Field: field, Disposition: migration.Disposition(row.Disposition), Mapping: mapping, Policy: policy, Operation: row.Operation, Mutation: mutation}, nil
}

func resultReceipt(row migrationdb.DataMigrationResultReceipt) (migration.ResultReceipt, error) {
	source, err := digest(row.SourceKeyDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	payload, err := digest(row.PayloadDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	field, err := digest(row.FieldDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	mapping, err := digest(row.MappingDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	policy, err := digest(row.PolicyDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	mutation, err := digest(row.MutationDigest)
	if err != nil {
		return migration.ResultReceipt{}, err
	}
	return migration.ResultReceipt{RunID: migration.RunID(row.RunID), RowReceipt: migration.RowReceipt{Adapter: migration.AdapterID(row.AdapterID), Table: migration.TableID(row.TableID), SourceKey: source, Payload: payload, Field: field, Disposition: migration.Disposition(row.Disposition), Mapping: mapping, Policy: policy, Operation: row.Operation, Mutation: mutation}, MutationDigest: mutation, Outcome: migration.Disposition(row.Outcome)}, nil
}

func digest(value []byte) (migration.Digest, error) {
	var result migration.Digest
	if len(value) != len(result) {
		return result, migration.ErrTargetTampered
	}
	copy(result[:], value)
	return result, nil
}

func bytes(value migration.Digest) []byte { return append([]byte(nil), value[:]...) }

func validFence(fence migration.LeaseFence) bool {
	return fence.RunID != "" && fence.Generation > 0 && fence.Token != (migration.Digest{}) && !fence.ExpiresAt.IsZero()
}

func validBound(bound migration.UpperBound) bool {
	return (bound.Empty && len(bound.Value) == 0) || (!bound.Empty && len(bound.Value) > 0)
}

func nullableBound(bound migration.UpperBound) []byte {
	if bound.Empty {
		return nil
	}
	return append([]byte(nil), bound.Value...)
}

func optionalText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func int8Value(value int64) pgtype.Int8     { return pgtype.Int8{Int64: value, Valid: true} }
func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}
func timeValue(value pgtype.Timestamptz) time.Time { return value.Time }

func sameBounds(state migration.RunState, bounds []migration.TableBound) bool {
	if len(state.Tables) != len(bounds) {
		return false
	}
	for _, bound := range bounds {
		checkpoint, found := state.Tables[bound.Table]
		if !found || checkpoint.SourceIdentity != bound.SourceIdentity || checkpoint.SchemaDigest != bound.SchemaDigest || !sameBytes(checkpoint.UpperBound.Value, bound.Value) || checkpoint.UpperBound.Empty != bound.Empty {
			return false
		}
	}
	return true
}

func sameBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.ErrInvalidRun
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		message := strings.ToLower(pgErr.Message)
		switch {
		case strings.Contains(message, "lease") || pgErr.ConstraintName == "data_migration_one_active_lease_per_run":
			return fmt.Errorf("%w: %s", migration.ErrLeaseFenced, pgErr.Message)
		case pgErr.Code == "23505" && pgErr.ConstraintName == "data_migration_row_receipts_pkey":
			return fmt.Errorf("%w: %s", migration.ErrSourcePayloadConflict, pgErr.Message)
		case pgErr.Code == "23505" || pgErr.Code == "23514" || pgErr.Code == "23503" || pgErr.Code == "55000":
			return fmt.Errorf("%w: %s", migration.ErrTargetTampered, pgErr.Message)
		}
	}
	return fmt.Errorf("data migration store: %w", err)
}

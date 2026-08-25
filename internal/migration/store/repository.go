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
	"github.com/jackc/pgx/v5/pgxpool"
	migration "github.com/qianlan33333-png/AI-CRM-v2/internal/migration"
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
	tx, err := repository.transaction(ctx)
	if err != nil {
		return migration.RunState{}, err
	}
	if start.ID == "" || start.Adapter == "" || start.SourceIdentity == "" || start.SourceSchemaDigest == (migration.Digest{}) || start.ManifestDigest == (migration.Digest{}) || len(start.Bounds) == 0 {
		return migration.RunState{}, migration.ErrInvalidRun
	}
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO data_migration_runs(
run_id,adapter_id,source_identity,source_schema_digest,manifest_digest
) VALUES($1,$2,$3,$4,$5) ON CONFLICT(run_id) DO NOTHING RETURNING run_id`,
		start.ID, start.Adapter, start.SourceIdentity, bytes(start.SourceSchemaDigest), bytes(start.ManifestDigest)).Scan(&inserted)
	if err == nil {
		for ordinal, bound := range start.Bounds {
			if bound.Table == "" || bound.SourceIdentity == "" || bound.SchemaDigest == (migration.Digest{}) || !validBound(bound.UpperBound) {
				return migration.RunState{}, migration.ErrInvalidRun
			}
			var upper any
			if !bound.Empty {
				upper = bound.Value
			}
			if _, err = tx.Exec(ctx, `INSERT INTO data_migration_run_tables(
run_id,table_id,ordinal,source_identity,schema_digest,upper_bound,upper_bound_empty
) VALUES($1,$2,$3,$4,$5,$6,$7)`, start.ID, bound.Table, ordinal, bound.SourceIdentity, bytes(bound.SchemaDigest), upper, bound.Empty); err != nil {
				return migration.RunState{}, translate(err)
			}
		}
		return repository.loadWith(ctx, tx, start.ID, true)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return migration.RunState{}, translate(err)
	}
	state, err := repository.loadWith(ctx, tx, start.ID, true)
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
	db, err := repository.queryer(ctx)
	if err != nil {
		return migration.RunState{}, err
	}
	return repository.loadWith(ctx, db, runID, false)
}

func (repository *Repository) loadWith(ctx context.Context, db queryer, runID migration.RunID, lock bool) (migration.RunState, error) {
	if runID == "" {
		return migration.RunState{}, migration.ErrInvalidRun
	}
	query := `SELECT run_id,adapter_id,source_identity,source_schema_digest,manifest_digest,phase FROM data_migration_runs WHERE run_id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	var state migration.RunState
	var sourceSchema, manifest []byte
	var phase string
	var err error
	if err := db.QueryRow(ctx, query, runID).Scan(&state.ID, &state.Adapter, &state.SourceIdentity, &sourceSchema, &manifest, &phase); err != nil {
		return migration.RunState{}, translate(err)
	}
	if state.SourceSchemaDigest, err = digest(sourceSchema); err != nil {
		return migration.RunState{}, err
	}
	if state.ManifestDigest, err = digest(manifest); err != nil {
		return migration.RunState{}, err
	}
	state.Phase = migration.RunPhase(phase)
	rows, err := db.Query(ctx, `SELECT table_id,source_identity,schema_digest,upper_bound,upper_bound_empty,cursor,processed,complete
FROM data_migration_run_tables WHERE run_id=$1 ORDER BY ordinal`, runID)
	if err != nil {
		return migration.RunState{}, translate(err)
	}
	defer rows.Close()
	state.Tables = make(map[migration.TableID]migration.TableCheckpoint)
	for rows.Next() {
		var table migration.TableID
		var sourceIdentity string
		var schema, upper []byte
		var empty bool
		var cursor *string
		var processed int64
		var complete bool
		if err = rows.Scan(&table, &sourceIdentity, &schema, &upper, &empty, &cursor, &processed, &complete); err != nil {
			return migration.RunState{}, translate(err)
		}
		schemaDigest, digestErr := digest(schema)
		if digestErr != nil || processed < 0 {
			return migration.RunState{}, migration.ErrTargetTampered
		}
		checkpoint := migration.TableCheckpoint{SourceIdentity: sourceIdentity, SchemaDigest: schemaDigest, UpperBound: migration.UpperBound{Value: append([]byte(nil), upper...), Empty: empty}, Processed: uint64(processed), Complete: complete}
		if cursor != nil {
			checkpoint.Cursor = migration.Cursor(*cursor)
		}
		state.Tables[table] = checkpoint
	}
	if err = rows.Err(); err != nil {
		return migration.RunState{}, translate(err)
	}
	if len(state.Tables) == 0 {
		return migration.RunState{}, migration.ErrTargetTampered
	}
	return state, nil
}

func (repository *Repository) AcquireLease(ctx context.Context, runID migration.RunID, now time.Time, ttl time.Duration) (migration.LeaseFence, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return migration.LeaseFence{}, err
	}
	if runID == "" || now.IsZero() || ttl <= 0 {
		return migration.LeaseFence{}, migration.ErrInvalidRun
	}
	var phase string
	if err = tx.QueryRow(ctx, `SELECT phase,clock_timestamp() FROM data_migration_runs WHERE run_id=$1 FOR UPDATE`, runID).Scan(&phase, &now); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	if phase != string(migration.PhaseRunning) && phase != string(migration.PhaseCompleted) {
		return migration.LeaseFence{}, migration.ErrInvalidRun
	}
	var activeGeneration int64
	var activeExpiry time.Time
	err = tx.QueryRow(ctx, `SELECT generation,expires_at FROM data_migration_run_leases WHERE run_id=$1 AND active FOR UPDATE`, runID).Scan(&activeGeneration, &activeExpiry)
	if err == nil {
		if activeExpiry.After(now) {
			return migration.LeaseFence{}, migration.ErrLeaseFenced
		}
		if _, err = tx.Exec(ctx, `UPDATE data_migration_run_leases SET active=FALSE,retired_at=$2 WHERE run_id=$1 AND generation=$3 AND active`, runID, now, activeGeneration); err != nil {
			return migration.LeaseFence{}, translate(err)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return migration.LeaseFence{}, translate(err)
	}
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(generation),0)+1 FROM data_migration_run_leases WHERE run_id=$1`, runID).Scan(&generation); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	token := migration.Digest{}
	if _, err = rand.Read(token[:]); err != nil {
		return migration.LeaseFence{}, fmt.Errorf("data migration fence entropy: %w", err)
	}
	expires := now.Add(ttl)
	if _, err = tx.Exec(ctx, `INSERT INTO data_migration_run_leases(run_id,generation,fence,acquired_at,expires_at) VALUES($1,$2,$3,$4,$5)`, runID, generation, bytes(token), now, expires); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	return migration.LeaseFence{RunID: runID, Generation: uint64(generation), Token: token, ExpiresAt: expires}, nil
}

func (repository *Repository) RenewLease(ctx context.Context, fence migration.LeaseFence, now time.Time, ttl time.Duration) (migration.LeaseFence, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return migration.LeaseFence{}, err
	}
	if !validFence(fence) || now.IsZero() || ttl <= 0 {
		return migration.LeaseFence{}, migration.ErrLeaseFenced
	}
	var expires, databaseNow time.Time
	err = tx.QueryRow(ctx, `SELECT expires_at FROM data_migration_run_leases
WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active FOR UPDATE`, fence.RunID, fence.Generation, bytes(fence.Token)).Scan(&expires)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow)
	}
	if err != nil || !expires.After(databaseNow) {
		return migration.LeaseFence{}, migration.ErrLeaseFenced
	}
	desired := databaseNow.Add(ttl)
	if !desired.After(expires) {
		fence.ExpiresAt = expires
		return fence, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE data_migration_run_leases SET expires_at=$4
WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active`, fence.RunID, fence.Generation, bytes(fence.Token), desired); err != nil {
		return migration.LeaseFence{}, translate(err)
	}
	fence.ExpiresAt = desired
	return fence, nil
}

func (repository *Repository) Advance(ctx context.Context, fence migration.LeaseFence, table migration.TableID, checkpoint migration.TableCheckpoint) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	if !validFence(fence) || table == "" || !validBound(checkpoint.UpperBound) {
		return migration.ErrInvalidRun
	}
	var cursor any
	if checkpoint.Cursor != "" {
		cursor = string(checkpoint.Cursor)
	}
	tag, err := tx.Exec(ctx, `UPDATE data_migration_run_tables SET cursor=$5,processed=$6,complete=$7,last_lease_generation=$3,last_lease_fence=$4
WHERE run_id=$1 AND table_id=$2
  AND aicrm_data_migration_active_fence($1,$3,$4)
  AND upper_bound IS NOT DISTINCT FROM $8 AND upper_bound_empty=$9 AND schema_digest=$10 AND source_identity=$11`,
		fence.RunID, table, fence.Generation, bytes(fence.Token), cursor, checkpoint.Processed, checkpoint.Complete, nullableBound(checkpoint.UpperBound), checkpoint.UpperBound.Empty, bytes(checkpoint.SchemaDigest), checkpoint.SourceIdentity)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() != 1 {
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
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	if !validFence(fence) {
		return migration.ErrLeaseFenced
	}
	var active bool
	var expires, now time.Time
	err = tx.QueryRow(ctx, `SELECT active,expires_at,clock_timestamp() FROM data_migration_run_leases
WHERE run_id=$1 AND generation=$2 AND fence=$3 FOR UPDATE`, fence.RunID, fence.Generation, bytes(fence.Token)).Scan(&active, &expires, &now)
	if err != nil || !active || !expires.After(now) {
		return migration.ErrLeaseFenced
	}
	if _, err = tx.Exec(ctx, `UPDATE data_migration_run_leases SET active=FALSE,retired_at=$4
WHERE run_id=$1 AND generation=$2 AND fence=$3 AND active`, fence.RunID, fence.Generation, bytes(fence.Token), now); err != nil {
		return translate(err)
	}
	var tag pgconn.CommandTag
	if phase == migration.PhaseCompleted {
		tag, err = tx.Exec(ctx, `UPDATE data_migration_runs SET phase='completed',completed_at=$2 WHERE run_id=$1 AND phase='running'`, fence.RunID, now)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE data_migration_runs SET phase='reconciled',reconciled_at=$2 WHERE run_id=$1 AND phase='completed'`, fence.RunID, now)
	}
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() != 1 {
		return migration.ErrTargetTampered
	}
	return nil
}

func (repository *Repository) FindRowReceipt(ctx context.Context, adapter migration.AdapterID, table migration.TableID, sourceKey migration.Digest) (migration.RowReceipt, bool, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return migration.RowReceipt{}, false, err
	}
	if tx, txErr := platformstore.TxFromContext(ctx); txErr == nil {
		lockKey := string(adapter) + ":" + string(table) + ":" + hex.EncodeToString(sourceKey[:])
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
			return migration.RowReceipt{}, false, translate(err)
		}
	}
	receipt, err := scanRowReceipt(db.QueryRow(ctx, `SELECT adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest
FROM data_migration_row_receipts WHERE adapter_id=$1 AND table_id=$2 AND source_key_digest=$3`, adapter, table, bytes(sourceKey)))
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.RowReceipt{}, false, nil
	}
	return receipt, err == nil, translate(err)
}

func (repository *Repository) AppendRowReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.RowReceipt) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO data_migration_row_receipts(
adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,run_id,lease_generation,lease_fence
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, receipt.Adapter, receipt.Table, bytes(receipt.SourceKey), bytes(receipt.Payload), bytes(receipt.Field), receipt.Disposition, bytes(receipt.Mapping), bytes(receipt.Policy), receipt.Operation, bytes(receipt.Mutation), fence.RunID, fence.Generation, bytes(fence.Token))
	return translate(err)
}

func (repository *Repository) Quarantine(ctx context.Context, fence migration.LeaseFence, value migration.Quarantine) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO data_migration_quarantines(
run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,reason,lease_generation,lease_fence
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, fence.RunID, value.Adapter, value.Table, bytes(value.SourceKey), bytes(value.Payload), bytes(value.Field), value.Reason, fence.Generation, bytes(fence.Token))
	return translate(err)
}

func (repository *Repository) Archive(ctx context.Context, fence migration.LeaseFence, value migration.Archive) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO data_migration_archives(
run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,lease_generation,lease_fence
) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, fence.RunID, value.Adapter, value.Table, bytes(value.SourceKey), bytes(value.Payload), bytes(value.Field), fence.Generation, bytes(fence.Token))
	return translate(err)
}

func (repository *Repository) FindResultReceipt(ctx context.Context, runID migration.RunID, adapter migration.AdapterID, table migration.TableID, sourceKey migration.Digest) (migration.ResultReceipt, bool, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return migration.ResultReceipt{}, false, err
	}
	receipt, err := scanResultReceipt(db.QueryRow(ctx, resultSelect+` WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3 AND source_key_digest=$4`, runID, adapter, table, bytes(sourceKey)))
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.ResultReceipt{}, false, nil
	}
	return receipt, err == nil, translate(err)
}

func (repository *Repository) AppendResultReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.ResultReceipt) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO data_migration_result_receipts(
run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,outcome,lease_generation,lease_fence
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, receipt.RunID, receipt.Adapter, receipt.Table, bytes(receipt.SourceKey), bytes(receipt.Payload), bytes(receipt.Field), receipt.Disposition, bytes(receipt.Mapping), bytes(receipt.Policy), receipt.Operation, bytes(receipt.Mutation), receipt.Outcome, fence.Generation, bytes(fence.Token))
	return translate(err)
}

func (repository *Repository) ListResultReceipts(ctx context.Context, runID migration.RunID) ([]migration.ResultReceipt, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, resultSelect+` WHERE run_id=$1 ORDER BY table_id,source_key_digest`, runID)
	if err != nil {
		return nil, translate(err)
	}
	defer rows.Close()
	values := make([]migration.ResultReceipt, 0)
	for rows.Next() {
		value, scanErr := scanResultReceipt(rows)
		if scanErr != nil {
			return nil, translate(scanErr)
		}
		values = append(values, value)
	}
	return values, translate(rows.Err())
}

func (repository *Repository) AppendReconciliationReceipt(ctx context.Context, fence migration.LeaseFence, receipt migration.ReconciliationReceipt) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO data_migration_reconciliation_receipts(
run_id,source_row_count,result_row_count,target_verified_count,comparison_digest,lease_generation,lease_fence
) VALUES($1,$2,$3,$4,$5,$6,$7)`, receipt.RunID, receipt.SourceRowCount, receipt.ResultRowCount, receipt.TargetVerifiedCount, bytes(receipt.ComparisonDigest), fence.Generation, bytes(fence.Token))
	return translate(err)
}

func (repository *Repository) Readiness(ctx context.Context, runID migration.RunID) (migration.Readiness, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return migration.Readiness{}, err
	}
	var value migration.Readiness
	var phase string
	var pending, processed, results, quarantined int64
	var reconciled bool
	err = db.QueryRow(ctx, `SELECT r.run_id,r.phase,
  count(*) FILTER (WHERE NOT t.complete),COALESCE(sum(t.processed),0),
  (SELECT count(*) FROM data_migration_result_receipts rr WHERE rr.run_id=r.run_id),
	  (SELECT count(*) FROM data_migration_result_receipts qr WHERE qr.run_id=r.run_id AND qr.disposition='quarantine'),
  EXISTS(SELECT 1 FROM data_migration_reconciliation_receipts x WHERE x.run_id=r.run_id)
FROM data_migration_runs r JOIN data_migration_run_tables t ON t.run_id=r.run_id
WHERE r.run_id=$1 GROUP BY r.run_id,r.phase`, runID).Scan(&value.RunID, &phase, &pending, &processed, &results, &quarantined, &reconciled)
	if err != nil {
		return migration.Readiness{}, translate(err)
	}
	if pending < 0 || processed < 0 || results < 0 || quarantined < 0 {
		return migration.Readiness{}, migration.ErrTargetTampered
	}
	value.Phase = migration.RunPhase(phase)
	value.PendingTables = uint64(pending)
	value.ProcessedRows = uint64(processed)
	value.ResultRows = uint64(results)
	value.QuarantinedRows = uint64(quarantined)
	value.Reconciled = reconciled
	value.Ready = value.Phase == migration.PhaseReconciled && value.PendingTables == 0 && value.ProcessedRows == value.ResultRows && value.QuarantinedRows == 0 && value.Reconciled
	return value, nil
}

const resultSelect = `SELECT run_id,adapter_id,table_id,source_key_digest,payload_digest,field_digest,disposition,mapping_digest,policy_digest,operation,mutation_digest,outcome FROM data_migration_result_receipts`

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type scanner interface{ Scan(...any) error }

func (repository *Repository) queryer(ctx context.Context) (queryer, error) {
	if repository == nil || repository.pool == nil {
		return nil, migration.ErrInvalidRun
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return tx, nil
	}
	return repository.pool, nil
}

func (repository *Repository) transaction(ctx context.Context) (pgx.Tx, error) {
	if repository == nil {
		return nil, migration.ErrInvalidRun
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, migration.ErrInvalidRun
	}
	return tx, nil
}

func scanRowReceipt(row scanner) (migration.RowReceipt, error) {
	var value migration.RowReceipt
	var source, payload, field, mapping, policy, mutation []byte
	var disposition string
	if err := row.Scan(&value.Adapter, &value.Table, &source, &payload, &field, &disposition, &mapping, &policy, &value.Operation, &mutation); err != nil {
		return migration.RowReceipt{}, err
	}
	var err error
	if value.SourceKey, err = digest(source); err != nil {
		return migration.RowReceipt{}, err
	}
	if value.Payload, err = digest(payload); err != nil {
		return migration.RowReceipt{}, err
	}
	if value.Field, err = digest(field); err != nil {
		return migration.RowReceipt{}, err
	}
	if value.Mapping, err = digest(mapping); err != nil {
		return migration.RowReceipt{}, err
	}
	if value.Policy, err = digest(policy); err != nil {
		return migration.RowReceipt{}, err
	}
	if value.Mutation, err = digest(mutation); err != nil {
		return migration.RowReceipt{}, err
	}
	value.Disposition = migration.Disposition(disposition)
	return value, nil
}

func scanResultReceipt(row scanner) (migration.ResultReceipt, error) {
	var value migration.ResultReceipt
	var source, payload, field, mapping, policy, mutation []byte
	var disposition, outcome string
	if err := row.Scan(&value.RunID, &value.Adapter, &value.Table, &source, &payload, &field, &disposition, &mapping, &policy, &value.Operation, &mutation, &outcome); err != nil {
		return migration.ResultReceipt{}, err
	}
	var err error
	if value.SourceKey, err = digest(source); err != nil {
		return migration.ResultReceipt{}, err
	}
	if value.Payload, err = digest(payload); err != nil {
		return migration.ResultReceipt{}, err
	}
	if value.Field, err = digest(field); err != nil {
		return migration.ResultReceipt{}, err
	}
	if value.Mapping, err = digest(mapping); err != nil {
		return migration.ResultReceipt{}, err
	}
	if value.Policy, err = digest(policy); err != nil {
		return migration.ResultReceipt{}, err
	}
	if value.Mutation, err = digest(mutation); err != nil {
		return migration.ResultReceipt{}, err
	}
	value.Disposition = migration.Disposition(disposition)
	value.Outcome = migration.Disposition(outcome)
	value.MutationDigest = value.Mutation
	return value, nil
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

func nullableBound(bound migration.UpperBound) any {
	if bound.Empty {
		return nil
	}
	return bound.Value
}

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

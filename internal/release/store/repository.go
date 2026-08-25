package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	releaseport "github.com/qianlan33333-png/AI-CRM-v2/internal/release/port"
)

type Repository struct{ pool *pgxpool.Pool }

var _ releaseport.Repository = (*Repository)(nil)

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) CreateCandidate(ctx context.Context, value releaseport.Candidate) (releaseport.Candidate, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	return scanCandidate(tx.QueryRow(ctx, `INSERT INTO release_candidates (
commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at
) VALUES($1,$2,$3,$4,$5,'draft',$6,$7)
RETURNING id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at`,
		value.CommitSHA, value.ArtifactDigest, value.ManifestDigest, value.ConfigDigest,
		value.TargetSchemaVersion, value.CreatedBy, value.CreatedAt))
}

func (repository *Repository) GetCandidate(ctx context.Context, candidateID int64) (releaseport.Candidate, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	return scanCandidate(db.QueryRow(ctx, `SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at FROM release_candidates WHERE id=$1`, candidateID))
}

func (repository *Repository) LockCandidate(ctx context.Context, candidateID int64) (releaseport.Candidate, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	return scanCandidate(tx.QueryRow(ctx, `SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at FROM release_candidates WHERE id=$1 FOR UPDATE`, candidateID))
}

func (repository *Repository) ListCandidates(ctx context.Context, limit int32) ([]releaseport.Candidate, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at FROM release_candidates ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	values := make([]releaseport.Candidate, 0)
	for rows.Next() {
		value, scanErr := scanCandidate(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		return nil, unavailable(err)
	}
	return values, nil
}

func (repository *Repository) TransitionCandidate(ctx context.Context, candidateID int64, from, to releaseport.CandidateState, now time.Time) (releaseport.Candidate, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE release_candidates SET
state=$3,
prepared_at=CASE WHEN $3='prepared' THEN $4 ELSE prepared_at END,
activated_at=CASE WHEN $3='activated' THEN $4 ELSE activated_at END,
rollback_requested_at=CASE WHEN $3='rollback_pending' THEN $4 ELSE rollback_requested_at END,
rolled_back_at=CASE WHEN $3='rolled_back' THEN $4 ELSE rolled_back_at END
WHERE id=$1 AND state=$2`, candidateID, string(from), string(to), now)
	if err != nil {
		return releaseport.Candidate{}, translate(err)
	}
	if tag.RowsAffected() != 1 {
		return releaseport.Candidate{}, releaseport.ErrConflict
	}
	return scanCandidate(tx.QueryRow(ctx, `SELECT id,commit_sha,artifact_digest,manifest_digest,config_digest,target_schema_version,state,created_by,created_at,prepared_at,activated_at,rollback_requested_at,rolled_back_at FROM release_candidates WHERE id=$1`, candidateID))
}

func (repository *Repository) CreatePrerequisite(ctx context.Context, value releaseport.PrerequisiteReceipt) (releaseport.PrerequisiteReceipt, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.PrerequisiteReceipt{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO release_prerequisite_receipts(
candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id,candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at`,
		value.CandidateID, value.CandidateCommitSHA, value.CandidateArtifactDigest, value.CandidateManifestDigest,
		value.CandidateConfigDigest, value.CandidateSchemaVersion, string(value.Kind), value.EvidenceSHA,
		value.RecordedBy, value.RecordedAt).Scan(
		&value.ID, &value.CandidateID, &value.CandidateCommitSHA, &value.CandidateArtifactDigest,
		&value.CandidateManifestDigest, &value.CandidateConfigDigest, &value.CandidateSchemaVersion,
		&value.Kind, &value.EvidenceSHA, &value.RecordedBy, &value.RecordedAt)
	return value, translate(err)
}

func (repository *Repository) ListPrerequisites(ctx context.Context, candidateID int64) ([]releaseport.PrerequisiteReceipt, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT id,candidate_id,candidate_commit_sha,candidate_artifact_digest,candidate_manifest_digest,candidate_config_digest,candidate_schema_version,kind,evidence_sha,recorded_by,recorded_at FROM release_prerequisite_receipts WHERE candidate_id=$1 ORDER BY kind`, candidateID)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	values := make([]releaseport.PrerequisiteReceipt, 0)
	for rows.Next() {
		var value releaseport.PrerequisiteReceipt
		if err = rows.Scan(&value.ID, &value.CandidateID, &value.CandidateCommitSHA, &value.CandidateArtifactDigest,
			&value.CandidateManifestDigest, &value.CandidateConfigDigest, &value.CandidateSchemaVersion,
			&value.Kind, &value.EvidenceSHA, &value.RecordedBy, &value.RecordedAt); err != nil {
			return nil, unavailable(err)
		}
		values = append(values, value)
	}
	return values, unavailable(rows.Err())
}

func (repository *Repository) StartWorker(ctx context.Context, value releaseport.WorkerLease) (releaseport.WorkerLease, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.WorkerLease{}, err
	}
	return scanWorker(tx.QueryRow(ctx, `INSERT INTO release_worker_leases(candidate_id,generation,fence,started_by,started_at,active)
VALUES($1,(SELECT COALESCE(max(generation),0)+1 FROM release_worker_leases WHERE candidate_id=$1),$2,$3,$4,TRUE)
RETURNING candidate_id,generation,fence,started_by,started_at,active,retired_at`, value.CandidateID, value.Fence, value.StartedBy, value.StartedAt))
}

func (repository *Repository) GetActiveWorker(ctx context.Context, candidateID int64) (releaseport.WorkerLease, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.WorkerLease{}, err
	}
	return scanWorker(tx.QueryRow(ctx, `SELECT candidate_id,generation,fence,started_by,started_at,active,retired_at FROM release_worker_leases WHERE candidate_id=$1 AND active FOR UPDATE`, candidateID))
}

func (repository *Repository) FindActiveWorkerSummary(ctx context.Context, candidateID int64) (*releaseport.WorkerSummary, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	value := releaseport.WorkerSummary{}
	err = db.QueryRow(ctx, `SELECT candidate_id,generation,started_by,started_at FROM release_worker_leases WHERE candidate_id=$1 AND active`, candidateID).Scan(
		&value.CandidateID, &value.Generation, &value.StartedBy, &value.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translate(err)
	}
	return &value, nil
}

func (repository *Repository) RetireWorker(ctx context.Context, candidateID, generation int64, fence string, now time.Time) error {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE release_worker_leases SET active=FALSE,retired_at=$4 WHERE candidate_id=$1 AND generation=$2 AND fence=$3 AND active`, candidateID, generation, fence, now)
	if err != nil {
		return translate(err)
	}
	if tag.RowsAffected() != 1 {
		return releaseport.ErrConflict
	}
	return nil
}

func (repository *Repository) AppendCutoverStep(ctx context.Context, value releaseport.CutoverJournalEntry) (releaseport.CutoverJournalEntry, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.CutoverJournalEntry{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO release_cutover_journal(candidate_id,generation,step,fence,completed_by,completed_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,candidate_id,generation,step,fence,completed_by,completed_at`,
		value.CandidateID, value.Generation, string(value.Step), value.Fence, value.CompletedBy, value.CompletedAt).Scan(
		&value.ID, &value.CandidateID, &value.Generation, &value.Step, &value.Fence, &value.CompletedBy, &value.CompletedAt)
	return value, translate(err)
}

func (repository *Repository) ListCutoverSteps(ctx context.Context, candidateID int64) ([]releaseport.CutoverJournalEntry, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT id,candidate_id,generation,step,fence,completed_by,completed_at FROM release_cutover_journal WHERE candidate_id=$1 ORDER BY id`, candidateID)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	values := make([]releaseport.CutoverJournalEntry, 0)
	for rows.Next() {
		var value releaseport.CutoverJournalEntry
		if err = rows.Scan(&value.ID, &value.CandidateID, &value.Generation, &value.Step, &value.Fence, &value.CompletedBy, &value.CompletedAt); err != nil {
			return nil, unavailable(err)
		}
		values = append(values, value)
	}
	return values, unavailable(rows.Err())
}

func (repository *Repository) CreateRollbackCheck(ctx context.Context, value releaseport.RollbackCheck) (releaseport.RollbackCheck, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.RollbackCheck{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO release_rollback_checks(candidate_id,kind,passed,evidence_sha,recorded_by,recorded_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,candidate_id,kind,passed,evidence_sha,recorded_by,recorded_at`,
		value.CandidateID, string(value.Kind), value.Passed, value.EvidenceSHA, value.RecordedBy, value.RecordedAt).Scan(
		&value.ID, &value.CandidateID, &value.Kind, &value.Passed, &value.EvidenceSHA, &value.RecordedBy, &value.RecordedAt)
	return value, translate(err)
}

func (repository *Repository) ListRollbackChecks(ctx context.Context, candidateID int64) ([]releaseport.RollbackCheck, error) {
	db, err := repository.queryer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT id,candidate_id,kind,passed,evidence_sha,recorded_by,recorded_at FROM release_rollback_checks WHERE candidate_id=$1 ORDER BY id`, candidateID)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	values := make([]releaseport.RollbackCheck, 0)
	for rows.Next() {
		var value releaseport.RollbackCheck
		if err = rows.Scan(&value.ID, &value.CandidateID, &value.Kind, &value.Passed, &value.EvidenceSHA, &value.RecordedBy, &value.RecordedAt); err != nil {
			return nil, unavailable(err)
		}
		values = append(values, value)
	}
	return values, unavailable(rows.Err())
}

func (repository *Repository) ReserveOperationReceipt(ctx context.Context, value releaseport.OperationReceipt) (releaseport.OperationReceipt, bool, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.OperationReceipt{}, false, err
	}
	created := value
	err = tx.QueryRow(ctx, `INSERT INTO release_operation_receipts(action,actor_id,key_digest,payload_digest,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(action,actor_id,key_digest) DO NOTHING RETURNING id,action,actor_id,key_digest,payload_digest,state,result_snapshot,created_at,completed_at`,
		value.Action, value.ActorID, value.KeyDigest, value.PayloadDigest, value.CreatedAt).Scan(
		&created.ID, &created.Action, &created.ActorID, &created.KeyDigest, &created.PayloadDigest,
		&created.State, &created.Result, &created.CreatedAt, &created.CompletedAt)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return releaseport.OperationReceipt{}, false, translate(err)
	}
	stored := releaseport.OperationReceipt{}
	err = tx.QueryRow(ctx, `SELECT id,action,actor_id,key_digest,payload_digest,state,result_snapshot,created_at,completed_at FROM release_operation_receipts WHERE action=$1 AND actor_id=$2 AND key_digest=$3 FOR UPDATE`,
		value.Action, value.ActorID, value.KeyDigest).Scan(
		&stored.ID, &stored.Action, &stored.ActorID, &stored.KeyDigest, &stored.PayloadDigest,
		&stored.State, &stored.Result, &stored.CreatedAt, &stored.CompletedAt)
	return stored, false, translate(err)
}

func (repository *Repository) CompleteOperationReceipt(ctx context.Context, receiptID int64, result json.RawMessage, now time.Time) (releaseport.OperationReceipt, error) {
	tx, err := repository.transaction(ctx)
	if err != nil {
		return releaseport.OperationReceipt{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE release_operation_receipts SET state='completed',result_snapshot=$2,completed_at=$3 WHERE id=$1 AND state='in_progress'`, receiptID, result, now)
	if err != nil {
		return releaseport.OperationReceipt{}, translate(err)
	}
	if tag.RowsAffected() != 1 {
		return releaseport.OperationReceipt{}, releaseport.ErrConflict
	}
	value := releaseport.OperationReceipt{}
	err = tx.QueryRow(ctx, `SELECT id,action,actor_id,key_digest,payload_digest,state,result_snapshot,created_at,completed_at FROM release_operation_receipts WHERE id=$1`, receiptID).Scan(
		&value.ID, &value.Action, &value.ActorID, &value.KeyDigest, &value.PayloadDigest,
		&value.State, &value.Result, &value.CreatedAt, &value.CompletedAt)
	return value, translate(err)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type scanner interface{ Scan(...any) error }

func (repository *Repository) queryer(ctx context.Context) (queryer, error) {
	if repository == nil || repository.pool == nil {
		return nil, releaseport.ErrUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return tx, nil
	}
	return repository.pool, nil
}

func (repository *Repository) transaction(ctx context.Context) (pgx.Tx, error) {
	if repository == nil {
		return nil, releaseport.ErrUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", releaseport.ErrUnavailable, err)
	}
	return tx, nil
}

func scanCandidate(row scanner) (releaseport.Candidate, error) {
	var value releaseport.Candidate
	var state string
	var preparedAt, activatedAt, rollbackRequestedAt, rolledBackAt pgtype.Timestamptz
	err := row.Scan(
		&value.ID, &value.CommitSHA, &value.ArtifactDigest, &value.ManifestDigest, &value.ConfigDigest,
		&value.TargetSchemaVersion, &state, &value.CreatedBy, &value.CreatedAt,
		&preparedAt, &activatedAt, &rollbackRequestedAt, &rolledBackAt,
	)
	if err != nil {
		return releaseport.Candidate{}, translate(err)
	}
	value.State = releaseport.CandidateState(state)
	value.PreparedAt = optionalTime(preparedAt)
	value.ActivatedAt = optionalTime(activatedAt)
	value.RollbackRequestedAt = optionalTime(rollbackRequestedAt)
	value.RolledBackAt = optionalTime(rolledBackAt)
	return value, nil
}

func scanWorker(row scanner) (releaseport.WorkerLease, error) {
	var value releaseport.WorkerLease
	var retiredAt pgtype.Timestamptz
	err := row.Scan(&value.CandidateID, &value.Generation, &value.Fence, &value.StartedBy, &value.StartedAt, &value.Active, &retiredAt)
	if err != nil {
		return releaseport.WorkerLease{}, translate(err)
	}
	value.RetiredAt = optionalTime(retiredAt)
	return value, nil
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return releaseport.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23514" || pgErr.Code == "55000") {
		return fmt.Errorf("%w: %s", releaseport.ErrConflict, pgErr.Message)
	}
	return unavailable(err)
}

func unavailable(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", releaseport.ErrUnavailable, err)
}

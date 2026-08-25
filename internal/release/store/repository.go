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
	releasedb "github.com/qianlan33333-png/AI-CRM-v2/internal/release/store/generated"
)

type Repository struct{ pool *pgxpool.Pool }

var _ releaseport.Repository = (*Repository)(nil)

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) CreateCandidate(ctx context.Context, value releaseport.Candidate) (releaseport.Candidate, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	row, err := queries.CreateReleaseCandidate(ctx, releasedb.CreateReleaseCandidateParams{
		CommitSha:           value.CommitSHA,
		ArtifactDigest:      value.ArtifactDigest,
		ManifestDigest:      value.ManifestDigest,
		ConfigDigest:        value.ConfigDigest,
		TargetSchemaVersion: value.TargetSchemaVersion,
		CreatedBy:           value.CreatedBy,
		CreatedAt:           timestamp(value.CreatedAt),
	})
	return candidate(row), translate(err)
}

func (repository *Repository) GetCandidate(ctx context.Context, candidateID int64) (releaseport.Candidate, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	row, err := queries.GetReleaseCandidate(ctx, candidateID)
	return candidate(row), translate(err)
}

func (repository *Repository) LockCandidate(ctx context.Context, candidateID int64) (releaseport.Candidate, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	row, err := queries.LockReleaseCandidate(ctx, candidateID)
	return candidate(row), translate(err)
}

func (repository *Repository) ListCandidates(ctx context.Context, limit int32) ([]releaseport.Candidate, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReleaseCandidates(ctx, limit)
	if err != nil {
		return nil, translate(err)
	}
	values := make([]releaseport.Candidate, 0, len(rows))
	for _, row := range rows {
		values = append(values, candidate(row))
	}
	return values, nil
}

func (repository *Repository) TransitionCandidate(ctx context.Context, candidateID int64, from, to releaseport.CandidateState, now time.Time) (releaseport.Candidate, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.Candidate{}, err
	}
	count, err := queries.TransitionReleaseCandidate(ctx, releasedb.TransitionReleaseCandidateParams{
		ID: candidateID, State: string(from), State_2: string(to), PreparedAt: timestamp(now),
	})
	if err != nil {
		return releaseport.Candidate{}, translate(err)
	}
	if count != 1 {
		return releaseport.Candidate{}, releaseport.ErrConflict
	}
	row, err := queries.GetReleaseCandidate(ctx, candidateID)
	return candidate(row), translate(err)
}

func (repository *Repository) CreatePrerequisite(ctx context.Context, value releaseport.PrerequisiteReceipt) (releaseport.PrerequisiteReceipt, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.PrerequisiteReceipt{}, err
	}
	row, err := queries.CreateReleasePrerequisite(ctx, releasedb.CreateReleasePrerequisiteParams{
		CandidateID:             value.CandidateID,
		CandidateCommitSha:      value.CandidateCommitSHA,
		CandidateArtifactDigest: value.CandidateArtifactDigest,
		CandidateManifestDigest: value.CandidateManifestDigest,
		CandidateConfigDigest:   value.CandidateConfigDigest,
		CandidateSchemaVersion:  value.CandidateSchemaVersion,
		Kind:                    string(value.Kind),
		EvidenceSha:             value.EvidenceSHA,
		RecordedBy:              value.RecordedBy,
		RecordedAt:              timestamp(value.RecordedAt),
	})
	return prerequisite(row), translate(err)
}

func (repository *Repository) ListPrerequisites(ctx context.Context, candidateID int64) ([]releaseport.PrerequisiteReceipt, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReleasePrerequisites(ctx, candidateID)
	if err != nil {
		return nil, translate(err)
	}
	values := make([]releaseport.PrerequisiteReceipt, 0, len(rows))
	for _, row := range rows {
		values = append(values, prerequisite(row))
	}
	return values, nil
}

func (repository *Repository) StartWorker(ctx context.Context, value releaseport.WorkerLease) (releaseport.WorkerLease, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.WorkerLease{}, err
	}
	row, err := queries.StartReleaseWorker(ctx, releasedb.StartReleaseWorkerParams{
		CandidateID: value.CandidateID,
		Fence:       value.Fence,
		StartedBy:   value.StartedBy,
		StartedAt:   timestamp(value.StartedAt),
	})
	return worker(row), translate(err)
}

func (repository *Repository) GetActiveWorker(ctx context.Context, candidateID int64) (releaseport.WorkerLease, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.WorkerLease{}, err
	}
	row, err := queries.GetActiveReleaseWorker(ctx, candidateID)
	return worker(row), translate(err)
}

func (repository *Repository) FindActiveWorkerSummary(ctx context.Context, candidateID int64) (*releaseport.WorkerSummary, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return nil, err
	}
	row, err := queries.FindActiveReleaseWorkerSummary(ctx, candidateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, translate(err)
	}
	value := releaseport.WorkerSummary{
		CandidateID: row.CandidateID,
		Generation:  row.Generation,
		StartedBy:   row.StartedBy,
		StartedAt:   row.StartedAt.Time,
	}
	return &value, nil
}

func (repository *Repository) RetireWorker(ctx context.Context, candidateID, generation int64, fence string, now time.Time) error {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return err
	}
	count, err := queries.RetireReleaseWorker(ctx, releasedb.RetireReleaseWorkerParams{
		CandidateID: candidateID, Generation: generation, Fence: fence, RetiredAt: timestamp(now),
	})
	if err != nil {
		return translate(err)
	}
	if count != 1 {
		return releaseport.ErrConflict
	}
	return nil
}

func (repository *Repository) AppendCutoverStep(ctx context.Context, value releaseport.CutoverJournalEntry) (releaseport.CutoverJournalEntry, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.CutoverJournalEntry{}, err
	}
	row, err := queries.AppendReleaseCutoverStep(ctx, releasedb.AppendReleaseCutoverStepParams{
		CandidateID: value.CandidateID,
		Generation:  value.Generation,
		Step:        string(value.Step),
		Fence:       value.Fence,
		CompletedBy: value.CompletedBy,
		CompletedAt: timestamp(value.CompletedAt),
	})
	return cutoverStep(row), translate(err)
}

func (repository *Repository) ListCutoverSteps(ctx context.Context, candidateID int64) ([]releaseport.CutoverJournalEntry, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReleaseCutoverSteps(ctx, candidateID)
	if err != nil {
		return nil, translate(err)
	}
	values := make([]releaseport.CutoverJournalEntry, 0, len(rows))
	for _, row := range rows {
		values = append(values, cutoverStep(row))
	}
	return values, nil
}

func (repository *Repository) CreateRollbackCheck(ctx context.Context, value releaseport.RollbackCheck) (releaseport.RollbackCheck, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.RollbackCheck{}, err
	}
	row, err := queries.CreateReleaseRollbackCheck(ctx, releasedb.CreateReleaseRollbackCheckParams{
		CandidateID: value.CandidateID,
		Kind:        string(value.Kind),
		Passed:      value.Passed,
		EvidenceSha: value.EvidenceSHA,
		RecordedBy:  value.RecordedBy,
		RecordedAt:  timestamp(value.RecordedAt),
	})
	return rollbackCheck(row), translate(err)
}

func (repository *Repository) ListRollbackChecks(ctx context.Context, candidateID int64) ([]releaseport.RollbackCheck, error) {
	queries, err := repository.queries(ctx, false)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReleaseRollbackChecks(ctx, candidateID)
	if err != nil {
		return nil, translate(err)
	}
	values := make([]releaseport.RollbackCheck, 0, len(rows))
	for _, row := range rows {
		values = append(values, rollbackCheck(row))
	}
	return values, nil
}

func (repository *Repository) ReserveOperationReceipt(ctx context.Context, value releaseport.OperationReceipt) (releaseport.OperationReceipt, bool, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.OperationReceipt{}, false, err
	}
	row, err := queries.ReserveReleaseOperationReceipt(ctx, releasedb.ReserveReleaseOperationReceiptParams{
		Action: value.Action, ActorID: value.ActorID, KeyDigest: value.KeyDigest,
		PayloadDigest: value.PayloadDigest, CreatedAt: timestamp(value.CreatedAt),
	})
	if err == nil {
		return operationReceipt(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return releaseport.OperationReceipt{}, false, translate(err)
	}
	row, err = queries.LockReleaseOperationReceipt(ctx, releasedb.LockReleaseOperationReceiptParams{
		Action: value.Action, ActorID: value.ActorID, KeyDigest: value.KeyDigest,
	})
	return operationReceipt(row), false, translate(err)
}

func (repository *Repository) CompleteOperationReceipt(ctx context.Context, receiptID int64, result json.RawMessage, now time.Time) (releaseport.OperationReceipt, error) {
	queries, err := repository.queries(ctx, true)
	if err != nil {
		return releaseport.OperationReceipt{}, err
	}
	count, err := queries.CompleteReleaseOperationReceipt(ctx, releasedb.CompleteReleaseOperationReceiptParams{
		ID: receiptID, ResultSnapshot: result, CompletedAt: timestamp(now),
	})
	if err != nil {
		return releaseport.OperationReceipt{}, translate(err)
	}
	if count != 1 {
		return releaseport.OperationReceipt{}, releaseport.ErrConflict
	}
	row, err := queries.GetReleaseOperationReceiptByID(ctx, receiptID)
	return operationReceipt(row), translate(err)
}

func (repository *Repository) queries(ctx context.Context, transactionRequired bool) (*releasedb.Queries, error) {
	if repository == nil || repository.pool == nil {
		return nil, releaseport.ErrUnavailable
	}
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return releasedb.New(tx), nil
	}
	if transactionRequired {
		return nil, fmt.Errorf("%w: transaction required", releaseport.ErrUnavailable)
	}
	return releasedb.New(repository.pool), nil
}

func candidate(row releasedb.ReleaseCandidate) releaseport.Candidate {
	return releaseport.Candidate{
		ID:                  row.ID,
		CommitSHA:           row.CommitSha,
		ArtifactDigest:      row.ArtifactDigest,
		ManifestDigest:      row.ManifestDigest,
		ConfigDigest:        row.ConfigDigest,
		TargetSchemaVersion: row.TargetSchemaVersion,
		State:               releaseport.CandidateState(row.State),
		CreatedBy:           row.CreatedBy,
		CreatedAt:           row.CreatedAt.Time,
		PreparedAt:          optionalTime(row.PreparedAt),
		ActivatedAt:         optionalTime(row.ActivatedAt),
		RollbackRequestedAt: optionalTime(row.RollbackRequestedAt),
		RolledBackAt:        optionalTime(row.RolledBackAt),
	}
}

func prerequisite(row releasedb.ReleasePrerequisiteReceipt) releaseport.PrerequisiteReceipt {
	return releaseport.PrerequisiteReceipt{
		ID:                      row.ID,
		CandidateID:             row.CandidateID,
		CandidateCommitSHA:      row.CandidateCommitSha,
		CandidateArtifactDigest: row.CandidateArtifactDigest,
		CandidateManifestDigest: row.CandidateManifestDigest,
		CandidateConfigDigest:   row.CandidateConfigDigest,
		CandidateSchemaVersion:  row.CandidateSchemaVersion,
		Kind:                    releaseport.PrerequisiteKind(row.Kind),
		EvidenceSHA:             row.EvidenceSha,
		RecordedBy:              row.RecordedBy,
		RecordedAt:              row.RecordedAt.Time,
	}
}

func worker(row releasedb.ReleaseWorkerLease) releaseport.WorkerLease {
	return releaseport.WorkerLease{
		CandidateID: row.CandidateID,
		Generation:  row.Generation,
		Fence:       row.Fence,
		StartedBy:   row.StartedBy,
		StartedAt:   row.StartedAt.Time,
		Active:      row.Active,
		RetiredAt:   optionalTime(row.RetiredAt),
	}
}

func cutoverStep(row releasedb.ReleaseCutoverJournal) releaseport.CutoverJournalEntry {
	return releaseport.CutoverJournalEntry{
		ID: row.ID, CandidateID: row.CandidateID, Generation: row.Generation,
		Step: releaseport.CutoverStep(row.Step), Fence: row.Fence,
		CompletedBy: row.CompletedBy, CompletedAt: row.CompletedAt.Time,
	}
}

func rollbackCheck(row releasedb.ReleaseRollbackCheck) releaseport.RollbackCheck {
	return releaseport.RollbackCheck{
		ID: row.ID, CandidateID: row.CandidateID, Kind: releaseport.RollbackCheckKind(row.Kind),
		Passed: row.Passed, EvidenceSHA: row.EvidenceSha, RecordedBy: row.RecordedBy,
		RecordedAt: row.RecordedAt.Time,
	}
}

func operationReceipt(row releasedb.ReleaseOperationReceipt) releaseport.OperationReceipt {
	return releaseport.OperationReceipt{
		ID: row.ID, Action: row.Action, ActorID: row.ActorID, KeyDigest: row.KeyDigest,
		PayloadDigest: row.PayloadDigest, State: row.State, Result: json.RawMessage(row.ResultSnapshot),
		CreatedAt: row.CreatedAt.Time, CompletedAt: optionalTime(row.CompletedAt),
	}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
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
	return fmt.Errorf("%w: %v", releaseport.ErrUnavailable, err)
}

// Package store persists only the externaleffects runtime's opaque control facts.
package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct {
	pool *pgxpool.Pool
	uow  platformport.UnitOfWork
}

var _ eer.Store = (*Repository)(nil)

func NewRepository(pool *pgxpool.Pool, uow platformport.UnitOfWork) *Repository {
	return &Repository{pool: pool, uow: uow}
}

type effectRow struct {
	id, generation, fence int64
	owner                 eer.Owner
	kind                  eer.Kind
	source, target        eer.Digest
	payload, policy       eer.Digest
	state                 eer.State
	attempts              int32
	updated               time.Time
	expires               *time.Time
}

const effectColumns = `id,owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,state,attempt_count,generation,lease_fence,lease_expires_at,updated_at`

func (r *Repository) Accept(ctx context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		fingerprint := command.Envelope.Fingerprint()
		if existing, err := r.lockByFingerprint(ctx, tx, fingerprint); err == nil {
			if got, found, err := r.acceptReceipt(ctx, tx, command.ReceiptKeyDigest); err != nil {
				return err
			} else if found && got.CommandDigest == command.CommandDigest() {
				effect, receipt = existing, got
				return nil
			}
			return eer.ErrPayloadMismatch
		} else if !errors.Is(err, eer.ErrNotFound) {
			return err
		}
		if _, found, err := r.acceptReceipt(ctx, tx, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			return eer.ErrPayloadMismatch
		}
		err := tx.QueryRow(ctx, `INSERT INTO external_effects(owner,kind,source_ref_digest,target_ref_digest,payload_digest,policy_version_hash,envelope_fingerprint,state) VALUES($1,$2,$3,$4,$5,$6,$7,'accepted') RETURNING `+effectColumns,
			command.Envelope.Owner(), command.Envelope.Kind(), command.Envelope.SourceRefDigest(), command.Envelope.TargetRefDigest(), command.Envelope.PayloadDigest(), command.Envelope.PolicyVersionHash(), fingerprint).Scan(effect.scan()...)
		if err != nil {
			return unavailable(err)
		}
		receipt, err = r.insertReceipt(ctx, tx, "accept", effect.id, command.ReceiptKeyDigest, command.CommandDigest(), effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) Queue(ctx context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	return r.queue(ctx, "queue", command.EffectID, command.Job, command.ReceiptKeyDigest, command.CommandDigest(), eer.StateAccepted)
}
func (r *Repository) Retry(ctx context.Context, command eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	return r.queue(ctx, "retry", command.EffectID, command.Job, command.ReceiptKeyDigest, command.CommandDigest(), eer.StateRetryableFailed)
}

func (r *Repository) queue(ctx context.Context, operation, effectID string, job eer.RiverJobLink, key, command eer.Digest, from eer.State) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(effectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if got, found, err := r.receipt(ctx, tx, operation, id, key); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if effect.state != from {
			if operation == "retry" {
				return eer.ErrRetryForbidden
			}
			return eer.ErrInvalidTransition
		}
		_, err = tx.Exec(ctx, `UPDATE external_effects SET state='queued',generation=generation+1,lease_fence=0,lease_expires_at=NULL,river_job_id=$2,river_queue=$3,river_args_digest=$4,river_scheduled_at=$5,updated_at=now() WHERE id=$1`, id, job.JobID, job.Queue, job.ArgsDigest, job.ScheduledAt)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		receipt, err = r.insertReceipt(ctx, tx, operation, id, key, command, effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) Claim(ctx context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	var effect effectRow
	var lease eer.Lease
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(command.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if effect.state != eer.StateQueued {
			return eer.ErrLeaseFence
		}
		var expires time.Time
		err = tx.QueryRow(ctx, `UPDATE external_effects SET lease_fence=lease_fence+1,lease_expires_at=now()+interval '30 seconds',updated_at=now() WHERE id=$1 RETURNING lease_fence,lease_expires_at`, id).Scan(&lease.Fence, &expires)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		lease = eer.Lease{EffectID: command.EffectID, Generation: effect.generation, Fence: lease.Fence, ExpiresAt: expires}
		return nil
	})
	return lease, effect.projection(), err
}

func (r *Repository) Cancel(ctx context.Context, command eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(command.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if got, found, err := r.receipt(ctx, tx, "cancel", id, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command.CommandDigest() {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if effect.state != eer.StateAccepted && effect.state != eer.StateQueued {
			return eer.ErrCancelForbidden
		}
		_, err = tx.Exec(ctx, `UPDATE external_effects SET state='cancelled',lease_expires_at=NULL,updated_at=now() WHERE id=$1`, id)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		receipt, err = r.insertReceipt(ctx, tx, "cancel", id, command.ReceiptKeyDigest, command.CommandDigest(), effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) PersistAttempt(ctx context.Context, lease eer.Lease) (eer.EffectEnvelope, eer.Attempt, eer.Projection, error) {
	var effect effectRow
	var attempt eer.Attempt
	var envelope eer.EffectEnvelope
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if effect.state != eer.StateQueued || effect.generation != lease.Generation || effect.fence != lease.Fence {
			return eer.ErrLeaseFence
		}
		if effect.expires == nil || !effect.expires.After(time.Now()) {
			return eer.ErrLeaseExpired
		}
		var started time.Time
		err = tx.QueryRow(ctx, `UPDATE external_effects SET state='attempted',attempt_count=attempt_count+1,updated_at=now() WHERE id=$1 RETURNING attempt_count,updated_at`, id).Scan(&attempt.Number, &started)
		if err != nil {
			return unavailable(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_effect_attempts(effect_id,number,generation,fence,started_at) VALUES($1,$2,$3,$4,$5)`, id, attempt.Number, lease.Generation, lease.Fence, started)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		attempt.Generation = lease.Generation
		attempt.Fence = lease.Fence
		attempt.StartedAt = started
		envelope, err = eer.NewEnvelope(eer.EnvelopeInput{Owner: effect.owner, Kind: effect.kind, SourceRefDigest: effect.source, TargetRefDigest: effect.target, PayloadDigest: effect.payload, PolicyVersionHash: effect.policy})
		return err
	})
	return envelope, attempt, effect.projection(), err
}

func (r *Repository) CompleteAttempt(ctx context.Context, lease eer.Lease, attempt eer.Attempt, result eer.AdapterResult) (eer.Projection, eer.OperationReceipt, error) {
	return r.complete(ctx, lease, attempt, result, "complete_attempt", result.ReceiptDigest)
}

func (r *Repository) Reconcile(ctx context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(command.Lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if got, found, err := r.receipt(ctx, tx, "reconcile", id, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command.CommandDigest() {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if effect.state != eer.StateOutcomeUnknown || effect.generation != command.Lease.Generation || effect.fence != command.Lease.Fence {
			return eer.ErrReconcileRequired
		}
		_, err = tx.Exec(ctx, `INSERT INTO external_effect_reconciliations(effect_id,generation,fence,evidence_digest) VALUES($1,$2,$3,$4)`, id, command.Lease.Generation, command.Lease.Fence, command.EvidenceDigest)
		if err != nil {
			return unavailable(err)
		}
		_, err = tx.Exec(ctx, `UPDATE external_effects SET state='reconciled',updated_at=now() WHERE id=$1`, id)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		receipt, err = r.insertReceipt(ctx, tx, "reconcile", id, command.ReceiptKeyDigest, command.CommandDigest(), effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) RecoverAttemptedToUnknown(ctx context.Context, command eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(command.Lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if effect.state != eer.StateAttempted || effect.generation != command.Lease.Generation || effect.fence != command.Lease.Fence || effect.expires == nil || effect.expires.After(time.Now()) {
			return eer.ErrRecoveryForbidden
		}
		var attemptNumber int32
		err = tx.QueryRow(ctx, `SELECT number FROM external_effect_attempts WHERE effect_id=$1 AND generation=$2 AND fence=$3 AND completion IS NULL FOR UPDATE`, id, command.Lease.Generation, command.Lease.Fence).Scan(&attemptNumber)
		if err != nil {
			return translate(err)
		}
		unknown := eer.Digest(command.CommandDigest())
		_, err = tx.Exec(ctx, `UPDATE external_effect_attempts SET completion='outcome_unknown',receipt_digest=$2,completed_at=now() WHERE effect_id=$1 AND number=$3`, id, unknown, attemptNumber)
		if err != nil {
			return unavailable(err)
		}
		_, err = tx.Exec(ctx, `UPDATE external_effects SET state='outcome_unknown',lease_expires_at=NULL,updated_at=now() WHERE id=$1`, id)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		receipt, err = r.insertReceipt(ctx, tx, "recover_attempted", id, unknown, unknown, effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) complete(ctx context.Context, lease eer.Lease, attempt eer.Attempt, result eer.AdapterResult, operation string, key eer.Digest) (eer.Projection, eer.OperationReceipt, error) {
	var effect effectRow
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(tx pgx.Tx) error {
		id, err := parseID(lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if effect.state != eer.StateAttempted || effect.generation != lease.Generation || effect.fence != lease.Fence {
			return eer.ErrLeaseFence
		}
		state := eer.State(result.Completion)
		if result.Completion == eer.CompletionOutcomeUnknown {
			state = eer.StateOutcomeUnknown
		}
		if result.Completion == eer.CompletionRetryableFailed {
			state = eer.StateRetryableFailed
		}
		if result.Completion == eer.CompletionFinalFailed {
			state = eer.StateFinalFailed
		}
		if result.Completion == eer.CompletionExecuted {
			state = eer.StateExecuted
		}
		_, err = tx.Exec(ctx, `UPDATE external_effect_attempts SET completion=$4,receipt_digest=$5,completed_at=now() WHERE effect_id=$1 AND number=$2 AND generation=$3 AND completion IS NULL`, id, attempt.Number, attempt.Generation, result.Completion, result.ReceiptDigest)
		if err != nil {
			return unavailable(err)
		}
		_, err = tx.Exec(ctx, `UPDATE external_effects SET state=$2,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, id, state)
		if err != nil {
			return unavailable(err)
		}
		effect, err = r.lock(ctx, tx, id)
		if err != nil {
			return err
		}
		receipt, err = r.insertReceipt(ctx, tx, operation, id, key, key, effect.state)
		return err
	})
	return effect.projection(), receipt, err
}

func (r *Repository) List(ctx context.Context, limit int32) ([]eer.Projection, error) {
	if r == nil || r.pool == nil || limit < 1 || limit > 100 {
		return nil, eer.ErrInvalidCommand
	}
	rows, err := r.pool.Query(ctx, `SELECT `+effectColumns+` FROM external_effects ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	out := []eer.Projection{}
	for rows.Next() {
		var row effectRow
		if err := rows.Scan(row.scan()...); err != nil {
			return nil, unavailable(err)
		}
		out = append(out, row.projection())
	}
	return out, unavailable(rows.Err())
}
func (r *Repository) Get(ctx context.Context, effectID string) (eer.Projection, error) {
	id, err := parseID(effectID)
	if err != nil {
		return eer.Projection{}, eer.ErrInvalidCommand
	}
	var row effectRow
	err = r.pool.QueryRow(ctx, `SELECT `+effectColumns+` FROM external_effects WHERE id=$1`, id).Scan(row.scan()...)
	if err != nil {
		return eer.Projection{}, translate(err)
	}
	return row.projection(), nil
}

func (r *Repository) Diagnostics(ctx context.Context) (eer.Diagnostics, error) {
	if r == nil || r.pool == nil {
		return eer.Diagnostics{}, eer.ErrUnavailable
	}
	var value eer.Diagnostics
	err := r.pool.QueryRow(ctx, `SELECT
COUNT(*) FILTER (WHERE state='accepted'), COUNT(*) FILTER (WHERE state='queued'),
COUNT(*) FILTER (WHERE state='attempted'), COUNT(*) FILTER (WHERE state='outcome_unknown'),
COUNT(*) FILTER (WHERE state='retryable_failed') FROM external_effects`).Scan(
		&value.Accepted, &value.Queued, &value.Attempted, &value.OutcomeUnknown, &value.RetryableFailed)
	if err != nil {
		return eer.Diagnostics{}, unavailable(err)
	}
	return value, nil
}

func (r *Repository) within(ctx context.Context, fn func(pgx.Tx) error) error {
	if r == nil || r.uow == nil {
		return eer.ErrUnavailable
	}
	return r.uow.Within(ctx, func(txctx context.Context) error {
		tx, err := platformstore.TxFromContext(txctx)
		if err != nil {
			return eer.ErrUnavailable
		}
		return fn(tx)
	})
}
func (r *Repository) lock(ctx context.Context, tx pgx.Tx, id int64) (effectRow, error) {
	var row effectRow
	err := tx.QueryRow(ctx, `SELECT `+effectColumns+` FROM external_effects WHERE id=$1 FOR UPDATE`, id).Scan(row.scan()...)
	return row, translate(err)
}
func (r *Repository) lockByFingerprint(ctx context.Context, tx pgx.Tx, f eer.Digest) (effectRow, error) {
	var row effectRow
	err := tx.QueryRow(ctx, `SELECT `+effectColumns+` FROM external_effects WHERE envelope_fingerprint=$1 FOR UPDATE`, f).Scan(row.scan()...)
	return row, translate(err)
}
func (r *Repository) receipt(ctx context.Context, tx pgx.Tx, op string, id int64, key eer.Digest) (eer.OperationReceipt, bool, error) {
	var got eer.OperationReceipt
	var rawID int64
	var nullable *int64
	if id > 0 {
		nullable = &id
	}
	err := tx.QueryRow(ctx, `SELECT id,COALESCE(effect_id,0),command_digest,state,completed_at FROM external_effect_receipts WHERE operation=$1 AND effect_id IS NOT DISTINCT FROM $2 AND receipt_key_digest=$3`, op, nullable, key).Scan(&rawID, &id, &got.CommandDigest, &got.State, &got.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return eer.OperationReceipt{}, false, nil
	}
	if err != nil {
		return eer.OperationReceipt{}, false, unavailable(err)
	}
	got.ID = "eerop_" + strconv.FormatInt(rawID, 10)
	got.EffectID = formatID(id)
	return got, true, nil
}
func (r *Repository) acceptReceipt(ctx context.Context, tx pgx.Tx, key eer.Digest) (eer.OperationReceipt, bool, error) {
	var rawID, effectID int64
	var out eer.OperationReceipt
	err := tx.QueryRow(ctx, `SELECT id,effect_id,command_digest,state,completed_at FROM external_effect_receipts WHERE operation='accept' AND receipt_key_digest=$1`, key).Scan(&rawID, &effectID, &out.CommandDigest, &out.State, &out.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return eer.OperationReceipt{}, false, nil
	}
	if err != nil {
		return eer.OperationReceipt{}, false, unavailable(err)
	}
	out.ID = "eerop_" + strconv.FormatInt(rawID, 10)
	out.EffectID = formatID(effectID)
	return out, true, nil
}
func (r *Repository) insertReceipt(ctx context.Context, tx pgx.Tx, op string, id int64, key, command eer.Digest, state eer.State) (eer.OperationReceipt, error) {
	var out eer.OperationReceipt
	var nullable *int64
	if id > 0 {
		nullable = &id
	}
	var rawID int64
	err := tx.QueryRow(ctx, `INSERT INTO external_effect_receipts(operation,effect_id,receipt_key_digest,command_digest,state) VALUES($1,$2,$3,$4,$5) RETURNING id,completed_at`, op, nullable, key, command, state).Scan(&rawID, &out.CompletedAt)
	if err != nil {
		return eer.OperationReceipt{}, unavailable(err)
	}
	out.ID = "eerop_" + strconv.FormatInt(rawID, 10)
	out.EffectID = formatID(id)
	out.CommandDigest = command
	out.State = state
	return out, nil
}
func (row *effectRow) scan() []any {
	return []any{&row.id, &row.owner, &row.kind, &row.source, &row.target, &row.payload, &row.policy, &row.state, &row.attempts, &row.generation, &row.fence, &row.expires, &row.updated}
}
func (row effectRow) projection() eer.Projection {
	return eer.Projection{ID: formatID(row.id), Owner: row.owner, Kind: row.kind, State: row.state, AttemptCount: row.attempts, Generation: row.generation, UpdatedAt: row.updated}
}
func formatID(id int64) string {
	if id < 1 {
		return ""
	}
	return "eer_" + strconv.FormatInt(id, 10)
}
func parseID(value string) (int64, error) {
	if !strings.HasPrefix(value, "eer_") {
		return 0, eer.ErrInvalidCommand
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "eer_"), 10, 64)
	if err != nil || id < 1 {
		return 0, eer.ErrInvalidCommand
	}
	return id, nil
}
func translate(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return eer.ErrNotFound
	}
	if err == nil {
		return nil
	}
	return unavailable(err)
}
func unavailable(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", eer.ErrUnavailable, err)
}

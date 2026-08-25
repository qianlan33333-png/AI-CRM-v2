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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerdb "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store/generated"
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

func (r *Repository) Accept(ctx context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		fingerprint := command.Envelope.Fingerprint()
		if existing, err := q.LockEffectByFingerprint(ctx, string(fingerprint)); err == nil {
			got, found, err := acceptReceipt(ctx, q, command.ReceiptKeyDigest)
			if err != nil {
				return err
			}
			if found && got.CommandDigest == command.CommandDigest() {
				effect, receipt = existing, got
				return nil
			}
			return eer.ErrPayloadMismatch
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return unavailable(err)
		}
		if _, found, err := acceptReceipt(ctx, q, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			return eer.ErrPayloadMismatch
		}
		created, err := q.CreateEffect(ctx, eerdb.CreateEffectParams{
			Owner:               string(command.Envelope.Owner()),
			Kind:                string(command.Envelope.Kind()),
			SourceRefDigest:     string(command.Envelope.SourceRefDigest()),
			TargetRefDigest:     string(command.Envelope.TargetRefDigest()),
			PayloadDigest:       string(command.Envelope.PayloadDigest()),
			PolicyVersionHash:   string(command.Envelope.PolicyVersionHash()),
			EnvelopeFingerprint: string(fingerprint),
		})
		if err != nil {
			return unavailable(err)
		}
		effect = created
		receipt, err = insertReceipt(ctx, q, "accept", effect.ID, command.ReceiptKeyDigest, command.CommandDigest(), effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) Queue(ctx context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	return r.queue(ctx, "queue", command.EffectID, command.Job, command.ReceiptKeyDigest, command.CommandDigest(), eer.StateAccepted)
}

func (r *Repository) Retry(ctx context.Context, command eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	return r.queue(ctx, "retry", command.EffectID, command.Job, command.ReceiptKeyDigest, command.CommandDigest(), eer.StateRetryableFailed)
}

func (r *Repository) queue(ctx context.Context, operation, effectID string, job eer.RiverJobLink, key, command eer.Digest, from eer.State) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(effectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if got, found, err := receiptFor(ctx, q, operation, id, key); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if eer.State(effect.State) != from {
			if operation == "retry" {
				return eer.ErrRetryForbidden
			}
			return eer.ErrInvalidTransition
		}
		if err := q.QueueEffect(ctx, eerdb.QueueEffectParams{ID: id, RiverJobID: int8Value(job.JobID), RiverQueue: textValue(job.Queue), RiverArgsDigest: textValue(string(job.ArgsDigest)), RiverScheduledAt: timestamp(job.ScheduledAt)}); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		receipt, err = insertReceipt(ctx, q, operation, id, key, command, effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) Claim(ctx context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	var effect eerdb.ExternalEffect
	var lease eer.Lease
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(command.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if eer.State(effect.State) != eer.StateQueued {
			return eer.ErrLeaseFence
		}
		claimed, err := q.ClaimEffect(ctx, id)
		if err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		lease = eer.Lease{EffectID: command.EffectID, Generation: effect.Generation, Fence: claimed.LeaseFence, ExpiresAt: timeValue(claimed.LeaseExpiresAt)}
		return nil
	})
	return lease, projection(effect), err
}

func (r *Repository) Cancel(ctx context.Context, command eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(command.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if got, found, err := receiptFor(ctx, q, "cancel", id, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command.CommandDigest() {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if eer.State(effect.State) != eer.StateAccepted && eer.State(effect.State) != eer.StateQueued {
			return eer.ErrCancelForbidden
		}
		if err := q.CancelEffect(ctx, id); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		receipt, err = insertReceipt(ctx, q, "cancel", id, command.ReceiptKeyDigest, command.CommandDigest(), effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) PersistAttempt(ctx context.Context, lease eer.Lease) (eer.EffectEnvelope, eer.Attempt, eer.Projection, error) {
	var effect eerdb.ExternalEffect
	var attempt eer.Attempt
	var envelope eer.EffectEnvelope
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if eer.State(effect.State) != eer.StateQueued || effect.Generation != lease.Generation || effect.LeaseFence != lease.Fence {
			return eer.ErrLeaseFence
		}
		if !effect.LeaseExpiresAt.Valid || !timeValue(effect.LeaseExpiresAt).After(time.Now()) {
			return eer.ErrLeaseExpired
		}
		started, err := q.StartAttempt(ctx, id)
		if err != nil {
			return unavailable(err)
		}
		if err := q.InsertAttempt(ctx, eerdb.InsertAttemptParams{EffectID: id, Number: started.AttemptCount, Generation: lease.Generation, Fence: lease.Fence, StartedAt: started.UpdatedAt}); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		attempt = eer.Attempt{Number: started.AttemptCount, Generation: lease.Generation, Fence: lease.Fence, StartedAt: timeValue(started.UpdatedAt)}
		envelope, err = eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.Owner(effect.Owner), Kind: eer.Kind(effect.Kind), SourceRefDigest: eer.Digest(effect.SourceRefDigest), TargetRefDigest: eer.Digest(effect.TargetRefDigest), PayloadDigest: eer.Digest(effect.PayloadDigest), PolicyVersionHash: eer.Digest(effect.PolicyVersionHash)})
		return err
	})
	return envelope, attempt, projection(effect), err
}

func (r *Repository) CompleteAttempt(ctx context.Context, lease eer.Lease, attempt eer.Attempt, result eer.AdapterResult) (eer.Projection, eer.OperationReceipt, error) {
	return r.complete(ctx, lease, attempt, result, "complete_attempt", result.ReceiptDigest)
}

func (r *Repository) Reconcile(ctx context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(command.Lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if got, found, err := receiptFor(ctx, q, "reconcile", id, command.ReceiptKeyDigest); err != nil {
			return err
		} else if found {
			if got.CommandDigest != command.CommandDigest() {
				return eer.ErrPayloadMismatch
			}
			receipt = got
			return nil
		}
		if eer.State(effect.State) != eer.StateOutcomeUnknown || effect.Generation != command.Lease.Generation || effect.LeaseFence != command.Lease.Fence {
			return eer.ErrReconcileRequired
		}
		if err := q.ReconcileAttempt(ctx, eerdb.ReconcileAttemptParams{EffectID: id, Generation: command.Lease.Generation, Fence: command.Lease.Fence, EvidenceDigest: string(command.EvidenceDigest)}); err != nil {
			return unavailable(err)
		}
		if err := q.MarkEffectReconciled(ctx, id); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		receipt, err = insertReceipt(ctx, q, "reconcile", id, command.ReceiptKeyDigest, command.CommandDigest(), effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) RecoverAttemptedToUnknown(ctx context.Context, command eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(command.Lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if eer.State(effect.State) != eer.StateAttempted || effect.Generation != command.Lease.Generation || effect.LeaseFence != command.Lease.Fence || !effect.LeaseExpiresAt.Valid || timeValue(effect.LeaseExpiresAt).After(time.Now()) {
			return eer.ErrRecoveryForbidden
		}
		attemptNumber, err := q.LockOpenAttempt(ctx, eerdb.LockOpenAttemptParams{EffectID: id, Generation: command.Lease.Generation, Fence: command.Lease.Fence})
		if err != nil {
			return translate(err)
		}
		unknown := eer.Digest(command.CommandDigest())
		if channelAcquisitionAssetTerminalEvidence(effect) {
			if err := q.CompleteChannelAcquisitionAssetAttempt(ctx, eerdb.CompleteChannelAcquisitionAssetAttemptParams{
				EffectID: id, Number: attemptNumber, Generation: command.Lease.Generation,
				Completion: textValue(string(eer.CompletionOutcomeUnknown)), ReceiptDigest: textValue(string(unknown)),
			}); err != nil {
				return unavailable(err)
			}
		} else if err := q.MarkAttemptOutcomeUnknown(ctx, eerdb.MarkAttemptOutcomeUnknownParams{EffectID: id, ReceiptDigest: textValue(string(unknown)), Number: attemptNumber}); err != nil {
			return unavailable(err)
		}
		if err := q.UpdateEffectState(ctx, eerdb.UpdateEffectStateParams{ID: id, State: string(eer.StateOutcomeUnknown)}); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		receipt, err = insertReceipt(ctx, q, "recover_attempted", id, unknown, unknown, effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) complete(ctx context.Context, lease eer.Lease, attempt eer.Attempt, result eer.AdapterResult, operation string, key eer.Digest) (eer.Projection, eer.OperationReceipt, error) {
	var effect eerdb.ExternalEffect
	var receipt eer.OperationReceipt
	err := r.within(ctx, func(q eerdb.Querier) error {
		id, err := parseID(lease.EffectID)
		if err != nil {
			return eer.ErrInvalidCommand
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		if eer.State(effect.State) != eer.StateAttempted || effect.Generation != lease.Generation || effect.LeaseFence != lease.Fence {
			return eer.ErrLeaseFence
		}
		state := eer.State(result.Completion)
		switch result.Completion {
		case eer.CompletionOutcomeUnknown:
			state = eer.StateOutcomeUnknown
		case eer.CompletionRetryableFailed:
			state = eer.StateRetryableFailed
		case eer.CompletionFinalFailed:
			state = eer.StateFinalFailed
		case eer.CompletionExecuted:
			state = eer.StateExecuted
		}
		if channelAcquisitionAssetTerminalEvidence(effect) {
			if err := q.CompleteChannelAcquisitionAssetAttempt(ctx, eerdb.CompleteChannelAcquisitionAssetAttemptParams{
				EffectID: id, Number: attempt.Number, Generation: attempt.Generation, Completion: textValue(string(result.Completion)), ReceiptDigest: textValue(string(result.ReceiptDigest)),
				ResultReferenceDigest: nullableText(string(result.ResultReferenceDigest)), BusinessCallDispatched: result.BusinessCallDispatched,
				RealExternalCallExecuted: result.RealExternalCallExecuted,
			}); err != nil {
				return unavailable(err)
			}
		} else if err := q.CompleteAttempt(ctx, eerdb.CompleteAttemptParams{EffectID: id, Number: attempt.Number, Generation: attempt.Generation, Completion: textValue(string(result.Completion)), ReceiptDigest: textValue(string(result.ReceiptDigest))}); err != nil {
			return unavailable(err)
		}
		if err := q.UpdateEffectState(ctx, eerdb.UpdateEffectStateParams{ID: id, State: string(state)}); err != nil {
			return unavailable(err)
		}
		effect, err = lock(ctx, q, id)
		if err != nil {
			return err
		}
		receipt, err = insertReceipt(ctx, q, operation, id, key, key, effect.State)
		return err
	})
	return projection(effect), receipt, err
}

func (r *Repository) List(ctx context.Context, limit int32) ([]eer.Projection, error) {
	if r == nil || r.pool == nil || limit < 1 || limit > 100 {
		return nil, eer.ErrInvalidCommand
	}
	rows, err := eerdb.New(r.pool).ListEffects(ctx, limit)
	if err != nil {
		return nil, unavailable(err)
	}
	out := make([]eer.Projection, len(rows))
	for i, row := range rows {
		out[i] = projection(row)
	}
	return out, nil
}

func (r *Repository) Get(ctx context.Context, effectID string) (eer.Projection, error) {
	if r == nil || r.pool == nil {
		return eer.Projection{}, eer.ErrUnavailable
	}
	id, err := parseID(effectID)
	if err != nil {
		return eer.Projection{}, eer.ErrInvalidCommand
	}
	row, err := eerdb.New(r.pool).GetEffect(ctx, id)
	if err != nil {
		return eer.Projection{}, translate(err)
	}
	return projection(row), nil
}

func (r *Repository) GetTerminalOutcome(ctx context.Context, effectID string) (eer.TerminalOutcome, error) {
	if r == nil || r.pool == nil {
		return eer.TerminalOutcome{}, eer.ErrUnavailable
	}
	id, err := parseID(effectID)
	if err != nil {
		return eer.TerminalOutcome{}, err
	}
	row, err := eerdb.New(r.pool).GetTerminalOutcome(ctx, id)
	if err != nil {
		return eer.TerminalOutcome{}, translate(err)
	}
	result := eer.TerminalOutcome{EffectID: effectID, Owner: eer.Owner(row.Owner), Kind: eer.Kind(row.Kind), State: eer.State(row.State), ReceiptID: "eerop_" + strconv.FormatInt(row.ReceiptID, 10), ReceiptDigest: eer.Digest(row.ReceiptDigest.String), Generation: row.Generation, Fence: row.Fence, LeaseExpiresAt: timeValue(row.LeaseExpiresAt)}
	if channelAcquisitionAssetTerminalEvidence(eerdb.ExternalEffect{Owner: row.Owner, Kind: row.Kind}) {
		evidence, err := eerdb.New(r.pool).GetChannelAcquisitionAssetTerminalEvidence(ctx, eerdb.GetChannelAcquisitionAssetTerminalEvidenceParams{EffectID: id, AttemptNumber: row.AttemptNumber, Generation: row.Generation, Fence: row.Fence})
		if err != nil {
			return eer.TerminalOutcome{}, unavailable(err)
		}
		result.ResultReferenceDigest = eer.Digest(evidence.ResultReferenceDigest.String)
		result.BusinessCallDispatched = evidence.BusinessCallDispatched
		result.RealExternalCallExecuted = evidence.RealExternalCallExecuted
	}
	return result, nil
}

func channelAcquisitionAssetTerminalEvidence(effect eerdb.ExternalEffect) bool {
	return effect.Owner == string(eer.OwnerContact) && effect.Kind == string(eer.KindContactAcquisitionAssetPublish)
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func (r *Repository) Diagnostics(ctx context.Context) (eer.Diagnostics, error) {
	if r == nil || r.pool == nil {
		return eer.Diagnostics{}, eer.ErrUnavailable
	}
	row, err := eerdb.New(r.pool).GetDiagnostics(ctx)
	if err != nil {
		return eer.Diagnostics{}, unavailable(err)
	}
	return eer.Diagnostics{Accepted: row.Accepted, Queued: row.Queued, Attempted: row.Attempted, OutcomeUnknown: row.OutcomeUnknown, RetryableFailed: row.RetryableFailed}, nil
}

func (r *Repository) within(ctx context.Context, fn func(eerdb.Querier) error) error {
	if r == nil || r.uow == nil {
		return eer.ErrUnavailable
	}
	// Domain integrations may need an external-effect acceptance and their
	// immutable business binding to commit together. Reuse the caller's active
	// UoW when one exists; starting a nested transaction would otherwise reject
	// that safe composition (or, worse, split the two facts).
	if tx, err := platformstore.TxFromContext(ctx); err == nil {
		return fn(eerdb.New(tx))
	}
	return r.uow.Within(ctx, func(txctx context.Context) error {
		tx, err := platformstore.TxFromContext(txctx)
		if err != nil {
			return eer.ErrUnavailable
		}
		return fn(eerdb.New(tx))
	})
}

func lock(ctx context.Context, q eerdb.Querier, id int64) (eerdb.ExternalEffect, error) {
	row, err := q.LockEffect(ctx, id)
	return row, translate(err)
}

func receiptFor(ctx context.Context, q eerdb.Querier, op string, id int64, key eer.Digest) (eer.OperationReceipt, bool, error) {
	row, err := q.GetReceipt(ctx, eerdb.GetReceiptParams{Operation: op, EffectID: optionalID(id), ReceiptKeyDigest: string(key)})
	if errors.Is(err, pgx.ErrNoRows) {
		return eer.OperationReceipt{}, false, nil
	}
	if err != nil {
		return eer.OperationReceipt{}, false, unavailable(err)
	}
	return receipt(row), true, nil
}

func acceptReceipt(ctx context.Context, q eerdb.Querier, key eer.Digest) (eer.OperationReceipt, bool, error) {
	row, err := q.GetAcceptReceipt(ctx, string(key))
	if errors.Is(err, pgx.ErrNoRows) {
		return eer.OperationReceipt{}, false, nil
	}
	if err != nil {
		return eer.OperationReceipt{}, false, unavailable(err)
	}
	return receipt(row), true, nil
}

func insertReceipt(ctx context.Context, q eerdb.Querier, op string, id int64, key, command eer.Digest, state string) (eer.OperationReceipt, error) {
	row, err := q.InsertReceipt(ctx, eerdb.InsertReceiptParams{Operation: op, EffectID: optionalID(id), ReceiptKeyDigest: string(key), CommandDigest: string(command), State: state})
	if err != nil {
		return eer.OperationReceipt{}, unavailable(err)
	}
	return receipt(row), nil
}

func projection(row eerdb.ExternalEffect) eer.Projection {
	return eer.Projection{ID: formatID(row.ID), Owner: eer.Owner(row.Owner), Kind: eer.Kind(row.Kind), State: eer.State(row.State), AttemptCount: row.AttemptCount, Generation: row.Generation, UpdatedAt: timeValue(row.UpdatedAt)}
}

func receipt(row eerdb.ExternalEffectReceipt) eer.OperationReceipt {
	return eer.OperationReceipt{ID: "eerop_" + strconv.FormatInt(row.ID, 10), EffectID: formatID(row.EffectID.Int64), CommandDigest: eer.Digest(row.CommandDigest), State: eer.State(row.State), CompletedAt: timeValue(row.CompletedAt)}
}

func optionalID(id int64) pgtype.Int8 {
	return pgtype.Int8{Int64: id, Valid: id > 0}
}

func int8Value(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func timeValue(value pgtype.Timestamptz) time.Time {
	return value.Time
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

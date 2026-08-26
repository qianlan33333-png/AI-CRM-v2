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
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
	wecomdb "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store/generated"
)

type ProfileEffectRepository struct {
	pool *pgxpool.Pool
	uow  *platformstore.UnitOfWork
}

var _ profile.Store = (*ProfileEffectRepository)(nil)

func NewProfileEffectRepository(pool *pgxpool.Pool) *ProfileEffectRepository {
	return &ProfileEffectRepository{pool: pool, uow: platformstore.NewUnitOfWork(pool)}
}

func (r *ProfileEffectRepository) Reserve(ctx context.Context, c profile.Effect) (profile.Effect, bool, error) {
	id, err := parseProfileID(c.EffectID, "eer_")
	if err != nil {
		return profile.Effect{}, false, err
	}
	receipt, err := parseProfileID(c.AcceptReceiptID, "eerop_")
	if err != nil || c.LegacyReceiptID < 1 || c.Actor < 1 || c.Generation < 1 || c.UpdatedAt.IsZero() {
		return profile.Effect{}, false, profile.ErrInvalidCommand
	}
	var row wecomdb.WecomContactProfileEffect
	inserted := false
	err = r.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.InsertWeComContactProfileEffect(ctx, wecomdb.InsertWeComContactProfileEffectParams{
			EffectID: id, LegacyReceiptID: c.LegacyReceiptID, ActorID: c.Actor, CorpID: c.CorpID,
			StaffUserid: c.StaffUserID, ExternalUserid: c.ExternalUserID, Remark: c.Remark, Description: c.Description,
			IdempotencyDigest: string(c.IdempotencyDigest), EnvelopeFingerprint: string(c.EnvelopeFingerprint),
			AcceptReceiptID: receipt, Generation: c.Generation, UpdatedAt: timestamp(c.UpdatedAt),
		})
		if err == nil {
			inserted = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		row, err = queries.GetWeComContactProfileEffectByIdempotency(ctx, wecomdb.GetWeComContactProfileEffectByIdempotencyParams{
			ActorID: c.Actor, IdempotencyDigest: string(c.IdempotencyDigest),
		})
		return err
	})
	if err != nil {
		return profile.Effect{}, false, profileStoreError(err)
	}
	return fromProfileRow(row), inserted, nil
}
func (r *ProfileEffectRepository) GetByIdempotency(ctx context.Context, actor int64, d eer.Digest) (profile.Effect, error) {
	if r == nil || r.pool == nil || ctx == nil || actor < 1 || d == "" {
		return profile.Effect{}, profile.ErrEffectUnavailable
	}
	queries := wecomdb.New(r.pool)
	if tx, txErr := platformstore.TxFromContext(ctx); txErr == nil {
		queries = wecomdb.New(tx)
	}
	row, err := queries.GetWeComContactProfileEffectByIdempotency(ctx, wecomdb.GetWeComContactProfileEffectByIdempotencyParams{
		ActorID: actor, IdempotencyDigest: string(d),
	})
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}
func (r *ProfileEffectRepository) Get(ctx context.Context, effectID string) (profile.Effect, error) {
	id, err := parseProfileID(effectID, "eer_")
	if err != nil {
		return profile.Effect{}, err
	}
	if r == nil || r.pool == nil || ctx == nil {
		return profile.Effect{}, profile.ErrEffectUnavailable
	}
	queries := wecomdb.New(r.pool)
	if tx, txErr := platformstore.TxFromContext(ctx); txErr == nil {
		queries = wecomdb.New(tx)
	}
	row, err := queries.GetWeComContactProfileEffect(ctx, id)
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}
func (r *ProfileEffectRepository) MarkQueued(ctx context.Context, effectID string, link eer.RiverJobLink, receiptID string, at time.Time) (profile.Effect, error) {
	id, err := parseProfileID(effectID, "eer_")
	if err != nil {
		return profile.Effect{}, err
	}
	receipt, err := parseProfileID(receiptID, "eerop_")
	if err != nil || link.JobID < 1 || link.Generation < 1 || at.IsZero() {
		return profile.Effect{}, profile.ErrInvalidCommand
	}
	var row wecomdb.WecomContactProfileEffect
	err = r.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.MarkWeComContactProfileEffectQueued(ctx, wecomdb.MarkWeComContactProfileEffectQueuedParams{
			QueueReceiptID: int8Value(receipt), RiverJobID: int8Value(link.JobID), Generation: link.Generation,
			UpdatedAt: timestamp(at), EffectID: id,
		})
		return err
	})
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}
func (r *ProfileEffectRepository) RecordClaim(ctx context.Context, effectID string, lease eer.Lease, at time.Time) (profile.Effect, error) {
	id, err := parseProfileID(effectID, "eer_")
	if err != nil || lease.EffectID != effectID || lease.Generation < 1 || lease.Fence < 1 || lease.ExpiresAt.IsZero() || at.IsZero() {
		return profile.Effect{}, profile.ErrInvalidCommand
	}
	var row wecomdb.WecomContactProfileEffect
	err = r.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.RecordWeComContactProfileEffectClaim(ctx, wecomdb.RecordWeComContactProfileEffectClaimParams{
			Generation: lease.Generation, Fence: lease.Fence, LeaseExpiresAt: timestamp(lease.ExpiresAt),
			UpdatedAt: timestamp(at), EffectID: id,
		})
		return err
	})
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}
func (r *ProfileEffectRepository) CompleteAttempt(ctx context.Context, c profile.AttemptCompletion) (profile.Effect, error) {
	id, err := parseProfileID(c.EffectID, "eer_")
	if err != nil || c.Lease.EffectID != c.EffectID || c.Lease.Generation < 1 || c.Lease.Fence < 1 || c.Lease.ExpiresAt.IsZero() || c.CompletedAt.IsZero() {
		return profile.Effect{}, profile.ErrInvalidCommand
	}
	receipt, err := parseProfileID(c.ReceiptID, "eerop_")
	if err != nil || (c.State != eer.StateExecuted && c.State != eer.StateOutcomeUnknown && c.State != eer.StateFinalFailed) || (!c.ProviderCallAttempted && c.RealExternalCallExecuted) {
		return profile.Effect{}, profile.ErrInvalidCommand
	}
	var row wecomdb.WecomContactProfileEffect
	err = r.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.CompleteWeComContactProfileEffectAttempt(ctx, wecomdb.CompleteWeComContactProfileEffectAttemptParams{
			State: string(c.State), AttemptReceiptID: int8Value(receipt), AttemptReceiptDigest: textValue(string(c.Receipt)),
			ProviderCallAttempted: c.ProviderCallAttempted, RealExternalCallExecuted: c.RealExternalCallExecuted,
			UpdatedAt: timestamp(c.CompletedAt), EffectID: id, Generation: c.Lease.Generation, Fence: c.Lease.Fence,
		})
		return err
	})
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}
func (r *ProfileEffectRepository) CompleteReconcile(ctx context.Context, c profile.ReconcileCompletion) (profile.Effect, error) {
	id, err := parseProfileID(c.EffectID, "eer_")
	if err != nil || c.Lease.EffectID != c.EffectID || c.Lease.Generation < 1 || c.Lease.Fence < 1 || c.Lease.ExpiresAt.IsZero() || c.CompletedAt.IsZero() {
		return profile.Effect{}, profile.ErrReconcileRequired
	}
	receipt, err := parseProfileID(c.ReceiptID, "eerop_")
	if err != nil || (c.Resolution != profile.ResolutionProviderApplied && c.Resolution != profile.ResolutionProviderNotApplied) {
		return profile.Effect{}, profile.ErrReconcileRequired
	}
	var row wecomdb.WecomContactProfileEffect
	err = r.within(ctx, func(queries *wecomdb.Queries) error {
		row, err = queries.CompleteWeComContactProfileEffectReconcile(ctx, wecomdb.CompleteWeComContactProfileEffectReconcileParams{
			ReconcileReceiptID: int8Value(receipt), ReconcileReceiptDigest: textValue(string(c.Receipt)),
			ReconcileEvidenceDigest: textValue(string(c.EvidenceDigest)), ReconcileResolution: textValue(string(c.Resolution)),
			UpdatedAt: timestamp(c.CompletedAt), EffectID: id, Generation: c.Lease.Generation, Fence: c.Lease.Fence,
		})
		return err
	})
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}

func (r *ProfileEffectRepository) within(ctx context.Context, fn func(*wecomdb.Queries) error) error {
	if r == nil || r.pool == nil || r.uow == nil || ctx == nil || fn == nil {
		return profile.ErrEffectUnavailable
	}
	if tx, e := platformstore.TxFromContext(ctx); e == nil {
		return fn(wecomdb.New(tx))
	}
	return r.uow.Within(ctx, func(txCtx context.Context) error {
		tx, e := platformstore.TxFromContext(txCtx)
		if e != nil {
			return e
		}
		return fn(wecomdb.New(tx))
	})
}
func fromProfileRow(r wecomdb.WecomContactProfileEffect) profile.Effect {
	return profile.Effect{EffectID: formatProfileID("eer_", r.EffectID), LegacyReceiptID: r.LegacyReceiptID, Actor: r.ActorID, CorpID: r.CorpID, StaffUserID: r.StaffUserid, ExternalUserID: r.ExternalUserid, Remark: r.Remark, Description: r.Description, IdempotencyDigest: eer.Digest(r.IdempotencyDigest), EnvelopeFingerprint: eer.Digest(r.EnvelopeFingerprint), State: eer.State(r.State), AcceptReceiptID: formatProfileID("eerop_", r.AcceptReceiptID), QueueReceiptID: optionalProfileID("eerop_", r.QueueReceiptID), RiverJobID: optionalInt(r.RiverJobID), Generation: r.Generation, Fence: r.Fence, LeaseExpiresAt: optionalTime(r.LeaseExpiresAt), AttemptReceiptID: optionalProfileID("eerop_", r.AttemptReceiptID), AttemptReceiptDigest: optionalDigest(r.AttemptReceiptDigest), AttemptCompletedAt: optionalTime(r.AttemptCompletedAt), ProviderCallAttempted: r.ProviderCallAttempted, RealExternalCallExecuted: r.RealExternalCallExecuted, ReconcileReceiptID: optionalProfileID("eerop_", r.ReconcileReceiptID), ReconcileReceiptDigest: optionalDigest(r.ReconcileReceiptDigest), ReconcileEvidenceDigest: optionalDigest(r.ReconcileEvidenceDigest), ReconcileResolution: optionalResolution(r.ReconcileResolution), ReconciledAt: optionalTime(r.ReconciledAt), UpdatedAt: timeValue(r.UpdatedAt)}
}
func parseProfileID(v, prefix string) (int64, error) {
	if !strings.HasPrefix(v, prefix) {
		return 0, profile.ErrInvalidCommand
	}
	d := strings.TrimPrefix(v, prefix)
	if d == "" || d[0] == '0' || strings.TrimSpace(d) != d {
		return 0, profile.ErrInvalidCommand
	}
	id, e := strconv.ParseInt(d, 10, 64)
	if e != nil || id < 1 || strconv.FormatInt(id, 10) != d {
		return 0, profile.ErrInvalidCommand
	}
	return id, nil
}
func formatProfileID(prefix string, id int64) string {
	if id < 1 {
		return ""
	}
	return prefix + strconv.FormatInt(id, 10)
}
func optionalProfileID(prefix string, v pgtype.Int8) string {
	if !v.Valid {
		return ""
	}
	return formatProfileID(prefix, v.Int64)
}
func optionalInt(v pgtype.Int8) int64 {
	if !v.Valid {
		return 0
	}
	return v.Int64
}
func optionalTime(v pgtype.Timestamptz) time.Time { return timeValue(v) }
func optionalDigest(v pgtype.Text) eer.Digest {
	if !v.Valid {
		return ""
	}
	return eer.Digest(v.String)
}
func optionalResolution(v pgtype.Text) profile.ReconcileResolution {
	if !v.Valid {
		return ""
	}
	return profile.ReconcileResolution(v.String)
}
func profileStoreError(err error) error {
	if errors.Is(err, profile.ErrInvalidCommand) || errors.Is(err, profile.ErrReconcileRequired) || errors.Is(err, profile.ErrEffectUnavailable) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return profile.ErrEffectUnavailable
	}
	return fmt.Errorf("%w: %v", profile.ErrEffectUnavailable, err)
}

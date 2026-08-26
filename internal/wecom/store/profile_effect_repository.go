package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/profile"
)

type ProfileEffectRepository struct {
	pool *pgxpool.Pool
	uow  *platformstore.UnitOfWork
}

var _ profile.Store = (*ProfileEffectRepository)(nil)

func NewProfileEffectRepository(pool *pgxpool.Pool) *ProfileEffectRepository {
	return &ProfileEffectRepository{pool: pool, uow: platformstore.NewUnitOfWork(pool)}
}

type profileEffectRow struct {
	EffectID                 int64      `json:"effect_id"`
	LegacyReceiptID          int64      `json:"legacy_receipt_id"`
	ActorID                  int64      `json:"actor_id"`
	CorpID                   string     `json:"corp_id"`
	StaffUserID              string     `json:"staff_userid"`
	ExternalUserID           string     `json:"external_userid"`
	Remark                   string     `json:"remark"`
	Description              string     `json:"description"`
	IdempotencyDigest        string     `json:"idempotency_digest"`
	EnvelopeFingerprint      string     `json:"envelope_fingerprint"`
	State                    string     `json:"state"`
	AcceptReceiptID          int64      `json:"accept_receipt_id"`
	QueueReceiptID           *int64     `json:"queue_receipt_id"`
	RiverJobID               *int64     `json:"river_job_id"`
	Generation               int64      `json:"generation"`
	Fence                    int64      `json:"fence"`
	LeaseExpiresAt           *time.Time `json:"lease_expires_at"`
	AttemptReceiptID         *int64     `json:"attempt_receipt_id"`
	AttemptReceiptDigest     *string    `json:"attempt_receipt_digest"`
	AttemptCompletedAt       *time.Time `json:"attempt_completed_at"`
	ProviderCallAttempted    bool       `json:"provider_call_attempted"`
	RealExternalCallExecuted bool       `json:"real_external_call_executed"`
	ReconcileReceiptID       *int64     `json:"reconcile_receipt_id"`
	ReconcileReceiptDigest   *string    `json:"reconcile_receipt_digest"`
	ReconcileEvidenceDigest  *string    `json:"reconcile_evidence_digest"`
	ReconcileResolution      *string    `json:"reconcile_resolution"`
	ReconciledAt             *time.Time `json:"reconciled_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
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
	var row profileEffectRow
	inserted := false
	err = r.within(ctx, func(tx pgx.Tx) error {
		q := `WITH inserted AS (INSERT INTO public.wecom_contact_profile_effects (effect_id,legacy_receipt_id,actor_id,corp_id,staff_userid,external_userid,remark,description,idempotency_digest,envelope_fingerprint,state,accept_receipt_id,generation,updated_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'accepted',$11,$12,$13 FROM public.external_effects e JOIN public.external_effect_receipts r ON r.id=$11 AND r.effect_id=e.id AND r.operation='accept' AND r.state='accepted' WHERE e.id=$1 AND e.owner='wecom' AND e.kind='wecom_profile_sync' AND e.state='accepted' AND e.generation=$12 AND e.envelope_fingerprint=$10 ON CONFLICT DO NOTHING RETURNING *) SELECT row_to_json(inserted) FROM inserted`
		data, e := queryJSON(ctx, tx, q, id, c.LegacyReceiptID, c.Actor, c.CorpID, c.StaffUserID, c.ExternalUserID, c.Remark, c.Description, string(c.IdempotencyDigest), string(c.EnvelopeFingerprint), receipt, c.Generation, c.UpdatedAt.UTC())
		if e == nil {
			inserted = true
			return json.Unmarshal(data, &row)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		data, e = queryJSON(ctx, tx, `SELECT row_to_json(p) FROM public.wecom_contact_profile_effects p WHERE actor_id=$1 AND idempotency_digest=$2`, c.Actor, string(c.IdempotencyDigest))
		if e != nil {
			return e
		}
		return json.Unmarshal(data, &row)
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
	row, err := r.read(ctx, `SELECT row_to_json(p) FROM public.wecom_contact_profile_effects p WHERE actor_id=$1 AND idempotency_digest=$2`, actor, string(d))
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
	row, err := r.read(ctx, `SELECT row_to_json(p) FROM public.wecom_contact_profile_effects p WHERE effect_id=$1`, id)
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
	row, err := r.mutate(ctx, `UPDATE public.wecom_contact_profile_effects p SET state='queued',queue_receipt_id=$1,river_job_id=$2,generation=$3,fence=0,lease_expires_at=NULL,updated_at=$4 FROM public.external_effects e,public.external_effect_receipts r WHERE p.effect_id=$5 AND p.state='accepted' AND e.id=p.effect_id AND e.owner='wecom' AND e.kind='wecom_profile_sync' AND e.state='queued' AND e.generation=$3 AND e.river_job_id=$2 AND r.id=$1 AND r.effect_id=p.effect_id AND r.operation='queue' AND r.state='queued' RETURNING row_to_json(p)`, receipt, link.JobID, link.Generation, at.UTC(), id)
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
	row, err := r.mutate(ctx, `UPDATE public.wecom_contact_profile_effects p SET generation=$1,fence=$2,lease_expires_at=$3,updated_at=$4 FROM public.external_effects e WHERE p.effect_id=$5 AND p.state='queued' AND e.id=p.effect_id AND e.owner='wecom' AND e.kind='wecom_profile_sync' AND e.state='queued' AND e.generation=$1 AND e.lease_fence=$2 AND e.lease_expires_at=$3 RETURNING row_to_json(p)`, lease.Generation, lease.Fence, lease.ExpiresAt.UTC(), at.UTC(), id)
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
	row, err := r.mutate(ctx, `UPDATE public.wecom_contact_profile_effects p SET state=$1,attempt_receipt_id=$2,attempt_receipt_digest=$3,attempt_completed_at=a.completed_at,provider_call_attempted=$4,real_external_call_executed=$5,updated_at=$6 FROM public.external_effects e,public.external_effect_receipts r,public.external_effect_attempts a WHERE p.effect_id=$7 AND p.state='queued' AND p.generation=$8 AND p.fence=$9 AND e.id=p.effect_id AND e.owner='wecom' AND e.kind='wecom_profile_sync' AND e.state=$1 AND e.generation=$8 AND e.lease_fence=$9 AND e.updated_at=$6 AND r.id=$2 AND r.effect_id=p.effect_id AND r.operation='complete_attempt' AND r.state=$1 AND r.command_digest=$3 AND a.effect_id=p.effect_id AND a.generation=$8 AND a.fence=$9 AND a.completion=$1 AND a.receipt_digest=$3 AND a.completed_at IS NOT NULL RETURNING row_to_json(p)`, string(c.State), receipt, string(c.Receipt), c.ProviderCallAttempted, c.RealExternalCallExecuted, c.CompletedAt.UTC(), id, c.Lease.Generation, c.Lease.Fence)
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
	row, err := r.mutate(ctx, `UPDATE public.wecom_contact_profile_effects p SET state='reconciled',reconcile_receipt_id=$1,reconcile_receipt_digest=$2,reconcile_evidence_digest=$3,reconcile_resolution=$4,reconciled_at=x.recorded_at,updated_at=$5 FROM public.external_effects e,public.external_effect_receipts r,public.external_effect_reconciliations x WHERE p.effect_id=$6 AND p.state='outcome_unknown' AND p.generation=$7 AND p.fence=$8 AND e.id=p.effect_id AND e.owner='wecom' AND e.kind='wecom_profile_sync' AND e.state='reconciled' AND e.generation=$7 AND e.lease_fence=$8 AND e.updated_at=$5 AND r.id=$1 AND r.effect_id=p.effect_id AND r.operation='reconcile' AND r.state='reconciled' AND r.command_digest=$2 AND x.effect_id=p.effect_id AND x.generation=$7 AND x.fence=$8 AND x.evidence_digest=$3 RETURNING row_to_json(p)`, receipt, string(c.Receipt), string(c.EvidenceDigest), string(c.Resolution), c.CompletedAt.UTC(), id, c.Lease.Generation, c.Lease.Fence)
	if err != nil {
		return profile.Effect{}, profileStoreError(err)
	}
	return fromProfileRow(row), nil
}

func (r *ProfileEffectRepository) read(ctx context.Context, q string, args ...any) (profileEffectRow, error) {
	var row profileEffectRow
	data, err := r.query(ctx, q, args...)
	if err != nil {
		return row, err
	}
	return row, json.Unmarshal(data, &row)
}
func (r *ProfileEffectRepository) mutate(ctx context.Context, q string, args ...any) (profileEffectRow, error) {
	var row profileEffectRow
	err := r.within(ctx, func(tx pgx.Tx) error {
		data, e := queryJSON(ctx, tx, q, args...)
		if e != nil {
			return e
		}
		return json.Unmarshal(data, &row)
	})
	return row, err
}
func (r *ProfileEffectRepository) query(ctx context.Context, q string, args ...any) ([]byte, error) {
	if tx, e := platformstore.TxFromContext(ctx); e == nil {
		return queryJSON(ctx, tx, q, args...)
	}
	if r == nil || r.pool == nil {
		return nil, profile.ErrEffectUnavailable
	}
	var data []byte
	err := r.pool.QueryRow(ctx, q, args...).Scan(&data)
	return data, err
}
func (r *ProfileEffectRepository) within(ctx context.Context, fn func(pgx.Tx) error) error {
	if r == nil || r.pool == nil || r.uow == nil || ctx == nil || fn == nil {
		return profile.ErrEffectUnavailable
	}
	if tx, e := platformstore.TxFromContext(ctx); e == nil {
		return fn(tx)
	}
	return r.uow.Within(ctx, func(txCtx context.Context) error {
		tx, e := platformstore.TxFromContext(txCtx)
		if e != nil {
			return e
		}
		return fn(tx)
	})
}
func queryJSON(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]byte, error) {
	var data []byte
	if err := tx.QueryRow(ctx, q, args...).Scan(&data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, pgx.ErrNoRows
	}
	return data, nil
}
func fromProfileRow(r profileEffectRow) profile.Effect {
	return profile.Effect{EffectID: formatProfileID("eer_", r.EffectID), LegacyReceiptID: r.LegacyReceiptID, Actor: r.ActorID, CorpID: r.CorpID, StaffUserID: r.StaffUserID, ExternalUserID: r.ExternalUserID, Remark: r.Remark, Description: r.Description, IdempotencyDigest: eer.Digest(r.IdempotencyDigest), EnvelopeFingerprint: eer.Digest(r.EnvelopeFingerprint), State: eer.State(r.State), AcceptReceiptID: formatProfileID("eerop_", r.AcceptReceiptID), QueueReceiptID: optionalProfileID("eerop_", r.QueueReceiptID), RiverJobID: optionalInt(r.RiverJobID), Generation: r.Generation, Fence: r.Fence, LeaseExpiresAt: optionalTime(r.LeaseExpiresAt), AttemptReceiptID: optionalProfileID("eerop_", r.AttemptReceiptID), AttemptReceiptDigest: optionalDigest(r.AttemptReceiptDigest), AttemptCompletedAt: optionalTime(r.AttemptCompletedAt), ProviderCallAttempted: r.ProviderCallAttempted, RealExternalCallExecuted: r.RealExternalCallExecuted, ReconcileReceiptID: optionalProfileID("eerop_", r.ReconcileReceiptID), ReconcileReceiptDigest: optionalDigest(r.ReconcileReceiptDigest), ReconcileEvidenceDigest: optionalDigest(r.ReconcileEvidenceDigest), ReconcileResolution: optionalResolution(r.ReconcileResolution), ReconciledAt: optionalTime(r.ReconciledAt), UpdatedAt: r.UpdatedAt.UTC()}
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
func optionalProfileID(prefix string, v *int64) string {
	if v == nil {
		return ""
	}
	return formatProfileID(prefix, *v)
}
func optionalInt(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
func optionalTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return v.UTC()
}
func optionalDigest(v *string) eer.Digest {
	if v == nil {
		return ""
	}
	return eer.Digest(*v)
}
func optionalResolution(v *string) profile.ReconcileResolution {
	if v == nil {
		return ""
	}
	return profile.ReconcileResolution(*v)
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

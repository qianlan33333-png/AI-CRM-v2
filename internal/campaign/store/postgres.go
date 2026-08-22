package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	"time"
)

type Repository struct {
	tx func(context.Context) (pgx.Tx, error)
}

func NewRepository() *Repository { return &Repository{tx: platformstore.TxFromContext} }

var _ campaign.Repository = (*Repository)(nil)

func (r *Repository) transaction(ctx context.Context) (pgx.Tx, error) {
	if r == nil || r.tx == nil {
		return nil, campaign.ErrUnavailable
	}
	return r.tx(ctx)
}
func (r *Repository) List(ctx context.Context, in campaign.ListInput) ([]campaign.Campaign, error) {
	tx, e := r.transaction(ctx)
	if e != nil {
		return nil, e
	}
	var approval, runtime *string
	if in.ApprovalStatus != nil {
		x := string(*in.ApprovalStatus)
		approval = &x
	}
	if in.RuntimeStatus != nil {
		x := string(*in.RuntimeStatus)
		runtime = &x
	}
	rows, e := tx.Query(ctx, `SELECT campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at FROM cloud_campaigns WHERE ($1::text IS NULL OR approval_status=$1) AND ($2::text IS NULL OR runtime_status=$2) ORDER BY campaign_code`, approval, runtime)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []campaign.Campaign{}
	for rows.Next() {
		c, e := scanCampaign(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (r *Repository) Get(ctx context.Context, code string) (campaign.Campaign, []campaign.Step, error) {
	return r.get(ctx, code, false)
}
func (r *Repository) Lock(ctx context.Context, code string) (campaign.Campaign, []campaign.Step, error) {
	return r.get(ctx, code, true)
}
func (r *Repository) get(ctx context.Context, code string, lock bool) (campaign.Campaign, []campaign.Step, error) {
	tx, e := r.transaction(ctx)
	if e != nil {
		return campaign.Campaign{}, nil, e
	}
	sql := `SELECT campaign_code,name,approval_status,runtime_status,version,created_by,updated_by,created_at,updated_at FROM cloud_campaigns WHERE campaign_code=$1`
	if lock {
		sql += ` FOR UPDATE`
	}
	c, e := scanCampaign(tx.QueryRow(ctx, sql, code))
	if errors.Is(e, pgx.ErrNoRows) {
		return campaign.Campaign{}, nil, campaign.ErrNotFound
	}
	if e != nil {
		return campaign.Campaign{}, nil, e
	}
	rows, e := tx.Query(ctx, `SELECT step_index,delay_minutes,content FROM cloud_campaign_steps WHERE campaign_code=$1 ORDER BY step_index`, code)
	if e != nil {
		return campaign.Campaign{}, nil, e
	}
	defer rows.Close()
	steps := []campaign.Step{}
	for rows.Next() {
		var s campaign.Step
		e = rows.Scan(&s.Index, &s.DelayMinutes, &s.Content)
		if e != nil {
			return campaign.Campaign{}, nil, e
		}
		steps = append(steps, s)
	}
	return c, steps, rows.Err()
}
func (r *Repository) Save(ctx context.Context, c campaign.Campaign, steps []campaign.Step) error {
	tx, e := r.transaction(ctx)
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE cloud_campaigns SET approval_status=$2,runtime_status=$3,version=$4,updated_by=$5,updated_at=$6 WHERE campaign_code=$1 AND version=$7`, c.Code, c.ApprovalStatus, c.RuntimeStatus, c.Version, c.UpdatedBy, c.UpdatedAt, c.Version-1)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return campaign.ErrConflict
	}
	if _, e = tx.Exec(ctx, `DELETE FROM cloud_campaign_steps WHERE campaign_code=$1`, c.Code); e != nil {
		return e
	}
	for _, s := range steps {
		if _, e = tx.Exec(ctx, `INSERT INTO cloud_campaign_steps(campaign_code,step_index,delay_minutes,content) VALUES($1,$2,$3,$4)`, c.Code, s.Index, s.DelayMinutes, s.Content); e != nil {
			return e
		}
	}
	return nil
}
func (r *Repository) Delete(ctx context.Context, code string, version int64) error {
	tx, e := r.transaction(ctx)
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `DELETE FROM cloud_campaigns WHERE campaign_code=$1 AND version=$2 AND approval_status='draft' AND runtime_status='idle'`, code, version)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return campaign.ErrConflict
	}
	return nil
}
func (r *Repository) CreateLocalPlanAndCommand(ctx context.Context, c campaign.Campaign, count int32, op campaign.CommandOperation, now time.Time) (campaign.Plan, campaign.Command, error) {
	tx, e := r.transaction(ctx)
	if e != nil {
		return campaign.Plan{}, campaign.Command{}, e
	}
	p := campaign.Plan{CampaignCode: c.Code, CampaignVersion: c.Version, StepCount: count, CreatedAt: now}
	e = tx.QueryRow(ctx, `INSERT INTO cloud_campaign_local_plans(campaign_code,campaign_version,step_count,created_at) VALUES($1,$2,$3,$4) RETURNING id`, p.CampaignCode, p.CampaignVersion, p.StepCount, p.CreatedAt).Scan(&p.ID)
	if e != nil {
		return campaign.Plan{}, campaign.Command{}, e
	}
	cmd := campaign.Command{Operation: op, CampaignCode: c.Code, PlanID: p.ID, RealSend: false, RuntimeExecuted: false, CreatedAt: now}
	e = tx.QueryRow(ctx, `INSERT INTO cloud_campaign_local_commands(operation,campaign_code,plan_id,real_send,runtime_executed,created_at) VALUES($1,$2,$3,FALSE,FALSE,$4) RETURNING id`, cmd.Operation, cmd.CampaignCode, cmd.PlanID, cmd.CreatedAt).Scan(&cmd.ID)
	return p, cmd, e
}
func (r *Repository) ReserveIdempotency(ctx context.Context, x campaign.Reservation) (campaign.IdempotencyRecord, bool, error) {
	tx, e := r.transaction(ctx)
	if e != nil {
		return campaign.IdempotencyRecord{}, false, e
	}
	var id int64
	e = tx.QueryRow(ctx, `INSERT INTO cloud_campaign_operation_receipts(actor_id,key_digest,operation,payload_digest,state,created_at) VALUES($1,$2,$3,$4,'reserved',$5) ON CONFLICT(actor_id,key_digest) DO NOTHING RETURNING id`, x.ActorID, x.KeyDigest[:], x.Operation, x.PayloadDigest[:], x.CreatedAt).Scan(&id)
	if e == nil {
		return campaign.IdempotencyRecord{ID: id, ActorID: x.ActorID, Operation: x.Operation, KeyDigest: x.KeyDigest, PayloadDigest: x.PayloadDigest, State: campaign.IdempotencyReserved}, false, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return campaign.IdempotencyRecord{}, false, e
	}
	var op, state string
	var payload, result []byte
	e = tx.QueryRow(ctx, `SELECT id,operation,payload_digest,state,result_json FROM cloud_campaign_operation_receipts WHERE actor_id=$1 AND key_digest=$2 FOR UPDATE`, x.ActorID, x.KeyDigest[:]).Scan(&id, &op, &payload, &state, &result)
	if e != nil {
		return campaign.IdempotencyRecord{}, false, e
	}
	if op != x.Operation || subtle.ConstantTimeCompare(payload, x.PayloadDigest[:]) != 1 {
		return campaign.IdempotencyRecord{}, false, campaign.ErrIdempotencyConflict
	}
	record := campaign.IdempotencyRecord{ID: id, ActorID: x.ActorID, Operation: op, KeyDigest: x.KeyDigest, PayloadDigest: x.PayloadDigest, State: campaign.IdempotencyState(state)}
	if state != "completed" || len(result) == 0 {
		return record, false, campaign.ErrUnavailable
	}
	var value campaign.OperationResult
	if json.Unmarshal(result, &value) != nil {
		return record, false, campaign.ErrUnavailable
	}
	record.Result = &value
	return record, true, nil
}
func (r *Repository) CompleteIdempotency(ctx context.Context, id int64, result campaign.OperationResult) error {
	tx, e := r.transaction(ctx)
	if e != nil {
		return e
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return campaign.ErrUnavailable
	}
	tag, e := tx.Exec(ctx, `UPDATE cloud_campaign_operation_receipts SET state='completed',result_json=$2,completed_at=now() WHERE id=$1 AND state='reserved'`, id, raw)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return campaign.ErrUnavailable
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanCampaign(s scanner) (campaign.Campaign, error) {
	var c campaign.Campaign
	var approval, runtime string
	e := s.Scan(&c.Code, &c.Name, &approval, &runtime, &c.Version, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt)
	c.ApprovalStatus = campaign.ApprovalStatus(approval)
	c.RuntimeStatus = campaign.RuntimeStatus(runtime)
	return c, e
}

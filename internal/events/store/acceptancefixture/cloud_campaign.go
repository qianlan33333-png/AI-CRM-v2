package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppendCloudCampaignTouchPlanCreated records the Campaign fact in the
// transaction that owns the plan and receipt acceptance scenario.
func AppendCloudCampaignTouchPlanCreated(
	ctx context.Context,
	tx pgx.Tx,
	planID, campaignCode string,
	targetCount int32,
) (int64, error) {
	if tx == nil || planID == "" || campaignCode == "" || targetCount < 0 {
		return 0, fmt.Errorf("valid cloud campaign fact fixture required")
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
VALUES (
  'cloud_campaign.fact_recorded',
  jsonb_build_object(
    'audit_type', 'touch_plan_created',
    'plan_id', $1::text,
    'campaign_code', $2::text,
    'owner_actor_id', 1,
    'target_digest', repeat('02', 32),
    'target_count', $3::integer,
    'content_digest', repeat('03', 32)
  ),
  now(),
  'campaign-initiation-acceptance:' || $1::text
)
RETURNING id`, planID, campaignCode, targetCount).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("append cloud campaign acceptance fact: %w", err)
	}
	return eventID, nil
}

// AppendCloudCampaignTouchPlanSubmitted records a valid Campaign review fact
// in the transaction that owns the tampered receipt snapshot scenario.
func AppendCloudCampaignTouchPlanSubmitted(
	ctx context.Context,
	tx pgx.Tx,
	planID, campaignCode string,
	reviewVersion, actorID int64,
	idempotencyKey string,
) (int64, error) {
	if tx == nil || planID == "" || campaignCode == "" || reviewVersion != 2 || actorID < 1 || idempotencyKey == "" {
		return 0, fmt.Errorf("valid cloud campaign review fact fixture required")
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
VALUES (
  'cloud_campaign.fact_recorded',
  jsonb_build_object(
    'audit_type', 'touch_plan_submitted',
    'plan_id', $1::text,
    'campaign_code', $2::text,
    'review_version', $3::bigint,
    'actor_id', $4::bigint
  ),
  now(),
  $5::text
)
RETURNING id`, planID, campaignCode, reviewVersion, actorID, idempotencyKey).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("append cloud campaign review acceptance fact: %w", err)
	}
	return eventID, nil
}

// DeleteCloudCampaignFacts removes only Campaign fact events created by the
// acceptance scenarios, deleting their delivery children first.
func DeleteCloudCampaignFacts(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("valid cloud campaign fact fixture required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cloud campaign fact cleanup: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- rollback is fixture cleanup on failure.
	if _, err = tx.Exec(ctx, `
DELETE FROM event_deliveries
WHERE event_id IN (
  SELECT id
  FROM event_log
  WHERE event_type = 'cloud_campaign.fact_recorded'
    AND payload ->> 'audit_type' IN ('touch_plan_created', 'touch_plan_submitted', 'approved', 'rejected', 'handoff_created')
)`); err != nil {
		return fmt.Errorf("delete cloud campaign acceptance fact deliveries: %w", err)
	}
	if _, err = tx.Exec(ctx, `
DELETE FROM event_log
WHERE event_type = 'cloud_campaign.fact_recorded'
  AND payload ->> 'audit_type' IN ('touch_plan_created', 'touch_plan_submitted', 'approved', 'rejected', 'handoff_created')`); err != nil {
		return fmt.Errorf("delete cloud campaign acceptance facts: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cloud campaign fact cleanup: %w", err)
	}
	return nil
}

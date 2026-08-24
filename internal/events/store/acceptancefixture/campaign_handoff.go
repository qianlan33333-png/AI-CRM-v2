package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const appendCampaignHandoffAcceptedFact = `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
SELECT
  'outbound.campaign_handoff_fact_recorded',
  jsonb_build_object(
    'audit_type', 'accepted', 'handoff_id', handoff.id, 'campaign_code', handoff.campaign_code, 'plan_id', handoff.plan_id,
    'review_version', handoff.review_version, 'target_digest', encode(handoff.target_digest, 'hex'), 'content_digest', encode(handoff.content_digest, 'hex'),
    'target_count', handoff.target_count, 'step_count', handoff.step_count, 'actor_id', handoff.accepted_by_actor_id,
    'local_only', handoff.local_only, 'provider_execution_eligible', handoff.provider_execution_eligible,
    'real_external_call_executed', handoff.real_external_call_executed, 'delivery_proven', handoff.delivery_proven
  ),
  now(),
  'outbound-campaign-handoff-tamper:' || handoff.plan_id
FROM public.outbound_campaign_handoffs AS handoff
WHERE handoff.id = $1::bigint AND handoff.plan_id = $2::text
RETURNING id`

const appendCampaignHandoffAcceptedFactWithForbiddenExtraKey = `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
SELECT
  'outbound.campaign_handoff_fact_recorded',
  jsonb_build_object(
    'audit_type', 'accepted', 'handoff_id', handoff.id, 'campaign_code', handoff.campaign_code, 'plan_id', handoff.plan_id,
    'review_version', handoff.review_version, 'target_digest', encode(handoff.target_digest, 'hex'), 'content_digest', encode(handoff.content_digest, 'hex'),
    'target_count', handoff.target_count, 'step_count', handoff.step_count, 'actor_id', handoff.accepted_by_actor_id,
    'local_only', handoff.local_only, 'provider_execution_eligible', handoff.provider_execution_eligible,
    'real_external_call_executed', handoff.real_external_call_executed, 'delivery_proven', handoff.delivery_proven
  ) || jsonb_build_object('provider_message_id', 'forbidden'),
  now(),
  'outbound-campaign-handoff-tamper:' || handoff.plan_id
FROM public.outbound_campaign_handoffs AS handoff
WHERE handoff.id = $1::bigint AND handoff.plan_id = $2::text
RETURNING id`

// AppendCampaignHandoffAcceptedFact records the exact accepted fact for one
// Campaign handoff owned by the caller's transaction.
func AppendCampaignHandoffAcceptedFact(ctx context.Context, tx pgx.Tx, handoffID int64, planID string) (int64, error) {
	return appendCampaignHandoffFact(ctx, tx, handoffID, planID, appendCampaignHandoffAcceptedFact)
}

// AppendCampaignHandoffAcceptedFactWithForbiddenExtraKey records the invalid
// payload used only by the migration tamper scenario.
func AppendCampaignHandoffAcceptedFactWithForbiddenExtraKey(ctx context.Context, tx pgx.Tx, handoffID int64, planID string) (int64, error) {
	return appendCampaignHandoffFact(ctx, tx, handoffID, planID, appendCampaignHandoffAcceptedFactWithForbiddenExtraKey)
}

// DeleteCampaignHandoffFacts removes only event IDs explicitly created by a
// Campaign handoff acceptance test. Delivery children are deleted first.
func DeleteCampaignHandoffFacts(ctx context.Context, pool *pgxpool.Pool, eventIDs []int64) error {
	if pool == nil {
		return fmt.Errorf("valid Campaign handoff Events fixture pool required")
	}
	if len(eventIDs) == 0 {
		return nil
	}
	if err := validCampaignHandoffEventIDs(eventIDs); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Campaign handoff Events fixture cleanup: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck -- rollback is fixture cleanup on failure.

	var deliveryCount int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs).Scan(&deliveryCount); err != nil {
		return fmt.Errorf("count Campaign handoff event deliveries: %w", err)
	}
	deliveries, err := tx.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs)
	if err != nil {
		return fmt.Errorf("delete Campaign handoff event deliveries: %w", err)
	}
	if deliveries.RowsAffected() != deliveryCount {
		return fmt.Errorf("delete Campaign handoff event deliveries: deleted %d rows, want %d", deliveries.RowsAffected(), deliveryCount)
	}
	events, err := tx.Exec(ctx, `DELETE FROM event_log WHERE id = ANY($1::bigint[])`, eventIDs)
	if err != nil {
		return fmt.Errorf("delete Campaign handoff events: %w", err)
	}
	if events.RowsAffected() != int64(len(eventIDs)) {
		return fmt.Errorf("delete Campaign handoff events: deleted %d rows, want %d", events.RowsAffected(), len(eventIDs))
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Campaign handoff Events fixture cleanup: %w", err)
	}
	return nil
}

func appendCampaignHandoffFact(ctx context.Context, tx pgx.Tx, handoffID int64, planID, statement string) (int64, error) {
	if tx == nil || handoffID < 1 || planID == "" {
		return 0, fmt.Errorf("valid Campaign handoff Events fact fixture required")
	}
	var eventID int64
	if err := tx.QueryRow(ctx, statement, handoffID, planID).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("append Campaign handoff Events fact: %w", err)
	}
	return eventID, nil
}

func validCampaignHandoffEventIDs(eventIDs []int64) error {
	seen := make(map[int64]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID < 1 {
			return fmt.Errorf("Campaign handoff event ID must be positive")
		}
		if _, exists := seen[eventID]; exists {
			return fmt.Errorf("Campaign handoff event IDs must be unique")
		}
		seen[eventID] = struct{}{}
	}
	return nil
}

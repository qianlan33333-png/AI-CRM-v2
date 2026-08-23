// Package acceptancefixture creates Events-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// CreateEvent persists one Events-owned fact inside the caller's acceptance
// transaction.
func CreateEvent(ctx context.Context, tx pgx.Tx, event eventport.Event) (int64, error) {
	if tx == nil || event.Type == "" || event.IdempotencyKey == "" || event.OccurredAt.IsZero() || !json.Valid(event.Payload) {
		return 0, fmt.Errorf("valid event fixture required")
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
VALUES ($1::text, $2::jsonb, $3::timestamptz, $4::text)
RETURNING id`, event.Type, event.Payload, event.OccurredAt.UTC(), event.IdempotencyKey).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("create Events-owned acceptance fact: %w", err)
	}
	return eventID, nil
}

func CreateCompletedDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "completed", at)
}

func CreateOutcomeUnknownDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "outcome_unknown", at)
}

// CreateCampaignTouchPlanCreatedFact creates the Events-owned audit fact used
// to satisfy Campaign receipt invariants in migration acceptance scenarios.
func CreateCampaignTouchPlanCreatedFact(ctx context.Context, tx pgx.Tx, planID, campaignCode string, targetCount int32) (int64, error) {
	if tx == nil || planID == "" || campaignCode == "" || targetCount <= 0 {
		return 0, fmt.Errorf("valid Campaign event fixture fields required")
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
		return 0, fmt.Errorf("create Campaign touch-plan event fixture: %w", err)
	}
	return eventID, nil
}

// DeleteEvents removes Events-owned delivery and log fixtures in FK order.
func DeleteEvents(ctx context.Context, db executor, eventIDs []int64) error {
	if db == nil || len(eventIDs) == 0 {
		return fmt.Errorf("valid events fixture transaction and event ids required")
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs); err != nil {
		return fmt.Errorf("delete event delivery fixtures: %w", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_log WHERE id = ANY($1::bigint[])`, eventIDs); err != nil {
		return fmt.Errorf("delete event log fixtures: %w", err)
	}
	return nil
}

// DeleteEventsByTypeAndAuditTypes removes Events-owned facts selected by the
// bounded event type and local audit-type payload values.
func DeleteEventsByTypeAndAuditTypes(ctx context.Context, db executor, eventType string, auditTypes []string) error {
	if db == nil || eventType == "" || len(auditTypes) == 0 {
		return fmt.Errorf("valid events fixture transaction, type, and audit types required")
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id IN (
  SELECT id FROM event_log WHERE event_type = $1::text AND payload ->> 'audit_type' = ANY($2::text[])
)`, eventType, auditTypes); err != nil {
		return fmt.Errorf("delete event delivery fixtures by type: %w", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_log WHERE event_type = $1::text AND payload ->> 'audit_type' = ANY($2::text[])`, eventType, auditTypes); err != nil {
		return fmt.Errorf("delete event log fixtures by type: %w", err)
	}
	return nil
}

// DeleteEventsByType removes all Events-owned delivery and log fixtures for a
// bounded event type.
func DeleteEventsByType(ctx context.Context, db executor, eventType string) error {
	if db == nil || eventType == "" {
		return fmt.Errorf("valid events fixture transaction and type required")
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id IN (SELECT id FROM event_log WHERE event_type = $1::text)`, eventType); err != nil {
		return fmt.Errorf("delete event delivery fixtures by type: %w", err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM event_log WHERE event_type = $1::text`, eventType); err != nil {
		return fmt.Errorf("delete event log fixtures by type: %w", err)
	}
	return nil
}

func createDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey, status string, at time.Time) (int64, error) {
	if tx == nil || consumer == "" || idempotencyKey == "" {
		return 0, fmt.Errorf("valid events fixture transaction, consumer, and idempotency key required")
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
VALUES ('delivery.lineage.acceptance', jsonb_build_object('marker', $1::text), $2::timestamptz, $3::text)
RETURNING id`, payloadMarker, at.UTC(), idempotencyKey).Scan(&eventID); err != nil {
		return 0, fmt.Errorf("create event log: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO event_deliveries (event_id, consumer, status, attempt_count, completed_at, updated_at)
VALUES ($1::bigint, $2::text, $3::text, 1, $4::timestamptz, $4::timestamptz)`, eventID, consumer, status, at.UTC()); err != nil {
		return 0, fmt.Errorf("create event delivery: %w", err)
	}
	return eventID, nil
}

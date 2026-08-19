// Package acceptancefixture creates Events-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func CreateCompletedDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "completed", at)
}

func CreateOutcomeUnknownDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "outcome_unknown", at)
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

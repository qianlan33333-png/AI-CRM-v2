// Package acceptancefixture creates Events-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func CreateCompletedDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "completed", at)
}

func CreateOutcomeUnknownDelivery(ctx context.Context, tx pgx.Tx, consumer, payloadMarker, idempotencyKey string, at time.Time) (int64, error) {
	return createDelivery(ctx, tx, consumer, payloadMarker, idempotencyKey, "outcome_unknown", at)
}

func DeleteDeliveryLineageEvents(ctx context.Context, pool *pgxpool.Pool, eventIDs []int64) error {
	if pool == nil || len(eventIDs) == 0 {
		return fmt.Errorf("valid events fixture pool and event IDs required")
	}
	seen := make(map[int64]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID < 1 {
			return fmt.Errorf("event ID must be positive")
		}
		if _, ok := seen[eventID]; ok {
			return fmt.Errorf("event IDs must be unique")
		}
		seen[eventID] = struct{}{}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin events fixture cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	deliveryResult, err := tx.Exec(ctx, `DELETE FROM event_deliveries WHERE event_id = ANY($1::bigint[])`, eventIDs)
	if err != nil {
		return fmt.Errorf("delete event deliveries: %w", err)
	}
	if deliveryResult.RowsAffected() != int64(len(eventIDs)) {
		return fmt.Errorf("delete event deliveries: deleted %d rows, want %d", deliveryResult.RowsAffected(), len(eventIDs))
	}
	eventResult, err := tx.Exec(ctx, `DELETE FROM event_log WHERE id = ANY($1::bigint[])`, eventIDs)
	if err != nil {
		return fmt.Errorf("delete event log: %w", err)
	}
	if eventResult.RowsAffected() != int64(len(eventIDs)) {
		return fmt.Errorf("delete event log: deleted %d rows, want %d", eventResult.RowsAffected(), len(eventIDs))
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit events fixture cleanup: %w", err)
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

// Package acceptancefixture creates Outbound-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func CreateSentTask(ctx context.Context, tx pgx.Tx, customerID, riverJobID int64, providerMessageID string, at time.Time) (int64, error) {
	taskID, err := createTask(ctx, tx, customerID, at)
	if err != nil {
		return 0, err
	}
	var attemptID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind, state, provider_message_id, dispatch_started_at, completed_at)
VALUES ($1::bigint, $2::bigint, 'outbound_enqueue_one', 'succeeded', $3::text, $4::timestamptz, $4::timestamptz)
RETURNING id`, riverJobID, taskID, providerMessageID, at.UTC()).Scan(&attemptID); err != nil {
		return 0, fmt.Errorf("create sent outbound attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE outbound_tasks
SET status='sent', attempt_count=1, current_attempt_id=$1::bigint, provider_message_id=$2::text,
    sent_at=$3::timestamptz, status_updated_at=$3::timestamptz
WHERE id=$4::bigint`, attemptID, providerMessageID, at.UTC(), taskID); err != nil {
		return 0, fmt.Errorf("mark outbound task sent: %w", err)
	}
	return taskID, nil
}

func CreateOutcomeUnknownTask(ctx context.Context, tx pgx.Tx, customerID, riverJobID int64, providerCode, errorText string, at time.Time) (int64, error) {
	taskID, err := createTask(ctx, tx, customerID, at)
	if err != nil {
		return 0, err
	}
	var attemptID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO outbound_send_attempts (river_job_id, task_id, job_kind, state, failure_kind, provider_code, dispatch_started_at, completed_at)
VALUES ($1::bigint, $2::bigint, 'outbound_enqueue_one', 'outcome_unknown', 'timeout', $3::text, $4::timestamptz, $4::timestamptz)
RETURNING id`, riverJobID, taskID, providerCode, at.UTC()).Scan(&attemptID); err != nil {
		return 0, fmt.Errorf("create unknown outbound attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE outbound_tasks
SET status='outcome_unknown', attempt_count=1, current_attempt_id=$1::bigint, last_failure_kind='timeout',
    last_error=$2::text, status_updated_at=$3::timestamptz
WHERE id=$4::bigint`, attemptID, errorText, at.UTC(), taskID); err != nil {
		return 0, fmt.Errorf("mark outbound task outcome unknown: %w", err)
	}
	return taskID, nil
}

func createTask(ctx context.Context, tx pgx.Tx, customerID int64, at time.Time) (int64, error) {
	if tx == nil || customerID <= 0 {
		return 0, fmt.Errorf("valid outbound fixture transaction and customer required")
	}
	var taskID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO outbound_tasks (customer_id, template_key, payload, status, attempt_count, status_updated_at)
VALUES ($1::bigint, 'text.notice.v1', '{}'::jsonb, 'pending', 0, $2::timestamptz)
RETURNING id`, customerID, at.UTC()).Scan(&taskID); err != nil {
		return 0, fmt.Errorf("create outbound task: %w", err)
	}
	return taskID, nil
}

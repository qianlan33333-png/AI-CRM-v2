-- +goose Up
ALTER TABLE outbound_control_receipts
  DROP CONSTRAINT outbound_control_receipts_operation,
  DROP CONSTRAINT outbound_control_receipts_result,
  ADD CONSTRAINT outbound_control_receipts_operation CHECK (
    operation IN ('cancel', 'manual_retry')
  ),
  ADD CONSTRAINT outbound_control_receipts_result CHECK (
    (state = 'reserved' AND customer_id IS NULL AND job_generation IS NULL
      AND river_job_id IS NULL AND job_kind IS NULL AND event_id IS NULL
      AND task_status IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND customer_id IS NOT NULL AND job_generation > 0
      AND river_job_id > 0 AND job_kind IS NOT NULL AND event_id IS NOT NULL
      AND completed_at IS NOT NULL
      AND (
        (operation = 'cancel' AND task_status = 'cancelled')
        OR (operation = 'manual_retry' AND task_status = 'pending')
      ))
  );

-- +goose Down
-- O6B2 control receipts are immutable business facts. The expanded constraint
-- must survive application rollback so manual-retry history remains readable.
SELECT 1;

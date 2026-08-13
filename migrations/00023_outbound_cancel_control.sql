-- +goose Up
CREATE TABLE IF NOT EXISTS outbound_task_job_links (
  task_id       BIGINT NOT NULL REFERENCES outbound_tasks(id),
  generation    INTEGER NOT NULL,
  river_job_id  BIGINT NOT NULL,
  job_kind      TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  cancelled_at  TIMESTAMPTZ,
  CONSTRAINT outbound_task_job_links_generation CHECK (generation > 0),
  CONSTRAINT outbound_task_job_links_river_job CHECK (river_job_id > 0),
  CONSTRAINT outbound_task_job_links_job_kind CHECK (
    job_kind IN ('outbound_enqueue_one', 'outbound_enqueue_batch_task')
  ),
  CONSTRAINT outbound_task_job_links_cancelled CHECK (
    cancelled_at IS NULL OR cancelled_at >= created_at
  ),
  CONSTRAINT outbound_task_job_links_task_generation_unique UNIQUE (task_id, generation),
  CONSTRAINT outbound_task_job_links_river_job_unique UNIQUE (river_job_id)
);

CREATE INDEX IF NOT EXISTS outbound_task_job_links_task_latest_idx
  ON outbound_task_job_links (task_id, generation DESC);

CREATE TABLE IF NOT EXISTS outbound_control_receipts (
  id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  idempotency_scope  TEXT NOT NULL,
  idempotency_key    TEXT NOT NULL,
  operation          TEXT NOT NULL DEFAULT 'cancel',
  task_id            BIGINT NOT NULL REFERENCES outbound_tasks(id),
  state              TEXT NOT NULL DEFAULT 'reserved',
  customer_id        BIGINT REFERENCES customers(id),
  job_generation     INTEGER,
  river_job_id       BIGINT,
  job_kind           TEXT,
  event_id           BIGINT REFERENCES event_log(id),
  task_status        TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at       TIMESTAMPTZ,
  CONSTRAINT outbound_control_receipts_scope CHECK (
    btrim(idempotency_scope) = idempotency_scope
    AND idempotency_scope <> ''
    AND char_length(idempotency_scope) <= 200
  ),
  CONSTRAINT outbound_control_receipts_key CHECK (
    btrim(idempotency_key) = idempotency_key
    AND char_length(idempotency_key) BETWEEN 16 AND 128
  ),
  CONSTRAINT outbound_control_receipts_operation CHECK (operation = 'cancel'),
  CONSTRAINT outbound_control_receipts_state CHECK (state IN ('reserved', 'completed')),
  CONSTRAINT outbound_control_receipts_job_kind CHECK (
    job_kind IS NULL OR job_kind IN ('outbound_enqueue_one', 'outbound_enqueue_batch_task')
  ),
  CONSTRAINT outbound_control_receipts_result CHECK (
    (state = 'reserved' AND customer_id IS NULL AND job_generation IS NULL
      AND river_job_id IS NULL AND job_kind IS NULL AND event_id IS NULL
      AND task_status IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND customer_id IS NOT NULL AND job_generation > 0
      AND river_job_id > 0 AND job_kind IS NOT NULL AND event_id IS NOT NULL
      AND task_status = 'cancelled' AND completed_at IS NOT NULL)
  ),
  CONSTRAINT outbound_control_receipts_idempotency_unique UNIQUE (idempotency_scope, idempotency_key)
);

WITH known_task_jobs AS (
  SELECT receipt.task_id, receipt.river_job_id, 'outbound_enqueue_one'::text AS job_kind
  FROM outbound_enqueue_receipts AS receipt
  WHERE receipt.state = 'accepted' AND receipt.task_id IS NOT NULL AND receipt.river_job_id IS NOT NULL
  UNION
  SELECT attempt.task_id, attempt.river_job_id, attempt.job_kind
  FROM outbound_send_attempts AS attempt
)
INSERT INTO outbound_task_job_links (task_id, generation, river_job_id, job_kind)
SELECT known.task_id, 1, known.river_job_id, known.job_kind
FROM known_task_jobs AS known
ON CONFLICT DO NOTHING;

-- +goose Down
-- O6B1 is an expand/contract compatibility migration. Control receipts and
-- task/job links must survive an application rollback and a later re-upgrade.
SELECT 1;

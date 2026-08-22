-- AI-CRM-v2 CRM-local AI Audience lifecycle and idempotency metadata.
--
-- The tables below store only CRM-local AI Audience grouping, lifecycle and
-- idempotency metadata. public.segments remains authoritative for definitions,
-- refresh configuration/status and member_count; public.segment_members remains
-- authoritative for member snapshots.

-- +goose Up
CREATE TABLE public.ai_audience_package_groups (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  version    BIGINT NOT NULL DEFAULT 1,
  created_by BIGINT NOT NULL REFERENCES public.admin_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_audience_package_groups_name CHECK (
    btrim(name) = name AND name <> '' AND char_length(name) <= 100
  ),
  CONSTRAINT ai_audience_package_groups_sort CHECK (
    sort_order BETWEEN 0 AND 1000000
  ),
  CONSTRAINT ai_audience_package_groups_version CHECK (version >= 1),
  CONSTRAINT ai_audience_package_groups_timestamps CHECK (created_at <= updated_at)
);

-- Group names are unique case-insensitively after the required trim check.
CREATE UNIQUE INDEX uq_ai_audience_package_groups_name_ci
  ON public.ai_audience_package_groups (lower(name));

CREATE INDEX idx_ai_audience_package_groups_sort
  ON public.ai_audience_package_groups (sort_order ASC, id ASC);

CREATE TABLE public.ai_audience_package_metadata (
  segment_id BIGINT PRIMARY KEY REFERENCES public.segments(id) ON DELETE CASCADE,
  group_id   BIGINT REFERENCES public.ai_audience_package_groups(id) ON DELETE RESTRICT,
  lifecycle TEXT NOT NULL DEFAULT 'active',
  version    BIGINT NOT NULL DEFAULT 1,
  -- Zero is reserved only for backfilled historical rows whose Segment actor
  -- is unknown. All new HTTP writes require an authenticated positive actor.
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT ai_audience_package_metadata_lifecycle CHECK (
    lifecycle IN ('paused', 'active', 'archived')
  ),
  CONSTRAINT ai_audience_package_metadata_version CHECK (version >= 1),
  CONSTRAINT ai_audience_package_metadata_actors CHECK (
    created_by >= 0 AND updated_by >= 0
  ),
  CONSTRAINT ai_audience_package_metadata_timestamps CHECK (created_at <= updated_at)
);

CREATE INDEX idx_ai_audience_package_metadata_group_page
  ON public.ai_audience_package_metadata (group_id, segment_id ASC);

CREATE INDEX idx_ai_audience_package_metadata_lifecycle_page
  ON public.ai_audience_package_metadata (lifecycle, segment_id ASC);

-- Existing Segment rows become local packages. There is no member copy,
-- member cache, refresh request or provider effect in this backfill.
INSERT INTO public.ai_audience_package_metadata (
  segment_id,
  group_id,
  lifecycle,
  version,
  created_by,
  updated_by,
  created_at,
  updated_at
)
SELECT
  s.id,
  NULL,
  CASE
    WHEN s.lifecycle_status = 'archived' THEN 'archived'
    ELSE 'active'
  END,
  1,
  COALESCE(s.created_by, 0),
  COALESCE(s.created_by, 0),
  s.created_at,
  s.updated_at
FROM public.segments AS s
ON CONFLICT (segment_id) DO NOTHING;

CREATE TABLE public.ai_audience_operation_receipts (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation      TEXT NOT NULL,
  actor_id       BIGINT NOT NULL,
  key_digest     BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state          TEXT NOT NULL DEFAULT 'in_progress',
  result_json    JSONB,
  created_at     TIMESTAMPTZ NOT NULL,
  completed_at   TIMESTAMPTZ,
  CONSTRAINT ai_audience_operation_receipts_operation CHECK (
    operation IN (
      'group_create',
      'group_update',
      'group_delete',
      'package_update',
      'package_copy',
      'package_pause',
      'package_activate',
      'package_archive'
    )
  ),
  CONSTRAINT ai_audience_operation_receipts_actor CHECK (actor_id > 0),
  CONSTRAINT ai_audience_operation_receipts_key_digest CHECK (octet_length(key_digest) = 32),
  CONSTRAINT ai_audience_operation_receipts_payload_digest CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT ai_audience_operation_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT ai_audience_operation_receipts_result CHECK (
    result_json IS NULL OR jsonb_typeof(result_json) = 'object'
  ),
  CONSTRAINT ai_audience_operation_receipts_completion CHECK (
    (state = 'in_progress' AND result_json IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND result_json IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_id, key_digest)
);

-- A reserved receipt must complete in the same business transaction. This
-- prevents a committed half-write from blocking safe replay.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_receipt_complete_before_commit()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM public.ai_audience_operation_receipts
    WHERE id = NEW.id
      AND state = 'completed'
      AND result_json IS NOT NULL
      AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'AI Audience receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER ai_audience_operation_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.ai_audience_operation_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_receipt_complete_before_commit();

-- Completed receipts are immutable. The only allowed update is the transition
-- from in_progress to completed with identity and digest columns unchanged.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ai_audience_receipt_transition()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed AI Audience receipts are immutable'
      USING ERRCODE = '55000';
  END IF;

  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_json IS NULL
     OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid AI Audience receipt transition'
      USING ERRCODE = '55000';
  END IF;

  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER ai_audience_operation_receipts_transition
BEFORE UPDATE OR DELETE ON public.ai_audience_operation_receipts
FOR EACH ROW
EXECUTE FUNCTION public.aicrm_ai_audience_receipt_transition();

-- +goose Down
DROP TRIGGER ai_audience_operation_receipts_transition
  ON public.ai_audience_operation_receipts;
DROP TRIGGER ai_audience_operation_receipts_complete_before_commit
  ON public.ai_audience_operation_receipts;
DROP FUNCTION public.aicrm_ai_audience_receipt_transition();
DROP FUNCTION public.aicrm_ai_audience_receipt_complete_before_commit();
DROP TABLE public.ai_audience_operation_receipts;
DROP TABLE public.ai_audience_package_metadata;
DROP TABLE public.ai_audience_package_groups;

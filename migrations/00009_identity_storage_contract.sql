-- +goose Up
CREATE TABLE identities (
  id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id             BIGINT REFERENCES customers(id),
  kind                    TEXT NOT NULL,
  scope                   TEXT NOT NULL,
  normalized_value        TEXT NOT NULL,
  normalizer_version      SMALLINT NOT NULL,
  assurance               TEXT NOT NULL,
  source                  TEXT NOT NULL,
  review_fingerprint      BYTEA NOT NULL,
  fingerprint_key_version SMALLINT NOT NULL,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  bound_at                TIMESTAMPTZ,
  CONSTRAINT identities_kind CHECK (kind IN (
    'wecom_external_userid', 'unionid', 'mp_openid', 'oa_openid',
    'alipay_user_id', 'phone', 'ext'
  )),
  CONSTRAINT identities_scope_not_blank CHECK (btrim(scope) <> '' AND char_length(scope) <= 256),
  CONSTRAINT identities_normalized_not_blank CHECK (
    btrim(normalized_value) <> '' AND char_length(normalized_value) <= 1024
  ),
  CONSTRAINT identities_normalizer_version CHECK (normalizer_version > 0),
  CONSTRAINT identities_assurance CHECK (assurance IN ('verified', 'declared')),
  CONSTRAINT identities_source_not_blank CHECK (btrim(source) <> '' AND char_length(source) <= 200),
  CONSTRAINT identities_review_fingerprint_size CHECK (octet_length(review_fingerprint) = 16),
  CONSTRAINT identities_fingerprint_key_version CHECK (fingerprint_key_version > 0),
  CONSTRAINT identities_bound_consistency CHECK (
    (customer_id IS NULL AND bound_at IS NULL) OR
    (customer_id IS NOT NULL AND bound_at IS NOT NULL)
  ),
  UNIQUE (kind, scope, normalized_value)
);

CREATE INDEX idx_identities_customer
  ON identities (customer_id, id) WHERE customer_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_identity_merge_detail_valid(merge_detail JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
STRICT
SET search_path = pg_catalog
AS $$
  SELECT
    jsonb_typeof(merge_detail) = 'object'
    AND NOT EXISTS (
      SELECT 1
      FROM jsonb_object_keys(merge_detail) AS detail_key
      WHERE detail_key <> ALL (ARRAY['source_identity_ids', 'trigger_kind', 'assurance'])
    )
    AND (NOT merge_detail ? 'source_identity_ids' OR (
      jsonb_typeof(merge_detail -> 'source_identity_ids') = 'array'
      AND jsonb_array_length(merge_detail -> 'source_identity_ids') BETWEEN 1 AND 16
      AND NOT EXISTS (
        SELECT 1
        FROM jsonb_array_elements(merge_detail -> 'source_identity_ids') AS identity_id
        WHERE jsonb_typeof(identity_id) <> 'number' OR identity_id::text !~ '^[1-9][0-9]*$'
      )
    ))
    AND (NOT merge_detail ? 'trigger_kind' OR (
      jsonb_typeof(merge_detail -> 'trigger_kind') = 'string'
      AND merge_detail ->> 'trigger_kind' IN (
        'wecom_external_userid', 'unionid', 'mp_openid', 'oa_openid',
        'alipay_user_id', 'phone', 'ext'
      )
    ))
    AND (NOT merge_detail ? 'assurance' OR (
      jsonb_typeof(merge_detail -> 'assurance') = 'string'
      AND merge_detail ->> 'assurance' IN ('verified', 'declared')
    ));
$$;
-- +goose StatementEnd

CREATE TABLE customer_merges (
  id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  primary_customer_id     BIGINT NOT NULL REFERENCES customers(id),
  merged_customer_id      BIGINT NOT NULL REFERENCES customers(id),
  mode                    TEXT NOT NULL,
  policy_version          TEXT NOT NULL,
  review_fingerprint      BYTEA NOT NULL,
  fingerprint_key_version SMALLINT NOT NULL,
  operated_by             TEXT NOT NULL,
  detail                  JSONB NOT NULL DEFAULT '{}',
  merged_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT customer_merges_distinct CHECK (primary_customer_id <> merged_customer_id),
  CONSTRAINT customer_merges_mode CHECK (mode IN ('auto', 'manual')),
  CONSTRAINT customer_merges_policy CHECK (btrim(policy_version) <> '' AND char_length(policy_version) <= 200),
  CONSTRAINT customer_merges_review_fingerprint_size CHECK (octet_length(review_fingerprint) = 16),
  CONSTRAINT customer_merges_fingerprint_key_version CHECK (fingerprint_key_version > 0),
  CONSTRAINT customer_merges_operator CHECK (btrim(operated_by) <> '' AND char_length(operated_by) <= 200),
  CONSTRAINT customer_merges_detail_closed CHECK (
    public.aicrm_identity_merge_detail_valid(detail)
  ),
  UNIQUE (merged_customer_id)
);

CREATE INDEX idx_customer_merges_primary
  ON customer_merges (primary_customer_id, merged_at DESC, id DESC);

CREATE TABLE pending_events (
  id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind                    TEXT NOT NULL,
  state                   TEXT NOT NULL DEFAULT 'pending',
  identity_ids            BIGINT[] NOT NULL,
  candidate_customer_ids  BIGINT[] NOT NULL DEFAULT '{}',
  event_type              TEXT,
  payload                 JSONB NOT NULL DEFAULT '{}',
  source                  TEXT NOT NULL,
  occurred_at             TIMESTAMPTZ,
  review_fingerprint      BYTEA,
  fingerprint_key_version SMALLINT,
  policy_version          TEXT NOT NULL,
  version                 BIGINT NOT NULL DEFAULT 1,
  created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at             TIMESTAMPTZ,
  CONSTRAINT pending_events_kind CHECK (kind IN ('attribution', 'conflict', 'merge_review')),
  CONSTRAINT pending_events_state CHECK (state IN ('pending', 'approved', 'rejected', 'replayed', 'failed')),
  CONSTRAINT pending_events_identity_ids CHECK (
    cardinality(identity_ids) > 0 AND array_position(identity_ids, NULL) IS NULL
  ),
  CONSTRAINT pending_events_candidate_ids CHECK (array_position(candidate_customer_ids, NULL) IS NULL),
  CONSTRAINT pending_events_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT pending_events_payload_no_identity_keys CHECK (
    NOT jsonb_path_exists(
      payload,
      '$.**.keyvalue() ? (@.key == "raw_identity" || @.key == "raw_value" || @.key == "normalized_value")'
    )
  ),
  CONSTRAINT pending_events_source CHECK (btrim(source) <> '' AND char_length(source) <= 200),
  CONSTRAINT pending_events_policy CHECK (btrim(policy_version) <> '' AND char_length(policy_version) <= 200),
  CONSTRAINT pending_events_version CHECK (version > 0),
  CONSTRAINT pending_events_resolution CHECK (
    (state = 'pending' AND resolved_at IS NULL) OR
    (state <> 'pending' AND resolved_at IS NOT NULL)
  ),
  CONSTRAINT pending_events_review_shape CHECK (
    (kind = 'merge_review' AND event_type IS NULL AND occurred_at IS NULL AND
      cardinality(candidate_customer_ids) = 2 AND
      candidate_customer_ids[1] < candidate_customer_ids[2] AND
      review_fingerprint IS NOT NULL AND octet_length(review_fingerprint) = 16 AND
      fingerprint_key_version > 0) OR
    (kind <> 'merge_review' AND event_type IS NOT NULL AND btrim(event_type) <> '' AND
      occurred_at IS NOT NULL AND cardinality(candidate_customer_ids) <= 2 AND
      review_fingerprint IS NULL AND fingerprint_key_version IS NULL)
  )
);

CREATE UNIQUE INDEX idx_pending_merge_review_active
  ON pending_events (
    review_fingerprint,
    fingerprint_key_version,
    candidate_customer_ids
  ) WHERE kind = 'merge_review' AND state = 'pending';

CREATE INDEX idx_pending_events_replay
  ON pending_events (kind, id) WHERE state = 'pending' AND kind IN ('attribution', 'conflict');

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_identity_receipt_result_valid(
  receipt_operation TEXT,
  receipt_result JSONB
) RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
STRICT
SET search_path = pg_catalog
AS $$
  SELECT
    jsonb_typeof(receipt_result) = 'object'
    AND receipt_result ? 'status'
    AND jsonb_typeof(receipt_result -> 'status') = 'string'
    AND NOT EXISTS (
      SELECT 1
      FROM jsonb_object_keys(receipt_result) AS result_key
      WHERE result_key <> ALL (ARRAY[
        'status', 'customer_id', 'primary_customer_id', 'merge_audit_id',
        'review_id', 'pending_event_id', 'event_id', 'policy_version', 'completed_at'
      ])
    )
    AND CASE receipt_operation
      WHEN 'bind' THEN receipt_result ->> 'status' IN (
        'bound', 'already_bound', 'merged', 'manual_review', 'rejected'
      )
      WHEN 'ingest' THEN receipt_result ->> 'status' IN ('attributed', 'pending', 'conflict')
      WHEN 'merge_review_approve' THEN receipt_result ->> 'status' = 'approved'
      WHEN 'merge_review_reject' THEN receipt_result ->> 'status' = 'rejected'
      ELSE FALSE
    END
    AND NOT EXISTS (
      SELECT 1
      FROM jsonb_each(receipt_result) AS result_entry(key, value)
      WHERE key IN (
        'customer_id', 'primary_customer_id', 'merge_audit_id', 'review_id',
        'pending_event_id', 'event_id'
      ) AND (jsonb_typeof(value) <> 'number' OR value::text !~ '^[1-9][0-9]*$')
    )
    AND (NOT receipt_result ? 'policy_version' OR
      (jsonb_typeof(receipt_result -> 'policy_version') = 'string' AND
       btrim(receipt_result ->> 'policy_version') <> '' AND
       char_length(receipt_result ->> 'policy_version') <= 200))
    AND (NOT receipt_result ? 'completed_at' OR
      jsonb_typeof(receipt_result -> 'completed_at') = 'string');
$$;
-- +goose StatementEnd

CREATE TABLE identity_operation_receipts (
  id                       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation                TEXT NOT NULL,
  idempotency_scope        TEXT NOT NULL,
  key_digest               BYTEA NOT NULL,
  command_schema_version   SMALLINT NOT NULL,
  payload_hmac             BYTEA NOT NULL,
  payload_hmac_key_version SMALLINT NOT NULL,
  state                    TEXT NOT NULL DEFAULT 'in_progress',
  result_schema_version    SMALLINT NOT NULL,
  result                   JSONB,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at             TIMESTAMPTZ,
  CONSTRAINT identity_receipts_operation CHECK (operation IN (
    'bind', 'ingest', 'merge_review_approve', 'merge_review_reject'
  )),
  CONSTRAINT identity_receipts_scope CHECK (
    btrim(idempotency_scope) <> '' AND char_length(idempotency_scope) <= 256
  ),
  CONSTRAINT identity_receipts_key_digest_size CHECK (octet_length(key_digest) = 32),
  CONSTRAINT identity_receipts_command_schema_version CHECK (command_schema_version > 0),
  CONSTRAINT identity_receipts_payload_hmac_size CHECK (octet_length(payload_hmac) = 32),
  CONSTRAINT identity_receipts_payload_hmac_key_version CHECK (payload_hmac_key_version > 0),
  CONSTRAINT identity_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT identity_receipts_result_schema_version CHECK (result_schema_version > 0),
  CONSTRAINT identity_receipts_result_shape CHECK (
    (state = 'in_progress' AND result IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result IS NOT NULL AND jsonb_typeof(result) = 'object' AND
      octet_length(result::text) <= 16384 AND completed_at IS NOT NULL AND
      public.aicrm_identity_receipt_result_valid(operation, result))
  ),
  UNIQUE (operation, idempotency_scope, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_identity_receipt()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM public.identity_operation_receipts
    WHERE id = NEW.id AND state <> 'completed'
  ) THEN
    RAISE EXCEPTION 'identity operation receipt must complete in its reservation transaction'
      USING ERRCODE = '55000';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER identity_receipts_completed_before_commit
AFTER INSERT OR UPDATE ON identity_operation_receipts
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_identity_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_identity_receipt_transition_valid()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'identity operation receipts are immutable after completion'
      USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR
     NEW.operation <> OLD.operation OR
     NEW.idempotency_scope <> OLD.idempotency_scope OR
     NEW.key_digest <> OLD.key_digest OR
     NEW.command_schema_version <> OLD.command_schema_version OR
     NEW.payload_hmac <> OLD.payload_hmac OR
     NEW.payload_hmac_key_version <> OLD.payload_hmac_key_version OR
     NEW.result_schema_version <> OLD.result_schema_version OR
     NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'invalid identity operation receipt transition'
      USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER identity_receipts_transition
BEFORE UPDATE OR DELETE ON identity_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_identity_receipt_transition_valid();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_customer_merge_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  RAISE EXCEPTION 'customer_merges is append-only'
    USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER customer_merges_append_only
BEFORE UPDATE OR DELETE ON customer_merges
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_customer_merge_mutation();

-- +goose Down
DROP TABLE identity_operation_receipts;
DROP TABLE pending_events;
DROP TABLE customer_merges;
DROP TABLE identities;
DROP FUNCTION IF EXISTS public.aicrm_identity_merge_detail_valid(JSONB);
DROP FUNCTION IF EXISTS public.aicrm_reject_customer_merge_mutation();
DROP FUNCTION IF EXISTS public.aicrm_identity_receipt_transition_valid();
DROP FUNCTION IF EXISTS public.aicrm_reject_incomplete_identity_receipt();
DROP FUNCTION IF EXISTS public.aicrm_identity_receipt_result_valid(TEXT, JSONB);

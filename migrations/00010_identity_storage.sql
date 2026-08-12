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

CREATE TABLE customer_merges (
  id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  primary_customer_id     BIGINT NOT NULL REFERENCES customers(id),
  merged_customer_id      BIGINT NOT NULL REFERENCES customers(id),
  mode                    TEXT NOT NULL,
  policy_version          TEXT NOT NULL,
  review_fingerprint      BYTEA NOT NULL,
  fingerprint_key_version SMALLINT NOT NULL,
  operated_by             TEXT NOT NULL,
  detail                  JSONB NOT NULL,
  merged_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT customer_merges_distinct CHECK (primary_customer_id <> merged_customer_id),
  CONSTRAINT customer_merges_mode CHECK (mode IN ('auto', 'manual')),
  CONSTRAINT customer_merges_policy CHECK (btrim(policy_version) <> '' AND char_length(policy_version) <= 200),
  CONSTRAINT customer_merges_review_fingerprint_size CHECK (octet_length(review_fingerprint) = 16),
  CONSTRAINT customer_merges_fingerprint_key_version CHECK (fingerprint_key_version > 0),
  CONSTRAINT customer_merges_operator CHECK (btrim(operated_by) <> '' AND char_length(operated_by) <= 200),
  CONSTRAINT customer_merges_detail_closed CHECK (
    jsonb_typeof(detail) = 'object'
    AND detail ?& ARRAY['policy_version', 'mode', 'fingerprint_version', 'fingerprint']
    AND detail - ARRAY['policy_version', 'mode', 'fingerprint_version', 'fingerprint'] = '{}'::jsonb
    AND jsonb_typeof(detail -> 'policy_version') = 'string'
    AND jsonb_typeof(detail -> 'mode') = 'string'
    AND jsonb_typeof(detail -> 'fingerprint_version') = 'number'
    AND jsonb_typeof(detail -> 'fingerprint') = 'string'
    AND detail ->> 'policy_version' = policy_version
    AND detail ->> 'mode' = mode
    AND detail ->> 'fingerprint_version' = fingerprint_key_version::text
    AND detail ->> 'fingerprint' LIKE 'hmac-sha256-v%'
  ),
  UNIQUE (merged_customer_id)
);

CREATE TABLE pending_events (
  id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind                    TEXT NOT NULL,
  state                   TEXT NOT NULL DEFAULT 'pending',
  identity_ids            BIGINT[] NOT NULL,
  candidate_customer_ids  BIGINT[] NOT NULL DEFAULT '{}',
  event_type              TEXT,
  source                  TEXT NOT NULL,
  idempotency_key         TEXT,
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
  CONSTRAINT pending_events_source CHECK (btrim(source) <> '' AND char_length(source) <= 200),
  CONSTRAINT pending_events_policy CHECK (btrim(policy_version) <> '' AND char_length(policy_version) <= 200),
  CONSTRAINT pending_events_version CHECK (version > 0),
  CONSTRAINT pending_events_resolution CHECK (
    (state = 'pending' AND resolved_at IS NULL) OR
    (state <> 'pending' AND resolved_at IS NOT NULL)
  ),
  CONSTRAINT pending_events_kind_shape CHECK (
    (kind = 'merge_review' AND event_type IS NULL AND idempotency_key IS NULL AND occurred_at IS NULL
      AND cardinality(candidate_customer_ids) = 2
      AND candidate_customer_ids[1] < candidate_customer_ids[2]
      AND review_fingerprint IS NOT NULL AND octet_length(review_fingerprint) = 16
      AND fingerprint_key_version IS NOT NULL AND fingerprint_key_version > 0)
    OR
    (kind <> 'merge_review' AND event_type IS NOT NULL AND btrim(event_type) <> ''
      AND idempotency_key IS NOT NULL AND btrim(idempotency_key) <> ''
      AND occurred_at IS NOT NULL AND cardinality(candidate_customer_ids) <= 2
      AND review_fingerprint IS NULL AND fingerprint_key_version IS NULL)
  )
);

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
  result_status            TEXT,
  result_customer_id       BIGINT,
  result_merge_audit_id    BIGINT,
  result_pending_event_id  BIGINT,
  result_event_id          BIGINT,
  result_policy_version    TEXT,
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
  CONSTRAINT identity_receipts_result_status CHECK (result_status IS NULL OR result_status IN (
    'bound', 'already_bound', 'merged', 'manual_review', 'rejected',
    'attributed', 'pending', 'conflict', 'approved'
  )),
  CONSTRAINT identity_receipts_completed_result CHECK (
    (state = 'in_progress' AND result_status IS NULL AND result_customer_id IS NULL
      AND result_merge_audit_id IS NULL AND result_pending_event_id IS NULL AND result_event_id IS NULL
      AND result_policy_version IS NULL AND completed_at IS NULL)
    OR
    (state = 'completed' AND result_status IS NOT NULL AND completed_at IS NOT NULL
      AND (result_customer_id IS NULL OR result_customer_id > 0)
      AND (result_merge_audit_id IS NULL OR result_merge_audit_id > 0)
      AND (result_pending_event_id IS NULL OR result_pending_event_id > 0)
      AND (result_event_id IS NULL OR result_event_id > 0)
      AND (result_policy_version IS NULL OR (btrim(result_policy_version) <> '' AND char_length(result_policy_version) <= 200))
      AND (
        (operation = 'bind' AND result_status IN ('bound', 'already_bound', 'merged', 'manual_review', 'rejected'))
        OR (operation = 'ingest' AND result_status IN ('attributed', 'pending', 'conflict'))
        OR (operation = 'merge_review_approve' AND result_status = 'approved')
        OR (operation = 'merge_review_reject' AND result_status = 'rejected')
      ))
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
  IF NEW.state <> 'completed' THEN
    RAISE EXCEPTION 'identity operation receipt must complete in its reservation transaction'
      USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER identity_operation_receipts_complete_before_commit
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
  IF NEW.state <> 'completed'
     OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.idempotency_scope IS DISTINCT FROM OLD.idempotency_scope
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.command_schema_version IS DISTINCT FROM OLD.command_schema_version
     OR NEW.payload_hmac IS DISTINCT FROM OLD.payload_hmac
     OR NEW.payload_hmac_key_version IS DISTINCT FROM OLD.payload_hmac_key_version
     OR NEW.result_schema_version IS DISTINCT FROM OLD.result_schema_version
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid identity operation receipt transition'
      USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER identity_operation_receipts_transition
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
DROP FUNCTION IF EXISTS public.aicrm_reject_customer_merge_mutation();
DROP FUNCTION IF EXISTS public.aicrm_identity_receipt_transition_valid();
DROP FUNCTION IF EXISTS public.aicrm_reject_incomplete_identity_receipt();

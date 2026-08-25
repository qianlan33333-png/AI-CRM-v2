-- +goose Up
-- The external-effects runtime persists only opaque digests and local control
-- facts. It does not contain provider credentials, request bodies, or results.
CREATE TABLE external_effects (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  owner TEXT NOT NULL CHECK (owner IN ('campaign','contact','outbound','wecom','survey','audience','order')),
  kind TEXT NOT NULL CHECK (kind IN ('campaign_dispatch','campaign_group_announcement','contact_touch','outbound_message','outbound_media','wecom_tag_sync','wecom_profile_sync','survey_webhook','audience_webhook','order_payment_capture','order_refund')),
  source_ref_digest TEXT NOT NULL CHECK (source_ref_digest ~ '^sha256:[0-9a-f]{64}$'),
  target_ref_digest TEXT NOT NULL CHECK (target_ref_digest ~ '^sha256:[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  policy_version_hash TEXT NOT NULL CHECK (policy_version_hash ~ '^sha256:[0-9a-f]{64}$'),
  envelope_fingerprint TEXT NOT NULL UNIQUE CHECK (envelope_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted','queued','attempted','executed','outcome_unknown','reconciled','retryable_failed','final_failed','cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
  lease_fence BIGINT NOT NULL DEFAULT 0 CHECK (lease_fence >= 0),
  lease_expires_at TIMESTAMPTZ,
  river_job_id BIGINT,
  river_queue TEXT,
  river_args_digest TEXT CHECK (river_args_digest IS NULL OR river_args_digest ~ '^sha256:[0-9a-f]{64}$'),
  river_scheduled_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((river_job_id IS NULL AND river_queue IS NULL AND river_args_digest IS NULL AND river_scheduled_at IS NULL) OR (river_job_id > 0 AND river_queue <> '' AND river_args_digest IS NOT NULL AND river_scheduled_at IS NOT NULL)),
  CHECK ((state = 'attempted' AND lease_fence > 0 AND lease_expires_at IS NOT NULL) OR state <> 'attempted')
);
CREATE INDEX external_effects_state_updated_idx ON external_effects(state, updated_at DESC, id DESC);

CREATE TABLE external_effect_attempts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effect_id BIGINT NOT NULL REFERENCES external_effects(id) ON DELETE RESTRICT,
  number INTEGER NOT NULL CHECK (number > 0),
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL CHECK (fence > 0),
  started_at TIMESTAMPTZ NOT NULL,
  completion TEXT CHECK (completion IN ('executed','retryable_failed','final_failed','outcome_unknown')),
  receipt_digest TEXT CHECK (receipt_digest IS NULL OR receipt_digest ~ '^sha256:[0-9a-f]{64}$'),
  completed_at TIMESTAMPTZ,
  UNIQUE(effect_id, number),
  UNIQUE(effect_id, generation, fence),
  CHECK ((completion IS NULL AND receipt_digest IS NULL AND completed_at IS NULL) OR (completion IS NOT NULL AND receipt_digest IS NOT NULL AND completed_at IS NOT NULL))
);

CREATE TABLE external_effect_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('accept','queue','retry','cancel','complete_attempt','reconcile','recover_attempted')),
  effect_id BIGINT REFERENCES external_effects(id) ON DELETE RESTRICT,
  receipt_key_digest TEXT NOT NULL CHECK (receipt_key_digest ~ '^sha256:[0-9a-f]{64}$'),
  command_digest TEXT NOT NULL CHECK (command_digest ~ '^sha256:[0-9a-f]{64}$'),
  state TEXT NOT NULL CHECK (state IN ('accepted','queued','attempted','executed','outcome_unknown','reconciled','retryable_failed','final_failed','cancelled')),
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(operation, effect_id, receipt_key_digest),
  UNIQUE(operation, command_digest)
);

CREATE TABLE external_effect_reconciliations (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  effect_id BIGINT NOT NULL REFERENCES external_effects(id) ON DELETE RESTRICT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence BIGINT NOT NULL CHECK (fence > 0),
  evidence_digest TEXT NOT NULL CHECK (evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(effect_id, generation, fence)
);
CREATE UNIQUE INDEX external_effect_receipt_key_once ON external_effect_receipts(operation, COALESCE(effect_id, 0), receipt_key_digest);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION aicrm_external_effects_reject_delete() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'external effect facts cannot be deleted' USING ERRCODE = '55000';
END $$;
-- +goose StatementEnd

CREATE TRIGGER external_effects_no_delete BEFORE DELETE ON external_effects FOR EACH ROW EXECUTE FUNCTION aicrm_external_effects_reject_delete();
CREATE TRIGGER external_effect_attempts_no_delete BEFORE DELETE ON external_effect_attempts FOR EACH ROW EXECUTE FUNCTION aicrm_external_effects_reject_delete();
CREATE TRIGGER external_effect_receipts_no_delete BEFORE DELETE ON external_effect_receipts FOR EACH ROW EXECUTE FUNCTION aicrm_external_effects_reject_delete();
CREATE TRIGGER external_effect_reconciliations_no_delete BEFORE DELETE ON external_effect_reconciliations FOR EACH ROW EXECUTE FUNCTION aicrm_external_effects_reject_delete();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM external_effects LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated external effects runtime' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE external_effect_reconciliations;
DROP TABLE external_effect_receipts;
DROP TABLE external_effect_attempts;
DROP TABLE external_effects;
DROP FUNCTION aicrm_external_effects_reject_delete();

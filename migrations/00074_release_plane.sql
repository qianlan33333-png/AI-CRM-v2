-- +goose Up
-- Release-plane facts are local attestations and journals only. No table in
-- this migration authorizes a deploy, backup, provider, payment, or WeCom call.
CREATE TABLE release_candidates (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  commit_sha TEXT NOT NULL UNIQUE CHECK (commit_sha ~ '^[0-9a-f]{40}$'),
  artifact_digest TEXT NOT NULL CHECK (artifact_digest ~ '^[0-9a-f]{64}$'),
  manifest_digest TEXT NOT NULL CHECK (manifest_digest ~ '^[0-9a-f]{64}$'),
  config_digest TEXT NOT NULL CHECK (config_digest ~ '^[0-9a-f]{64}$'),
  target_schema_version BIGINT NOT NULL CHECK (target_schema_version > 0),
  state TEXT NOT NULL CHECK (state IN ('draft','prepared','cutover_active','activated','rollback_pending','rolled_back')),
  created_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  prepared_at TIMESTAMPTZ,
  activated_at TIMESTAMPTZ,
  rollback_requested_at TIMESTAMPTZ,
  rolled_back_at TIMESTAMPTZ
);
CREATE TABLE release_prerequisite_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  candidate_id BIGINT NOT NULL REFERENCES release_candidates(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (kind IN ('nightly','backup_restore_drill','migration','contact_closure','campaign_closure','outbound_closure','commerce_closure')),
  evidence_sha TEXT NOT NULL CHECK (evidence_sha ~ '^[0-9a-f]{64}$'),
  recorded_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(candidate_id, kind)
);
CREATE TABLE release_worker_leases (
  candidate_id BIGINT PRIMARY KEY REFERENCES release_candidates(id) ON DELETE RESTRICT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence TEXT NOT NULL CHECK (fence ~ '^[0-9a-f]{64}$'),
  started_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  active BOOLEAN NOT NULL DEFAULT TRUE,
  UNIQUE(fence)
);
CREATE UNIQUE INDEX release_one_active_worker ON release_worker_leases ((active)) WHERE active;
CREATE TABLE release_cutover_journal (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  candidate_id BIGINT NOT NULL REFERENCES release_candidates(id) ON DELETE RESTRICT,
  step TEXT NOT NULL CHECK (step IN ('announce','quiesce','schema_verify','switch','verify')),
  fence TEXT NOT NULL CHECK (fence ~ '^[0-9a-f]{64}$'),
  completed_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(candidate_id, step),
  UNIQUE(candidate_id, id)
);
CREATE TABLE release_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  action TEXT NOT NULL CHECK (action IN ('candidate.register','prerequisite.record','candidate.prepare','cutover.start','cutover.step.complete','candidate.activate','rollback.request','rollback.complete')),
  actor_id BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  key_digest TEXT NOT NULL CHECK (key_digest ~ '^[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^[0-9a-f]{64}$'),
  state TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress','completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE(action, actor_id, key_digest),
  CHECK ((state='in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR (state='completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL))
);

CREATE FUNCTION aicrm_release_candidate_guard() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NEW.commit_sha <> OLD.commit_sha OR NEW.artifact_digest <> OLD.artifact_digest OR NEW.manifest_digest <> OLD.manifest_digest
     OR NEW.config_digest <> OLD.config_digest OR NEW.target_schema_version <> OLD.target_schema_version OR NEW.created_by <> OLD.created_by OR NEW.created_at <> OLD.created_at THEN
    RAISE EXCEPTION 'release candidate facts are immutable' USING ERRCODE='55000';
  END IF;
  IF NOT ((OLD.state='draft' AND NEW.state='prepared' AND NEW.prepared_at IS NOT NULL)
       OR (OLD.state='prepared' AND NEW.state='cutover_active')
       OR (OLD.state='cutover_active' AND NEW.state='activated' AND NEW.activated_at IS NOT NULL)
       OR (OLD.state='activated' AND NEW.state='rollback_pending' AND NEW.rollback_requested_at IS NOT NULL)
       OR (OLD.state='rollback_pending' AND NEW.state='rolled_back' AND NEW.rolled_back_at IS NOT NULL)) THEN
    RAISE EXCEPTION 'invalid release candidate transition' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER release_candidate_guard BEFORE UPDATE OR DELETE ON release_candidates FOR EACH ROW EXECUTE FUNCTION aicrm_release_candidate_guard();
CREATE FUNCTION aicrm_release_append_only() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$ BEGIN RAISE EXCEPTION 'release facts are append only' USING ERRCODE='55000'; END $$;
CREATE TRIGGER release_prerequisites_immutable BEFORE UPDATE OR DELETE ON release_prerequisite_receipts FOR EACH ROW EXECUTE FUNCTION aicrm_release_append_only();
CREATE TRIGGER release_journal_immutable BEFORE UPDATE OR DELETE ON release_cutover_journal FOR EACH ROW EXECUTE FUNCTION aicrm_release_append_only();
CREATE FUNCTION aicrm_release_receipt_guard() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP='DELETE' OR OLD.state='completed' OR NEW.id<>OLD.id OR NEW.action<>OLD.action OR NEW.actor_id<>OLD.actor_id OR NEW.key_digest<>OLD.key_digest OR NEW.payload_digest<>OLD.payload_digest OR NEW.created_at<>OLD.created_at OR NEW.state<>'completed' OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid release operation receipt transition' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER release_receipt_guard BEFORE UPDATE OR DELETE ON release_operation_receipts FOR EACH ROW EXECUTE FUNCTION aicrm_release_receipt_guard();
-- +goose Down
LOCK TABLE release_operation_receipts, release_cutover_journal, release_worker_leases, release_prerequisite_receipts, release_candidates IN SHARE ROW EXCLUSIVE MODE;
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM release_operation_receipts) OR EXISTS (SELECT 1 FROM release_cutover_journal)
     OR EXISTS (SELECT 1 FROM release_worker_leases) OR EXISTS (SELECT 1 FROM release_prerequisite_receipts) OR EXISTS (SELECT 1 FROM release_candidates) THEN
    RAISE EXCEPTION 'cannot roll back populated release plane' USING ERRCODE='55000';
  END IF;
END $$;
DROP TRIGGER release_receipt_guard ON release_operation_receipts;
DROP FUNCTION aicrm_release_receipt_guard();
DROP TRIGGER release_journal_immutable ON release_cutover_journal;
DROP TRIGGER release_prerequisites_immutable ON release_prerequisite_receipts;
DROP FUNCTION aicrm_release_append_only();
DROP TRIGGER release_candidate_guard ON release_candidates;
DROP FUNCTION aicrm_release_candidate_guard();
DROP TABLE release_operation_receipts;
DROP TABLE release_cutover_journal;
DROP TABLE release_worker_leases;
DROP TABLE release_prerequisite_receipts;
DROP TABLE release_candidates;

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
  rolled_back_at TIMESTAMPTZ,
  UNIQUE(id, commit_sha, artifact_digest, manifest_digest, config_digest, target_schema_version)
);

CREATE TABLE release_prerequisite_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  candidate_id BIGINT NOT NULL,
  candidate_commit_sha TEXT NOT NULL CHECK (candidate_commit_sha ~ '^[0-9a-f]{40}$'),
  candidate_artifact_digest TEXT NOT NULL CHECK (candidate_artifact_digest ~ '^[0-9a-f]{64}$'),
  candidate_manifest_digest TEXT NOT NULL CHECK (candidate_manifest_digest ~ '^[0-9a-f]{64}$'),
  candidate_config_digest TEXT NOT NULL CHECK (candidate_config_digest ~ '^[0-9a-f]{64}$'),
  candidate_schema_version BIGINT NOT NULL CHECK (candidate_schema_version > 0),
  kind TEXT NOT NULL CHECK (kind IN ('nightly','backup_restore_drill','migration','contact_closure','campaign_closure','outbound_closure','commerce_closure')),
  evidence_sha TEXT NOT NULL CHECK (evidence_sha ~ '^[0-9a-f]{64}$'),
  recorded_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY(candidate_id, candidate_commit_sha, candidate_artifact_digest, candidate_manifest_digest, candidate_config_digest, candidate_schema_version)
    REFERENCES release_candidates(id, commit_sha, artifact_digest, manifest_digest, config_digest, target_schema_version) ON DELETE RESTRICT,
  UNIQUE(candidate_id, kind)
);

CREATE TABLE release_worker_leases (
  candidate_id BIGINT NOT NULL REFERENCES release_candidates(id) ON DELETE RESTRICT,
  generation BIGINT NOT NULL CHECK (generation > 0),
  fence TEXT NOT NULL UNIQUE CHECK (fence ~ '^[0-9a-f]{64}$'),
  started_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  active BOOLEAN NOT NULL DEFAULT TRUE,
  retired_at TIMESTAMPTZ,
  PRIMARY KEY(candidate_id, generation),
  CHECK ((active AND retired_at IS NULL) OR (NOT active AND retired_at IS NOT NULL))
);
CREATE UNIQUE INDEX release_one_active_worker ON release_worker_leases ((TRUE)) WHERE active;

CREATE TABLE release_cutover_journal (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  candidate_id BIGINT NOT NULL,
  generation BIGINT NOT NULL,
  step TEXT NOT NULL CHECK (step IN ('announce','quiesce','schema_verify','switch','verify')),
  fence TEXT NOT NULL CHECK (fence ~ '^[0-9a-f]{64}$'),
  completed_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY(candidate_id, generation) REFERENCES release_worker_leases(candidate_id, generation) ON DELETE RESTRICT,
  UNIQUE(candidate_id, step)
);

CREATE TABLE release_rollback_checks (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  candidate_id BIGINT NOT NULL REFERENCES release_candidates(id) ON DELETE RESTRICT,
  kind TEXT NOT NULL CHECK (kind IN ('schema_compatibility','data_reconciliation','outbound_reconciliation','rollback_execution_reconciliation')),
  passed BOOLEAN NOT NULL,
  evidence_sha TEXT NOT NULL CHECK (evidence_sha ~ '^[0-9a-f]{64}$'),
  recorded_by BIGINT NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX release_rollback_checks_latest ON release_rollback_checks(candidate_id, kind, id DESC);

CREATE TABLE release_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  action TEXT NOT NULL CHECK (action IN ('candidate.register','prerequisite.record','candidate.prepare','cutover.start','cutover.restart','cutover.step.complete','candidate.activate','rollback.check.record','rollback.request','rollback.complete')),
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

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_reject_delete() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'release facts cannot be deleted' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_candidate_update_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  completed_steps TEXT[];
  passed_count INTEGER;
  check_count INTEGER;
  post_verified BOOLEAN;
BEGIN
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.commit_sha IS DISTINCT FROM OLD.commit_sha
     OR NEW.artifact_digest IS DISTINCT FROM OLD.artifact_digest
     OR NEW.manifest_digest IS DISTINCT FROM OLD.manifest_digest
     OR NEW.config_digest IS DISTINCT FROM OLD.config_digest
     OR NEW.target_schema_version IS DISTINCT FROM OLD.target_schema_version
     OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'release candidate identity and artifacts are immutable' USING ERRCODE='55000';
  END IF;

  IF OLD.state='draft' AND NEW.state='prepared' THEN
    IF OLD.prepared_at IS NOT NULL OR NEW.prepared_at IS NULL
       OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
       OR NEW.rollback_requested_at IS DISTINCT FROM OLD.rollback_requested_at
       OR NEW.rolled_back_at IS DISTINCT FROM OLD.rolled_back_at THEN
      RAISE EXCEPTION 'invalid prepared transition facts' USING ERRCODE='55000';
    END IF;
  ELSIF OLD.state='prepared' AND NEW.state='cutover_active' THEN
    IF NEW.prepared_at IS DISTINCT FROM OLD.prepared_at
       OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
       OR NEW.rollback_requested_at IS DISTINCT FROM OLD.rollback_requested_at
       OR NEW.rolled_back_at IS DISTINCT FROM OLD.rolled_back_at
       OR NOT EXISTS (SELECT 1 FROM public.release_worker_leases WHERE candidate_id=OLD.id AND active) THEN
      RAISE EXCEPTION 'cutover requires one active fenced worker' USING ERRCODE='55000';
    END IF;
  ELSIF OLD.state='cutover_active' AND NEW.state='activated' THEN
    SELECT array_agg(step ORDER BY id) INTO completed_steps
    FROM public.release_cutover_journal WHERE candidate_id=OLD.id;
    IF OLD.activated_at IS NOT NULL OR NEW.activated_at IS NULL
       OR NEW.prepared_at IS DISTINCT FROM OLD.prepared_at
       OR NEW.rollback_requested_at IS DISTINCT FROM OLD.rollback_requested_at
       OR NEW.rolled_back_at IS DISTINCT FROM OLD.rolled_back_at
       OR completed_steps IS DISTINCT FROM ARRAY['announce','quiesce','schema_verify','switch','verify']::TEXT[]
       OR NOT EXISTS (SELECT 1 FROM public.release_worker_leases WHERE candidate_id=OLD.id AND active) THEN
      RAISE EXCEPTION 'activation requires the complete journal and active fence' USING ERRCODE='55000';
    END IF;
  ELSIF OLD.state='activated' AND NEW.state='rollback_pending' THEN
    SELECT count(*), count(*) FILTER (WHERE passed) INTO check_count, passed_count
    FROM (
      SELECT DISTINCT ON (kind) kind, passed
      FROM public.release_rollback_checks
      WHERE candidate_id=OLD.id AND kind IN ('schema_compatibility','data_reconciliation','outbound_reconciliation')
      ORDER BY kind, id DESC
    ) latest;
    IF OLD.rollback_requested_at IS NOT NULL OR NEW.rollback_requested_at IS NULL
       OR NEW.prepared_at IS DISTINCT FROM OLD.prepared_at
       OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
       OR NEW.rolled_back_at IS DISTINCT FROM OLD.rolled_back_at
       OR check_count<>3 OR passed_count<>3 THEN
      RAISE EXCEPTION 'rollback is not eligible' USING ERRCODE='55000';
    END IF;
  ELSIF OLD.state='rollback_pending' AND NEW.state='rolled_back' THEN
    SELECT passed INTO post_verified
    FROM public.release_rollback_checks
    WHERE candidate_id=OLD.id AND kind='rollback_execution_reconciliation'
    ORDER BY id DESC LIMIT 1;
    IF OLD.rolled_back_at IS NOT NULL OR NEW.rolled_back_at IS NULL
       OR NEW.prepared_at IS DISTINCT FROM OLD.prepared_at
       OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
       OR NEW.rollback_requested_at IS DISTINCT FROM OLD.rollback_requested_at
       OR post_verified IS DISTINCT FROM TRUE THEN
      RAISE EXCEPTION 'rollback reconciliation is incomplete' USING ERRCODE='55000';
    END IF;
  ELSE
    RAISE EXCEPTION 'invalid release candidate transition' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_candidate_update_guard BEFORE UPDATE ON release_candidates
FOR EACH ROW EXECUTE FUNCTION aicrm_release_candidate_update_guard();
CREATE TRIGGER release_candidate_delete_guard BEFORE DELETE ON release_candidates
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_prerequisite_insert_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE candidate_state TEXT;
BEGIN
  SELECT state INTO candidate_state FROM public.release_candidates WHERE id=NEW.candidate_id FOR UPDATE;
  IF candidate_state IS DISTINCT FROM 'draft' THEN
    RAISE EXCEPTION 'prerequisites may only be recorded for draft candidates' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_prerequisite_insert_guard BEFORE INSERT ON release_prerequisite_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_release_prerequisite_insert_guard();
CREATE TRIGGER release_prerequisite_update_guard BEFORE UPDATE OR DELETE ON release_prerequisite_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_worker_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  candidate_state TEXT;
  next_generation BIGINT;
BEGIN
  IF TG_OP='INSERT' THEN
    PERFORM pg_catalog.pg_advisory_xact_lock(pg_catalog.hashtextextended('aicrm.release.active-worker', 0));
    SELECT state INTO candidate_state FROM public.release_candidates WHERE id=NEW.candidate_id FOR UPDATE;
    SELECT COALESCE(max(generation),0)+1 INTO next_generation FROM public.release_worker_leases WHERE candidate_id=NEW.candidate_id;
    IF candidate_state NOT IN ('prepared','cutover_active') OR NEW.generation<>next_generation OR NOT NEW.active OR NEW.retired_at IS NOT NULL
       OR EXISTS (SELECT 1 FROM public.release_worker_leases WHERE active) THEN
      RAISE EXCEPTION 'invalid or competing release worker generation' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
  END IF;
  PERFORM 1 FROM public.release_candidates WHERE id=OLD.candidate_id FOR UPDATE;
  IF TG_OP='UPDATE' AND OLD.active AND NOT NEW.active AND OLD.retired_at IS NULL AND NEW.retired_at IS NOT NULL
     AND NEW.candidate_id=OLD.candidate_id AND NEW.generation=OLD.generation
     AND NEW.fence=OLD.fence AND NEW.started_by=OLD.started_by AND NEW.started_at=OLD.started_at THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid release worker lifecycle change' USING ERRCODE='55000';
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_worker_insert_update_guard BEFORE INSERT OR UPDATE ON release_worker_leases
FOR EACH ROW EXECUTE FUNCTION aicrm_release_worker_guard();
CREATE TRIGGER release_worker_delete_guard BEFORE DELETE ON release_worker_leases
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_journal_insert_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  candidate_state TEXT;
  step_count INTEGER;
  expected_step TEXT;
BEGIN
  SELECT state INTO candidate_state FROM public.release_candidates WHERE id=NEW.candidate_id FOR UPDATE;
  IF candidate_state IS DISTINCT FROM 'cutover_active'
     OR NOT EXISTS (
       SELECT 1 FROM public.release_worker_leases
       WHERE candidate_id=NEW.candidate_id AND generation=NEW.generation AND fence=NEW.fence AND active
     ) THEN
    RAISE EXCEPTION 'cutover journal fence rejected' USING ERRCODE='55000';
  END IF;
  SELECT count(*) INTO step_count FROM public.release_cutover_journal WHERE candidate_id=NEW.candidate_id;
  expected_step := (ARRAY['announce','quiesce','schema_verify','switch','verify']::TEXT[])[step_count+1];
  IF expected_step IS NULL OR NEW.step<>expected_step THEN
    RAISE EXCEPTION 'cutover journal step is out of order' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_journal_insert_guard BEFORE INSERT ON release_cutover_journal
FOR EACH ROW EXECUTE FUNCTION aicrm_release_journal_insert_guard();
CREATE TRIGGER release_journal_update_guard BEFORE UPDATE OR DELETE ON release_cutover_journal
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_rollback_check_insert_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE candidate_state TEXT;
BEGIN
  SELECT state INTO candidate_state FROM public.release_candidates WHERE id=NEW.candidate_id FOR UPDATE;
  IF (NEW.kind='rollback_execution_reconciliation' AND candidate_state IS DISTINCT FROM 'rollback_pending')
     OR (NEW.kind<>'rollback_execution_reconciliation' AND candidate_state IS DISTINCT FROM 'activated') THEN
    RAISE EXCEPTION 'rollback check is invalid for candidate state' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_rollback_check_insert_guard BEFORE INSERT ON release_rollback_checks
FOR EACH ROW EXECUTE FUNCTION aicrm_release_rollback_check_insert_guard();
CREATE TRIGGER release_rollback_check_update_guard BEFORE UPDATE OR DELETE ON release_rollback_checks
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose StatementBegin
CREATE FUNCTION aicrm_release_receipt_update_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF OLD.state<>'in_progress' OR NEW.state<>'completed'
     OR NEW.id IS DISTINCT FROM OLD.id OR NEW.action IS DISTINCT FROM OLD.action
     OR NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.key_digest IS DISTINCT FROM OLD.key_digest
     OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest OR NEW.created_at IS DISTINCT FROM OLD.created_at
     OR NEW.result_snapshot IS NULL OR NEW.completed_at IS NULL THEN
    RAISE EXCEPTION 'invalid release operation receipt transition' USING ERRCODE='55000';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER release_receipt_update_guard BEFORE UPDATE ON release_operation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_release_receipt_update_guard();
CREATE TRIGGER release_receipt_delete_guard BEFORE DELETE ON release_operation_receipts
FOR EACH ROW EXECUTE FUNCTION aicrm_release_reject_delete();

-- +goose Down
LOCK TABLE release_operation_receipts, release_rollback_checks, release_cutover_journal, release_worker_leases, release_prerequisite_receipts, release_candidates IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM release_operation_receipts)
     OR EXISTS (SELECT 1 FROM release_rollback_checks)
     OR EXISTS (SELECT 1 FROM release_cutover_journal)
     OR EXISTS (SELECT 1 FROM release_worker_leases)
     OR EXISTS (SELECT 1 FROM release_prerequisite_receipts)
     OR EXISTS (SELECT 1 FROM release_candidates) THEN
    RAISE EXCEPTION 'cannot roll back populated release plane' USING ERRCODE='55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER release_receipt_delete_guard ON release_operation_receipts;
DROP TRIGGER release_receipt_update_guard ON release_operation_receipts;
DROP FUNCTION aicrm_release_receipt_update_guard();
DROP TRIGGER release_rollback_check_update_guard ON release_rollback_checks;
DROP TRIGGER release_rollback_check_insert_guard ON release_rollback_checks;
DROP FUNCTION aicrm_release_rollback_check_insert_guard();
DROP TRIGGER release_journal_update_guard ON release_cutover_journal;
DROP TRIGGER release_journal_insert_guard ON release_cutover_journal;
DROP FUNCTION aicrm_release_journal_insert_guard();
DROP TRIGGER release_worker_delete_guard ON release_worker_leases;
DROP TRIGGER release_worker_insert_update_guard ON release_worker_leases;
DROP FUNCTION aicrm_release_worker_guard();
DROP TRIGGER release_prerequisite_update_guard ON release_prerequisite_receipts;
DROP TRIGGER release_prerequisite_insert_guard ON release_prerequisite_receipts;
DROP FUNCTION aicrm_release_prerequisite_insert_guard();
DROP TRIGGER release_candidate_delete_guard ON release_candidates;
DROP TRIGGER release_candidate_update_guard ON release_candidates;
DROP FUNCTION aicrm_release_candidate_update_guard();
DROP FUNCTION aicrm_release_reject_delete();
DROP TABLE release_operation_receipts;
DROP TABLE release_rollback_checks;
DROP TABLE release_cutover_journal;
DROP TABLE release_worker_leases;
DROP TABLE release_prerequisite_receipts;
DROP TABLE release_candidates;

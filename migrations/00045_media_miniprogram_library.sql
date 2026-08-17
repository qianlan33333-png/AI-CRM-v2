-- +goose Up
CREATE TABLE public.media_thumbnail_cache_entries (
  image_id BIGINT PRIMARY KEY REFERENCES public.media_images(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  cache_receipt TEXT NOT NULL,
  media_id TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_thumbnail_cache_state CHECK (state IN ('resolved', 'outcome_unknown')),
  CONSTRAINT media_thumbnail_cache_receipt CHECK (char_length(cache_receipt) BETWEEN 1 AND 512),
  CONSTRAINT media_thumbnail_cache_media CHECK (
    (state = 'resolved' AND char_length(media_id) BETWEEN 1 AND 255) OR
    (state = 'outcome_unknown' AND media_id = '' AND expires_at IS NULL)
  )
);

CREATE TABLE public.media_miniprograms (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  legacy_source_id BIGINT UNIQUE,
  name TEXT NOT NULL,
  app_id TEXT NOT NULL,
  page_path TEXT NOT NULL,
  title TEXT NOT NULL,
  thumbnail_image_url TEXT NOT NULL DEFAULT '',
  thumbnail_image_id BIGINT REFERENCES public.media_images(id) ON DELETE RESTRICT,
  thumbnail_media_id TEXT NOT NULL DEFAULT '',
  thumbnail_media_expires_at TIMESTAMPTZ,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_by BIGINT NOT NULL,
  updated_by BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_miniprograms_legacy_source CHECK (legacy_source_id IS NULL OR legacy_source_id > 0),
  CONSTRAINT media_miniprograms_name CHECK (char_length(name) <= 200),
  CONSTRAINT media_miniprograms_app_id CHECK (char_length(app_id) BETWEEN 1 AND 120),
  CONSTRAINT media_miniprograms_page_path CHECK (char_length(page_path) BETWEEN 1 AND 500),
  CONSTRAINT media_miniprograms_title CHECK (char_length(title) BETWEEN 1 AND 200),
  CONSTRAINT media_miniprograms_thumbnail_url CHECK (char_length(thumbnail_image_url) <= 2048),
  CONSTRAINT media_miniprograms_thumbnail_image CHECK (thumbnail_image_id IS NULL OR thumbnail_image_id > 0),
  CONSTRAINT media_miniprograms_thumbnail_cache CHECK (
    char_length(thumbnail_media_id) <= 255 AND
    (thumbnail_media_id <> '' OR thumbnail_media_expires_at IS NULL)
  ),
  CONSTRAINT media_miniprograms_actors CHECK (created_by > 0 AND updated_by > 0),
  CONSTRAINT media_miniprograms_version CHECK (version > 0)
);

ALTER TABLE public.media_miniprograms
ADD CONSTRAINT media_miniprograms_id_legacy_source_unique UNIQUE (id, legacy_source_id);

CREATE INDEX media_miniprograms_updated_id_idx
ON public.media_miniprograms (updated_at DESC, id DESC);

CREATE INDEX media_miniprograms_enabled_updated_id_idx
ON public.media_miniprograms (enabled, updated_at DESC, id DESC);

CREATE INDEX media_miniprograms_thumbnail_image_id_idx
ON public.media_miniprograms (thumbnail_image_id)
WHERE thumbnail_image_id IS NOT NULL;

-- This is an evidence boundary only. No migration runner reads legacy data in this
-- release, and creating these tables does not claim that historical rows were read.
CREATE TABLE public.media_miniprogram_import_preflights (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_system TEXT NOT NULL DEFAULT 'legacy.miniprogram_library',
  source_snapshot_digest BYTEA NOT NULL,
  source_row_count BIGINT NOT NULL,
  url_only_row_count BIGINT NOT NULL,
  unresolved_image_row_count BIGINT NOT NULL,
  state TEXT NOT NULL DEFAULT 'external_gate_required',
  external_gate_ref TEXT,
  url_only_decision TEXT,
  human_decision_ref TEXT,
  recorded_at TIMESTAMPTZ NOT NULL,
  ready_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  CONSTRAINT media_miniprogram_preflights_source CHECK (source_system = 'legacy.miniprogram_library'),
  CONSTRAINT media_miniprogram_preflights_digest CHECK (octet_length(source_snapshot_digest) = 32),
  CONSTRAINT media_miniprogram_preflights_counts CHECK (
    source_row_count >= 0 AND url_only_row_count >= 0 AND unresolved_image_row_count >= 0 AND
    url_only_row_count <= source_row_count AND unresolved_image_row_count <= source_row_count
  ),
  CONSTRAINT media_miniprogram_preflights_state CHECK (
    state IN ('external_gate_required', 'human_decision_required', 'ready', 'completed')
  ),
  CONSTRAINT media_miniprogram_preflights_external_gate CHECK (
    external_gate_ref IS NULL OR char_length(external_gate_ref) BETWEEN 1 AND 512
  ),
  CONSTRAINT media_miniprogram_preflights_url_only_decision CHECK (
    (url_only_row_count = 0 AND url_only_decision IS NULL AND human_decision_ref IS NULL AND state <> 'human_decision_required') OR
    (url_only_row_count > 0 AND state IN ('external_gate_required', 'human_decision_required') AND
      url_only_decision IS NULL AND human_decision_ref IS NULL) OR
    (url_only_row_count > 0 AND state IN ('ready', 'completed') AND
      url_only_decision IS NOT NULL AND human_decision_ref IS NOT NULL AND
      url_only_decision IN ('retain_metadata_without_fetch', 'quarantine_row') AND
      char_length(human_decision_ref) BETWEEN 1 AND 512)
  ),
  CONSTRAINT media_miniprogram_preflights_timestamps CHECK (
    (state IN ('external_gate_required', 'human_decision_required') AND ready_at IS NULL AND completed_at IS NULL) OR
    (state = 'ready' AND ready_at IS NOT NULL AND ready_at >= recorded_at AND completed_at IS NULL) OR
    (state = 'completed' AND ready_at IS NOT NULL AND ready_at >= recorded_at AND
      completed_at IS NOT NULL AND completed_at >= ready_at)
  )
);

CREATE TABLE public.media_miniprogram_import_ledger (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  preflight_id BIGINT NOT NULL REFERENCES public.media_miniprogram_import_preflights(id) ON DELETE RESTRICT,
  legacy_source_id BIGINT NOT NULL,
  source_row_digest BYTEA NOT NULL,
  disposition TEXT NOT NULL,
  target_miniprogram_id BIGINT,
  image_disposition TEXT NOT NULL,
  legacy_thumbnail_image_id BIGINT,
  target_media_image_id BIGINT,
  rebuild_content_digest BYTEA,
  source_url_only BOOLEAN NOT NULL,
  source_image_unresolved BOOLEAN NOT NULL,
  provider_cache_disposition TEXT NOT NULL,
  reason TEXT NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT media_miniprogram_import_ledger_source CHECK (legacy_source_id > 0 AND octet_length(source_row_digest) = 32),
  CONSTRAINT media_miniprogram_import_ledger_disposition CHECK (
    disposition IN ('migrated', 'quarantined')
  ),
  CONSTRAINT media_miniprogram_import_ledger_target CHECK (
    (disposition = 'migrated' AND target_miniprogram_id > 0) OR
    (disposition = 'quarantined' AND target_miniprogram_id IS NULL)
  ),
  CONSTRAINT media_miniprogram_import_ledger_image CHECK (
    (image_disposition = 'remapped' AND disposition = 'migrated' AND
      legacy_thumbnail_image_id IS NOT NULL AND legacy_thumbnail_image_id > 0 AND
      target_media_image_id IS NOT NULL AND target_media_image_id > 0 AND rebuild_content_digest IS NULL AND
      NOT source_url_only AND NOT source_image_unresolved) OR
    (image_disposition = 'rebuilt' AND disposition = 'migrated' AND
      target_media_image_id IS NOT NULL AND target_media_image_id > 0 AND
      rebuild_content_digest IS NOT NULL AND octet_length(rebuild_content_digest) = 32 AND
      NOT source_url_only AND source_image_unresolved) OR
    (image_disposition = 'metadata_url_only' AND
      legacy_thumbnail_image_id IS NULL AND target_media_image_id IS NULL AND rebuild_content_digest IS NULL AND
      source_url_only AND source_image_unresolved) OR
    (image_disposition = 'none' AND disposition = 'migrated' AND
      legacy_thumbnail_image_id IS NULL AND target_media_image_id IS NULL AND rebuild_content_digest IS NULL AND
      NOT source_url_only AND NOT source_image_unresolved) OR
    (image_disposition = 'quarantined_unresolved' AND disposition = 'quarantined' AND
      target_media_image_id IS NULL AND rebuild_content_digest IS NULL AND
      NOT source_url_only AND source_image_unresolved)
  ),
  CONSTRAINT media_miniprogram_import_ledger_image_ids CHECK (
    (legacy_thumbnail_image_id IS NULL OR legacy_thumbnail_image_id > 0) AND
    (target_media_image_id IS NULL OR target_media_image_id > 0)
  ),
  CONSTRAINT media_miniprogram_import_ledger_provider_cache CHECK (provider_cache_disposition = 'dropped'),
  CONSTRAINT media_miniprogram_import_ledger_reason CHECK (char_length(reason) BETWEEN 1 AND 1024),
  UNIQUE (preflight_id, legacy_source_id)
);

CREATE INDEX media_miniprogram_import_ledger_target_idx
ON public.media_miniprogram_import_ledger (target_miniprogram_id)
WHERE target_miniprogram_id IS NOT NULL;

-- The ledger deliberately has no target foreign key: legacy physical delete must
-- remain possible while the immutable ledger preserves the consumed source ID.
-- At ledger write time, however, a migrated target must exist and carry that ID.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_validate_media_miniprogram_import_ledger()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  preflight_state TEXT;
  preflight_url_only_decision TEXT;
BEGIN
  SELECT state, url_only_decision
    INTO preflight_state, preflight_url_only_decision
    FROM public.media_miniprogram_import_preflights
    WHERE id = NEW.preflight_id
    FOR UPDATE;
  IF NOT FOUND OR preflight_state <> 'ready' THEN
    RAISE EXCEPTION 'media miniprogram import ledger requires a ready preflight' USING ERRCODE = '23514';
  END IF;
  IF NEW.disposition = 'migrated' AND NOT EXISTS (
    SELECT 1 FROM public.media_miniprograms
    WHERE id = NEW.target_miniprogram_id AND legacy_source_id = NEW.legacy_source_id
  ) THEN
    RAISE EXCEPTION 'media miniprogram import ledger target/source mismatch' USING ERRCODE = '23514';
  END IF;
  IF NEW.disposition = 'migrated' AND NOT EXISTS (
    SELECT 1 FROM public.media_miniprograms
    WHERE id = NEW.target_miniprogram_id AND
      thumbnail_media_id = '' AND thumbnail_media_expires_at IS NULL
  ) THEN
    RAISE EXCEPTION 'media miniprogram import ledger target contains provider cache state' USING ERRCODE = '23514';
  END IF;
  IF NEW.image_disposition IN ('remapped', 'rebuilt') AND NOT EXISTS (
    SELECT 1 FROM public.media_miniprograms
    WHERE id = NEW.target_miniprogram_id AND thumbnail_image_id = NEW.target_media_image_id
  ) THEN
    RAISE EXCEPTION 'media miniprogram import ledger image lineage mismatch' USING ERRCODE = '23514';
  END IF;
  IF NEW.image_disposition = 'rebuilt' AND NOT EXISTS (
    SELECT 1
    FROM public.media_images image
    JOIN public.media_image_blobs blob ON blob.image_id = image.id
    WHERE image.id = NEW.target_media_image_id AND
      image.checksum = NEW.rebuild_content_digest AND
      blob.checksum = NEW.rebuild_content_digest
  ) THEN
    RAISE EXCEPTION 'media miniprogram import ledger rebuild digest mismatch' USING ERRCODE = '23514';
  END IF;
  IF NEW.image_disposition = 'metadata_url_only' AND (
    (NEW.disposition = 'migrated' AND preflight_url_only_decision <> 'retain_metadata_without_fetch') OR
    (NEW.disposition = 'quarantined' AND preflight_url_only_decision <> 'quarantine_row') OR
    (NEW.disposition = 'migrated' AND NOT EXISTS (
      SELECT 1 FROM public.media_miniprograms
      WHERE id = NEW.target_miniprogram_id AND thumbnail_image_id IS NULL AND thumbnail_image_url <> ''
    ))
  ) THEN
    RAISE EXCEPTION 'media miniprogram import ledger URL-only decision mismatch' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_miniprogram_import_ledger_validate
BEFORE INSERT ON public.media_miniprogram_import_ledger
FOR EACH ROW EXECUTE FUNCTION public.aicrm_validate_media_miniprogram_import_ledger();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_media_miniprogram_import_ledger_mutation()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'media miniprogram import ledger is immutable' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_miniprogram_import_ledger_immutable
BEFORE UPDATE OR DELETE ON public.media_miniprogram_import_ledger
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_media_miniprogram_import_ledger_mutation();

-- Preflight rows are immutable source snapshots with one-way, evidence-bound
-- transitions. Completion is accepted only after every frozen source row has a
-- durable terminal ledger row and the frozen URL/unresolved counts reconcile.
-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_miniprogram_preflight_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
  ledger_rows BIGINT;
  ledger_url_only_rows BIGINT;
  ledger_unresolved_rows BIGINT;
  ledger_invalid_target_rows BIGINT;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.state <> 'external_gate_required' OR NEW.external_gate_ref IS NOT NULL OR
       NEW.url_only_decision IS NOT NULL OR NEW.human_decision_ref IS NOT NULL OR
       NEW.ready_at IS NOT NULL OR NEW.completed_at IS NOT NULL THEN
      RAISE EXCEPTION 'media miniprogram preflight must begin at external_gate_required' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' OR
     NEW.id IS DISTINCT FROM OLD.id OR NEW.source_system IS DISTINCT FROM OLD.source_system OR
     NEW.source_snapshot_digest IS DISTINCT FROM OLD.source_snapshot_digest OR
     NEW.source_row_count IS DISTINCT FROM OLD.source_row_count OR
     NEW.url_only_row_count IS DISTINCT FROM OLD.url_only_row_count OR
     NEW.unresolved_image_row_count IS DISTINCT FROM OLD.unresolved_image_row_count OR
     NEW.recorded_at IS DISTINCT FROM OLD.recorded_at THEN
    RAISE EXCEPTION 'media miniprogram preflight snapshot is immutable' USING ERRCODE = '55000';
  END IF;
  IF OLD.state = 'external_gate_required' AND NEW.state = 'human_decision_required' AND
     OLD.url_only_row_count > 0 AND NEW.external_gate_ref IS NOT NULL AND
     NEW.url_only_decision IS NULL AND NEW.human_decision_ref IS NULL AND
     NEW.ready_at IS NULL AND NEW.completed_at IS NULL THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'external_gate_required' AND NEW.state = 'ready' AND
     OLD.url_only_row_count = 0 AND NEW.external_gate_ref IS NOT NULL AND
     NEW.url_only_decision IS NULL AND NEW.human_decision_ref IS NULL AND
     NEW.ready_at IS NOT NULL AND NEW.completed_at IS NULL THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'human_decision_required' AND NEW.state = 'ready' AND
     NEW.external_gate_ref IS NOT DISTINCT FROM OLD.external_gate_ref AND
     NEW.url_only_decision IN ('retain_metadata_without_fetch', 'quarantine_row') AND
     NEW.human_decision_ref IS NOT NULL AND NEW.ready_at IS NOT NULL AND NEW.completed_at IS NULL THEN
    RETURN NEW;
  END IF;
  IF OLD.state = 'ready' AND NEW.state = 'completed' AND
     NEW.external_gate_ref IS NOT DISTINCT FROM OLD.external_gate_ref AND
     NEW.url_only_decision IS NOT DISTINCT FROM OLD.url_only_decision AND
     NEW.human_decision_ref IS NOT DISTINCT FROM OLD.human_decision_ref AND
     NEW.ready_at IS NOT DISTINCT FROM OLD.ready_at AND NEW.completed_at IS NOT NULL THEN
    SELECT count(*), count(*) FILTER (WHERE source_url_only), count(*) FILTER (WHERE source_image_unresolved)
      INTO ledger_rows, ledger_url_only_rows, ledger_unresolved_rows
      FROM public.media_miniprogram_import_ledger WHERE preflight_id = OLD.id;
    SELECT count(*) INTO ledger_invalid_target_rows
      FROM public.media_miniprogram_import_ledger ledger
      LEFT JOIN public.media_miniprograms target
        ON target.id = ledger.target_miniprogram_id AND target.legacy_source_id = ledger.legacy_source_id
      LEFT JOIN public.media_images image ON image.id = ledger.target_media_image_id
      LEFT JOIN public.media_image_blobs blob ON blob.image_id = ledger.target_media_image_id
      WHERE ledger.preflight_id = OLD.id AND ledger.disposition = 'migrated' AND
        (target.id IS NULL OR
         target.thumbnail_media_id <> '' OR target.thumbnail_media_expires_at IS NOT NULL OR
         (ledger.image_disposition IN ('remapped', 'rebuilt') AND target.thumbnail_image_id IS DISTINCT FROM ledger.target_media_image_id) OR
         (ledger.image_disposition = 'rebuilt' AND
           (image.id IS NULL OR image.checksum IS DISTINCT FROM ledger.rebuild_content_digest OR
            blob.image_id IS NULL OR blob.checksum IS DISTINCT FROM ledger.rebuild_content_digest)) OR
         (ledger.image_disposition = 'metadata_url_only' AND (target.thumbnail_image_id IS NOT NULL OR target.thumbnail_image_url = '')));
    IF ledger_rows <> OLD.source_row_count OR ledger_url_only_rows <> OLD.url_only_row_count OR
       ledger_unresolved_rows <> OLD.unresolved_image_row_count OR ledger_invalid_target_rows <> 0 THEN
      RAISE EXCEPTION 'media miniprogram preflight reconciliation is incomplete' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'invalid media miniprogram preflight transition' USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_miniprogram_preflights_transition
BEFORE INSERT OR UPDATE OR DELETE ON public.media_miniprogram_import_preflights
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_miniprogram_preflight_transition_valid();

CREATE TABLE public.media_miniprogram_operation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL,
  actor_scope TEXT NOT NULL,
  business_key TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT media_miniprogram_receipts_operation CHECK (operation IN ('create', 'update', 'delete', 'test-resolve')),
  CONSTRAINT media_miniprogram_receipts_actor CHECK (actor_scope ~ '^admin:[1-9][0-9]*$' AND char_length(actor_scope) <= 200),
  CONSTRAINT media_miniprogram_receipts_business CHECK (business_key = 'create' OR business_key ~ '^[1-9][0-9]*$'),
  CONSTRAINT media_miniprogram_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT media_miniprogram_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT media_miniprogram_receipts_state CHECK (state IN ('in_progress', 'completed')),
  CONSTRAINT media_miniprogram_receipts_completion CHECK (
    (state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR
    (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (operation, actor_scope, business_key, key_digest)
);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_incomplete_media_miniprogram_receipt()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM public.media_miniprogram_operation_receipts
    WHERE id = NEW.id AND state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION 'media miniprogram receipt must complete in its reservation transaction' USING ERRCODE = '23514';
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER media_miniprogram_receipts_complete_before_commit
AFTER INSERT OR UPDATE ON public.media_miniprogram_operation_receipts DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_incomplete_media_miniprogram_receipt();

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_media_miniprogram_receipt_transition_valid()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  IF TG_OP = 'DELETE' OR OLD.state = 'completed' THEN
    RAISE EXCEPTION 'completed media miniprogram receipts are immutable' USING ERRCODE = '55000';
  END IF;
  IF NEW.state <> 'completed' OR NEW.operation IS DISTINCT FROM OLD.operation
     OR NEW.actor_scope IS DISTINCT FROM OLD.actor_scope OR NEW.business_key IS DISTINCT FROM OLD.business_key
     OR NEW.key_digest IS DISTINCT FROM OLD.key_digest OR NEW.payload_digest IS DISTINCT FROM OLD.payload_digest
     OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
    RAISE EXCEPTION 'invalid media miniprogram receipt transition' USING ERRCODE = '55000';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER media_miniprogram_receipts_transition
BEFORE UPDATE OR DELETE ON public.media_miniprogram_operation_receipts
FOR EACH ROW EXECUTE FUNCTION public.aicrm_media_miniprogram_receipt_transition_valid();

-- +goose Down
DROP TRIGGER media_miniprogram_receipts_transition ON public.media_miniprogram_operation_receipts;
DROP TRIGGER media_miniprogram_receipts_complete_before_commit ON public.media_miniprogram_operation_receipts;
DROP TABLE public.media_miniprogram_operation_receipts;
DROP TRIGGER media_miniprogram_import_ledger_immutable ON public.media_miniprogram_import_ledger;
DROP TRIGGER media_miniprogram_import_ledger_validate ON public.media_miniprogram_import_ledger;
DROP TRIGGER media_miniprogram_preflights_transition ON public.media_miniprogram_import_preflights;
DROP TABLE public.media_miniprogram_import_ledger;
DROP TABLE public.media_miniprogram_import_preflights;
DROP TABLE public.media_miniprograms;
DROP TABLE public.media_thumbnail_cache_entries;
DROP FUNCTION public.aicrm_reject_media_miniprogram_import_ledger_mutation();
DROP FUNCTION public.aicrm_validate_media_miniprogram_import_ledger();
DROP FUNCTION public.aicrm_media_miniprogram_preflight_transition_valid();
DROP FUNCTION public.aicrm_media_miniprogram_receipt_transition_valid();
DROP FUNCTION public.aicrm_reject_incomplete_media_miniprogram_receipt();

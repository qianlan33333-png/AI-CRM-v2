-- +goose Up
ALTER TABLE public.wecom_message_archive_records
  ALTER COLUMN customer_id DROP NOT NULL,
  DROP CONSTRAINT wecom_message_archive_records_external_user,
  ADD COLUMN provider_seq BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN identity_state TEXT NOT NULL DEFAULT 'resolved',
  ADD COLUMN source_payload_digest BYTEA NOT NULL DEFAULT decode(repeat('00', 32), 'hex'),
  ADD CONSTRAINT wecom_message_archive_records_provider_seq CHECK (provider_seq >= 0),
  ADD CONSTRAINT wecom_message_archive_records_external_user CHECK (
    btrim(external_userid) = external_userid AND char_length(external_userid) <= 1024
  ),
  ADD CONSTRAINT wecom_message_archive_records_identity_state CHECK (identity_state IN ('resolved', 'unresolved', 'unattributed')),
  ADD CONSTRAINT wecom_message_archive_records_identity_consistency CHECK (
    (identity_state = 'resolved' AND customer_id IS NOT NULL AND customer_id > 0)
    OR (identity_state = 'unresolved' AND customer_id IS NULL AND external_userid <> '')
    OR (identity_state = 'unattributed' AND customer_id IS NULL AND external_userid = '' AND roomid <> '')
  ),
  ADD CONSTRAINT wecom_message_archive_records_source_digest CHECK (octet_length(source_payload_digest) = 32);

CREATE INDEX wecom_message_archive_records_unresolved_idx
  ON public.wecom_message_archive_records (external_userid, sent_at, id)
  WHERE identity_state = 'unresolved';

CREATE TABLE public.wecom_message_archive_sync_state (
  singleton        BOOLEAN PRIMARY KEY DEFAULT TRUE,
  last_seq         BIGINT NOT NULL DEFAULT 0,
  last_success_at  TIMESTAMPTZ,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT wecom_message_archive_sync_state_singleton CHECK (singleton),
  CONSTRAINT wecom_message_archive_sync_state_seq CHECK (last_seq >= 0)
);

INSERT INTO public.wecom_message_archive_sync_state (singleton, last_seq)
VALUES (TRUE, 0)
ON CONFLICT (singleton) DO NOTHING;

CREATE TABLE public.wecom_message_archive_sync_runs (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  state             TEXT NOT NULL DEFAULT 'running',
  cursor_from       BIGINT NOT NULL,
  cursor_to         BIGINT NOT NULL,
  fetched_count     BIGINT NOT NULL DEFAULT 0,
  accepted_count    BIGINT NOT NULL DEFAULT 0,
  inserted_count    BIGINT NOT NULL DEFAULT 0,
  unresolved_count  BIGINT NOT NULL DEFAULT 0,
  failure_code      TEXT NOT NULL DEFAULT '',
  started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at       TIMESTAMPTZ,
  CONSTRAINT wecom_message_archive_sync_runs_state CHECK (state IN ('running', 'succeeded', 'failed')),
  CONSTRAINT wecom_message_archive_sync_runs_counts CHECK (
    cursor_from >= 0 AND cursor_to >= cursor_from
    AND fetched_count >= 0 AND accepted_count >= 0
    AND inserted_count >= 0 AND unresolved_count >= 0
  ),
  CONSTRAINT wecom_message_archive_sync_runs_finish CHECK (
    (state = 'running' AND finished_at IS NULL AND failure_code = '')
    OR (state = 'succeeded' AND finished_at IS NOT NULL AND failure_code = '')
    OR (state = 'failed' AND finished_at IS NOT NULL AND btrim(failure_code) <> '')
  )
);

CREATE INDEX wecom_message_archive_sync_runs_started_idx
  ON public.wecom_message_archive_sync_runs (started_at DESC, id DESC);

-- +goose Down
DROP TABLE public.wecom_message_archive_sync_runs;
DROP TABLE public.wecom_message_archive_sync_state;
DROP INDEX public.wecom_message_archive_records_unresolved_idx;
ALTER TABLE public.wecom_message_archive_records
  DROP CONSTRAINT wecom_message_archive_records_source_digest,
  DROP CONSTRAINT wecom_message_archive_records_identity_consistency,
  DROP CONSTRAINT wecom_message_archive_records_identity_state,
  DROP CONSTRAINT wecom_message_archive_records_provider_seq,
  DROP CONSTRAINT wecom_message_archive_records_external_user,
  DROP COLUMN source_payload_digest,
  DROP COLUMN identity_state,
  DROP COLUMN provider_seq,
  ADD CONSTRAINT wecom_message_archive_records_external_user CHECK (
    btrim(external_userid) = external_userid AND char_length(external_userid) BETWEEN 1 AND 1024
  ),
  ALTER COLUMN customer_id SET NOT NULL;

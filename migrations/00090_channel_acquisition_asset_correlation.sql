-- +goose Up
-- CH02 correlation is an opaque public callback handle, not a credential.  It
-- is stored verbatim so an asynchronous Provider worker can replay it exactly.
ALTER TABLE public.channel_acquisition_asset_bindings
  ADD COLUMN corp_id TEXT,
  ADD COLUMN correlation_key TEXT,
  ADD CONSTRAINT channel_acquisition_asset_correlation_shape CHECK (
    (corp_id IS NULL AND correlation_key IS NULL) OR
    (btrim(corp_id) = corp_id AND char_length(corp_id) BETWEEN 1 AND 128 AND
     correlation_key ~ '^ch02_[A-Za-z0-9_-]{43}$')
  );

CREATE UNIQUE INDEX channel_acquisition_asset_correlation_exact_idx
  ON public.channel_acquisition_asset_bindings(corp_id, correlation_key)
  WHERE corp_id IS NOT NULL AND correlation_key IS NOT NULL;

-- The terminal EER attempt owns the safe crash-recovery evidence.  No raw
-- Provider response, URL, token, or credential is persisted here.
ALTER TABLE public.external_effect_attempts
  ADD COLUMN result_reference_digest TEXT CHECK (result_reference_digest IS NULL OR result_reference_digest ~ '^sha256:[0-9a-f]{64}$'),
  ADD COLUMN business_call_dispatched BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (
    SELECT 1 FROM public.channel_acquisition_asset_bindings
    WHERE corp_id IS NOT NULL OR correlation_key IS NOT NULL
  ) OR EXISTS (
    SELECT 1 FROM public.external_effect_attempts
    WHERE result_reference_digest IS NOT NULL OR business_call_dispatched OR real_external_call_executed
  ) THEN
    RAISE EXCEPTION 'cannot roll back populated CH02 correlation or terminal recovery facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE public.external_effect_attempts
  DROP COLUMN real_external_call_executed,
  DROP COLUMN business_call_dispatched,
  DROP COLUMN result_reference_digest;

DROP INDEX public.channel_acquisition_asset_correlation_exact_idx;
ALTER TABLE public.channel_acquisition_asset_bindings
  DROP CONSTRAINT channel_acquisition_asset_correlation_shape,
  DROP COLUMN correlation_key,
  DROP COLUMN corp_id;

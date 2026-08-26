-- +goose Up
-- Provider asset IDs and public URLs are required for the administrator to
-- download a generated QR code or copy a generated acquisition link. They are
-- bound to one terminal CH02 effect and never contain access tokens.
CREATE TABLE public.channel_acquisition_asset_provider_results (
  effect_id BIGINT PRIMARY KEY REFERENCES public.channel_acquisition_asset_bindings(effect_id) ON DELETE RESTRICT,
  provider_asset_id TEXT NOT NULL CHECK (btrim(provider_asset_id) = provider_asset_id AND char_length(provider_asset_id) BETWEEN 1 AND 1024),
  asset_url TEXT NOT NULL CHECK (btrim(asset_url) = asset_url AND char_length(asset_url) BETWEEN 1 AND 10000 AND asset_url ~ '^https://'),
  created_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.channel_acquisition_asset_provider_results) THEN
    RAISE EXCEPTION 'cannot roll back populated channel acquisition asset provider results' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.channel_acquisition_asset_provider_results;

-- +goose Up
CREATE TABLE public.service_period_member_grid_external_shares (
  service_product_id BIGINT PRIMARY KEY REFERENCES public.products(id) ON DELETE CASCADE,
  share_id TEXT UNIQUE,
  enabled BOOLEAN NOT NULL,
  version BIGINT NOT NULL,
  updated_by BIGINT NOT NULL REFERENCES public.admin_users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT service_period_member_grid_external_shares_state CHECK (
    enabled = (share_id IS NOT NULL)
  ),
  CONSTRAINT service_period_member_grid_external_shares_share_id CHECK (
    share_id IS NULL OR share_id ~ '^[A-Za-z0-9_-]{16,128}$'
  ),
  CONSTRAINT service_period_member_grid_external_shares_version CHECK (version >= 1),
  CONSTRAINT service_period_member_grid_external_shares_timestamps CHECK (updated_at >= created_at)
);

COMMENT ON TABLE public.service_period_member_grid_external_shares IS
  'Current revocable Member Grid public-share state; share_id is opaque and is not the bearer token.';

-- +goose Down
LOCK TABLE public.service_period_member_grid_external_shares IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.service_period_member_grid_external_shares LIMIT 1) THEN
    RAISE EXCEPTION 'cannot roll back populated Member Grid external share state' USING ERRCODE = '55000';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.service_period_member_grid_external_shares;

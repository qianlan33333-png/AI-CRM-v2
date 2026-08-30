-- +goose Up
ALTER TABLE public.tag_groups
  ADD COLUMN wecom_group_id TEXT UNIQUE;

-- +goose Down
ALTER TABLE public.tag_groups
  DROP COLUMN wecom_group_id;

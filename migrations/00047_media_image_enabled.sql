-- +goose Up
ALTER TABLE public.media_images
  ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down
ALTER TABLE public.media_images
  DROP COLUMN enabled;

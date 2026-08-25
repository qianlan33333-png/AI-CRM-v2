-- +goose Up
-- Media Content Package & Delivery Binding is single-enterprise/local only.
-- It reuses canonical media IDs and stores neither public URLs nor provider payloads.
CREATE TABLE public.media_content_packages (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL CHECK (name <> '' AND btrim(name) = name AND char_length(name) <= 200),
  content_text TEXT NOT NULL DEFAULT '' CHECK (btrim(content_text) = content_text AND char_length(content_text) <= 10000),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

-- Exactly one canonical local material is named by each ref. FKs intentionally
-- protect all libraries from deletion while a package snapshot uses the item.
CREATE TABLE public.media_content_package_refs (
  package_id BIGINT NOT NULL REFERENCES public.media_content_packages(id) ON DELETE CASCADE,
  position INTEGER NOT NULL CHECK (position BETWEEN 1 AND 100),
  ref_kind TEXT NOT NULL CHECK (ref_kind IN ('image','attachment','miniprogram','group_invite')),
  image_id BIGINT REFERENCES public.media_images(id) ON DELETE RESTRICT,
  attachment_id BIGINT REFERENCES public.media_attachments(id) ON DELETE RESTRICT,
  miniprogram_id BIGINT REFERENCES public.media_miniprograms(id) ON DELETE RESTRICT,
  group_invite_id BIGINT REFERENCES public.media_group_invites(id) ON DELETE RESTRICT,
  PRIMARY KEY (package_id, position),
  CONSTRAINT media_content_package_refs_shape CHECK (
    (ref_kind = 'image' AND image_id IS NOT NULL AND attachment_id IS NULL AND miniprogram_id IS NULL AND group_invite_id IS NULL)
    OR (ref_kind = 'attachment' AND image_id IS NULL AND attachment_id IS NOT NULL AND miniprogram_id IS NULL AND group_invite_id IS NULL)
    OR (ref_kind = 'miniprogram' AND image_id IS NULL AND attachment_id IS NULL AND miniprogram_id IS NOT NULL AND group_invite_id IS NULL)
    OR (ref_kind = 'group_invite' AND image_id IS NULL AND attachment_id IS NULL AND miniprogram_id IS NULL AND group_invite_id IS NOT NULL)
  )
);

CREATE TABLE public.media_content_package_mutation_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('create','update','delete','bind','unbind','upload_initiate','upload_part','upload_complete')),
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  result_snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE (operation, actor_id, key_digest)
);

CREATE TABLE public.media_attachment_uploads (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  file_name TEXT NOT NULL CHECK (file_name <> '' AND file_name !~ '[\\/[:cntrl:]]'),
  name TEXT NOT NULL CHECK (name <> ''),
  description TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(tags) = 'array'),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  expected_size INTEGER NOT NULL CHECK (expected_size BETWEEN 1 AND 10485760),
  expected_digest BYTEA NOT NULL CHECK (octet_length(expected_digest) = 32),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  state TEXT NOT NULL CHECK (state IN ('initiated','completed')),
  attachment_id BIGINT UNIQUE REFERENCES public.media_attachments(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CHECK ((state = 'initiated' AND attachment_id IS NULL AND completed_at IS NULL) OR (state = 'completed' AND attachment_id IS NOT NULL AND completed_at IS NOT NULL))
);
CREATE TABLE public.media_attachment_upload_parts (
  upload_id BIGINT NOT NULL REFERENCES public.media_attachment_uploads(id) ON DELETE CASCADE,
  part_number INTEGER NOT NULL CHECK (part_number BETWEEN 1 AND 1000),
  digest BYTEA NOT NULL CHECK (octet_length(digest) = 32),
  content BYTEA NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 10485760),
  created_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (upload_id, part_number)
);

-- A mutable local composition binds an existing campaign/plan/package to one
-- enabled group-invite. It never means the invite has been sent.
CREATE TABLE public.media_campaign_delivery_bindings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  campaign_code TEXT NOT NULL REFERENCES public.cloud_campaigns(campaign_code) ON DELETE RESTRICT,
  plan_id TEXT NOT NULL REFERENCES public.cloud_campaign_touch_plans(id) ON DELETE RESTRICT,
  package_id BIGINT NOT NULL REFERENCES public.media_content_packages(id) ON DELETE RESTRICT,
  group_invite_id BIGINT NOT NULL REFERENCES public.media_group_invites(id) ON DELETE RESTRICT,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_by BIGINT NOT NULL CHECK (created_by > 0),
  updated_by BIGINT NOT NULL CHECK (updated_by > 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
  UNIQUE (campaign_code, plan_id)
);

-- Outbound-owned immutable local acceptance snapshot. This deliberately does
-- not enqueue or invoke a provider; state may only represent local/EER facts.
CREATE TABLE public.outbound_media_acceptances (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  handoff_id BIGINT NOT NULL REFERENCES public.outbound_campaign_handoffs(id) ON DELETE RESTRICT,
  binding_id BIGINT NOT NULL REFERENCES public.media_campaign_delivery_bindings(id) ON DELETE RESTRICT,
  package_id BIGINT NOT NULL REFERENCES public.media_content_packages(id) ON DELETE RESTRICT,
  media_refs JSONB NOT NULL CHECK (jsonb_typeof(media_refs) = 'array'),
  source_digest TEXT NOT NULL CHECK (source_digest ~ '^sha256:[0-9a-f]{64}$'),
  payload_digest TEXT NOT NULL CHECK (payload_digest ~ '^sha256:[0-9a-f]{64}$'),
  external_effect_id BIGINT UNIQUE REFERENCES public.external_effects(id) ON DELETE RESTRICT,
  state TEXT NOT NULL CHECK (state IN ('accepted','queued','attempted','outcome_unknown','reconciled')),
  provider_execution_eligible BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT provider_execution_eligible),
  real_external_call_executed BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT real_external_call_executed),
  delivery_proven BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT delivery_proven),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (handoff_id, binding_id)
);
CREATE INDEX media_content_package_refs_image_idx ON public.media_content_package_refs(image_id) WHERE image_id IS NOT NULL;
CREATE INDEX media_content_package_refs_attachment_idx ON public.media_content_package_refs(attachment_id) WHERE attachment_id IS NOT NULL;
CREATE INDEX media_content_package_refs_miniprogram_idx ON public.media_content_package_refs(miniprogram_id) WHERE miniprogram_id IS NOT NULL;
CREATE INDEX media_content_package_refs_group_invite_idx ON public.media_content_package_refs(group_invite_id) WHERE group_invite_id IS NOT NULL;

-- +goose Down
LOCK TABLE public.outbound_media_acceptances, public.media_campaign_delivery_bindings, public.media_attachment_uploads, public.media_content_packages IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.outbound_media_acceptances) OR EXISTS (SELECT 1 FROM public.media_campaign_delivery_bindings)
     OR EXISTS (SELECT 1 FROM public.media_attachment_uploads) OR EXISTS (SELECT 1 FROM public.media_content_packages) THEN
    RAISE EXCEPTION 'cannot roll back populated media content package and delivery facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.outbound_media_acceptances;
DROP TABLE public.media_campaign_delivery_bindings;
DROP TABLE public.media_attachment_upload_parts;
DROP TABLE public.media_attachment_uploads;
DROP TABLE public.media_content_package_mutation_receipts;
DROP TABLE public.media_content_package_refs;
DROP TABLE public.media_content_packages;

-- +goose Up
-- Active V1 audience definitions are restored as paused, editable V2 packages.
-- Their initial definition selects only the verified frozen V1 membership
-- snapshot; no V1 SQL, scheduler, send, or Provider effect is executed.
CREATE TABLE public.ai_audience_v1_editable_group_projections (
  group_history_id BIGINT PRIMARY KEY
    REFERENCES public.segment_v1_audience_groups(id) ON DELETE RESTRICT,
  group_id BIGINT NOT NULL UNIQUE
    REFERENCES public.ai_audience_package_groups(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT ai_audience_v1_editable_group_projection_created CHECK (created_at > '-infinity'::timestamptz)
);

CREATE TABLE public.ai_audience_v1_editable_package_projections (
  package_history_id BIGINT PRIMARY KEY
    REFERENCES public.segment_v1_audience_packages(id) ON DELETE RESTRICT,
  segment_id BIGINT NOT NULL UNIQUE
    REFERENCES public.ai_audience_package_metadata(segment_id) ON DELETE RESTRICT,
  source_member_count BIGINT NOT NULL,
  mapped_member_count BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT ai_audience_v1_editable_package_projection_counts CHECK (
    source_member_count >= 0
    AND mapped_member_count >= 0
    AND mapped_member_count <= source_member_count
  ),
  CONSTRAINT ai_audience_v1_editable_package_projection_created CHECK (created_at > '-infinity'::timestamptz)
);

-- Product configuration is restored separately from immutable payment/order
-- history. The projection row prevents a replay from overwriting later V2
-- edits, while legacy_materials_cleared_at proves old image/material references
-- were deliberately excluded from the current Product.
CREATE TABLE public.product_v1_editable_projections (
  product_id BIGINT PRIMARY KEY REFERENCES public.products(id) ON DELETE RESTRICT,
  source_id BIGINT NOT NULL UNIQUE,
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  configuration_projected_at TIMESTAMPTZ NOT NULL,
  service_period_definition_id BIGINT UNIQUE
    REFERENCES public.product_service_period_history(id) ON DELETE RESTRICT,
  service_period_projected_at TIMESTAMPTZ,
  legacy_materials_cleared_at TIMESTAMPTZ,
  cleared_material_reference_count INTEGER NOT NULL DEFAULT 0 CHECK (cleared_material_reference_count >= 0),
  CONSTRAINT product_v1_editable_projection_times CHECK (
    configuration_projected_at > '-infinity'::timestamptz
    AND (service_period_projected_at IS NULL OR service_period_projected_at >= configuration_projected_at)
    AND (legacy_materials_cleared_at IS NULL OR legacy_materials_cleared_at >= configuration_projected_at)
  )
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.ai_audience_v1_editable_package_projections)
     OR EXISTS (SELECT 1 FROM public.ai_audience_v1_editable_group_projections)
     OR EXISTS (SELECT 1 FROM public.product_v1_editable_projections) THEN
    RAISE EXCEPTION 'refusing to remove populated V1 editable projections';
  END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE public.product_v1_editable_projections;
DROP TABLE public.ai_audience_v1_editable_package_projections;
DROP TABLE public.ai_audience_v1_editable_group_projections;

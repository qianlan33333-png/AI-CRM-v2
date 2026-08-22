-- +goose Up
CREATE TABLE public.service_period_member_views (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  service_product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  state TEXT NOT NULL,
  sort TEXT NOT NULL,
  columns TEXT[] NOT NULL,
  source_view_id BIGINT,
  version BIGINT NOT NULL DEFAULT 1,
  created_by BIGINT NOT NULL REFERENCES public.admin_users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT service_period_member_views_name CHECK (
    btrim(name) = name AND name <> '' AND char_length(name) <= 200
  ),
  CONSTRAINT service_period_member_views_state CHECK (state IN ('active', 'revoked', 'all')),
  CONSTRAINT service_period_member_views_sort CHECK (sort = 'granted_at_desc'),
  CONSTRAINT service_period_member_views_columns_count CHECK (cardinality(columns) BETWEEN 1 AND 8),
  CONSTRAINT service_period_member_views_columns_one_dimensional CHECK (
    array_ndims(columns) = 1 AND array_lower(columns, 1) = 1
  ),
  CONSTRAINT service_period_member_views_columns_not_null CHECK (array_position(columns, NULL) IS NULL),
  CONSTRAINT service_period_member_views_columns_closed CHECK (
    columns <@ ARRAY[
      'entitlement_id', 'product_id', 'state', 'version',
      'granted_at', 'revoked_at', 'display_name', 'masked_mobile'
    ]::TEXT[]
  ),
  CONSTRAINT service_period_member_views_columns_unique CHECK (
    cardinality(array_positions(columns, 'entitlement_id')) <= 1
    AND cardinality(array_positions(columns, 'product_id')) <= 1
    AND cardinality(array_positions(columns, 'state')) <= 1
    AND cardinality(array_positions(columns, 'version')) <= 1
    AND cardinality(array_positions(columns, 'granted_at')) <= 1
    AND cardinality(array_positions(columns, 'revoked_at')) <= 1
    AND cardinality(array_positions(columns, 'display_name')) <= 1
    AND cardinality(array_positions(columns, 'masked_mobile')) <= 1
  ),
  CONSTRAINT service_period_member_views_source_positive CHECK (source_view_id IS NULL OR source_view_id > 0),
  CONSTRAINT service_period_member_views_source_not_self CHECK (source_view_id IS NULL OR source_view_id <> id),
  CONSTRAINT service_period_member_views_version CHECK (version >= 1),
  CONSTRAINT service_period_member_views_timestamps CHECK (updated_at >= created_at),
  CONSTRAINT service_period_member_views_product_id_id_unique UNIQUE (service_product_id, id),
  CONSTRAINT service_period_member_views_source_same_product
    FOREIGN KEY (service_product_id, source_view_id)
    REFERENCES public.service_period_member_views (service_product_id, id)
    ON DELETE SET NULL (source_view_id)
);

CREATE INDEX service_period_member_views_source_view_id_idx
  ON public.service_period_member_views (service_product_id, source_view_id)
  WHERE source_view_id IS NOT NULL;

COMMENT ON TABLE public.service_period_member_views IS
  'Local saved member-grid views; no arbitrary SQL, public share, or member mutation.';

CREATE TABLE public.service_period_member_grid_collaborators (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  service_product_id BIGINT NOT NULL REFERENCES public.products(id) ON DELETE CASCADE,
  staff_id BIGINT NOT NULL REFERENCES public.staff(id) ON DELETE RESTRICT,
  permission TEXT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  invited_by BIGINT NOT NULL REFERENCES public.admin_users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT service_period_member_grid_collaborators_permission CHECK (permission IN ('view', 'edit')),
  CONSTRAINT service_period_member_grid_collaborators_version CHECK (version >= 1),
  CONSTRAINT service_period_member_grid_collaborators_timestamps CHECK (updated_at >= created_at),
  CONSTRAINT service_period_member_grid_collaborators_product_staff_unique UNIQUE (service_product_id, staff_id)
);

CREATE INDEX service_period_member_grid_collaborators_product_id_idx
  ON public.service_period_member_grid_collaborators (service_product_id, id ASC);

COMMENT ON TABLE public.service_period_member_grid_collaborators IS
  'Local staff metadata only; edit does not grant products.write and sends no invitation.';

-- +goose Down
DROP TABLE public.service_period_member_grid_collaborators;
DROP TABLE public.service_period_member_views;

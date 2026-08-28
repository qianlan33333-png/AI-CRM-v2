-- +goose Up
CREATE TABLE public.group_ops_v1_history_plans (
  plan_id BIGINT PRIMARY KEY REFERENCES public.group_ops_plans(id) ON DELETE RESTRICT,
  source_plan_id BIGINT NOT NULL UNIQUE CHECK (source_plan_id > 0),
  source_code TEXT NOT NULL,
  plan_type TEXT NOT NULL,
  original_status TEXT NOT NULL,
  owner_staff_id BIGINT REFERENCES public.staff(id) ON DELETE RESTRICT,
  archived_at TIMESTAMPTZ
);

CREATE TABLE public.group_ops_v1_history_directory (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('group_chats', 'wecom_group_chat_snapshots')),
  source_id BIGINT,
  chat_reference TEXT NOT NULL CHECK (chat_reference <> ''),
  display_name TEXT,
  owner_staff_id BIGINT REFERENCES public.staff(id) ON DELETE RESTRICT,
  owner_name TEXT,
  member_count INTEGER CHECK (member_count >= 0),
  internal_member_count INTEGER CHECK (internal_member_count >= 0),
  external_member_count INTEGER CHECK (external_member_count >= 0),
  original_status TEXT NOT NULL,
  recorded_at TIMESTAMPTZ NOT NULL,
  CHECK (
    (source_kind = 'group_chats' AND source_id > 0 AND source_id IS NOT NULL AND member_count IS NOT NULL AND internal_member_count IS NULL AND external_member_count IS NULL AND owner_name IS NULL)
    OR (source_kind = 'wecom_group_chat_snapshots' AND source_id IS NULL AND member_count IS NULL AND internal_member_count IS NOT NULL AND external_member_count IS NOT NULL)
  )
);
CREATE UNIQUE INDEX group_ops_v1_history_directory_source_id ON public.group_ops_v1_history_directory(source_id) WHERE source_kind = 'group_chats';
CREATE UNIQUE INDEX group_ops_v1_history_directory_snapshot ON public.group_ops_v1_history_directory(chat_reference) WHERE source_kind = 'wecom_group_chat_snapshots';

CREATE TABLE public.group_ops_v1_history_groups (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_group_id BIGINT NOT NULL UNIQUE CHECK (source_group_id > 0),
  source_plan_id BIGINT NOT NULL CHECK (source_plan_id > 0),
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_v1_history_plans(plan_id) ON DELETE RESTRICT,
  chat_reference TEXT NOT NULL CHECK (chat_reference <> ''),
  display_name TEXT NOT NULL,
  owner_staff_id BIGINT REFERENCES public.staff(id) ON DELETE RESTRICT,
  internal_member_count INTEGER NOT NULL CHECK (internal_member_count >= 0),
  external_member_count INTEGER NOT NULL CHECK (external_member_count >= 0),
  original_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  removed_at TIMESTAMPTZ
);
CREATE INDEX group_ops_v1_history_groups_plan ON public.group_ops_v1_history_groups(plan_id, id);

CREATE TABLE public.group_ops_v1_history_nodes (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_node_id BIGINT NOT NULL UNIQUE CHECK (source_node_id > 0),
  source_plan_id BIGINT NOT NULL CHECK (source_plan_id > 0),
  plan_id BIGINT NOT NULL REFERENCES public.group_ops_v1_history_plans(plan_id) ON DELETE RESTRICT,
  day_index INTEGER NOT NULL CHECK (day_index >= 0),
  trigger_time TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  original_status TEXT NOT NULL,
  content_package JSONB NOT NULL CHECK (jsonb_typeof(content_package) = 'object'),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX group_ops_v1_history_nodes_plan ON public.group_ops_v1_history_nodes(plan_id, sort_order, id);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS(SELECT 1 FROM public.group_ops_v1_history_plans)
    OR EXISTS(SELECT 1 FROM public.group_ops_v1_history_directory)
    OR EXISTS(SELECT 1 FROM public.group_ops_v1_history_groups)
    OR EXISTS(SELECT 1 FROM public.group_ops_v1_history_nodes) THEN
    RAISE EXCEPTION 'Group Ops historical data requires snapshot restore, not destructive down migration';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.group_ops_v1_history_nodes;
DROP TABLE public.group_ops_v1_history_groups;
DROP TABLE public.group_ops_v1_history_directory;
DROP TABLE public.group_ops_v1_history_plans;

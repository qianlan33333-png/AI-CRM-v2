-- +goose Up
-- Read-only source facts, not current attribution, assignment or permissions.
CREATE TABLE public.channel_historical_contacts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  channel_id BIGINT NOT NULL REFERENCES public.channels(id),
  source_contact_id BIGINT NOT NULL UNIQUE CHECK (source_contact_id > 0),
  customer_id BIGINT REFERENCES public.customers(id),
  owner_reference TEXT NOT NULL,
  first_entered_at TIMESTAMPTZ NOT NULL,
  last_entered_at TIMESTAMPTZ NOT NULL CHECK (last_entered_at >= first_entered_at),
  enter_count INTEGER NOT NULL CHECK (enter_count > 0),
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);
CREATE INDEX channel_historical_contacts_page_idx ON public.channel_historical_contacts(channel_id, id);

CREATE TABLE public.channel_historical_assignees (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  channel_id BIGINT NOT NULL REFERENCES public.channels(id),
  source_assignee_id BIGINT NOT NULL UNIQUE CHECK (source_assignee_id > 0),
  staff_reference TEXT NOT NULL,
  display_name_snapshot TEXT NOT NULL,
  priority INTEGER NOT NULL CHECK (priority >= 0),
  ratio_percent INTEGER,
  max_scans_24h INTEGER,
  status TEXT NOT NULL CHECK (status <> ''),
  source_created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL,
  source_updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL CHECK (source_updated_at >= source_created_at)
);
CREATE INDEX channel_historical_assignees_page_idx ON public.channel_historical_assignees(channel_id, id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.channel_historical_contacts)
    OR EXISTS (SELECT 1 FROM public.channel_historical_assignees) THEN
    RAISE EXCEPTION 'cannot drop populated channel history; restore a pre-import snapshot instead';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.channel_historical_assignees;
DROP TABLE public.channel_historical_contacts;

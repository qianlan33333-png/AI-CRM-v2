-- +goose Up
CREATE TABLE customer_events (
  id          BIGINT GENERATED ALWAYS AS IDENTITY,
  customer_id BIGINT NOT NULL REFERENCES customers(id),
  event_type  TEXT NOT NULL,
  payload     JSONB NOT NULL DEFAULT '{}',
  actor       TEXT NOT NULL DEFAULT 'system',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (occurred_at, id),
  CONSTRAINT customer_events_type_not_blank CHECK (btrim(event_type) <> ''),
  CONSTRAINT customer_events_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT customer_events_actor_length CHECK (
    btrim(actor) <> '' AND char_length(actor) <= 200
  )
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_customer_events_customer_timeline
  ON customer_events (customer_id, occurred_at DESC, id DESC);
CREATE INDEX idx_customer_events_occurred_brin
  ON customer_events USING BRIN (occurred_at) WITH (pages_per_range = 32);

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_ensure_customer_event_partitions(
  anchor TIMESTAMPTZ,
  future_months INTEGER
) RETURNS VOID
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
DECLARE
  month_offset INTEGER;
  month_start TIMESTAMPTZ;
  month_end TIMESTAMPTZ;
  partition_name TEXT;
BEGIN
  IF anchor IS NULL OR future_months IS NULL OR future_months < 0 OR future_months > 36 THEN
    RAISE EXCEPTION 'invalid customer event partition request'
      USING ERRCODE = '22023';
  END IF;

  PERFORM pg_advisory_xact_lock(
    hashtextextended('aicrm.customer_events.partitions', 0)
  );

  FOR month_offset IN 0..future_months LOOP
    month_start := (
      date_trunc('month', anchor AT TIME ZONE 'UTC')
      + make_interval(months => month_offset)
    ) AT TIME ZONE 'UTC';
    month_end := month_start + INTERVAL '1 month';
    partition_name := 'customer_events_' ||
      to_char(month_start AT TIME ZONE 'UTC', 'YYYY_MM');

    EXECUTE format(
      'CREATE TABLE IF NOT EXISTS public.%I PARTITION OF public.customer_events FOR VALUES FROM (%L) TO (%L)',
      partition_name,
      month_start,
      month_end
    );
  END LOOP;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_reject_customer_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
  RAISE EXCEPTION 'customer_events is append-only'
    USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER customer_events_append_only
BEFORE UPDATE OR DELETE ON public.customer_events
FOR EACH ROW EXECUTE FUNCTION public.aicrm_reject_customer_event_mutation();

SELECT public.aicrm_ensure_customer_event_partitions(now(), 3);

-- +goose Down
DROP TABLE public.customer_events;
DROP FUNCTION IF EXISTS public.aicrm_reject_customer_event_mutation();
DROP FUNCTION IF EXISTS public.aicrm_ensure_customer_event_partitions(TIMESTAMPTZ, INTEGER);

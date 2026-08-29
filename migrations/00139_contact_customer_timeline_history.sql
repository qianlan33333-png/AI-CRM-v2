-- +goose Up
-- Immutable V1 timeline observations. This table never feeds customer_events.
CREATE TABLE contact_v1_customer_timeline_history (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_key_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(source_key_digest) = 32),
  source_payload_digest BYTEA NOT NULL CHECK (octet_length(source_payload_digest) = 32),
  source_field_digest BYTEA NOT NULL CHECK (octet_length(source_field_digest) = 32),
  source_id BIGINT NOT NULL,
  event_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  event_time TIMESTAMPTZ NOT NULL,
  title TEXT NOT NULL,
  summary TEXT NOT NULL,
  source_table TEXT NOT NULL,
  source_value TEXT NOT NULL,
  metadata_json TEXT NOT NULL CHECK (metadata_json::jsonb IS NOT NULL),
  created_at TIMESTAMPTZ NOT NULL,
  unionid TEXT NOT NULL,
  customer_id BIGINT REFERENCES customers(id),
  UNIQUE (source_id)
);
CREATE INDEX contact_v1_customer_timeline_history_event_time
  ON contact_v1_customer_timeline_history (event_time DESC, id DESC);
CREATE INDEX contact_v1_customer_timeline_history_customer
  ON contact_v1_customer_timeline_history (customer_id, event_time DESC, id DESC)
  WHERE customer_id IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION public.aicrm_contact_v1_customer_timeline_history_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
  RAISE EXCEPTION 'Customer timeline history is immutable' USING ERRCODE='55000';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER contact_v1_customer_timeline_history_immutable
BEFORE UPDATE OR DELETE ON public.contact_v1_customer_timeline_history
FOR EACH ROW EXECUTE FUNCTION public.aicrm_contact_v1_customer_timeline_history_immutable();

-- +goose Down
LOCK TABLE public.contact_v1_customer_timeline_history,
  public.v1_domain_import_receipts,
  public.v1_domain_import_reconciliation_receipts
  IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.contact_v1_customer_timeline_history)
     OR EXISTS (
       SELECT 1 FROM public.v1_domain_import_receipts
       WHERE import_version = 'v1-customer-timeline-history-a1'
     )
     OR EXISTS (
       SELECT 1 FROM public.v1_domain_import_reconciliation_receipts
       WHERE import_version = 'v1-customer-timeline-history-a1'
     ) THEN
    RAISE EXCEPTION 'Customer timeline history requires snapshot restore, not destructive down migration';
  END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER contact_v1_customer_timeline_history_immutable ON public.contact_v1_customer_timeline_history;
DROP FUNCTION public.aicrm_contact_v1_customer_timeline_history_immutable();
DROP TABLE public.contact_v1_customer_timeline_history;

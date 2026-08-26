-- +goose Up
CREATE TABLE public.cloud_campaign_touch_plan_recipient_reviews (
  plan_id TEXT NOT NULL,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  campaign_code TEXT NOT NULL,
  message_override TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('pending_review', 'approved', 'rejected')),
  version BIGINT NOT NULL CHECK (version > 0),
  updated_by_actor_id BIGINT NOT NULL CHECK (updated_by_actor_id > 0),
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (plan_id, customer_id),
  FOREIGN KEY (plan_id, customer_id) REFERENCES public.cloud_campaign_touch_plan_targets(plan_id, customer_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  FOREIGN KEY (plan_id) REFERENCES public.cloud_campaign_touch_plan_reviews(plan_id) ON DELETE RESTRICT
);

CREATE TABLE public.cloud_campaign_touch_plan_recipient_review_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id BIGINT NOT NULL CHECK (actor_id > 0),
  operation TEXT NOT NULL CHECK (operation IN ('recipient_message_override', 'recipient_approve', 'recipient_reject')),
  key_digest BYTEA NOT NULL CHECK (octet_length(key_digest) = 32),
  payload_digest BYTEA NOT NULL CHECK (octet_length(payload_digest) = 32),
  plan_id TEXT NOT NULL,
  campaign_code TEXT NOT NULL,
  customer_id BIGINT NOT NULL CHECK (customer_id > 0),
  event_id BIGINT REFERENCES public.event_log(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved', 'completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  FOREIGN KEY (plan_id, customer_id) REFERENCES public.cloud_campaign_touch_plan_targets(plan_id, customer_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT cloud_campaign_touch_plan_recipient_review_receipts_completion CHECK (
    (state = 'reserved' AND event_id IS NULL AND result_snapshot IS NULL AND completed_at IS NULL)
    OR (state = 'completed' AND event_id IS NOT NULL AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)
  ),
  UNIQUE (actor_id, key_digest),
  UNIQUE (event_id)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plan_recipient_review_receipts)
     OR EXISTS (SELECT 1 FROM public.cloud_campaign_touch_plan_recipient_reviews) THEN
    RAISE EXCEPTION 'cannot remove populated campaign recipient review tables';
  END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE public.cloud_campaign_touch_plan_recipient_review_receipts;
DROP TABLE public.cloud_campaign_touch_plan_recipient_reviews;

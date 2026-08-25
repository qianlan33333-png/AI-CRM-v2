-- +goose Up
-- CH03 persists only the typed, non-secret portion of a verified callback.
-- WelcomeCode, Source and FailReason are represented by presence/digest only.
ALTER TABLE public.wecom_contact_inbox
  ADD COLUMN external_contact_change_type TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_contact_wecom_userid TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_contact_state TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_contact_welcome_present BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN external_contact_welcome_digest TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_contact_source_digest TEXT NOT NULL DEFAULT '',
  ADD COLUMN external_contact_fail_reason_digest TEXT NOT NULL DEFAULT '',
  ADD CONSTRAINT wecom_contact_inbox_external_contact_typed_shape CHECK (
    (external_contact_change_type = '' AND external_contact_wecom_userid = ''
      AND external_contact_state = '' AND NOT external_contact_welcome_present
      AND external_contact_welcome_digest = '' AND external_contact_source_digest = ''
      AND external_contact_fail_reason_digest = '')
    OR
    (source = 'callback_inbox'
      AND external_contact_change_type ~ '^[a-z0-9_]{1,128}$'
      AND btrim(external_contact_wecom_userid) = external_contact_wecom_userid
      AND char_length(external_contact_wecom_userid) <= 1024
      AND btrim(external_contact_state) = external_contact_state
      AND char_length(external_contact_state) <= 512
      AND (external_contact_welcome_present = (external_contact_welcome_digest <> ''))
      AND (external_contact_welcome_digest = '' OR external_contact_welcome_digest ~ '^sha256:[0-9a-f]{64}$')
      AND (external_contact_source_digest = '' OR external_contact_source_digest ~ '^sha256:[0-9a-f]{64}$')
      AND (external_contact_fail_reason_digest = '' OR external_contact_fail_reason_digest ~ '^sha256:[0-9a-f]{64}$'))
  );

-- This Contact-owned receipt is deliberately separate from the shared inbox.
-- It has no external user identifier, callback secret, Provider response, or
-- mutable Customer projection.  An event id is an audit reference: customer
-- events are partitioned, so PostgreSQL cannot enforce a single-column FK.
CREATE TABLE public.channel_acquisition_entrant_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  inbox_id BIGINT NOT NULL UNIQUE REFERENCES public.wecom_contact_inbox(id) ON DELETE RESTRICT,
  input_digest TEXT NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (status IN ('correlated', 'attributed', 'pending_identity', 'unmatched_asset', 'ambiguous_asset', 'conflict', 'ignored', 'reconciled')),
  effect_id BIGINT,
  channel_id BIGINT,
  asset_kind TEXT,
  asset_version BIGINT,
  customer_id BIGINT REFERENCES public.customers(id) ON DELETE RESTRICT,
  customer_event_id BIGINT,
  customer_event_occurred_at TIMESTAMPTZ,
  occurred_at TIMESTAMPTZ NOT NULL,
  reconciled_at TIMESTAMPTZ,
  reconcile_reason TEXT NOT NULL DEFAULT '' CHECK (char_length(reconcile_reason) <= 200 AND btrim(reconcile_reason) = reconcile_reason),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (effect_id, channel_id, asset_kind, asset_version)
    REFERENCES public.channel_acquisition_asset_bindings(effect_id, channel_id, asset_kind, asset_version) ON DELETE RESTRICT,
  FOREIGN KEY (customer_event_occurred_at, customer_event_id)
    REFERENCES public.customer_events(occurred_at, id) ON DELETE RESTRICT,
  CONSTRAINT channel_acquisition_entrant_receipt_shape CHECK (
    ((effect_id IS NULL AND channel_id IS NULL AND asset_kind IS NULL AND asset_version IS NULL)
      OR (effect_id IS NOT NULL AND channel_id IS NOT NULL AND asset_kind IN ('contact_way_qrcode', 'customer_acquisition_link') AND asset_version > 0))
    AND ((customer_id IS NULL AND customer_event_id IS NULL AND customer_event_occurred_at IS NULL)
      OR (customer_id IS NOT NULL AND customer_event_id IS NOT NULL AND customer_event_occurred_at IS NOT NULL))
    AND (status NOT IN ('attributed', 'reconciled') OR (effect_id IS NOT NULL AND customer_id IS NOT NULL))
    AND (status <> 'attributed' OR customer_event_id IS NOT NULL)
    AND ((status <> 'reconciled' AND reconciled_at IS NULL AND reconcile_reason = '')
      OR (status = 'reconciled' AND reconciled_at IS NOT NULL AND reconcile_reason <> ''))
  )
);
CREATE INDEX channel_acquisition_entrant_receipts_status_idx
  ON public.channel_acquisition_entrant_receipts(status, occurred_at DESC, id DESC);

-- +goose Down
LOCK TABLE public.channel_acquisition_entrant_receipts, public.wecom_contact_inbox IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM public.channel_acquisition_entrant_receipts)
    OR EXISTS (SELECT 1 FROM public.wecom_contact_inbox WHERE external_contact_change_type <> '') THEN
    RAISE EXCEPTION 'cannot roll back populated CH03 entrant facts' USING ERRCODE = '55000';
  END IF;
END $$;
-- +goose StatementEnd
DROP TABLE public.channel_acquisition_entrant_receipts;
ALTER TABLE public.wecom_contact_inbox
  DROP CONSTRAINT wecom_contact_inbox_external_contact_typed_shape,
  DROP COLUMN external_contact_fail_reason_digest,
  DROP COLUMN external_contact_source_digest,
  DROP COLUMN external_contact_welcome_digest,
  DROP COLUMN external_contact_welcome_present,
  DROP COLUMN external_contact_state,
  DROP COLUMN external_contact_wecom_userid,
  DROP COLUMN external_contact_change_type;

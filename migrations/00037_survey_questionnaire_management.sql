-- +goose Up
-- F01B keeps F01A create receipts immutable and adds an independently owned
-- receipt ledger for definition management. It adds no deployment
-- discriminator and creates no submission, identity, or external-effect state.
CREATE TABLE public.questionnaire_management_receipts (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  operation TEXT NOT NULL CHECK (operation IN ('update', 'enable', 'disable', 'delete')),
  actor_scope TEXT NOT NULL,
  key_digest BYTEA NOT NULL,
  payload_digest BYTEA NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  result_snapshot JSONB,
  created_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  CONSTRAINT questionnaire_management_receipts_actor CHECK (btrim(actor_scope) = actor_scope AND actor_scope <> '' AND char_length(actor_scope) <= 200),
  CONSTRAINT questionnaire_management_receipts_key CHECK (octet_length(key_digest) = 32),
  CONSTRAINT questionnaire_management_receipts_payload CHECK (octet_length(payload_digest) = 32),
  CONSTRAINT questionnaire_management_receipts_completion CHECK ((state = 'in_progress' AND result_snapshot IS NULL AND completed_at IS NULL) OR (state = 'completed' AND result_snapshot IS NOT NULL AND completed_at IS NOT NULL)),
  UNIQUE (operation, actor_scope, key_digest)
);

-- +goose Down
DROP TABLE IF EXISTS public.questionnaire_management_receipts;

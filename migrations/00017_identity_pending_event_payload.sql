-- +goose Up
ALTER TABLE pending_events
  ADD COLUMN payload JSONB;

ALTER TABLE pending_events
  DROP CONSTRAINT pending_events_kind_shape;

ALTER TABLE pending_events
  ADD CONSTRAINT pending_events_kind_shape CHECK (
    (kind = 'merge_review' AND event_type IS NULL AND payload IS NULL AND idempotency_key IS NULL AND occurred_at IS NULL AND cardinality(candidate_customer_ids) = 2 AND candidate_customer_ids[1] < candidate_customer_ids[2] AND review_fingerprint IS NOT NULL AND octet_length(review_fingerprint) = 16 AND fingerprint_key_version IS NOT NULL AND fingerprint_key_version > 0)
    OR
    (kind <> 'merge_review' AND event_type IS NOT NULL AND btrim(event_type) <> '' AND payload IS NOT NULL AND jsonb_typeof(payload) = 'object' AND idempotency_key IS NOT NULL AND btrim(idempotency_key) <> '' AND occurred_at IS NOT NULL AND cardinality(candidate_customer_ids) <= 2 AND review_fingerprint IS NULL AND fingerprint_key_version IS NULL)
  );

-- +goose Down
ALTER TABLE pending_events
  DROP CONSTRAINT pending_events_kind_shape;

ALTER TABLE pending_events
  DROP COLUMN payload;

ALTER TABLE pending_events
  ADD CONSTRAINT pending_events_kind_shape CHECK (
    (kind = 'merge_review' AND event_type IS NULL AND idempotency_key IS NULL AND occurred_at IS NULL AND cardinality(candidate_customer_ids) = 2 AND candidate_customer_ids[1] < candidate_customer_ids[2] AND review_fingerprint IS NOT NULL AND octet_length(review_fingerprint) = 16 AND fingerprint_key_version IS NOT NULL AND fingerprint_key_version > 0)
    OR
    (kind <> 'merge_review' AND event_type IS NOT NULL AND btrim(event_type) <> '' AND idempotency_key IS NOT NULL AND btrim(idempotency_key) <> '' AND occurred_at IS NOT NULL AND cardinality(candidate_customer_ids) <= 2 AND review_fingerprint IS NULL AND fingerprint_key_version IS NULL)
  );

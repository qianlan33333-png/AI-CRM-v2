-- +goose Up
CREATE TABLE customer_merge_lineage (
  merged_customer_id  BIGINT PRIMARY KEY REFERENCES customers(id),
  primary_customer_id BIGINT NOT NULL REFERENCES customers(id),
  actor               TEXT NOT NULL,
  reason              TEXT NOT NULL,
  merged_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT customer_merge_lineage_distinct CHECK (merged_customer_id <> primary_customer_id),
  CONSTRAINT customer_merge_lineage_actor CHECK (
    btrim(actor) <> '' AND char_length(actor) <= 200
  ),
  CONSTRAINT customer_merge_lineage_reason CHECK (
    btrim(reason) <> '' AND char_length(reason) <= 1000
  )
);

CREATE INDEX idx_customer_merge_lineage_primary
  ON customer_merge_lineage (primary_customer_id, merged_customer_id);

-- +goose Down
DROP TABLE customer_merge_lineage;

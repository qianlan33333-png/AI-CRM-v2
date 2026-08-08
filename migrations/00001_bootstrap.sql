-- +goose Up
-- The first application DDL is deliberately narrow: stages is the P2 vertical
-- sample's only product prerequisite and remains owned exclusively by contact.
CREATE TABLE stages (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  config     JSONB NOT NULL DEFAULT '{}'
);

-- +goose Down
DROP TABLE stages;

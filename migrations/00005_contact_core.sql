-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE staff (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  wecom_userid TEXT NOT NULL UNIQUE,
  name         TEXT NOT NULL,
  department   TEXT,
  is_active    BOOLEAN NOT NULL DEFAULT TRUE,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE channels (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL,
  code       TEXT UNIQUE,
  config     JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT channels_config_object CHECK (jsonb_typeof(config) = 'object')
);

CREATE TABLE customers (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name             TEXT NOT NULL DEFAULT '',
  avatar_url       TEXT,
  gender           SMALLINT,
  stage_id         BIGINT REFERENCES stages(id),
  owner_staff_id   BIGINT REFERENCES staff(id),
  channel_id       BIGINT REFERENCES channels(id),
  added_at         TIMESTAMPTZ,
  last_interact_at TIMESTAMPTZ,
  is_deleted       BOOLEAN NOT NULL DEFAULT FALSE,
  extra            JSONB NOT NULL DEFAULT '{}',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT customers_extra_object CHECK (jsonb_typeof(extra) = 'object')
);

CREATE INDEX idx_customers_deleted_keyset
  ON customers (is_deleted, updated_at DESC, id DESC);
CREATE INDEX idx_customers_owner_keyset
  ON customers (owner_staff_id, updated_at DESC, id DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_stage_keyset
  ON customers (stage_id, updated_at DESC, id DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_channel_keyset
  ON customers (channel_id, updated_at DESC, id DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_added_keyset
  ON customers (added_at, updated_at DESC, id DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_interact_keyset
  ON customers (last_interact_at, updated_at DESC, id DESC) WHERE NOT is_deleted;
CREATE INDEX idx_customers_name_trgm
  ON customers USING GIN (lower(name) gin_trgm_ops);

CREATE TABLE tag_groups (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE tags (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  group_id     BIGINT REFERENCES tag_groups(id),
  name         TEXT NOT NULL,
  wecom_tag_id TEXT UNIQUE,
  sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_tags_catalog ON tags (group_id, sort_order, id);

CREATE TABLE customer_tags (
  customer_id BIGINT NOT NULL REFERENCES customers(id),
  tag_id      BIGINT NOT NULL REFERENCES tags(id),
  tagged_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  tagged_by   TEXT NOT NULL DEFAULT 'system',
  PRIMARY KEY (customer_id, tag_id)
);

CREATE INDEX idx_customer_tags_tag ON customer_tags (tag_id, customer_id);

-- +goose Down
DROP TABLE customer_tags;
DROP TABLE tags;
DROP TABLE tag_groups;
DROP TABLE customers;
DROP TABLE channels;
DROP TABLE staff;

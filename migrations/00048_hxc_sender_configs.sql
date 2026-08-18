-- +goose Up
CREATE TABLE hxc_sender_configs (
  id TEXT PRIMARY KEY,
  sender_userid TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT hxc_sender_configs_id_canonical CHECK (id = btrim(id) AND id <> ''),
  CONSTRAINT hxc_sender_configs_sender_userid_canonical CHECK (sender_userid = btrim(sender_userid) AND sender_userid <> ''),
  CONSTRAINT hxc_sender_configs_display_name_bounded CHECK (char_length(display_name) <= 200),
  CONSTRAINT hxc_sender_configs_priority_bounded CHECK (priority BETWEEN 0 AND 100000)
);

-- +goose Down
DROP TABLE hxc_sender_configs;

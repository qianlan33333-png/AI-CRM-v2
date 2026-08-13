-- +goose Up
CREATE TABLE outbound_tasks (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id  BIGINT NOT NULL REFERENCES customers(id),
  template_key TEXT NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT outbound_tasks_template_key CHECK (template_key = 'text.notice.v1'),
  CONSTRAINT outbound_tasks_payload_object CHECK (jsonb_typeof(payload) = 'object')
);

-- +goose Down
DROP TABLE outbound_tasks;

-- +goose Up
CREATE TABLE identity_operation_receipts (state text, payload jsonb);
CREATE INDEX "public"."receipt_state_idx"
ON "public"."identity_operation_receipts" ("state");
-- +goose Down
DROP TABLE identity_operation_receipts;

-- +goose Up
CREATE TABLE identity_operation_receipts (state text, payload jsonb);
CREATE INDEX "receipt_payload_gin"
ON "identity_operation_receipts" USING "gin" (payload);
-- +goose Down
DROP TABLE identity_operation_receipts;

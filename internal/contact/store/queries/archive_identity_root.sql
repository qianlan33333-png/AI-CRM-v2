-- name: LockVerifiedDM01CustomerRoot :one
SELECT c.id
FROM legacy_contact_identity_source_mappings m
JOIN legacy_contact_identity_import_row_receipts r
  ON r.run_id = m.last_run_id AND r.source_table = m.source_table
 AND r.source_key_hmac = m.source_key_hmac AND r.payload_hmac = m.payload_hmac
JOIN legacy_contact_identity_import_runs run ON run.id = r.run_id
JOIN customers c ON c.id = m.customer_id AND NOT c.is_deleted
WHERE m.source_table = 'crm_user_identity'
  AND m.source_key_hmac = sqlc.arg(source_key_hmac)::bytea
  AND m.last_run_id = sqlc.arg(run_id)::bigint
  AND m.customer_id > 0 AND r.disposition = 'imported'
  AND octet_length(r.field_digest) = 32
  AND run.mode IN ('full', 'incremental') AND run.state = 'imported'
FOR SHARE OF m, r, run, c;

-- name: CreateHistoricalDeferredPerson :one
INSERT INTO contact_v1_deferred_person_history (source_key_digest, source_payload_digest, source_field_digest, source_id, mobile_digest, third_party_user_id_digest, private_digest, redacted_roots, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: GetHistoricalDeferredPerson :one
SELECT * FROM contact_v1_deferred_person_history WHERE id = $1;

-- name: CountHistoricalDeferredPerson :one
SELECT count(*) FROM contact_v1_deferred_person_history;

-- name: ListHistoricalDeferredPerson :many
SELECT * FROM contact_v1_deferred_person_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CreateHistoricalDeferredIdentityConflict :one
INSERT INTO contact_v1_deferred_identity_conflict_history (source_key_digest, source_payload_digest, source_field_digest, source_id, conflict_type, source_type, status, resolution_status, union_id_digest, candidate_union_id_digest, external_user_id_digest, open_id_digest, mobile_digest, legacy_source_key_digest, payload_json_digest, source_payload_json_digest, resolution_note_digest, private_digest, redacted_roots, created_at, updated_at, resolved_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22) RETURNING *;

-- name: GetHistoricalDeferredIdentityConflict :one
SELECT * FROM contact_v1_deferred_identity_conflict_history WHERE id = $1;

-- name: CountHistoricalDeferredIdentityConflict :one
SELECT count(*) FROM contact_v1_deferred_identity_conflict_history;

-- name: ListHistoricalDeferredIdentityConflict :many
SELECT * FROM contact_v1_deferred_identity_conflict_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CreateHistoricalMissingRootIdentity :one
INSERT INTO contact_v1_missing_root_identity_history (source_key_digest, source_payload_digest, source_field_digest, source_id, dm01_run_id, dm01_source_key_digest, dm01_source_hmac_key_version, quarantine_reason, type, status, corp_id_digest, external_user_id_digest, union_id_digest, open_id_digest, follow_user_id_digest, name_digest, avatar_digest, gender_digest, raw_profile_digest, private_digest, redacted_roots, first_seen_at, last_seen_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25) RETURNING *;

-- name: GetHistoricalMissingRootIdentity :one
SELECT * FROM contact_v1_missing_root_identity_history WHERE id = $1;

-- name: CountHistoricalMissingRootIdentity :one
SELECT count(*) FROM contact_v1_missing_root_identity_history;

-- name: ListHistoricalMissingRootIdentity :many
SELECT * FROM contact_v1_missing_root_identity_history ORDER BY id LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

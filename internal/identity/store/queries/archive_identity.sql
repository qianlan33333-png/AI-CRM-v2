-- name: ImportArchiveWeComIdentity :one
INSERT INTO identities (
  customer_id, kind, scope, normalized_value, normalizer_version,
  assurance, source, review_fingerprint, fingerprint_key_version, bound_at
) VALUES (
  sqlc.narg(customer_id)::bigint, 'wecom_external_userid', sqlc.arg(scope)::text,
  sqlc.arg(external_userid)::text, 1, 'declared', 'v1.archive_identity_gap',
  substring(sqlc.arg(source_key_hmac)::bytea FROM 1 FOR 16),
  sqlc.arg(fingerprint_key_version)::smallint,
  CASE WHEN sqlc.narg(customer_id)::bigint IS NULL THEN NULL ELSE now() END
)
ON CONFLICT (kind, scope, normalized_value) DO UPDATE SET source = identities.source
WHERE identities.customer_id IS NOT DISTINCT FROM EXCLUDED.customer_id
  AND identities.source = EXCLUDED.source AND identities.assurance = 'declared'
  AND identities.normalizer_version = 1
  AND identities.review_fingerprint = EXCLUDED.review_fingerprint
  AND identities.fingerprint_key_version = EXCLUDED.fingerprint_key_version
  AND (identities.bound_at IS NULL) = (EXCLUDED.customer_id IS NULL)
RETURNING id, customer_id, kind, scope, normalized_value, normalizer_version,
  assurance, source, review_fingerprint, fingerprint_key_version, created_at, bound_at;

-- name: ReadArchiveWeComIdentity :one
SELECT id, customer_id, kind, scope, normalized_value, normalizer_version,
  assurance, source, review_fingerprint, fingerprint_key_version, created_at, bound_at
FROM identities
WHERE id = sqlc.arg(identity_id)::bigint
FOR SHARE;

-- name: ListCoupons :many
SELECT c.*, COALESCE(t.refs, '[]'::jsonb) AS target_refs
FROM coupons c
LEFT JOIN LATERAL (SELECT jsonb_agg(target_ref ORDER BY position) refs FROM coupon_targets WHERE coupon_id=c.id) t ON true
WHERE (sqlc.arg(search)::text = '' OR c.name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.arg(status_filter)::text = '' OR c.status = sqlc.arg(status_filter)::text)
ORDER BY c.id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountCoupons :one
SELECT CASE WHEN sqlc.arg(search)::text = '' AND sqlc.arg(status_filter)::text = ''
  THEN (SELECT total_coupons FROM coupon_catalog_counters WHERE singleton=TRUE)
  ELSE (SELECT count(*) FROM coupons WHERE (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%') AND (sqlc.arg(status_filter)::text = '' OR status=sqlc.arg(status_filter)::text)) END::bigint;

-- name: GetCoupon :one
SELECT c.*, COALESCE(t.refs, '[]'::jsonb) AS target_refs,
  EXISTS(SELECT 1 FROM coupon_v1_history_definitions h WHERE h.coupon_id=c.id) AS history_only
FROM coupons c
LEFT JOIN LATERAL (SELECT jsonb_agg(target_ref ORDER BY position) refs FROM coupon_targets WHERE coupon_id=c.id) t ON true
WHERE c.id=sqlc.arg(coupon_id)::bigint;

-- name: LockCoupon :one
SELECT * FROM coupons WHERE id=sqlc.arg(coupon_id)::bigint FOR UPDATE;

-- name: CreateCoupon :one
INSERT INTO coupons (name,discount_amount_total,total_issue_limit,per_user_issue_limit,claim_starts_at,claim_ends_at,validity_mode,use_starts_at,use_ends_at,relative_validity_days,instructions,created_by,updated_by,created_at,updated_at)
VALUES (sqlc.arg(name)::text,sqlc.arg(discount_amount_total)::bigint,sqlc.arg(total_issue_limit)::bigint,sqlc.arg(per_user_issue_limit)::bigint,sqlc.arg(claim_starts_at)::timestamptz,sqlc.arg(claim_ends_at)::timestamptz,sqlc.arg(validity_mode)::text,sqlc.narg(use_starts_at)::timestamptz,sqlc.narg(use_ends_at)::timestamptz,sqlc.narg(relative_validity_days)::integer,sqlc.arg(instructions)::text,sqlc.arg(actor)::bigint,sqlc.arg(actor)::bigint,sqlc.arg(now)::timestamptz,sqlc.arg(now)::timestamptz)
RETURNING id;

-- name: UpdateCoupon :exec
UPDATE coupons SET name=sqlc.arg(name)::text,discount_amount_total=sqlc.arg(discount_amount_total)::bigint,total_issue_limit=sqlc.arg(total_issue_limit)::bigint,per_user_issue_limit=sqlc.arg(per_user_issue_limit)::bigint,claim_starts_at=sqlc.arg(claim_starts_at)::timestamptz,claim_ends_at=sqlc.arg(claim_ends_at)::timestamptz,validity_mode=sqlc.arg(validity_mode)::text,use_starts_at=sqlc.narg(use_starts_at)::timestamptz,use_ends_at=sqlc.narg(use_ends_at)::timestamptz,relative_validity_days=sqlc.narg(relative_validity_days)::integer,instructions=sqlc.arg(instructions)::text,updated_by=sqlc.arg(actor)::bigint,version=version+1,updated_at=sqlc.arg(now)::timestamptz WHERE id=sqlc.arg(coupon_id)::bigint;

-- name: SetCouponStatus :exec
UPDATE coupons SET status=sqlc.arg(status)::text,updated_by=sqlc.arg(actor)::bigint,version=version+1,updated_at=sqlc.arg(now)::timestamptz WHERE id=sqlc.arg(coupon_id)::bigint;

-- name: DeleteDraftCoupon :one
DELETE FROM coupons WHERE id=sqlc.arg(coupon_id)::bigint AND status='draft' AND issued_count=0 RETURNING id;

-- name: DecrementCouponCount :one
UPDATE coupon_catalog_counters SET total_coupons=total_coupons-1 WHERE singleton=TRUE AND total_coupons > 0 RETURNING total_coupons;

-- name: DeleteCouponTargets :exec
DELETE FROM coupon_targets WHERE coupon_id=sqlc.arg(coupon_id)::bigint;
-- name: InsertCouponTarget :exec
INSERT INTO coupon_targets (coupon_id,position,target_ref,product_id) VALUES (sqlc.arg(coupon_id)::bigint,sqlc.arg(position)::integer,sqlc.arg(target_ref)::text,sqlc.arg(product_id)::bigint);
-- name: IncrementCouponCount :one
UPDATE coupon_catalog_counters SET total_coupons=total_coupons+1 WHERE singleton=TRUE RETURNING total_coupons;

-- name: ReserveCouponReceipt :one
INSERT INTO coupon_operation_receipts(operation,actor_scope,key_digest,payload_digest,created_at)
VALUES(sqlc.arg(operation)::text,sqlc.arg(actor_scope)::text,sqlc.arg(key_digest)::bytea,sqlc.arg(payload_digest)::bytea,sqlc.arg(created_at)::timestamptz)
ON CONFLICT(operation,actor_scope,key_digest) DO NOTHING
RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot;
-- name: GetCouponReceipt :one
SELECT id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot FROM coupon_operation_receipts WHERE operation=sqlc.arg(operation)::text AND actor_scope=sqlc.arg(actor_scope)::text AND key_digest=sqlc.arg(key_digest)::bytea;
-- name: CompleteCouponReceipt :one
UPDATE coupon_operation_receipts SET state='completed',result_snapshot=sqlc.arg(result_snapshot)::jsonb,completed_at=sqlc.arg(completed_at)::timestamptz WHERE id=sqlc.arg(id)::bigint AND state='in_progress'
RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot;

-- name: ListCouponClaims :many
SELECT id,coupon_id,customer_id,claim_number,claim_ref,status,claimed_at
FROM coupon_claims WHERE coupon_id=sqlc.arg(coupon_id)::bigint
ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;

-- name: CountCouponClaims :one
SELECT count(*)::bigint FROM coupon_claims WHERE coupon_id=sqlc.arg(coupon_id)::bigint;

-- name: CountCustomerCouponClaims :one
SELECT count(*)::bigint FROM coupon_claims WHERE coupon_id=sqlc.arg(coupon_id)::bigint AND customer_id=sqlc.arg(customer_id)::bigint;

-- name: CreateCouponClaim :one
INSERT INTO coupon_claims(coupon_id,customer_id,claim_number,claim_ref,claimed_at)
VALUES(sqlc.arg(coupon_id)::bigint,sqlc.arg(customer_id)::bigint,sqlc.arg(claim_number)::integer,sqlc.arg(claim_ref)::text,sqlc.arg(claimed_at)::timestamptz)
RETURNING id,coupon_id,customer_id,claim_number,claim_ref,status,claimed_at;

-- name: IncrementCouponIssuedCount :one
UPDATE coupons SET issued_count=issued_count+1,first_claim_at=COALESCE(first_claim_at,sqlc.arg(now)::timestamptz),version=version+1,updated_at=sqlc.arg(now)::timestamptz
WHERE id=sqlc.arg(coupon_id)::bigint AND issued_count < total_issue_limit
RETURNING id;

-- name: ListAvailableCoupons :many
SELECT c.*, COALESCE(t.refs, '[]'::jsonb) AS target_refs
FROM coupons c
JOIN coupon_targets target ON target.coupon_id=c.id AND target.target_ref=sqlc.arg(target_ref)::text
LEFT JOIN LATERAL (SELECT jsonb_agg(target_ref ORDER BY position) refs FROM coupon_targets WHERE coupon_id=c.id) t ON true
WHERE c.status='published' AND c.issued_count < c.total_issue_limit AND c.claim_starts_at <= sqlc.arg(now)::timestamptz AND c.claim_ends_at > sqlc.arg(now)::timestamptz
  AND (SELECT count(*) FROM coupon_claims claim WHERE claim.coupon_id=c.id AND claim.customer_id=sqlc.arg(customer_id)::bigint) < c.per_user_issue_limit
ORDER BY c.id LIMIT sqlc.arg(row_limit)::integer;

-- name: ResolveCouponPaymentIdentitySession :one
SELECT customer_id FROM coupon_payment_identity_sessions
WHERE token_digest=sqlc.arg(token_digest)::bytea AND expires_at > sqlc.arg(now)::timestamptz AND revoked_at IS NULL AND replaced_at IS NULL;

-- name: ResolveCouponSidebarGrant :one
SELECT customer_id FROM coupon_sidebar_grants
WHERE token_digest=sqlc.arg(token_digest)::bytea AND expires_at > sqlc.arg(now)::timestamptz AND revoked_at IS NULL AND replaced_at IS NULL;

-- name: ListCouponSidebarClaims :many
SELECT c.id AS coupon_id,c.name AS coupon_name,c.status AS coupon_status,claim.claim_ref,claim.claimed_at
FROM coupon_claims claim
JOIN coupons c ON c.id=claim.coupon_id
WHERE claim.customer_id=sqlc.arg(customer_id)::bigint
ORDER BY claim.id DESC LIMIT sqlc.arg(row_limit)::integer;

-- name: CreateHistoricalCoupon :one
INSERT INTO coupons(name,discount_amount_total,currency,status,total_issue_limit,per_user_issue_limit,issued_count,claim_starts_at,claim_ends_at,validity_mode,use_starts_at,use_ends_at,relative_validity_days,instructions,first_claim_at,created_by,updated_by,version,created_at,updated_at)
VALUES(sqlc.arg(name),sqlc.arg(discount_amount_total),'CNY','archived',sqlc.arg(total_issue_limit),sqlc.arg(per_user_issue_limit),0,sqlc.arg(claim_starts_at),sqlc.arg(claim_ends_at),sqlc.arg(validity_mode),sqlc.narg(use_starts_at),sqlc.narg(use_ends_at),sqlc.narg(relative_validity_days),sqlc.arg(instructions),NULL,sqlc.arg(actor),sqlc.arg(actor),1,sqlc.arg(created_at),sqlc.arg(updated_at)) RETURNING id;

-- name: RestoreHistoricalCouponClaimFacts :execrows
UPDATE coupons SET issued_count=sqlc.arg(issued_count),first_claim_at=sqlc.narg(first_claim_at)
WHERE id=sqlc.arg(coupon_id) AND status='archived' AND issued_count=0 AND first_claim_at IS NULL;

-- name: CreateHistoricalCouponMarker :exec
INSERT INTO coupon_v1_history_definitions(coupon_id,source_coupon_id,original_status)
VALUES(sqlc.arg(coupon_id),sqlc.arg(source_coupon_id),sqlc.arg(original_status));

-- name: GetHistoricalCouponMarker :one
SELECT h.*, c.first_claim_at FROM coupon_v1_history_definitions h JOIN coupons c ON c.id=h.coupon_id WHERE h.coupon_id=sqlc.arg(coupon_id);

-- name: ListHistoricalCouponIDs :many
SELECT coupon_id FROM coupon_v1_history_definitions ORDER BY coupon_id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;
-- name: CountHistoricalCoupons :one
SELECT count(*) FROM coupon_v1_history_definitions;

-- name: GetHistoricalCouponTarget :one
SELECT coupon_id,position,target_ref,product_id FROM coupon_targets WHERE coupon_id=sqlc.arg(coupon_id) AND position=sqlc.arg(position);

-- name: CreateHistoricalCouponClaim :one
INSERT INTO coupon_v1_history_claims(source_claim_id,source_coupon_id,coupon_id,customer_id,claim_no,status,discount_amount_total,currency,valid_from,valid_until,claimed_at,reserved_at,consumed_at,expired_at,created_at,updated_at)
VALUES(sqlc.arg(source_claim_id),sqlc.arg(source_coupon_id),sqlc.arg(coupon_id),sqlc.narg(customer_id),sqlc.arg(claim_no),sqlc.arg(status),sqlc.arg(discount_amount_total),sqlc.arg(currency),sqlc.arg(valid_from),sqlc.arg(valid_until),sqlc.arg(claimed_at),sqlc.narg(reserved_at),sqlc.narg(consumed_at),sqlc.narg(expired_at),sqlc.arg(created_at),sqlc.arg(updated_at)) RETURNING *;
-- name: GetHistoricalCouponClaim :one
SELECT * FROM coupon_v1_history_claims WHERE id=sqlc.arg(id);
-- name: ListHistoricalCouponClaims :many
SELECT * FROM coupon_v1_history_claims WHERE coupon_id=sqlc.arg(coupon_id) ORDER BY id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;
-- name: CountHistoricalCouponClaims :one
SELECT count(*) FROM coupon_v1_history_claims WHERE coupon_id=sqlc.arg(coupon_id);

-- name: CreateHistoricalCouponRedemption :one
INSERT INTO coupon_v1_history_redemptions(source_redemption_id,source_claim_id,source_order_id,claim_history_id,order_id,out_trade_no,status,original_amount_total,discount_amount_total,payable_amount_total,currency,reserved_until,release_reason,reserved_at,consumed_at,released_at,created_at,updated_at)
VALUES(sqlc.arg(source_redemption_id),sqlc.arg(source_claim_id),sqlc.arg(source_order_id),sqlc.arg(claim_history_id),sqlc.narg(order_id),sqlc.arg(out_trade_no),sqlc.arg(status),sqlc.arg(original_amount_total),sqlc.arg(discount_amount_total),sqlc.arg(payable_amount_total),sqlc.arg(currency),sqlc.arg(reserved_until),sqlc.arg(release_reason),sqlc.arg(reserved_at),sqlc.narg(consumed_at),sqlc.narg(released_at),sqlc.arg(created_at),sqlc.arg(updated_at)) RETURNING *;
-- name: GetHistoricalCouponRedemption :one
SELECT * FROM coupon_v1_history_redemptions WHERE id=sqlc.arg(id);
-- name: ListHistoricalCouponRedemptions :many
SELECT r.* FROM coupon_v1_history_redemptions r JOIN coupon_v1_history_claims c ON c.id=r.claim_history_id WHERE c.coupon_id=sqlc.arg(coupon_id) ORDER BY r.id LIMIT sqlc.arg(row_limit)::integer OFFSET sqlc.arg(row_offset)::integer;
-- name: CountHistoricalCouponRedemptions :one
SELECT count(*) FROM coupon_v1_history_redemptions r JOIN coupon_v1_history_claims c ON c.id=r.claim_history_id WHERE c.coupon_id=sqlc.arg(coupon_id);

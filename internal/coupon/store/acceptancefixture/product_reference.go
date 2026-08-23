// Package acceptancefixture creates Coupon-owned rows for cross-domain
// acceptance scenarios.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CreateDraftCoupon(ctx context.Context, db queryer, name string, startsAt, endsAt time.Time) (int64, error) {
	if db == nil || name == "" || startsAt.IsZero() || !endsAt.After(startsAt) {
		return 0, fmt.Errorf("valid Coupon fixture required")
	}
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO coupons (
  name,discount_amount_total,currency,status,total_issue_limit,per_user_issue_limit,
  claim_starts_at,claim_ends_at,validity_mode,relative_validity_days,created_by,updated_by,created_at,updated_at
) VALUES ($1,1,'CNY','draft',1,1,$2,$3,'relative_days',1,1,1,$2,$2)
RETURNING id`, name, startsAt.UTC(), endsAt.UTC()).Scan(&id); err != nil {
		return 0, fmt.Errorf("create Coupon-owned draft fixture: %w", err)
	}
	return id, nil
}

func CreateProductTarget(ctx context.Context, db queryer, couponID, productID int64) error {
	if db == nil || couponID <= 0 || productID <= 0 {
		return fmt.Errorf("valid Coupon target fixture required")
	}
	_, err := db.Exec(ctx, `INSERT INTO coupon_targets(coupon_id,position,target_ref,product_id) VALUES ($1,0,$2,$3)`, couponID, "standard_product:"+fmt.Sprint(productID), productID)
	if err != nil {
		return fmt.Errorf("create Coupon-owned product target fixture: %w", err)
	}
	return nil
}

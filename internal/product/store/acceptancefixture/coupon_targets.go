// Package acceptancefixture creates Product-owned rows needed by cross-domain
// acceptance tests without granting those tests direct Product table writes.
package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateProductsForCouponIDs creates one Product-owned FK target for every
// coupon selected by the bounded fixture name prefix. The caller owns the
// surrounding transaction and rolls the rows back with its scenario.
func CreateProductsForCouponIDs(ctx context.Context, tx pgx.Tx, couponNamePattern string) error {
	if tx == nil || couponNamePattern == "" {
		return fmt.Errorf("valid product fixture transaction and coupon pattern required")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO products (
  id, product_code, name, price_minor, currency, stock_quantity,
  created_by, created_at, updated_at, legacy_admin_projection, version, local_lifecycle
) OVERRIDING SYSTEM VALUE
SELECT id, 'p4ab-index-product-' || id, 'P4AB index product', 1, 'CNY', 0,
       771, now(), now(), '{"schema_version":1}'::jsonb, 1, 'draft'
FROM coupons
WHERE name LIKE $1::text`, couponNamePattern)
	if err != nil {
		return fmt.Errorf("create Product-owned coupon target fixtures: %w", err)
	}
	return nil
}

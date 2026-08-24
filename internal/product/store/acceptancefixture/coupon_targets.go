// Package acceptancefixture creates Product-owned facts for isolated
// acceptance tests without giving another domain write access to those tables.
package acceptancefixture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateCouponTargetProducts creates the Product facts required by the Coupon
// target-index acceptance scenario in the caller's transaction.
func CreateCouponTargetProducts(ctx context.Context, tx pgx.Tx, productIDs []int64) error {
	if tx == nil || len(productIDs) == 0 {
		return fmt.Errorf("valid Product coupon-target fixture required")
	}
	for index, productID := range productIDs {
		if productID < 1 || index > 0 && productIDs[index-1] >= productID {
			return fmt.Errorf("canonical Product IDs required")
		}
	}
	result, err := tx.Exec(ctx, `
INSERT INTO products
  (id, product_code, name, price_minor, currency, stock_quantity, created_by,
   created_at, updated_at, legacy_admin_projection, version, local_lifecycle)
OVERRIDING SYSTEM VALUE
SELECT product_id, 'p4ab-index-product-' || product_id, 'P4AB index product',
       1, 'CNY', 0, 771, now(), now(), '{"schema_version":1}'::jsonb, 1, 'draft'
FROM unnest($1::bigint[]) AS target(product_id)`, productIDs)
	if err != nil {
		return fmt.Errorf("create Product coupon-target fixtures: %w", err)
	}
	if result.RowsAffected() != int64(len(productIDs)) {
		return fmt.Errorf("create Product coupon-target fixtures: inserted %d of %d", result.RowsAffected(), len(productIDs))
	}
	return nil
}

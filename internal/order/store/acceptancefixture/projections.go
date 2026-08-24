// Package acceptancefixture creates Order-owned projections for acceptance
// scenarios without granting consumers direct Order writes.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type projectionStore interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PaidProjection struct {
	ProviderLabel         string
	MerchantOrderNo       string
	PlatformTransactionNo string
	CustomerID            int64
	ProductID             int64
	ProductCode           string
	ProductName           string
	AmountMinor           int64
	Currency              string
	StatusLabel           string
	DetailURL             string
}

func CreatePaidProjection(ctx context.Context, db projectionStore, projection PaidProjection) (int64, error) {
	if db == nil || projection.MerchantOrderNo == "" || projection.CustomerID < 0 || projection.ProductID < 1 || projection.ProductCode == "" || projection.ProductName == "" || projection.Currency == "" || projection.DetailURL == "" {
		return 0, fmt.Errorf("valid Order projection fixture required")
	}
	if projection.ProviderLabel == "" {
		projection.ProviderLabel = "acceptance"
	}
	if projection.StatusLabel == "" {
		projection.StatusLabel = "paid"
	}
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO order_list_projections (
  provider,provider_label,merchant_order_no,platform_transaction_no,customer_id,product_id,product_code,product_name_snapshot,
  amount_minor,currency,status,status_label,detail_url,created_at,updated_at
) VALUES (
  'wechat',$1,$2,$3,NULLIF($4,0),$5,$6,$7,$8,$9,'paid',$10,$11,$12,$12
) RETURNING id`,
		projection.ProviderLabel, projection.MerchantOrderNo, projection.PlatformTransactionNo, projection.CustomerID,
		projection.ProductID, projection.ProductCode, projection.ProductName, projection.AmountMinor, projection.Currency,
		projection.StatusLabel, projection.DetailURL, time.Now().UTC(),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("create Order-owned paid projection fixture: %w", err)
	}
	return id, nil
}

func DeletePaidProjections(ctx context.Context, db projectionStore, orderIDs []int64) error {
	if db == nil || len(orderIDs) == 0 {
		return fmt.Errorf("valid Order projection fixture IDs required")
	}
	result, err := db.Exec(ctx, `DELETE FROM order_list_projections WHERE id = ANY($1::bigint[])`, orderIDs)
	if err != nil {
		return fmt.Errorf("delete Order-owned paid projections: %w", err)
	}
	if result.RowsAffected() != int64(len(orderIDs)) {
		return fmt.Errorf("delete Order-owned paid projections: deleted %d rows, want %d", result.RowsAffected(), len(orderIDs))
	}
	return nil
}

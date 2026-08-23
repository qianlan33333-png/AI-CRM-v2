// Package acceptancefixture creates Order-owned projections for cross-domain
// acceptance scenarios without granting those tests direct Order writes.
package acceptancefixture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type PaidProjection struct {
	ProviderLabel         string
	MerchantOrderNo       string
	PlatformTransactionNo string
	CustomerID            int64
	PayerName             string
	Mobile                string
	IdentityKind          string
	IdentityValue         string
	ProductID             int64
	ProductCode           string
	ProductName           string
	AmountMinor           int64
	Currency              string
	StatusLabel           string
	DetailURL             string
	CreatedAt             time.Time
}

func CreatePaidProjection(ctx context.Context, db executor, projection PaidProjection) (int64, error) {
	if db == nil || projection.MerchantOrderNo == "" || projection.ProductID <= 0 || projection.ProductCode == "" || projection.ProductName == "" || projection.Currency == "" || projection.DetailURL == "" {
		return 0, fmt.Errorf("valid Order projection fixture required")
	}
	if projection.ProviderLabel == "" {
		projection.ProviderLabel = "acceptance"
	}
	if projection.StatusLabel == "" {
		projection.StatusLabel = "paid"
	}
	if projection.CreatedAt.IsZero() {
		projection.CreatedAt = time.Now().UTC()
	}
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO order_list_projections (
  provider,provider_label,merchant_order_no,platform_transaction_no,customer_id,
  payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
  product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,
  detail_url,created_at,updated_at
) VALUES (
  'wechat',$1,$2,$3,NULLIF($4,0),$5,$6,$7,$8,
  $9,$10,$11,$12,$13,'paid',$14,$15,$16,$16
) RETURNING id`,
		projection.ProviderLabel, projection.MerchantOrderNo, projection.PlatformTransactionNo, projection.CustomerID,
		projection.PayerName, projection.Mobile, projection.IdentityKind, projection.IdentityValue,
		projection.ProductID, projection.ProductCode, projection.ProductName, projection.AmountMinor,
		projection.Currency, projection.StatusLabel, projection.DetailURL, projection.CreatedAt.UTC(),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("create Order-owned paid projection fixture: %w", err)
	}
	return id, nil
}

func DeleteByProductIDs(ctx context.Context, db executor, productIDs []int64) error {
	if db == nil || len(productIDs) == 0 {
		return fmt.Errorf("valid Order projection fixture product ids required")
	}
	if _, err := db.Exec(ctx, `DELETE FROM order_list_projections WHERE product_id = ANY($1::bigint[])`, productIDs); err != nil {
		return fmt.Errorf("delete Order-owned projection fixtures: %w", err)
	}
	return nil
}

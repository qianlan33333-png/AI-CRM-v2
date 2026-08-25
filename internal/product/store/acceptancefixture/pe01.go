package acceptancefixture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

// CreatePE01Product creates one enabled Product-owned ordinary-product fact
// for cross-domain PostgreSQL acceptance tests.
func CreatePE01Product(ctx context.Context, pool *pgxpool.Pool, code string, at time.Time) (int64, error) {
	return productdb.New(pool).CreatePE01AcceptanceProduct(ctx, productdb.CreatePE01AcceptanceProductParams{
		ProductCode: code,
		CreatedAt:   pgtype.Timestamptz{Time: at, Valid: true},
	})
}

package acceptance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateIdentityStorageFKParents creates Contact-owned customers for Identity
// storage acceptance. Identity acceptance must not write Contact tables itself.
func CreateIdentityStorageFKParents(ctx context.Context, tx pgx.Tx, count int) ([]int64, error) {
	if count < 1 {
		return nil, fmt.Errorf("identity storage fixture customer count must be positive")
	}
	ids := make([]int64, 0, count)
	for index := 0; index < count; index++ {
		var id int64
		if err := tx.QueryRow(ctx, `
INSERT INTO customers (name)
VALUES ($1::text)
RETURNING id`, fmt.Sprintf("identity-storage-fixture-%d", index)).Scan(&id); err != nil {
			return nil, fmt.Errorf("create contact-owned identity storage parent %d: %w", index, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

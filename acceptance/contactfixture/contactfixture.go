// Package contactfixture creates Contact-owned parent rows for acceptance tests.
package contactfixture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrNilTransaction = errors.New("contact fixture requires a transaction")

// CreateCustomer creates one channel-neutral Contact customer and returns its OneID.
// Callers must provide the transaction that owns their acceptance scenario.
func CreateCustomer(ctx context.Context, tx pgx.Tx) (int64, error) {
	if tx == nil {
		return 0, ErrNilTransaction
	}
	var id int64
	if err := tx.QueryRow(ctx, `
INSERT INTO customers (name)
VALUES ($1::text)
RETURNING id`, "acceptance-contact-fixture").Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance customer: %w", err)
	}
	return id, nil
}

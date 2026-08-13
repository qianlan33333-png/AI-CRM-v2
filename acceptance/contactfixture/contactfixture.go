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

// SoftDeleteCustomer marks a Contact-owned parent unavailable to an acceptance
// scenario without allowing another domain to write customers directly.
func SoftDeleteCustomer(ctx context.Context, tx pgx.Tx, customerID int64) error {
	if tx == nil || customerID <= 0 {
		return ErrNilTransaction
	}
	commandTag, err := tx.Exec(ctx, `
UPDATE customers
SET is_deleted = TRUE
WHERE id = $1::bigint`, customerID)
	if err != nil {
		return fmt.Errorf("soft-delete contact-owned acceptance customer: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("soft-delete contact-owned acceptance customer: customer not found")
	}
	return nil
}

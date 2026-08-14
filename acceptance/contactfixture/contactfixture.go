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

// CreateStaff creates a Contact-owned owner row for acceptance scenarios.
func CreateStaff(ctx context.Context, tx pgx.Tx, wecomUserID string) (int64, error) {
	if tx == nil || wecomUserID == "" {
		return 0, ErrNilTransaction
	}
	var id int64
	if err := tx.QueryRow(ctx, `
INSERT INTO staff (wecom_userid, name)
VALUES ($1::text, $2::text)
RETURNING id`, wecomUserID, "acceptance-contact-owner").Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance staff: %w", err)
	}
	return id, nil
}

// AssignCustomerOwner updates the Contact-owned relationship for a fixture.
func AssignCustomerOwner(ctx context.Context, tx pgx.Tx, customerID, staffID int64) error {
	if tx == nil || customerID <= 0 || staffID <= 0 {
		return ErrNilTransaction
	}
	commandTag, err := tx.Exec(ctx, `
UPDATE customers
SET owner_staff_id = $2::bigint
WHERE id = $1::bigint`, customerID, staffID)
	if err != nil {
		return fmt.Errorf("assign contact-owned acceptance owner: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("assign contact-owned acceptance owner: customer not found")
	}
	return nil
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

// CreateTag creates one Contact-owned tag for an acceptance scenario.
func CreateTag(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	if tx == nil || name == "" {
		return 0, ErrNilTransaction
	}
	var id int64
	if err := tx.QueryRow(ctx, `
INSERT INTO tags (name)
VALUES ($1::text)
RETURNING id`, name).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance tag: %w", err)
	}
	return id, nil
}

// AttachTag adds a Contact-owned tag association for an acceptance scenario.
func AttachTag(ctx context.Context, tx pgx.Tx, customerID, tagID int64, actor string) error {
	if tx == nil || customerID <= 0 || tagID <= 0 || actor == "" {
		return ErrNilTransaction
	}
	commandTag, err := tx.Exec(ctx, `
INSERT INTO customer_tags (customer_id, tag_id, tagged_by)
VALUES ($1::bigint, $2::bigint, $3::text)`, customerID, tagID, actor)
	if err != nil {
		return fmt.Errorf("attach contact-owned acceptance tag: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("attach contact-owned acceptance tag: expected one association")
	}
	return nil
}

// AppendTimelineEvent appends one Contact-owned timeline fact for an
// acceptance scenario. The database owns append-only enforcement.
func AppendTimelineEvent(ctx context.Context, tx pgx.Tx, customerID int64, eventType string, payload []byte, actor string) error {
	if tx == nil || customerID <= 0 || eventType == "" || len(payload) == 0 || actor == "" {
		return ErrNilTransaction
	}
	commandTag, err := tx.Exec(ctx, `
INSERT INTO customer_events (customer_id, event_type, payload, actor, occurred_at)
VALUES ($1::bigint, $2::text, $3::jsonb, $4::text, now())`, customerID, eventType, payload, actor)
	if err != nil {
		return fmt.Errorf("append contact-owned acceptance timeline event: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("append contact-owned acceptance timeline event: expected one event")
	}
	return nil
}

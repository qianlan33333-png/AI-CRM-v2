// Package contactfixture creates Contact-owned parent rows for acceptance tests.
package contactfixture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNilTransaction = errors.New("contact fixture requires a transaction")
var ErrInvalidCustomerFixture = errors.New("invalid contact customer fixture")
var ErrInvalidStaffFixture = errors.New("invalid contact staff fixture")

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

// CreateCustomerRecord creates a committed Contact-owned customer for an
// acceptance scenario that spans multiple transactions or connections.
func CreateCustomerRecord(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	if pool == nil {
		return 0, ErrInvalidCustomerFixture
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO customers (name)
VALUES ($1::text)
RETURNING id`, "acceptance-contact-fixture").Scan(&id); err != nil {
		return 0, fmt.Errorf("create committed contact-owned acceptance customer: %w", err)
	}
	return id, nil
}

// CreateCustomerWithDetails creates a committed Contact-owned customer whose
// display projection is needed by a cross-domain acceptance scenario.
func CreateCustomerWithDetails(ctx context.Context, pool *pgxpool.Pool, name string, extra []byte) (int64, error) {
	if pool == nil || name == "" || len(extra) == 0 {
		return 0, ErrInvalidCustomerFixture
	}
	var id int64
	if err := pool.QueryRow(ctx, `
INSERT INTO customers (name, extra)
VALUES ($1::text, $2::jsonb)
RETURNING id`, name, extra).Scan(&id); err != nil {
		return 0, fmt.Errorf("create detailed Contact-owned acceptance customer: %w", err)
	}
	return id, nil
}

// DeleteCustomer removes a committed Contact-owned acceptance customer.
func DeleteCustomer(ctx context.Context, pool *pgxpool.Pool, customerID int64) error {
	if pool == nil || customerID <= 0 {
		return ErrInvalidCustomerFixture
	}
	result, err := pool.Exec(ctx, `DELETE FROM customers WHERE id = $1::bigint`, customerID)
	if err != nil {
		return fmt.Errorf("delete contact-owned acceptance customer: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delete contact-owned acceptance customer: not found")
	}
	return nil
}

// DeleteCustomers removes only committed Contact-owned customer fixtures.
func DeleteCustomers(ctx context.Context, pool *pgxpool.Pool, customerIDs []int64) error {
	if pool == nil || len(customerIDs) == 0 {
		return ErrInvalidCustomerFixture
	}
	result, err := pool.Exec(ctx, `DELETE FROM customers WHERE id = ANY($1::bigint[])`, customerIDs)
	if err != nil {
		return fmt.Errorf("delete Contact-owned acceptance customers: %w", err)
	}
	if result.RowsAffected() != int64(len(customerIDs)) {
		return fmt.Errorf("delete Contact-owned acceptance customers: deleted %d rows, want %d", result.RowsAffected(), len(customerIDs))
	}
	return nil
}

// SetCustomerName changes the Contact-owned display name required by an
// acceptance scenario, including an explicitly empty name.
func SetCustomerName(ctx context.Context, pool *pgxpool.Pool, customerID int64, name string) error {
	if pool == nil || customerID <= 0 {
		return ErrInvalidCustomerFixture
	}
	result, err := pool.Exec(ctx, `UPDATE customers SET name = $2::text WHERE id = $1::bigint`, customerID, name)
	if err != nil {
		return fmt.Errorf("set contact-owned acceptance customer name: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("set contact-owned acceptance customer name: not found")
	}
	return nil
}

// CreateStaff creates a Contact-owned owner row for acceptance scenarios.
func CreateStaff(ctx context.Context, tx pgx.Tx, wecomUserID string) (int64, error) {
	return CreateStaffWithState(ctx, tx, wecomUserID, true, time.Now().UTC())
}

// CreateStaffWithState creates a Contact-owned staff fixture while preserving
// the explicit activity and update facts required by an acceptance scenario.
func CreateStaffWithState(ctx context.Context, tx pgx.Tx, wecomUserID string, active bool, updatedAt time.Time) (int64, error) {
	return CreateStaffWithDetails(ctx, tx, wecomUserID, "acceptance-contact-owner", active, updatedAt)
}

// CreateStaffWithDetails creates a Contact-owned staff fixture while retaining
// the fixture name where the consumer's read projection depends on it.
func CreateStaffWithDetails(ctx context.Context, tx pgx.Tx, wecomUserID, name string, active bool, updatedAt time.Time) (int64, error) {
	if tx == nil || wecomUserID == "" {
		return 0, ErrNilTransaction
	}
	return createStaff(ctx, tx, wecomUserID, name, active, updatedAt)
}

// CreateStaffRecord creates committed Contact-owned staff for an acceptance
// scenario that spans multiple transactions or database connections.
func CreateStaffRecord(ctx context.Context, pool *pgxpool.Pool, wecomUserID, name string, active bool, updatedAt time.Time) (int64, error) {
	if pool == nil || wecomUserID == "" {
		return 0, ErrInvalidStaffFixture
	}
	return createStaff(ctx, pool, wecomUserID, name, active, updatedAt)
}

type staffRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func createStaff(ctx context.Context, db staffRowQuerier, wecomUserID, name string, active bool, updatedAt time.Time) (int64, error) {
	var id int64
	if err := db.QueryRow(ctx, `
INSERT INTO staff (wecom_userid, name, is_active, updated_at)
VALUES ($1::text, $2::text, $3::boolean, $4::timestamptz)
RETURNING id`, wecomUserID, name, active, updatedAt.UTC()).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance staff: %w", err)
	}
	return id, nil
}

// SetStaffActive changes the Contact-owned activity fact used by an acceptance
// scenario while retaining real PostgreSQL locking behavior.
func SetStaffActive(ctx context.Context, pool *pgxpool.Pool, staffID int64, active bool) error {
	if pool == nil || staffID <= 0 {
		return ErrInvalidStaffFixture
	}
	result, err := pool.Exec(ctx, `UPDATE staff SET is_active = $2::boolean WHERE id = $1::bigint`, staffID, active)
	if err != nil {
		return fmt.Errorf("set contact-owned acceptance staff activity: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("set contact-owned acceptance staff activity: not found")
	}
	return nil
}

// DeleteStaff removes Contact-owned staff created only for an acceptance
// scenario after all referencing transactions have rolled back.
func DeleteStaff(ctx context.Context, pool *pgxpool.Pool, staffID int64) error {
	if pool == nil || staffID <= 0 {
		return ErrInvalidStaffFixture
	}
	result, err := pool.Exec(ctx, `DELETE FROM staff WHERE id = $1::bigint`, staffID)
	if err != nil {
		return fmt.Errorf("delete contact-owned acceptance staff: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("delete contact-owned acceptance staff: not found")
	}
	return nil
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

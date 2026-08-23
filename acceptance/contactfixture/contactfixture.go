// Package contactfixture creates Contact-owned parent rows for acceptance tests.
package contactfixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNilTransaction = errors.New("contact fixture requires a transaction")

// Executor is the narrow database seam used by Contact-owned acceptance
// fixtures. Both pgx transactions and pools satisfy it.
type Executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// CreateCustomer creates one channel-neutral Contact customer and returns its OneID.
// Callers must provide the transaction that owns their acceptance scenario.
func CreateCustomer(ctx context.Context, executor Executor) (int64, error) {
	return CreateCustomerWithDetails(ctx, executor, "acceptance-contact-fixture", json.RawMessage(`{}`))
}

// CreateCustomerWithDetails retains the local name and channel-neutral extra
// facts required by a cross-domain acceptance scenario.
func CreateCustomerWithDetails(ctx context.Context, executor Executor, name string, extra json.RawMessage) (int64, error) {
	if executor == nil || !json.Valid(extra) {
		return 0, ErrNilTransaction
	}
	var id int64
	if err := executor.QueryRow(ctx, `
INSERT INTO customers (name, extra)
VALUES ($1::text, $2::jsonb)
RETURNING id`, name, extra).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance customer: %w", err)
	}
	return id, nil
}

// DeleteCustomers removes Contact-owned parent fixtures after an acceptance
// scenario has deleted its dependent facts.
func DeleteCustomers(ctx context.Context, executor Executor, customerIDs []int64) error {
	if executor == nil || len(customerIDs) == 0 {
		return ErrNilTransaction
	}
	for _, customerID := range customerIDs {
		if customerID <= 0 {
			return ErrNilTransaction
		}
	}
	_, err := executor.Exec(ctx, `DELETE FROM customers WHERE id = ANY($1::bigint[])`, customerIDs)
	if err != nil {
		return fmt.Errorf("delete contact-owned acceptance customers: %w", err)
	}
	return nil
}

// CreateStaff creates a Contact-owned owner row for acceptance scenarios.
func CreateStaff(ctx context.Context, executor Executor, wecomUserID string) (int64, error) {
	return CreateStaffWithState(ctx, executor, wecomUserID, true, time.Now().UTC())
}

// CreateStaffWithState creates a Contact-owned staff fixture while preserving
// the explicit activity and update facts required by an acceptance scenario.
func CreateStaffWithState(ctx context.Context, executor Executor, wecomUserID string, active bool, updatedAt time.Time) (int64, error) {
	return CreateStaffWithDetails(ctx, executor, wecomUserID, "acceptance-contact-owner", active, updatedAt)
}

// CreateStaffWithDetails creates a Contact-owned staff fixture while retaining
// the fixture name where the consumer's read projection depends on it.
func CreateStaffWithDetails(ctx context.Context, executor Executor, wecomUserID, name string, active bool, updatedAt time.Time) (int64, error) {
	if executor == nil || wecomUserID == "" {
		return 0, ErrNilTransaction
	}
	var id int64
	if err := executor.QueryRow(ctx, `
INSERT INTO staff (wecom_userid, name, is_active, updated_at)
VALUES ($1::text, $2::text, $3::boolean, $4::timestamptz)
RETURNING id`, wecomUserID, name, active, updatedAt.UTC()).Scan(&id); err != nil {
		return 0, fmt.Errorf("create contact-owned acceptance staff: %w", err)
	}
	return id, nil
}

// SetStaffActiveByWeComUserID updates a Contact-owned activity fixture by its
// local WeCom user key.
func SetStaffActiveByWeComUserID(ctx context.Context, executor Executor, wecomUserID string, active bool) error {
	if executor == nil || wecomUserID == "" {
		return ErrNilTransaction
	}
	_, err := executor.Exec(ctx, `UPDATE staff SET is_active = $2::boolean WHERE wecom_userid = $1::text`, wecomUserID, active)
	if err != nil {
		return fmt.Errorf("update contact-owned acceptance staff: %w", err)
	}
	return nil
}

// DeleteStaff removes one Contact-owned staff fixture.
func DeleteStaff(ctx context.Context, executor Executor, staffID int64) error {
	if executor == nil || staffID <= 0 {
		return ErrNilTransaction
	}
	_, err := executor.Exec(ctx, `DELETE FROM staff WHERE id = $1::bigint`, staffID)
	if err != nil {
		return fmt.Errorf("delete contact-owned acceptance staff: %w", err)
	}
	return nil
}

// DeleteStaffByWeComUserID removes one Contact-owned staff fixture by its
// local WeCom user key.
func DeleteStaffByWeComUserID(ctx context.Context, executor Executor, wecomUserID string) error {
	if executor == nil || wecomUserID == "" {
		return ErrNilTransaction
	}
	_, err := executor.Exec(ctx, `DELETE FROM staff WHERE wecom_userid = $1::text`, wecomUserID)
	if err != nil {
		return fmt.Errorf("delete contact-owned acceptance staff: %w", err)
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

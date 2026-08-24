package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type OwnerReassignmentRepository struct{}

var _ contactapp.OwnerReassignmentStore = (*OwnerReassignmentRepository)(nil)

func NewOwnerReassignmentRepository() *OwnerReassignmentRepository {
	return &OwnerReassignmentRepository{}
}

func (r *OwnerReassignmentRepository) CreateOwnerReassignmentPreview(ctx context.Context, p contactapp.OwnerReassignmentPreview, actor int64, digest, key []byte, now time.Time) (contactapp.OwnerReassignmentPreview, bool, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, ownerReassignmentUnavailable(e)
	}
	rows, e := json.Marshal(p.Rows)
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, contactapp.ErrOwnerReassignmentInvalid
	}
	issues, e := json.Marshal(p.Issues)
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, contactapp.ErrOwnerReassignmentInvalid
	}
	hash, e := hex.DecodeString(p.Hash)
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, contactapp.ErrOwnerReassignmentInvalid
	}
	_, e = tx.Exec(ctx, `INSERT INTO public.contact_owner_reassignment_previews(id,actor_id,idempotency_key_digest,payload_digest,preview_hash,rows,errors,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(actor_id,idempotency_key_digest) DO NOTHING`, p.ID, actor, key, digest, hash, rows, issues, now, p.ExpiresAt)
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, ownerReassignmentUnavailable(e)
	}
	var existingDigest []byte
	var id string
	e = tx.QueryRow(ctx, `SELECT id,payload_digest FROM public.contact_owner_reassignment_previews WHERE actor_id=$1 AND idempotency_key_digest=$2 FOR UPDATE`, actor, key).Scan(&id, &existingDigest)
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, ownerReassignmentUnavailable(e)
	}
	if string(existingDigest) != string(digest) {
		return contactapp.OwnerReassignmentPreview{}, false, contactapp.ErrOwnerReassignmentConflict
	}
	stored, e := readOwnerReassignmentPreview(ctx, tx, id, actor, false, time.Time{})
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, false, e
	}
	return stored, id == p.ID, nil
}
func (r *OwnerReassignmentRepository) ReadOwnerReassignmentPreview(ctx context.Context, id string, actor int64) (contactapp.OwnerReassignmentPreview, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return contactapp.OwnerReassignmentPreview{}, ownerReassignmentUnavailable(e)
	}
	return readOwnerReassignmentPreview(ctx, tx, id, actor, false, time.Time{})
}
func (r *OwnerReassignmentRepository) ReserveOwnerReassignmentReceipt(ctx context.Context, actor int64, key, payload []byte, now time.Time) (contactapp.OwnerReassignmentReceipt, bool, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return contactapp.OwnerReassignmentReceipt{}, false, ownerReassignmentUnavailable(e)
	}
	var out contactapp.OwnerReassignmentReceipt
	e = tx.QueryRow(ctx, `INSERT INTO public.contact_owner_reassignment_operation_receipts(actor_id,idempotency_key_digest,payload_digest,created_at) VALUES($1,$2,$3,$4) ON CONFLICT(actor_id,idempotency_key_digest) DO NOTHING RETURNING id,payload_digest`, actor, key, payload, now).Scan(&out.ID, &out.PayloadDigest)
	if e == nil {
		return out, true, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, false, ownerReassignmentUnavailable(e)
	}
	var result []byte
	var state string
	e = tx.QueryRow(ctx, `SELECT id,payload_digest,state,result FROM public.contact_owner_reassignment_operation_receipts WHERE actor_id=$1 AND idempotency_key_digest=$2 FOR UPDATE`, actor, key).Scan(&out.ID, &out.PayloadDigest, &state, &result)
	if e != nil {
		return out, false, ownerReassignmentUnavailable(e)
	}
	out.Completed = state == "completed"
	if out.Completed {
		if json.Unmarshal(result, &out.Result) != nil {
			return out, false, contactapp.ErrOwnerReassignmentUnavailable
		}
	}
	return out, false, nil
}
func (r *OwnerReassignmentRepository) LockOwnerReassignmentPreview(ctx context.Context, id string, actor int64, hash []byte, now time.Time) (contactapp.OwnerReassignmentPreview, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return contactapp.OwnerReassignmentPreview{}, ownerReassignmentUnavailable(e)
	}
	p, e := readOwnerReassignmentPreview(ctx, tx, id, actor, true, now)
	if e != nil {
		return p, e
	}
	want, e := hex.DecodeString(p.Hash)
	if e != nil || string(want) != string(hash) {
		return p, contactapp.ErrOwnerReassignmentConflict
	}
	if p.Executed {
		return p, contactapp.ErrOwnerReassignmentConflict
	}
	return p, nil
}
func (r *OwnerReassignmentRepository) LockActiveOwnerReassignmentStaff(ctx context.Context, id int64) error {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return ownerReassignmentUnavailable(e)
	}
	var found bool
	e = tx.QueryRow(ctx, `SELECT TRUE FROM public.staff WHERE id=$1 AND is_active FOR SHARE`, id).Scan(&found)
	if errors.Is(e, pgx.ErrNoRows) {
		return contactapp.ErrOwnerReassignmentConflict
	}
	return ownerReassignmentUnavailable(e)
}
func (r *OwnerReassignmentRepository) LockOwnerReassignmentCustomer(ctx context.Context, id int64) (contactapp.OwnerReassignmentRow, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return contactapp.OwnerReassignmentRow{}, ownerReassignmentUnavailable(e)
	}
	var row contactapp.OwnerReassignmentRow
	e = tx.QueryRow(ctx, `SELECT id,owner_staff_id,updated_at FROM public.customers WHERE id=$1 AND NOT is_deleted FOR UPDATE`, id).Scan(&row.CustomerID, &row.ExpectedOwnerStaffID, &row.ExpectedUpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return row, contactapp.ErrOwnerReassignmentConflict
	}
	return row, ownerReassignmentUnavailable(e)
}
func (r *OwnerReassignmentRepository) UpdateOwnerReassignmentCustomer(ctx context.Context, id, target int64) (time.Time, error) {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return time.Time{}, ownerReassignmentUnavailable(e)
	}
	var at time.Time
	e = tx.QueryRow(ctx, `UPDATE public.customers SET owner_staff_id=$2,updated_at=now() WHERE id=$1 AND NOT is_deleted RETURNING updated_at`, id, target).Scan(&at)
	if errors.Is(e, pgx.ErrNoRows) {
		return time.Time{}, contactapp.ErrOwnerReassignmentConflict
	}
	return at, ownerReassignmentUnavailable(e)
}
func (r *OwnerReassignmentRepository) AppendOwnerReassignmentCustomerEvent(ctx context.Context, id int64, payload []byte, actor int64, at time.Time) error {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return ownerReassignmentUnavailable(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO public.customer_events(customer_id,event_type,payload,actor,occurred_at) VALUES($1,'customer.updated',$2,$3,$4)`, id, payload, "admin:"+strconv.FormatInt(actor, 10), at)
	return ownerReassignmentUnavailable(e)
}
func (r *OwnerReassignmentRepository) MarkOwnerReassignmentPreviewExecuted(ctx context.Context, id string, result []contactapp.OwnerReassignmentResultRow, now time.Time) error {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return ownerReassignmentUnavailable(e)
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return contactapp.ErrOwnerReassignmentUnavailable
	}
	tag, e := tx.Exec(ctx, `UPDATE public.contact_owner_reassignment_previews SET result=$2,executed_at=$3 WHERE id=$1 AND executed_at IS NULL`, id, raw, now)
	if e != nil {
		return ownerReassignmentUnavailable(e)
	}
	if tag.RowsAffected() != 1 {
		return contactapp.ErrOwnerReassignmentConflict
	}
	return nil
}
func (r *OwnerReassignmentRepository) CompleteOwnerReassignmentReceipt(ctx context.Context, id int64, result []contactapp.OwnerReassignmentResultRow, now time.Time) error {
	tx, e := ownerReassignmentTx(ctx)
	if r == nil || e != nil {
		return ownerReassignmentUnavailable(e)
	}
	raw, e := json.Marshal(result)
	if e != nil {
		return contactapp.ErrOwnerReassignmentUnavailable
	}
	tag, e := tx.Exec(ctx, `UPDATE public.contact_owner_reassignment_operation_receipts SET state='completed',result=$2,completed_at=$3 WHERE id=$1 AND state='reserved'`, id, raw, now)
	if e != nil {
		return ownerReassignmentUnavailable(e)
	}
	if tag.RowsAffected() != 1 {
		return contactapp.ErrOwnerReassignmentUnavailable
	}
	return nil
}

func ownerReassignmentTx(ctx context.Context) (pgx.Tx, error) {
	return platformstore.TxFromContext(ctx)
}
func ownerReassignmentUnavailable(e error) error {
	if e == nil {
		return nil
	}
	return errors.Join(contactapp.ErrOwnerReassignmentUnavailable, e)
}
func readOwnerReassignmentPreview(ctx context.Context, tx pgx.Tx, id string, actor int64, lock bool, now time.Time) (contactapp.OwnerReassignmentPreview, error) {
	q := `SELECT rows,errors,result,preview_hash,expires_at,executed_at FROM public.contact_owner_reassignment_previews WHERE id=$1 AND actor_id=$2`
	if lock {
		q += " FOR UPDATE"
	}
	var rows, issues, result, hash []byte
	var expires time.Time
	var executed *time.Time
	e := tx.QueryRow(ctx, q, id, actor).Scan(&rows, &issues, &result, &hash, &expires, &executed)
	if errors.Is(e, pgx.ErrNoRows) {
		return contactapp.OwnerReassignmentPreview{}, contactapp.ErrOwnerReassignmentNotFound
	}
	if e != nil {
		return contactapp.OwnerReassignmentPreview{}, ownerReassignmentUnavailable(e)
	}
	p := contactapp.OwnerReassignmentPreview{ID: id, Hash: hex.EncodeToString(hash), ExpiresAt: expires, Executed: executed != nil}
	if json.Unmarshal(rows, &p.Rows) != nil || json.Unmarshal(issues, &p.Issues) != nil {
		return p, contactapp.ErrOwnerReassignmentUnavailable
	}
	if result != nil && json.Unmarshal(result, &p.Result) != nil {
		return p, contactapp.ErrOwnerReassignmentUnavailable
	}
	if !now.IsZero() && now.After(expires) {
		return p, contactapp.ErrOwnerReassignmentExpired
	}
	return p, nil
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	memberdomain "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/domain"
	memberport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/serviceperiodmember/port"
)

const memberColumns = `member_ref, service_product_id, customer_id, state, source,
starts_at, expires_at, expired_at, removed_at, remark, alliance, version, created_at, updated_at`

type Repository struct{}

var _ memberport.Store = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (*Repository) ServiceProductExists(ctx context.Context, productID int64) (bool, error) {
	if ctx == nil || productID < 1 {
		return false, memberport.ErrInvalidInput
	}
	var exists bool
	err := tx(ctx).QueryRow(ctx, `SELECT EXISTS (
SELECT 1 FROM public.products WHERE id=$1 AND (
  legacy_admin_projection->>'status'='service_period_enabled' AND legacy_admin_projection->>'enabled'='true'
  OR legacy_admin_projection->>'status' IN ('service_period_draft','service_period_disabled','service_period_archived')
     AND legacy_admin_projection->>'enabled'='false'))`, productID).Scan(&exists)
	return exists, classify(err)
}

func (*Repository) LockServiceProductForMemberAdd(ctx context.Context, productID int64) (bool, error) {
	if ctx == nil || productID < 1 {
		return false, memberport.ErrInvalidInput
	}
	var accepted bool
	err := tx(ctx).QueryRow(ctx, `SELECT true FROM public.products
WHERE id=$1 AND legacy_admin_projection->>'status'='service_period_enabled'
  AND legacy_admin_projection->>'enabled'='true' FOR SHARE`, productID).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return accepted, classify(err)
}

func (*Repository) CustomerExists(ctx context.Context, customerID int64) (bool, error) {
	if ctx == nil || customerID < 1 {
		return false, memberport.ErrInvalidInput
	}
	var exists bool
	err := tx(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.customers WHERE id=$1)`, customerID).Scan(&exists)
	return exists, classify(err)
}

func (*Repository) Get(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	return get(ctx, `SELECT `+memberColumns+` FROM public.service_period_members WHERE service_product_id=$1 AND member_ref=$2`, productID, memberRef)
}

func (*Repository) GetForUpdate(ctx context.Context, productID int64, memberRef string) (memberdomain.Member, error) {
	return get(ctx, `SELECT `+memberColumns+` FROM public.service_period_members WHERE service_product_id=$1 AND member_ref=$2 FOR UPDATE`, productID, memberRef)
}

func (*Repository) Create(ctx context.Context, record memberport.CreateRecord) (memberdomain.Member, error) {
	row := tx(ctx).QueryRow(ctx, `INSERT INTO public.service_period_members
(member_ref,service_product_id,customer_id,state,source,starts_at,expires_at,remark,alliance,version,created_at,updated_at)
VALUES ($1,$2,$3,'active',$4,$5,$6,$7,$8,1,$9,$9) RETURNING `+memberColumns,
		record.MemberRef, record.ServiceProductID, record.CustomerID, record.Source, record.StartsAt,
		record.ExpiresAt, record.Remark, record.Alliance, record.CreatedAt)
	return scanMember(row)
}

func (*Repository) Transition(ctx context.Context, record memberport.TransitionRecord) (memberdomain.Member, error) {
	row := tx(ctx).QueryRow(ctx, `UPDATE public.service_period_members SET
state=$4, expired_at=CASE WHEN $4='expired' THEN $5 ELSE expired_at END,
removed_at=CASE WHEN $4='removed' THEN $5 ELSE removed_at END,
version=version+1, updated_at=$5
WHERE service_product_id=$1 AND member_ref=$2 AND version=$3
  AND (($4='expired' AND state='active') OR ($4='removed' AND state<>'removed'))
RETURNING `+memberColumns, record.ServiceProductID, record.MemberRef, record.ExpectedVersion, record.Target, record.TransitionedAt)
	member, err := scanMember(row)
	if errors.Is(err, memberport.ErrNotFound) {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	return member, err
}

func (*Repository) UpdateFields(ctx context.Context, record memberport.UpdateFieldsRecord) (memberdomain.Member, error) {
	row := tx(ctx).QueryRow(ctx, `UPDATE public.service_period_members SET
remark=$4, alliance=$5, version=version+1, updated_at=$6
WHERE service_product_id=$1 AND member_ref=$2 AND version=$3 AND state<>'removed'
RETURNING `+memberColumns, record.ServiceProductID, record.MemberRef, record.ExpectedVersion,
		record.Remark, record.Alliance, record.UpdatedAt)
	member, err := scanMember(row)
	if errors.Is(err, memberport.ErrNotFound) {
		return memberdomain.Member{}, memberport.ErrConflict
	}
	return member, err
}

func (*Repository) List(ctx context.Context, query memberport.StoreListQuery) ([]memberdomain.Member, error) {
	var state, source any
	if query.State != nil {
		state = string(*query.State)
	}
	if query.Source != nil {
		source = string(*query.Source)
	}
	var updatedAt, memberRef any
	if query.After != nil {
		updatedAt, memberRef = query.After.UpdatedAt, query.After.MemberRef
	}
	rows, err := tx(ctx).Query(ctx, `SELECT `+memberColumns+` FROM public.service_period_members
WHERE service_product_id=$1 AND ($2::text IS NULL OR state=$2) AND ($3::text IS NULL OR source=$3)
  AND ($4::timestamptz IS NULL OR (updated_at,member_ref)<($4,$5::text))
ORDER BY updated_at DESC, member_ref DESC LIMIT $6`, query.ServiceProductID, state, source, updatedAt, memberRef, query.Limit)
	if err != nil {
		return nil, classify(err)
	}
	defer rows.Close()
	result := make([]memberdomain.Member, 0, query.Limit)
	for rows.Next() {
		member, scanErr := scanMember(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, member)
	}
	if err = rows.Err(); err != nil {
		return nil, classify(err)
	}
	return result, nil
}

func (*Repository) ReserveReceipt(ctx context.Context, reservation memberport.ReceiptReservation) (memberport.Receipt, bool, error) {
	row := tx(ctx).QueryRow(ctx, `INSERT INTO public.service_period_member_operation_receipts
(operation,actor_scope,key_digest,payload_digest,state,created_at)
VALUES ($1,$2,$3,$4,'reserved',$5)
ON CONFLICT (operation,actor_scope,key_digest) DO NOTHING
RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at`, reservation.Operation,
		reservation.ActorScope, reservation.KeyDigest[:], reservation.PayloadDigest[:], reservation.CreatedAt)
	receipt, err := scanReceipt(row)
	if err == nil {
		return receipt, true, nil
	}
	if !errors.Is(err, memberport.ErrNotFound) {
		return memberport.Receipt{}, false, err
	}
	row = tx(ctx).QueryRow(ctx, `SELECT id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at
FROM public.service_period_member_operation_receipts WHERE operation=$1 AND actor_scope=$2 AND key_digest=$3`,
		reservation.Operation, reservation.ActorScope, reservation.KeyDigest[:])
	receipt, err = scanReceipt(row)
	return receipt, false, err
}

func (*Repository) CompleteReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, completedAt time.Time) (memberport.Receipt, error) {
	row := tx(ctx).QueryRow(ctx, `UPDATE public.service_period_member_operation_receipts
SET state='completed',result_snapshot=$2,completed_at=$3 WHERE id=$1 AND state='reserved'
RETURNING id,operation,actor_scope,key_digest,payload_digest,state,result_snapshot,created_at`, receiptID, snapshot, completedAt)
	return scanReceipt(row)
}

type scanner interface{ Scan(...any) error }

func get(ctx context.Context, statement string, productID int64, memberRef string) (memberdomain.Member, error) {
	if ctx == nil || productID < 1 || !memberdomain.ValidMemberRef(memberRef) {
		return memberdomain.Member{}, memberport.ErrInvalidInput
	}
	return scanMember(tx(ctx).QueryRow(ctx, statement, productID, memberRef))
}

func scanMember(row scanner) (memberdomain.Member, error) {
	var member memberdomain.Member
	var state, source string
	var expiresAt, expiredAt, removedAt pgtype.Timestamptz
	err := row.Scan(&member.MemberRef, &member.ServiceProductID, &member.CustomerID, &state, &source,
		&member.StartsAt, &expiresAt, &expiredAt, &removedAt, &member.Remark, &member.Alliance,
		&member.Version, &member.CreatedAt, &member.UpdatedAt)
	if err != nil {
		return memberdomain.Member{}, classify(err)
	}
	member.State, member.Source = memberdomain.State(state), memberdomain.Source(source)
	member.StartsAt, member.CreatedAt, member.UpdatedAt = member.StartsAt.UTC(), member.CreatedAt.UTC(), member.UpdatedAt.UTC()
	member.ExpiresAt, member.ExpiredAt, member.RemovedAt = optionalTime(expiresAt), optionalTime(expiredAt), optionalTime(removedAt)
	return member, nil
}

func scanReceipt(row scanner) (memberport.Receipt, error) {
	var receipt memberport.Receipt
	var keyDigest, payloadDigest []byte
	err := row.Scan(&receipt.ID, &receipt.Operation, &receipt.ActorScope, &keyDigest, &payloadDigest,
		&receipt.State, &receipt.ResultSnapshot, &receipt.CreatedAt)
	if err != nil {
		return memberport.Receipt{}, classify(err)
	}
	if len(keyDigest) != 32 || len(payloadDigest) != 32 {
		return memberport.Receipt{}, memberport.ErrUnavailable
	}
	copy(receipt.KeyDigest[:], keyDigest)
	copy(receipt.PayloadDigest[:], payloadDigest)
	return receipt, nil
}

func tx(ctx context.Context) pgx.Tx {
	transaction, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return invalidTx{err: err}
	}
	return transaction
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return memberport.ErrNotFound
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		switch postgresError.Code {
		case "23503":
			return memberport.ErrNotFound
		case "23505":
			return memberport.ErrConflict
		}
	}
	return errors.Join(memberport.ErrUnavailable, err)
}

// invalidTx keeps missing transaction failures on the normal repository error
// path without allowing a pool or implicit cross-transaction fallback.
type invalidTx struct {
	pgx.Tx
	err error
}

func (transaction invalidTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{transaction.err}
}
func (transaction invalidTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, transaction.err
}

type errorRow struct{ err error }

func (row errorRow) Scan(...any) error { return row.err }

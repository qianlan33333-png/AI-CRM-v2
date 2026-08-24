package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CustomerSafeExportRepository struct{}

var _ contactapp.CustomerSafeExportStore = (*CustomerSafeExportRepository)(nil)

func NewCustomerSafeExportRepository() *CustomerSafeExportRepository {
	return &CustomerSafeExportRepository{}
}

func (repository *CustomerSafeExportRepository) ReserveCustomerSafeExportReceipt(ctx context.Context, actorID int64, keyDigest, payloadDigest [32]byte, createdAt time.Time) (contactapp.CustomerSafeExportReceipt, bool, error) {
	tx, err := customerSafeExportTx(ctx)
	if repository == nil || err != nil {
		return contactapp.CustomerSafeExportReceipt{}, false, customerSafeExportUnavailable(err)
	}
	var result contactapp.CustomerSafeExportReceipt
	var returnedDigest []byte
	err = tx.QueryRow(ctx, `
INSERT INTO public.customer_safe_export_receipts(actor_id,key_digest,payload_digest,created_at)
VALUES($1,$2,$3,$4) ON CONFLICT(actor_id,key_digest) DO NOTHING
RETURNING id,payload_digest,result_snapshot,state='completed'`, actorID, keyDigest[:], payloadDigest[:], createdAt).Scan(&result.ID, &returnedDigest, &result.ResultSnapshot, &result.Completed)
	if err == nil {
		if len(returnedDigest) != len(result.PayloadDigest) {
			return contactapp.CustomerSafeExportReceipt{}, false, contactapp.ErrCustomerSafeExportUnavailable
		}
		copy(result.PayloadDigest[:], returnedDigest)
		return result, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExportReceipt{}, false, customerSafeExportUnavailable(err)
	}
	var storedDigest []byte
	err = tx.QueryRow(ctx, `
SELECT id,payload_digest,result_snapshot,state='completed'
FROM public.customer_safe_export_receipts
WHERE actor_id=$1 AND key_digest=$2 FOR UPDATE`, actorID, keyDigest[:]).Scan(&result.ID, &storedDigest, &result.ResultSnapshot, &result.Completed)
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, false, customerSafeExportUnavailable(err)
	}
	if len(storedDigest) != len(result.PayloadDigest) || subtle.ConstantTimeCompare(storedDigest, payloadDigest[:]) != 1 {
		return contactapp.CustomerSafeExportReceipt{}, false, contactapp.ErrCustomerSafeExportConflict
	}
	copy(result.PayloadDigest[:], storedDigest)
	return result, false, nil
}

func (repository *CustomerSafeExportRepository) CreateCustomerSafeExport(ctx context.Context, export contactapp.CustomerSafeExport, actorID int64, ownerScopeStaffID *int64, filterDigest [32]byte, query contactapp.CustomerListQuery) ([]contactapp.CustomerSafeExportRow, error) {
	tx, err := customerSafeExportTx(ctx)
	if repository == nil || err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	rows, err := tx.Query(ctx, `
SELECT c.id,c.name,c.owner_staff_id,c.stage_id,c.channel_id,c.added_at,c.last_interact_at
FROM public.customers c
WHERE c.updated_at <= $1
  AND NOT c.is_deleted
  AND ($2::text IS NULL OR lower(c.name) % lower($2))
  AND ($3::bigint IS NULL OR c.owner_staff_id=$3)
  AND ($4::bigint IS NULL OR c.stage_id=$4)
  AND ($5::bigint IS NULL OR c.channel_id=$5)
  AND ($6::bigint IS NULL OR EXISTS (SELECT 1 FROM public.customer_tags ct WHERE ct.customer_id=c.id AND ct.tag_id=$6))
  AND ($7::timestamptz IS NULL OR c.added_at >= $7)
  AND ($8::timestamptz IS NULL OR c.added_at <= $8)
  AND ($9::timestamptz IS NULL OR c.last_interact_at >= $9)
  AND ($10::timestamptz IS NULL OR c.last_interact_at <= $10)
ORDER BY c.updated_at DESC,c.id DESC
LIMIT $11`, query.Watermark, nullableString(query.Keyword), query.OwnerStaffID, query.StageID, query.ChannelID, query.TagID, query.AddedAfter, query.AddedBefore, query.LastInteractAfter, query.LastInteractBefore, contactapp.CustomerSafeExportMaximumRows+1)
	if err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	defer rows.Close()
	result := make([]contactapp.CustomerSafeExportRow, 0)
	for rows.Next() {
		var row contactapp.CustomerSafeExportRow
		if err = rows.Scan(&row.CustomerID, &row.DisplayName, &row.OwnerStaffID, &row.StageID, &row.ChannelID, &row.AddedAt, &row.LastInteractAt); err != nil {
			return nil, customerSafeExportUnavailable(err)
		}
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	if len(result) > contactapp.CustomerSafeExportMaximumRows {
		return nil, contactapp.ErrCustomerSafeExportConflict
	}
	if _, err = tx.Exec(ctx, `
INSERT INTO public.customer_safe_exports(id,actor_id,owner_scope_staff_id,filter_digest,watermark,record_count,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7)`, export.ID, actorID, ownerScopeStaffID, filterDigest[:], export.Watermark, len(result), export.CreatedAt); err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	for index, row := range result {
		if _, err = tx.Exec(ctx, `
INSERT INTO public.customer_safe_export_rows(export_id,row_index,customer_id,display_name,owner_staff_id,stage_id,channel_id,added_at,last_interact_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, export.ID, index+1, row.CustomerID, row.DisplayName, row.OwnerStaffID, row.StageID, row.ChannelID, row.AddedAt, row.LastInteractAt); err != nil {
			return nil, customerSafeExportUnavailable(err)
		}
	}
	return result, nil
}

func (repository *CustomerSafeExportRepository) CompleteCustomerSafeExportReceipt(ctx context.Context, receiptID int64, exportID string, snapshot json.RawMessage, completedAt time.Time) (contactapp.CustomerSafeExportReceipt, error) {
	tx, err := customerSafeExportTx(ctx)
	if repository == nil || err != nil || receiptID < 1 || len(snapshot) == 0 {
		return contactapp.CustomerSafeExportReceipt{}, customerSafeExportUnavailable(err)
	}
	var result contactapp.CustomerSafeExportReceipt
	var returnedDigest []byte
	err = tx.QueryRow(ctx, `
UPDATE public.customer_safe_export_receipts
SET export_id=$2,state='completed',result_snapshot=$3,completed_at=$4
WHERE id=$1 AND state='reserved'
RETURNING id,payload_digest,result_snapshot,state='completed'`, receiptID, exportID, snapshot, completedAt).Scan(&result.ID, &returnedDigest, &result.ResultSnapshot, &result.Completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExportReceipt{}, contactapp.ErrCustomerSafeExportConflict
	}
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, customerSafeExportUnavailable(err)
	}
	if len(returnedDigest) != len(result.PayloadDigest) {
		return contactapp.CustomerSafeExportReceipt{}, contactapp.ErrCustomerSafeExportUnavailable
	}
	copy(result.PayloadDigest[:], returnedDigest)
	return result, nil
}

func (repository *CustomerSafeExportRepository) ReadCustomerSafeExport(ctx context.Context, exportID string, actorID int64) (contactapp.CustomerSafeExport, error) {
	tx, err := customerSafeExportTx(ctx)
	if repository == nil || err != nil {
		return contactapp.CustomerSafeExport{}, customerSafeExportUnavailable(err)
	}
	var result contactapp.CustomerSafeExport
	err = tx.QueryRow(ctx, `SELECT id,record_count,watermark,created_at FROM public.customer_safe_exports WHERE id=$1 AND actor_id=$2`, exportID, actorID).Scan(&result.ID, &result.RecordCount, &result.Watermark, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExport{}, contactapp.ErrCustomerSafeExportNotFound
	}
	if err != nil {
		return contactapp.CustomerSafeExport{}, customerSafeExportUnavailable(err)
	}
	return result, nil
}

func (repository *CustomerSafeExportRepository) ReadCustomerSafeExportRows(ctx context.Context, exportID string, actorID int64, ownerScopeStaffID *int64) ([]contactapp.CustomerSafeExportRow, error) {
	tx, err := customerSafeExportTx(ctx)
	if repository == nil || err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	if ownerScopeStaffID != nil {
		var drifted bool
		err = tx.QueryRow(ctx, `
SELECT EXISTS(
 SELECT 1 FROM public.customer_safe_export_rows r
 LEFT JOIN public.customers c ON c.id=r.customer_id
 WHERE r.export_id=$1 AND (c.id IS NULL OR c.is_deleted OR c.owner_staff_id IS DISTINCT FROM $2)
)`, exportID, *ownerScopeStaffID).Scan(&drifted)
		if err != nil {
			return nil, customerSafeExportUnavailable(err)
		}
		if drifted {
			return nil, contactapp.ErrCustomerSafeExportConflict
		}
	}
	rows, err := tx.Query(ctx, `
SELECT r.customer_id,r.display_name,r.owner_staff_id,r.stage_id,r.channel_id,r.added_at,r.last_interact_at
FROM public.customer_safe_export_rows r
JOIN public.customer_safe_exports e ON e.id=r.export_id
WHERE r.export_id=$1 AND e.actor_id=$2
ORDER BY r.row_index`, exportID, actorID)
	if err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	defer rows.Close()
	result := make([]contactapp.CustomerSafeExportRow, 0)
	for rows.Next() {
		var row contactapp.CustomerSafeExportRow
		if err = rows.Scan(&row.CustomerID, &row.DisplayName, &row.OwnerStaffID, &row.StageID, &row.ChannelID, &row.AddedAt, &row.LastInteractAt); err != nil {
			return nil, customerSafeExportUnavailable(err)
		}
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	return result, nil
}

func customerSafeExportTx(ctx context.Context) (pgx.Tx, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	return platformstore.TxFromContext(ctx)
}

func customerSafeExportUnavailable(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(contactapp.ErrCustomerSafeExportUnavailable, err)
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

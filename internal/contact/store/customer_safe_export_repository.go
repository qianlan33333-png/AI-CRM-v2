package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CustomerSafeExportRepository struct{}

var _ contactapp.CustomerSafeExportStore = (*CustomerSafeExportRepository)(nil)

func NewCustomerSafeExportRepository() *CustomerSafeExportRepository {
	return &CustomerSafeExportRepository{}
}

func (repository *CustomerSafeExportRepository) ReserveCustomerSafeExportReceipt(ctx context.Context, actorID int64, keyDigest, payloadDigest [32]byte, createdAt time.Time) (contactapp.CustomerSafeExportReceipt, bool, error) {
	queries, err := customerSafeExportQueries(ctx, repository)
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, false, err
	}
	reserved, err := queries.ReserveCustomerSafeExportReceipt(ctx, contactdb.ReserveCustomerSafeExportReceiptParams{
		ActorID: actorID, KeyDigest: keyDigest[:], PayloadDigest: payloadDigest[:], CreatedAt: timestampValue(createdAt),
	})
	if err == nil {
		receipt, receiptErr := customerSafeExportReceipt(reserved.ID, reserved.PayloadDigest, reserved.ResultSnapshot, reserved.Completed)
		if receiptErr != nil {
			return contactapp.CustomerSafeExportReceipt{}, false, receiptErr
		}
		return receipt, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExportReceipt{}, false, customerSafeExportUnavailable(err)
	}
	existing, err := queries.GetCustomerSafeExportReceipt(ctx, contactdb.GetCustomerSafeExportReceiptParams{ActorID: actorID, KeyDigest: keyDigest[:]})
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, false, customerSafeExportUnavailable(err)
	}
	receipt, receiptErr := customerSafeExportReceipt(existing.ID, existing.PayloadDigest, existing.ResultSnapshot, existing.Completed)
	if receiptErr != nil {
		return contactapp.CustomerSafeExportReceipt{}, false, receiptErr
	}
	if subtle.ConstantTimeCompare(receipt.PayloadDigest[:], payloadDigest[:]) != 1 {
		return contactapp.CustomerSafeExportReceipt{}, false, contactapp.ErrCustomerSafeExportConflict
	}
	return receipt, false, nil
}

func (repository *CustomerSafeExportRepository) CreateCustomerSafeExport(ctx context.Context, export contactapp.CustomerSafeExport, actorID int64, ownerScopeStaffID *int64, filterDigest [32]byte, query contactapp.CustomerListQuery) ([]contactapp.CustomerSafeExportRow, error) {
	queries, err := customerSafeExportQueries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListCustomerSafeExportSnapshotRows(ctx, contactdb.ListCustomerSafeExportSnapshotRowsParams{
		Watermark:          timestampValue(query.Watermark),
		Keyword:            nullableKeyword(query.Keyword),
		OwnerStaffID:       nullableInt64(query.OwnerStaffID),
		StageID:            nullableInt64(query.StageID),
		ChannelID:          nullableInt64(query.ChannelID),
		TagID:              nullableInt64(query.TagID),
		AddedAfter:         nullableTimestamp(query.AddedAfter),
		AddedBefore:        nullableTimestamp(query.AddedBefore),
		LastInteractAfter:  nullableTimestamp(query.LastInteractAfter),
		LastInteractBefore: nullableTimestamp(query.LastInteractBefore),
		RowLimit:           int32(contactapp.CustomerSafeExportMaximumRows + 1),
	})
	if err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	if len(rows) > contactapp.CustomerSafeExportMaximumRows {
		return nil, contactapp.ErrCustomerSafeExportConflict
	}
	result := make([]contactapp.CustomerSafeExportRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, customerSafeExportRowFromSnapshot(row))
	}
	if err := queries.InsertCustomerSafeExport(ctx, contactdb.InsertCustomerSafeExportParams{
		ID: export.ID, ActorID: actorID, OwnerScopeStaffID: nullableInt64(ownerScopeStaffID), FilterDigest: filterDigest[:],
		Watermark: timestampValue(export.Watermark), RecordCount: int32(len(result)), CreatedAt: timestampValue(export.CreatedAt),
	}); err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	for index, row := range result {
		if err := queries.InsertCustomerSafeExportRow(ctx, contactdb.InsertCustomerSafeExportRowParams{
			ExportID: export.ID, RowIndex: int32(index + 1), CustomerID: row.CustomerID, DisplayName: row.DisplayName,
			OwnerStaffID: nullableInt64(row.OwnerStaffID), StageID: nullableInt64(row.StageID), ChannelID: nullableInt64(row.ChannelID),
			AddedAt: nullableTimestamp(row.AddedAt), LastInteractAt: nullableTimestamp(row.LastInteractAt),
		}); err != nil {
			return nil, customerSafeExportUnavailable(err)
		}
	}
	return result, nil
}

func (repository *CustomerSafeExportRepository) CompleteCustomerSafeExportReceipt(ctx context.Context, receiptID int64, exportID string, snapshot json.RawMessage, completedAt time.Time) (contactapp.CustomerSafeExportReceipt, error) {
	if receiptID < 1 || len(snapshot) == 0 {
		return contactapp.CustomerSafeExportReceipt{}, contactapp.ErrCustomerSafeExportUnavailable
	}
	queries, err := customerSafeExportQueries(ctx, repository)
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, err
	}
	completed, err := queries.CompleteCustomerSafeExportReceipt(ctx, contactdb.CompleteCustomerSafeExportReceiptParams{
		ID: receiptID, ExportID: pgtype.Text{String: exportID, Valid: true}, ResultSnapshot: snapshot, CompletedAt: timestampValue(completedAt),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExportReceipt{}, contactapp.ErrCustomerSafeExportConflict
	}
	if err != nil {
		return contactapp.CustomerSafeExportReceipt{}, customerSafeExportUnavailable(err)
	}
	return customerSafeExportReceipt(completed.ID, completed.PayloadDigest, completed.ResultSnapshot, completed.Completed)
}

func (repository *CustomerSafeExportRepository) ReadCustomerSafeExport(ctx context.Context, exportID string, actorID int64) (contactapp.CustomerSafeExport, error) {
	queries, err := customerSafeExportQueries(ctx, repository)
	if err != nil {
		return contactapp.CustomerSafeExport{}, err
	}
	row, err := queries.GetCustomerSafeExport(ctx, contactdb.GetCustomerSafeExportParams{ID: exportID, ActorID: actorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerSafeExport{}, contactapp.ErrCustomerSafeExportNotFound
	}
	if err != nil {
		return contactapp.CustomerSafeExport{}, customerSafeExportUnavailable(err)
	}
	return contactapp.CustomerSafeExport{
		ID: row.ID, RecordCount: int(row.RecordCount), Watermark: row.Watermark.Time, CreatedAt: row.CreatedAt.Time,
		OwnerScopeStaffID: int64Pointer(row.OwnerScopeStaffID),
	}, nil
}

func (repository *CustomerSafeExportRepository) ReadCustomerSafeExportRows(ctx context.Context, exportID string, actorID int64, ownerScopeStaffID *int64) ([]contactapp.CustomerSafeExportRow, error) {
	queries, err := customerSafeExportQueries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListLockedCustomerSafeExportRows(ctx, contactdb.ListLockedCustomerSafeExportRowsParams{ExportID: exportID, ActorID: actorID})
	if err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	result := make([]contactapp.CustomerSafeExportRow, 0, len(rows))
	for _, row := range rows {
		currentOwnerStaffID := int64Pointer(row.CurrentOwnerStaffID)
		if ownerScopeStaffID != nil && (row.IsDeleted || currentOwnerStaffID == nil || *currentOwnerStaffID != *ownerScopeStaffID) {
			return nil, contactapp.ErrCustomerSafeExportConflict
		}
		result = append(result, contactapp.CustomerSafeExportRow{
			CustomerID: row.CustomerID, DisplayName: row.DisplayName, OwnerStaffID: int64Pointer(row.OwnerStaffID),
			StageID: int64Pointer(row.StageID), ChannelID: int64Pointer(row.ChannelID), AddedAt: timestampPointer(row.AddedAt), LastInteractAt: timestampPointer(row.LastInteractAt),
		})
	}
	return result, nil
}

func customerSafeExportQueries(ctx context.Context, repository *CustomerSafeExportRepository) (*contactdb.Queries, error) {
	if repository == nil {
		return nil, contactapp.ErrCustomerSafeExportUnavailable
	}
	tx, err := customerSafeExportTx(ctx)
	if err != nil {
		return nil, customerSafeExportUnavailable(err)
	}
	return contactdb.New(tx), nil
}

func customerSafeExportReceipt(id int64, payloadDigest, resultSnapshot []byte, completed bool) (contactapp.CustomerSafeExportReceipt, error) {
	if len(payloadDigest) != 32 {
		return contactapp.CustomerSafeExportReceipt{}, contactapp.ErrCustomerSafeExportUnavailable
	}
	var digest [32]byte
	copy(digest[:], payloadDigest)
	return contactapp.CustomerSafeExportReceipt{ID: id, PayloadDigest: digest, ResultSnapshot: append(json.RawMessage(nil), resultSnapshot...), Completed: completed}, nil
}

func customerSafeExportRowFromSnapshot(row contactdb.ListCustomerSafeExportSnapshotRowsRow) contactapp.CustomerSafeExportRow {
	return contactapp.CustomerSafeExportRow{
		CustomerID: row.ID, DisplayName: row.Name, OwnerStaffID: int64Pointer(row.OwnerStaffID), StageID: int64Pointer(row.StageID),
		ChannelID: int64Pointer(row.ChannelID), AddedAt: timestampPointer(row.AddedAt), LastInteractAt: timestampPointer(row.LastInteractAt),
	}
}

func timestampValue(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
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

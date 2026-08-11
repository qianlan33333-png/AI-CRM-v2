package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// CustomerDetailRepository serves contact-owned customer detail reads through
// the transaction-bound unit-of-work context.
type CustomerDetailRepository struct{}

var _ contactapp.CustomerDetailStore = (*CustomerDetailRepository)(nil)

func NewCustomerDetailRepository() *CustomerDetailRepository {
	return &CustomerDetailRepository{}
}

func (*CustomerDetailRepository) GetCustomerDetail(
	ctx context.Context,
	input contactapp.CustomerDetailInput,
) (contactapp.CustomerDetailStoreResult, error) {
	if input.ID <= 0 || (input.OwnerStaffID != nil && *input.OwnerStaffID <= 0) {
		return contactapp.CustomerDetailStoreResult{}, contactapp.ErrInvalidCustomerDetailQuery
	}

	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactapp.CustomerDetailStoreResult{}, err
	}
	queries := contactdb.New(tx)

	customer, err := queries.GetCustomerDetailCustomer(ctx, contactdb.GetCustomerDetailCustomerParams{
		CustomerID:   int64(input.ID),
		OwnerStaffID: nullableInt64(input.OwnerStaffID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactapp.CustomerDetailStoreResult{}, contactapp.ErrCustomerNotFound
	}
	if err != nil {
		return contactapp.CustomerDetailStoreResult{}, err
	}

	rows, err := queries.ListCustomerDetailTags(ctx, int64(input.ID))
	if err != nil {
		return contactapp.CustomerDetailStoreResult{}, err
	}
	tags := make([]contactapp.CustomerTagRecord, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, contactapp.CustomerTagRecord{
			ID:             row.ID,
			GroupID:        int64Pointer(row.GroupID),
			GroupName:      textPointer(row.GroupName),
			GroupSortOrder: row.GroupSortOrder,
			Name:           row.Name,
			SortOrder:      row.SortOrder,
		})
	}

	return contactapp.CustomerDetailStoreResult{
		Customer: customerRecordFromRow(customer),
		Tags:     tags,
	}, nil
}

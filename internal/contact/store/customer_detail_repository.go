package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// CustomerDetailRepository serves contact-owned customer detail reads through
// the transaction-bound unit-of-work context.
type CustomerDetailRepository struct{}

var _ contactapp.CustomerDetailStore = (*CustomerDetailRepository)(nil)
var _ contactport.CustomerReader = (*CustomerDetailRepository)(nil)

func NewCustomerDetailRepository() *CustomerDetailRepository {
	return &CustomerDetailRepository{}
}

func (*CustomerDetailRepository) ReadCustomer(ctx context.Context, id contactport.CustomerID) (contactport.CustomerProjection, error) {
	if id < 1 {
		return contactport.CustomerProjection{}, contactport.ErrCustomerReadNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return contactport.CustomerProjection{}, errors.Join(contactport.ErrCustomerReadUnavailable, err)
	}
	row, err := contactdb.New(tx).ReadCustomerProjection(ctx, int64(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.CustomerProjection{}, contactport.ErrCustomerReadNotFound
	}
	if err != nil {
		return contactport.CustomerProjection{}, errors.Join(contactport.ErrCustomerReadUnavailable, err)
	}
	return contactport.CustomerProjection{ID: contactport.CustomerID(row.ID), Name: row.Name}, nil
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

	rows, err := queries.GetCustomerDetailSnapshot(ctx, contactdb.GetCustomerDetailSnapshotParams{
		CustomerID:   int64(input.ID),
		OwnerStaffID: nullableInt64(input.OwnerStaffID),
	})
	if err != nil {
		return contactapp.CustomerDetailStoreResult{}, err
	}
	if len(rows) == 0 {
		return contactapp.CustomerDetailStoreResult{}, contactapp.ErrCustomerNotFound
	}
	first := rows[0]
	tags := make([]contactapp.CustomerTagRecord, 0, len(rows))
	for _, row := range rows {
		if !row.TagID.Valid {
			if row.GroupID.Valid || row.GroupName.Valid || row.TagName.Valid || row.TagSortOrder.Valid {
				return contactapp.CustomerDetailStoreResult{}, errors.New("customer detail snapshot has an invalid empty tag")
			}
			continue
		}
		if !row.TagName.Valid || !row.TagSortOrder.Valid || row.GroupID.Valid != row.GroupName.Valid {
			return contactapp.CustomerDetailStoreResult{}, errors.New("customer detail snapshot has an invalid tag")
		}
		tags = append(tags, contactapp.CustomerTagRecord{
			ID:             row.TagID.Int64,
			GroupID:        int64Pointer(row.GroupID),
			GroupName:      textPointer(row.GroupName),
			GroupSortOrder: row.GroupSortOrder,
			Name:           row.TagName.String,
			SortOrder:      row.TagSortOrder.Int32,
		})
	}

	return contactapp.CustomerDetailStoreResult{
		Customer: customerRecordFromRow(contactdb.Customer{
			ID: first.ID, Name: first.Name, AvatarUrl: first.AvatarUrl, Gender: first.Gender,
			StageID: first.StageID, OwnerStaffID: first.OwnerStaffID, ChannelID: first.ChannelID,
			AddedAt: first.AddedAt, LastInteractAt: first.LastInteractAt, IsDeleted: first.IsDeleted,
			Extra: first.Extra, CreatedAt: first.CreatedAt, UpdatedAt: first.UpdatedAt,
		}),
		Tags: tags,
	}, nil
}

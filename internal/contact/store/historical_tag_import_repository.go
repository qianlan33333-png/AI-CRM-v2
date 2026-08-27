package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactdb "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// HistoricalTagImportRepository is the Contact-owned physical writer. Durable
// source lineage is intentionally supplied by the main receipt journal rather
// than inferred from mutable names or a new table in this leaf.
type HistoricalTagImportRepository struct{}

var _ contactport.HistoricalTagStore = (*HistoricalTagImportRepository)(nil)

func NewHistoricalTagImportRepository() *HistoricalTagImportRepository {
	return &HistoricalTagImportRepository{}
}

func historicalTagImportTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, contactport.ErrHistoricalTagUnavailable
	}
	return tx, nil
}

func (*HistoricalTagImportRepository) GetHistoricalTagGroup(ctx context.Context, id int64) (contactport.HistoricalTagGroup, error) {
	if id < 1 {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTagGroup{}, err
	}
	value, err := contactdb.New(tx).LockHistoricalTagImportGroup(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagBlocked
	}
	if err != nil {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagUnavailable
	}
	return contactport.HistoricalTagGroup{ID: value.ID, Name: value.Name, SortOrder: value.SortOrder}, nil
}

func (*HistoricalTagImportRepository) CreateHistoricalTagGroup(ctx context.Context, value contactport.HistoricalTagGroup) (contactport.HistoricalTagGroup, error) {
	if value.Name == "" {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTagGroup{}, err
	}
	row, err := contactdb.New(tx).CreateHistoricalTagImportGroup(ctx, contactdb.CreateHistoricalTagImportGroupParams{Name: value.Name, SortOrder: value.SortOrder})
	if err != nil {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagUnavailable
	}
	return contactport.HistoricalTagGroup{ID: row.ID, Name: row.Name, SortOrder: row.SortOrder}, nil
}

func (*HistoricalTagImportRepository) GetHistoricalTag(ctx context.Context, id int64) (contactport.HistoricalTag, error) {
	if id < 1 {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTag{}, err
	}
	row, err := contactdb.New(tx).LockHistoricalTagImport(ctx, id)
	return historicalTagFromRow(row.ID, row.GroupID, row.WecomTagID, row.Name, row.SortOrder, err)
}

func (*HistoricalTagImportRepository) FindHistoricalTagByProviderID(ctx context.Context, providerTagID string) (contactport.HistoricalTag, bool, error) {
	if providerTagID == "" {
		return contactport.HistoricalTag{}, false, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTag{}, false, err
	}
	row, err := contactdb.New(tx).LockHistoricalTagImportByProviderID(ctx, providerTagID)
	value, err := historicalTagFromRow(row.ID, row.GroupID, row.WecomTagID, row.Name, row.SortOrder, err)
	if errors.Is(err, contactport.ErrHistoricalTagBlocked) {
		return contactport.HistoricalTag{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalTag{}, false, err
	}
	return value, true, nil
}

func (*HistoricalTagImportRepository) CreateHistoricalTag(ctx context.Context, value contactport.HistoricalTag) (contactport.HistoricalTag, bool, error) {
	if value.GroupID < 1 || value.ProviderTagID == "" || value.Name == "" {
		return contactport.HistoricalTag{}, false, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTag{}, false, err
	}
	row, err := contactdb.New(tx).CreateHistoricalTagImport(ctx, contactdb.CreateHistoricalTagImportParams{GroupID: value.GroupID, ProviderTagID: value.ProviderTagID, Name: value.Name, SortOrder: value.SortOrder})
	if err == nil {
		created, mapErr := historicalTagFromRow(row.ID, row.GroupID, row.WecomTagID, row.Name, row.SortOrder, nil)
		if mapErr != nil {
			return contactport.HistoricalTag{}, false, mapErr
		}
		return created, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalTag{}, false, contactport.ErrHistoricalTagUnavailable
	}
	value, found, err := (&HistoricalTagImportRepository{}).FindHistoricalTagByProviderID(ctx, value.ProviderTagID)
	if err != nil || !found {
		return contactport.HistoricalTag{}, false, contactport.ErrHistoricalTagUnavailable
	}
	return value, false, nil
}

func (*HistoricalTagImportRepository) GetHistoricalCustomerTag(ctx context.Context, customerID contactport.CustomerID, tagID int64) (contactport.HistoricalCustomerTag, bool, error) {
	if customerID < 1 || tagID < 1 {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalCustomerTag{}, false, err
	}
	row, err := contactdb.New(tx).LockHistoricalCustomerTagImport(ctx, contactdb.LockHistoricalCustomerTagImportParams{CustomerID: int64(customerID), TagID: tagID})
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalCustomerTag{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagUnavailable
	}
	value, err := historicalCustomerTagFromRow(row.CustomerID, row.TagID, row.TaggedAt, row.TaggedBy)
	if err != nil {
		return contactport.HistoricalCustomerTag{}, false, err
	}
	return value, true, nil
}

func (*HistoricalTagImportRepository) BindHistoricalCustomerTag(ctx context.Context, value contactport.HistoricalCustomerTag) (contactport.HistoricalCustomerTag, bool, error) {
	if value.CustomerID < 1 || value.TagID < 1 || value.TaggedAt.IsZero() || value.TaggedBy == "" {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalCustomerTag{}, false, err
	}
	row, err := contactdb.New(tx).BindHistoricalCustomerTagImport(ctx, contactdb.BindHistoricalCustomerTagImportParams{CustomerID: int64(value.CustomerID), TagID: value.TagID, TaggedAt: pgtype.Timestamptz{Time: value.TaggedAt.UTC(), Valid: true}, TaggedBy: value.TaggedBy})
	if err == nil {
		bound, mapErr := historicalCustomerTagFromRow(row.CustomerID, row.TagID, row.TaggedAt, row.TaggedBy)
		if mapErr != nil {
			return contactport.HistoricalCustomerTag{}, false, mapErr
		}
		return bound, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagUnavailable
	}
	value, found, err := (&HistoricalTagImportRepository{}).GetHistoricalCustomerTag(ctx, value.CustomerID, value.TagID)
	if err != nil || !found {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagUnavailable
	}
	return value, false, nil
}

func historicalTagFromRow(id int64, groupID pgtype.Int8, providerTagID pgtype.Text, name string, sortOrder int32, err error) (contactport.HistoricalTag, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagBlocked
	}
	if err != nil || id < 1 || !groupID.Valid || groupID.Int64 < 1 || !providerTagID.Valid || providerTagID.String == "" {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagUnavailable
	}
	return contactport.HistoricalTag{ID: id, GroupID: groupID.Int64, ProviderTagID: providerTagID.String, Name: name, SortOrder: sortOrder}, nil
}

func historicalCustomerTagFromRow(customerID, tagID int64, taggedAt pgtype.Timestamptz, taggedBy string) (contactport.HistoricalCustomerTag, error) {
	if customerID < 1 || tagID < 1 || !taggedAt.Valid || taggedBy == "" {
		return contactport.HistoricalCustomerTag{}, contactport.ErrHistoricalTagUnavailable
	}
	return contactport.HistoricalCustomerTag{CustomerID: contactport.CustomerID(customerID), TagID: tagID, TaggedAt: taggedAt.Time.UTC(), TaggedBy: taggedBy}, nil
}

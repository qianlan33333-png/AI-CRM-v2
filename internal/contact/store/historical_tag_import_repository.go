package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
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
	var value contactport.HistoricalTagGroup
	err = tx.QueryRow(ctx, `SELECT id, name, sort_order FROM public.tag_groups WHERE id=$1 FOR KEY SHARE`, id).Scan(&value.ID, &value.Name, &value.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagBlocked
	}
	if err != nil {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagUnavailable
	}
	return value, nil
}

func (*HistoricalTagImportRepository) CreateHistoricalTagGroup(ctx context.Context, value contactport.HistoricalTagGroup) (contactport.HistoricalTagGroup, error) {
	if value.Name == "" {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTagGroup{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO public.tag_groups (name, sort_order) VALUES ($1, $2) RETURNING id, name, sort_order`, value.Name, value.SortOrder).Scan(&value.ID, &value.Name, &value.SortOrder)
	if err != nil {
		return contactport.HistoricalTagGroup{}, contactport.ErrHistoricalTagUnavailable
	}
	return value, nil
}

func (*HistoricalTagImportRepository) GetHistoricalTag(ctx context.Context, id int64) (contactport.HistoricalTag, error) {
	if id < 1 {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTag{}, err
	}
	return scanHistoricalTag(tx.QueryRow(ctx, `SELECT id, group_id, wecom_tag_id, name, sort_order FROM public.tags WHERE id=$1 FOR KEY SHARE`, id))
}

func (*HistoricalTagImportRepository) FindHistoricalTagByProviderID(ctx context.Context, providerTagID string) (contactport.HistoricalTag, bool, error) {
	if providerTagID == "" {
		return contactport.HistoricalTag{}, false, contactport.ErrHistoricalTagInput
	}
	tx, err := historicalTagImportTx(ctx)
	if err != nil {
		return contactport.HistoricalTag{}, false, err
	}
	value, err := scanHistoricalTag(tx.QueryRow(ctx, `SELECT id, group_id, wecom_tag_id, name, sort_order FROM public.tags WHERE wecom_tag_id=$1 FOR KEY SHARE`, providerTagID))
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
	err = tx.QueryRow(ctx, `INSERT INTO public.tags (group_id, wecom_tag_id, name, sort_order) VALUES ($1, $2, $3, $4) ON CONFLICT (wecom_tag_id) DO NOTHING RETURNING id, group_id, wecom_tag_id, name, sort_order`, value.GroupID, value.ProviderTagID, value.Name, value.SortOrder).Scan(&value.ID, &value.GroupID, &value.ProviderTagID, &value.Name, &value.SortOrder)
	if err == nil {
		return value, true, nil
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
	var value contactport.HistoricalCustomerTag
	err = tx.QueryRow(ctx, `SELECT customer_id, tag_id, tagged_at, tagged_by FROM public.customer_tags WHERE customer_id=$1 AND tag_id=$2 FOR KEY SHARE`, customerID, tagID).Scan(&value.CustomerID, &value.TagID, &value.TaggedAt, &value.TaggedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalCustomerTag{}, false, nil
	}
	if err != nil {
		return contactport.HistoricalCustomerTag{}, false, contactport.ErrHistoricalTagUnavailable
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
	err = tx.QueryRow(ctx, `INSERT INTO public.customer_tags (customer_id, tag_id, tagged_at, tagged_by) VALUES ($1, $2, $3, $4) ON CONFLICT (customer_id, tag_id) DO NOTHING RETURNING customer_id, tag_id, tagged_at, tagged_by`, value.CustomerID, value.TagID, value.TaggedAt.UTC(), value.TaggedBy).Scan(&value.CustomerID, &value.TagID, &value.TaggedAt, &value.TaggedBy)
	if err == nil {
		return value, true, nil
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

func scanHistoricalTag(row pgx.Row) (contactport.HistoricalTag, error) {
	var value contactport.HistoricalTag
	if err := row.Scan(&value.ID, &value.GroupID, &value.ProviderTagID, &value.Name, &value.SortOrder); errors.Is(err, pgx.ErrNoRows) {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagBlocked
	} else if err != nil {
		return contactport.HistoricalTag{}, contactport.ErrHistoricalTagUnavailable
	}
	return value, nil
}

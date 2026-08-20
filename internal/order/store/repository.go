package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type Repository struct{}

var _ orderapp.Store = (*Repository)(nil)
var _ orderport.PaidOrderReader = (*Repository)(nil)

func NewRepository() *Repository { return &Repository{} }

func (repository *Repository) ReadPaidOrder(ctx context.Context, id orderport.ID) (orderport.PaidOrderProjection, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil || id < 1 {
		return orderport.PaidOrderProjection{}, orderport.ErrPaidOrderReadUnavailable
	}
	row, err := queries.GetPaidOrderProjection(ctx, int64(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return orderport.PaidOrderProjection{}, orderport.ErrPaidOrderReadNotFound
		}
		return orderport.PaidOrderProjection{}, errors.Join(orderport.ErrPaidOrderReadUnavailable, err)
	}
	if !row.ProductID.Valid || !row.CustomerID.Valid || row.ID < 1 || row.ProductID.Int64 < 1 || row.CustomerID.Int64 < 1 {
		return orderport.PaidOrderProjection{}, orderport.ErrPaidOrderReadUnavailable
	}
	return orderport.PaidOrderProjection{ID: orderport.ID(row.ID), ProductID: row.ProductID.Int64, CustomerID: row.CustomerID.Int64}, nil
}

func (repository *Repository) List(ctx context.Context, filter orderport.Filter) ([]orderport.Record, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return nil, unavailable(err)
	}
	rows, err := queries.ListOrderProjections(ctx, orderdb.ListOrderProjectionsParams{
		Provider: optionalText(filter.Provider, "all"), OrderNo: optionalText(filter.OrderNo, ""),
		Mobile: optionalText(filter.Mobile, ""), ProductCode: optionalText(filter.ProductCode, ""),
		Status: optionalText(filter.Status, ""), CreatedFrom: optionalTime(filter.CreatedFrom),
		CreatedTo: optionalTime(filter.CreatedTo), RowLimit: filter.Limit, RowOffset: filter.Offset,
	})
	if err != nil {
		return nil, unavailable(err)
	}
	records := make([]orderport.Record, len(rows))
	for index, row := range rows {
		records[index] = orderport.Record{
			ID: orderport.ID(row.ID), Provider: row.Provider, ProviderLabel: row.ProviderLabel,
			MerchantOrderNo: row.MerchantOrderNo, PlatformTransactionNo: row.PlatformTransactionNo,
			CustomerID: optionalInt64(row.CustomerID), PayerNameSnapshot: row.PayerNameSnapshot,
			MobileSnapshot: row.MobileSnapshot, IdentityKind: row.IdentityKind, IdentityValue: row.IdentityValue,
			ProductID: optionalInt64(row.ProductID), ProductCode: row.ProductCode, ProductNameSnapshot: row.ProductNameSnapshot,
			AmountMinor: row.AmountMinor, Currency: row.Currency, Status: row.Status, StatusLabel: row.StatusLabel,
			DetailURL: row.DetailUrl, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
		}
	}
	return records, nil
}

func (repository *Repository) Count(ctx context.Context, filter orderport.Filter) (int64, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return 0, unavailable(err)
	}
	if filter.Provider == "all" && filter.OrderNo == "" && filter.Mobile == "" && filter.ProductCode == "" && filter.Status == "" && filter.CreatedFrom == nil && filter.CreatedTo == nil {
		count, countErr := queries.CountAllOrderProjections(ctx)
		if countErr != nil {
			return 0, unavailable(countErr)
		}
		return count, nil
	}
	count, err := queries.CountFilteredOrderProjections(ctx, orderdb.CountFilteredOrderProjectionsParams{
		Provider: optionalText(filter.Provider, "all"), OrderNo: optionalText(filter.OrderNo, ""),
		Mobile: optionalText(filter.Mobile, ""), ProductCode: optionalText(filter.ProductCode, ""),
		Status: optionalText(filter.Status, ""), CreatedFrom: optionalTime(filter.CreatedFrom), CreatedTo: optionalTime(filter.CreatedTo),
	})
	if err != nil {
		return 0, unavailable(err)
	}
	return count, nil
}

func transactionQueries(ctx context.Context) (*orderdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return orderdb.New(tx), nil
}
func optionalText(value, empty string) pgtype.Text {
	if value == empty {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}
func optionalTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}
func optionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
func unavailable(err error) error {
	if err == nil {
		return orderapp.ErrUnavailable
	}
	return errors.Join(orderapp.ErrUnavailable, err)
}

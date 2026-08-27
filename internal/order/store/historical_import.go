package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
)

var _ orderport.HistoricalImportStore = (*Repository)(nil)
var _ orderport.HistoricalRefundReader = (*Repository)(nil)

func (repository *Repository) CreateHistoricalOrder(ctx context.Context, record orderport.Record) (orderport.Record, error) {
	if !validHistoricalOrderInput(record) {
		return orderport.Record{}, orderport.ErrHistoricalInput
	}
	queries, err := historicalQueries(ctx, repository)
	if err != nil {
		return orderport.Record{}, err
	}
	row, err := queries.CreateHistoricalOrder(ctx, orderdb.CreateHistoricalOrderParams{
		ProviderLabel: record.ProviderLabel, MerchantOrderNo: record.MerchantOrderNo, PlatformTransactionNo: record.PlatformTransactionNo,
		CustomerID: optionalInt64Value(record.CustomerID), PayerNameSnapshot: record.PayerNameSnapshot, MobileSnapshot: record.MobileSnapshot,
		IdentityKind: record.IdentityKind, IdentityValue: record.IdentityValue, ProductID: optionalInt64Value(record.ProductID),
		ProductCode: record.ProductCode, ProductNameSnapshot: record.ProductNameSnapshot, AmountMinor: record.AmountMinor,
		Status: record.Status, StatusLabel: record.StatusLabel, DetailUrl: record.DetailURL, CreatedAt: pgTime(record.CreatedAt), UpdatedAt: pgTime(record.UpdatedAt),
	})
	if err != nil {
		return orderport.Record{}, historicalCreateError(err)
	}
	return historicalOrderFromCreate(row), nil
}

func (repository *Repository) GetHistoricalOrder(ctx context.Context, id orderport.ID) (orderport.Record, error) {
	if id < 1 {
		return orderport.Record{}, orderport.ErrHistoricalInput
	}
	queries, err := historicalQueries(ctx, repository)
	if err != nil {
		return orderport.Record{}, err
	}
	row, err := queries.GetHistoricalOrder(ctx, int64(id))
	if err != nil {
		return orderport.Record{}, historicalReadError(err)
	}
	return historicalOrderFromGet(row), nil
}

func (repository *Repository) CreateHistoricalRefund(ctx context.Context, refund orderport.HistoricalRefund) (orderport.HistoricalRefund, error) {
	if !validHistoricalRefundInput(refund) {
		return orderport.HistoricalRefund{}, orderport.ErrHistoricalInput
	}
	queries, err := historicalQueries(ctx, repository)
	if err != nil {
		return orderport.HistoricalRefund{}, err
	}
	row, err := queries.CreateHistoricalRefund(ctx, orderdb.CreateHistoricalRefundParams{
		SourceRefundID: refund.SourceRefundID, RefundNumber: refund.RefundNumber, ProviderRefundID: refund.ProviderRefundID,
		TransactionID: refund.TransactionID, Status: refund.Status, AmountMinor: refund.AmountMinor,
		OrderAmountMinor: refund.OrderAmountMinor, Reason: refund.Reason, CreatedAt: pgTime(refund.CreatedAt),
		UpdatedAt: pgTime(refund.UpdatedAt), OrderID: int64(refund.OrderID),
	})
	if err != nil {
		return orderport.HistoricalRefund{}, historicalCreateError(err)
	}
	return historicalRefund(row), nil
}

func (repository *Repository) GetHistoricalRefund(ctx context.Context, id int64) (orderport.HistoricalRefund, error) {
	if id < 1 {
		return orderport.HistoricalRefund{}, orderport.ErrHistoricalInput
	}
	queries, err := historicalQueries(ctx, repository)
	if err != nil {
		return orderport.HistoricalRefund{}, err
	}
	row, err := queries.GetHistoricalRefund(ctx, id)
	if err != nil {
		return orderport.HistoricalRefund{}, historicalReadError(err)
	}
	return historicalRefund(row), nil
}

func (repository *Repository) ListHistoricalRefunds(ctx context.Context, orderID orderport.ID) ([]orderport.HistoricalRefund, error) {
	if orderID < 1 {
		return nil, orderport.ErrHistoricalInput
	}
	queries, err := historicalQueries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListHistoricalOrderRefunds(ctx, int64(orderID))
	if err != nil {
		return nil, historicalReadError(err)
	}
	result := make([]orderport.HistoricalRefund, len(rows))
	for index, row := range rows {
		result[index] = historicalRefund(row)
	}
	return result, nil
}

func historicalQueries(ctx context.Context, repository *Repository) (*orderdb.Queries, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return nil, errors.Join(orderport.ErrHistoricalUnavailable, err)
	}
	return queries, nil
}

func historicalOrderFromCreate(row orderdb.CreateHistoricalOrderRow) orderport.Record {
	return historicalOrder(row.ID, row.RecordOrigin, row.Provider, row.ProviderLabel, row.MerchantOrderNo, row.PlatformTransactionNo,
		row.CustomerID, row.PayerNameSnapshot, row.MobileSnapshot, row.IdentityKind, row.IdentityValue, row.ProductID,
		row.ProductCode, row.ProductNameSnapshot, row.AmountMinor, row.Currency, row.Status, row.StatusLabel, row.DetailUrl, row.CreatedAt, row.UpdatedAt)
}

func historicalOrderFromGet(row orderdb.GetHistoricalOrderRow) orderport.Record {
	return historicalOrder(row.ID, row.RecordOrigin, row.Provider, row.ProviderLabel, row.MerchantOrderNo, row.PlatformTransactionNo,
		row.CustomerID, row.PayerNameSnapshot, row.MobileSnapshot, row.IdentityKind, row.IdentityValue, row.ProductID,
		row.ProductCode, row.ProductNameSnapshot, row.AmountMinor, row.Currency, row.Status, row.StatusLabel, row.DetailUrl, row.CreatedAt, row.UpdatedAt)
}

func historicalOrder(id int64, origin, provider, providerLabel, merchantOrderNo, transactionNo string, customerID pgtype.Int8, payerName, mobile, identityKind, identityValue string, productID pgtype.Int8, productCode, productName string, amountMinor int64, currency, status, statusLabel, detailURL string, createdAt, updatedAt pgtype.Timestamptz) orderport.Record {
	record := boardRecord(legacyOrderProjection{
		ID: id, Provider: provider, ProviderLabel: providerLabel, MerchantOrderNo: merchantOrderNo, PlatformTransactionNo: transactionNo,
		CustomerID: customerID, PayerNameSnapshot: payerName, MobileSnapshot: mobile, IdentityKind: identityKind, IdentityValue: identityValue,
		ProductID: productID, ProductCode: productCode, ProductNameSnapshot: productName, AmountMinor: amountMinor, Currency: currency,
		Status: status, StatusLabel: statusLabel, DetailUrl: detailURL, CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
	record.RecordOrigin = origin
	return record
}

func historicalRefund(row orderdb.OrderHistoricalRefund) orderport.HistoricalRefund {
	return orderport.HistoricalRefund{
		ID: row.ID, OrderID: orderport.ID(row.OrderID), SourceRefundID: row.SourceRefundID, RefundNumber: row.RefundNumber,
		ProviderRefundID: row.ProviderRefundID, TransactionID: row.TransactionID, Status: row.Status, AmountMinor: row.AmountMinor,
		OrderAmountMinor: row.OrderAmountMinor, Currency: row.Currency, Reason: row.Reason, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func validHistoricalOrderInput(record orderport.Record) bool {
	return record.RecordOrigin == orderport.RecordOriginV1History && record.Provider == "wechat" && record.Currency == "CNY"
}

func validHistoricalRefundInput(refund orderport.HistoricalRefund) bool {
	return refund.OrderID > 0 && refund.SourceRefundID > 0 && refund.Currency == "CNY"
}

func historicalCreateError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || uniqueViolation(err) {
		return errors.Join(orderport.ErrHistoricalConflict, err)
	}
	return errors.Join(orderport.ErrHistoricalUnavailable, err)
}

func historicalReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.Join(orderport.ErrHistoricalConflict, err)
	}
	return errors.Join(orderport.ErrHistoricalUnavailable, err)
}

func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

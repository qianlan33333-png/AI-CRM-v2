package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var _ orderapp.BoardStore = (*Repository)(nil)

func (repository *Repository) ListBoardOrders(ctx context.Context, filter orderport.BoardFilter) ([]orderport.Record, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBoardOrders(ctx, boardListParams(filter))
	if err != nil {
		return nil, boardStoreError(err)
	}
	result := make([]orderport.Record, len(rows))
	for i, row := range rows {
		result[i] = boardRecord(row)
	}
	return result, nil
}

func (repository *Repository) CountBoardOrders(ctx context.Context, filter orderport.BoardFilter) (int64, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountBoardOrders(ctx, orderdb.CountBoardOrdersParams{
		Provider: optionalText(filter.Provider, "all"), Status: optionalText(filter.Status, ""), ProductCode: optionalText(filter.ProductCode, ""),
		Mobile: optionalText(filter.Mobile, ""), Identity: optionalText(filter.Identity, ""), TransactionID: optionalText(filter.TransactionID, ""),
		OrderNo: optionalText(filter.OrderNo, ""), CreatedFrom: optionalTime(filter.CreatedFrom), CreatedTo: optionalTime(filter.CreatedTo),
	})
	if err != nil {
		return 0, boardStoreError(err)
	}
	return count, nil
}

func (repository *Repository) GetBoardOrder(ctx context.Context, provider, reference string) (orderport.Record, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.Record{}, err
	}
	row, err := queries.GetBoardOrder(ctx, orderdb.GetBoardOrderParams{Provider: provider, OrderReference: reference})
	if err != nil {
		return orderport.Record{}, boardStoreError(err)
	}
	return boardRecord(row), nil
}

// GetBoardOrderByID is intentionally separate from GetBoardOrder: a local ID
// must never be interpreted as a merchant or provider transaction reference.
func (repository *Repository) GetBoardOrderByID(ctx context.Context, id orderport.ID) (orderport.Record, error) {
	if repository == nil || id < 1 {
		return orderport.Record{}, orderapp.ErrNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return orderport.Record{}, unavailable(err)
	}
	var row orderdb.OrderListProjection
	err = tx.QueryRow(ctx, exactBoardOrderByID, int64(id)).Scan(
		&row.ID, &row.Provider, &row.ProviderLabel, &row.MerchantOrderNo, &row.PlatformTransactionNo,
		&row.CustomerID, &row.PayerNameSnapshot, &row.MobileSnapshot, &row.IdentityKind, &row.IdentityValue,
		&row.ProductID, &row.ProductCode, &row.ProductNameSnapshot, &row.AmountMinor, &row.Currency,
		&row.Status, &row.StatusLabel, &row.DetailUrl, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		return orderport.Record{}, boardStoreError(err)
	}
	return boardRecord(row), nil
}

func (repository *Repository) LockBoardOrder(ctx context.Context, provider, reference string) (orderport.Record, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.Record{}, err
	}
	row, err := queries.GetBoardOrderForUpdate(ctx, orderdb.GetBoardOrderForUpdateParams{Provider: provider, OrderReference: reference})
	if err != nil {
		return orderport.Record{}, boardStoreError(err)
	}
	return boardRecord(row), nil
}

func (repository *Repository) CountActiveRefundAmount(ctx context.Context, orderID orderport.ID) (int64, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountActiveRefundAmount(ctx, int64(orderID))
	if err != nil {
		return 0, boardStoreError(err)
	}
	return count, nil
}

func (repository *Repository) ReserveBoardReceipt(ctx context.Context, reservation orderapp.BoardReservation) (orderapp.BoardReceipt, bool, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderapp.BoardReceipt{}, false, err
	}
	row, err := queries.ReserveOrderOperationReceipt(ctx, orderdb.ReserveOrderOperationReceiptParams{
		Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:], PayloadDigest: reservation.PayloadDigest[:], CreatedAt: pgTime(reservation.CreatedAt),
	})
	if err == nil {
		receipt, mapErr := boardReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot)
		return receipt, true, mapErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return orderapp.BoardReceipt{}, false, boardStoreError(err)
	}
	existing, getErr := queries.GetOrderOperationReceipt(ctx, orderdb.GetOrderOperationReceiptParams{Operation: reservation.Operation, ActorScope: reservation.ActorScope, KeyDigest: reservation.KeyDigest[:]})
	if getErr != nil {
		return orderapp.BoardReceipt{}, false, boardStoreError(getErr)
	}
	receipt, mapErr := boardReceipt(existing.ID, existing.Operation, existing.ActorScope, existing.KeyDigest, existing.PayloadDigest, existing.State, existing.ResultSnapshot)
	return receipt, false, mapErr
}

func (repository *Repository) CompleteBoardReceipt(ctx context.Context, receiptID int64, snapshot json.RawMessage, completedAt time.Time) (orderapp.BoardReceipt, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderapp.BoardReceipt{}, err
	}
	row, err := queries.CompleteOrderOperationReceipt(ctx, orderdb.CompleteOrderOperationReceiptParams{ID: receiptID, ResultSnapshot: snapshot, CompletedAt: pgTime(completedAt)})
	if err != nil {
		return orderapp.BoardReceipt{}, boardStoreError(err)
	}
	return boardReceipt(row.ID, row.Operation, row.ActorScope, row.KeyDigest, row.PayloadDigest, row.State, row.ResultSnapshot)
}

func (repository *Repository) CreateExportJob(ctx context.Context, job orderport.ExportJob) (orderport.ExportJob, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.ExportJob{}, err
	}
	row, err := queries.CreateOrderExportJob(ctx, orderdb.CreateOrderExportJobParams{JobID: job.JobID, Resource: job.Resource, Format: job.Format, OperatorID: job.Operator, ContentText: job.ContentText, CreatedAt: pgTime(job.CreatedAt)})
	if err != nil {
		return orderport.ExportJob{}, boardStoreError(err)
	}
	return boardExportJob(row.JobID, row.Resource, row.Format, row.OperatorID, row.ContentText, row.CreatedAt), nil
}

func (repository *Repository) GetExportJob(ctx context.Context, jobID string) (orderport.ExportJob, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.ExportJob{}, err
	}
	row, err := queries.GetOrderExportJob(ctx, jobID)
	if err != nil {
		return orderport.ExportJob{}, boardStoreError(err)
	}
	return boardExportJob(row.JobID, row.Resource, row.Format, row.OperatorID, row.ContentText, row.CreatedAt), nil
}

func (repository *Repository) CreateExternalEffect(ctx context.Context, effect orderport.ExternalEffect) (orderport.ExternalEffect, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.ExternalEffect{}, err
	}
	row, err := queries.CreateOrderExternalEffect(ctx, orderdb.CreateOrderExternalEffectParams{OrderID: int64(effect.OrderID), Provider: effect.Provider, EffectKind: effect.EffectKind, State: effect.State, ProviderReceipt: effect.ProviderReceipt, CreatedAt: pgTime(effect.CreatedAt), UpdatedAt: pgTime(effect.UpdatedAt)})
	if err != nil {
		return orderport.ExternalEffect{}, boardStoreError(err)
	}
	return boardEffect(row), nil
}

func (repository *Repository) GetExternalEffect(ctx context.Context, id int64, lock bool) (orderport.ExternalEffect, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.ExternalEffect{}, err
	}
	var row orderdb.OrderExternalEffect
	if lock {
		row, err = queries.GetOrderExternalEffectForUpdate(ctx, id)
	} else {
		row, err = queries.GetOrderExternalEffect(ctx, id)
	}
	if err != nil {
		return orderport.ExternalEffect{}, boardStoreError(err)
	}
	return boardEffect(row), nil
}

func (repository *Repository) ListExternalEffects(ctx context.Context, orderID orderport.ID) ([]orderport.ExternalEffect, int64, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return nil, 0, err
	}
	rows, err := queries.ListOrderExternalEffects(ctx, int64(orderID))
	if err != nil {
		return nil, 0, boardStoreError(err)
	}
	count, err := queries.CountOrderExternalEffects(ctx, int64(orderID))
	if err != nil {
		return nil, 0, boardStoreError(err)
	}
	items := make([]orderport.ExternalEffect, len(rows))
	for i, row := range rows {
		items[i] = boardEffect(row)
	}
	return items, count, nil
}

func (repository *Repository) RequestExternalEffectReview(ctx context.Context, effectID int64, reviewedAt time.Time) (orderport.ExternalEffect, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.ExternalEffect{}, err
	}
	row, err := queries.MarkOrderExternalEffectManualReview(ctx, orderdb.MarkOrderExternalEffectManualReviewParams{ID: effectID, ReviewedAt: pgTime(reviewedAt)})
	if err != nil {
		return orderport.ExternalEffect{}, boardStoreError(err)
	}
	return boardEffect(row), nil
}

func (repository *Repository) CreateRefund(ctx context.Context, refund orderport.Refund) (orderport.Refund, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return orderport.Refund{}, err
	}
	row, err := queries.CreateOrderRefund(ctx, orderdb.CreateOrderRefundParams{OrderID: int64(refund.OrderID), ExternalEffectID: refund.ExternalEffectID, Provider: refund.Provider, RefundID: refund.RefundID, OutRefundNo: refund.OutRefundNo, RefundAmountTotal: refund.RefundAmountTotal, Currency: refund.Currency, Reason: refund.Reason, Status: refund.Status, CreatedAt: pgTime(refund.CreatedAt)})
	if err != nil {
		return orderport.Refund{}, boardStoreError(err)
	}
	return orderport.Refund{ID: row.ID, OrderID: orderport.ID(row.OrderID), Provider: row.Provider, OrderNo: refund.OrderNo, TransactionID: refund.TransactionID, RefundID: row.RefundID, OutRefundNo: row.OutRefundNo, RefundAmountTotal: row.RefundAmountTotal, Currency: row.Currency, Reason: row.Reason, Status: row.Status, ExternalEffectID: row.ExternalEffectID, ExternalEffectState: refund.ExternalEffectState, AutoRetryAllowed: false, CreatedAt: row.CreatedAt.Time}, nil
}

func (repository *Repository) ListRefunds(ctx context.Context, filter orderport.RefundFilter) ([]orderport.Refund, int64, error) {
	queries, err := boardQueries(ctx, repository)
	if err != nil {
		return nil, 0, err
	}
	params := boardRefundParams(filter)
	rows, err := queries.ListOrderRefunds(ctx, params)
	if err != nil {
		return nil, 0, boardStoreError(err)
	}
	count, err := queries.CountOrderRefunds(ctx, orderdb.CountOrderRefundsParams{Provider: params.Provider, OrderNo: params.OrderNo, TransactionID: params.TransactionID, RefundID: params.RefundID, OutRefundNo: params.OutRefundNo, Status: params.Status, CreatedFrom: params.CreatedFrom, CreatedTo: params.CreatedTo})
	if err != nil {
		return nil, 0, boardStoreError(err)
	}
	items := make([]orderport.Refund, len(rows))
	for i, row := range rows {
		items[i] = boardRefund(row)
	}
	return items, count, nil
}

func (repository *Repository) GetRefundByID(ctx context.Context, id int64) (orderport.Refund, error) {
	if repository == nil || id < 1 {
		return orderport.Refund{}, orderapp.ErrNotFound
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return orderport.Refund{}, unavailable(err)
	}
	var row orderdb.ListOrderRefundsRow
	err = tx.QueryRow(ctx, exactBoardRefundByID, id).Scan(
		&row.ID, &row.OrderID, &row.ExternalEffectID, &row.Provider, &row.RefundID, &row.OutRefundNo,
		&row.RefundAmountTotal, &row.Currency, &row.Reason, &row.Status, &row.CreatedAt, &row.MerchantOrderNo,
		&row.PlatformTransactionNo, &row.ExternalEffectState, &row.AutoRetryAllowed,
	)
	if err != nil {
		return orderport.Refund{}, boardStoreError(err)
	}
	return boardRefund(row), nil
}

func boardQueries(ctx context.Context, repository *Repository) (*orderdb.Queries, error) {
	queries, err := transactionQueries(ctx)
	if repository == nil || err != nil {
		return nil, unavailable(err)
	}
	return queries, nil
}

func boardListParams(filter orderport.BoardFilter) orderdb.ListBoardOrdersParams {
	return orderdb.ListBoardOrdersParams{Provider: optionalText(filter.Provider, "all"), Status: optionalText(filter.Status, ""), ProductCode: optionalText(filter.ProductCode, ""), Mobile: optionalText(filter.Mobile, ""), Identity: optionalText(filter.Identity, ""), TransactionID: optionalText(filter.TransactionID, ""), OrderNo: optionalText(filter.OrderNo, ""), CreatedFrom: optionalTime(filter.CreatedFrom), CreatedTo: optionalTime(filter.CreatedTo), RowOffset: filter.Offset, RowLimit: filter.Limit}
}

func boardRefundParams(filter orderport.RefundFilter) orderdb.ListOrderRefundsParams {
	return orderdb.ListOrderRefundsParams{Provider: optionalText(filter.Provider, "all"), OrderNo: optionalText(filter.OrderNo, ""), TransactionID: optionalText(filter.TransactionID, ""), RefundID: optionalText(filter.RefundID, ""), OutRefundNo: optionalText(filter.OutRefundNo, ""), Status: optionalText(filter.Status, ""), CreatedFrom: optionalTime(filter.CreatedFrom), CreatedTo: optionalTime(filter.CreatedTo), RowOffset: filter.Offset, RowLimit: filter.Limit}
}

func boardRecord(row orderdb.OrderListProjection) orderport.Record {
	return orderport.Record{ID: orderport.ID(row.ID), Provider: row.Provider, ProviderLabel: row.ProviderLabel, MerchantOrderNo: row.MerchantOrderNo, PlatformTransactionNo: row.PlatformTransactionNo, CustomerID: optionalInt64(row.CustomerID), PayerNameSnapshot: row.PayerNameSnapshot, MobileSnapshot: row.MobileSnapshot, IdentityKind: row.IdentityKind, IdentityValue: row.IdentityValue, ProductID: optionalInt64(row.ProductID), ProductCode: row.ProductCode, ProductNameSnapshot: row.ProductNameSnapshot, AmountMinor: row.AmountMinor, Currency: row.Currency, Status: row.Status, StatusLabel: row.StatusLabel, DetailURL: row.DetailUrl, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}

func boardRefund(row orderdb.ListOrderRefundsRow) orderport.Refund {
	return orderport.Refund{ID: row.ID, OrderID: orderport.ID(row.OrderID), Provider: row.Provider, OrderNo: row.MerchantOrderNo, TransactionID: row.PlatformTransactionNo, RefundID: row.RefundID, OutRefundNo: row.OutRefundNo, RefundAmountTotal: row.RefundAmountTotal, Currency: row.Currency, Reason: row.Reason, Status: row.Status, ExternalEffectID: row.ExternalEffectID, ExternalEffectState: row.ExternalEffectState, AutoRetryAllowed: row.AutoRetryAllowed, CreatedAt: row.CreatedAt.Time}
}

const exactBoardOrderByID = `
SELECT id, provider, provider_label, merchant_order_no, platform_transaction_no,
       customer_id, payer_name_snapshot, mobile_snapshot, identity_kind, identity_value,
       product_id, product_code, product_name_snapshot, amount_minor, currency,
       status, status_label, detail_url, created_at, updated_at
FROM order_list_projections
WHERE id = $1::bigint
`

const exactBoardRefundByID = `
SELECT refund.id, refund.order_id, refund.external_effect_id, refund.provider, refund.refund_id,
       refund.out_refund_no, refund.refund_amount_total, refund.currency, refund.reason,
       refund.status, refund.created_at, order_projection.merchant_order_no,
       order_projection.platform_transaction_no, effect.state, effect.auto_retry_allowed
FROM order_refunds AS refund
JOIN order_list_projections AS order_projection ON order_projection.id = refund.order_id
JOIN order_external_effects AS effect ON effect.id = refund.external_effect_id
WHERE refund.id = $1::bigint
`

func boardReceipt(id int64, operation, actorScope string, keyDigest, payloadDigest []byte, state string, snapshot []byte) (orderapp.BoardReceipt, error) {
	if len(keyDigest) != 32 || len(payloadDigest) != 32 {
		return orderapp.BoardReceipt{}, orderapp.ErrBoardUnavailable
	}
	var key, payload [32]byte
	copy(key[:], keyDigest)
	copy(payload[:], payloadDigest)
	return orderapp.BoardReceipt{ID: id, Operation: operation, ActorScope: actorScope, KeyDigest: key, PayloadDigest: payload, State: state, ResultSnapshot: snapshot}, nil
}

func boardExportJob(jobID, resource, format string, operator int64, content string, createdAt pgtype.Timestamptz) orderport.ExportJob {
	return orderport.ExportJob{JobID: jobID, Resource: resource, Format: format, Status: "completed", Operator: operator, ContentText: content, ContentType: "text/csv", FileName: jobID + ".csv", DownloadURL: "/api/admin/exports/" + jobID, CreatedAt: createdAt.Time}
}

func boardEffect(row orderdb.OrderExternalEffect) orderport.ExternalEffect {
	return orderport.ExternalEffect{ID: row.ID, OrderID: orderport.ID(row.OrderID), Provider: row.Provider, EffectKind: row.EffectKind, State: row.State, AutoRetryAllowed: row.AutoRetryAllowed, ProviderReceipt: row.ProviderReceipt, ManualReviewRequested: row.ManualReviewRequestedAt.Time, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}

func pgTime(value time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: value, Valid: true} }

func boardStoreError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return orderapp.ErrNotFound
	}
	return unavailable(err)
}

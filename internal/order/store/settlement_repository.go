package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type FinancialRepository struct {
	base   *Repository
	client *platformjobqueue.InsertOnlyClient
}

var _ orderapp.SettlementStore = (*FinancialRepository)(nil)

func NewFinancialRepository(pool *pgxpool.Pool) (*FinancialRepository, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(orderport.ErrSettlementUnavailable, err)
	}
	return &FinancialRepository{base: NewRepository(), client: client}, nil
}

func (repository *FinancialRepository) ReserveBoardReceipt(ctx context.Context, reservation orderapp.BoardReservation) (orderapp.BoardReceipt, bool, error) {
	if repository == nil || repository.base == nil {
		return orderapp.BoardReceipt{}, false, orderport.ErrSettlementUnavailable
	}
	return repository.base.ReserveBoardReceipt(ctx, reservation)
}

func (repository *FinancialRepository) CompleteBoardReceipt(ctx context.Context, id int64, snapshot json.RawMessage, at time.Time) (orderapp.BoardReceipt, error) {
	if repository == nil || repository.base == nil {
		return orderapp.BoardReceipt{}, orderport.ErrSettlementUnavailable
	}
	return repository.base.CompleteBoardReceipt(ctx, id, snapshot, at)
}

func (*FinancialRepository) CreateCheckout(ctx context.Context, command orderport.CheckoutCommand, product productport.Product, orderNo string, at time.Time) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.CreatePE01Order(ctx, orderdb.CreatePE01OrderParams{MerchantOrderNo: orderNo, CustomerID: int8Value(command.CustomerID), ProductID: int8Value(int64(product.ID)), ProductCode: product.ProductCode, ProductName: product.Name, AmountMinor: product.PriceMinor, CreatedAt: pgTime(at), ProductVersion: int8Value(product.Version), ProductKind: textValue(string(command.ProductKind)), PaymentIdentityDigest: command.PaymentIdentityDigest[:]})
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	return shortOrder(row.ID, row.MerchantOrderNo, row.CustomerID, row.ProductID, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.CreatedAt, row.Version, product.Version, command.PaymentIdentityDigest), nil
}

func (*FinancialRepository) CreatePaymentCommand(ctx context.Context, order orderapp.FinancialOrderRecord, source, target, payload, policy [32]byte, at time.Time) (orderport.PaymentCommand, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.PaymentCommand{}, err
	}
	row, err := q.CreatePE01PaymentCommand(ctx, orderdb.CreatePE01PaymentCommandParams{OrderID: int64(order.ID), SourceRefDigest: source[:], TargetRefDigest: target[:], PayloadDigest: payload[:], PolicyVersionDigest: policy[:], CreatedAt: pgTime(at)})
	return mapPayment(row), settlementError(err)
}

func (repository *FinancialRepository) EnqueuePaymentBridge(ctx context.Context, commandID int64) (int64, error) {
	return repository.enqueue(ctx, orderapp.PaymentEffectBridgeArgs{CommandID: commandID})
}

func (repository *FinancialRepository) EnqueueRefundBridge(ctx context.Context, refundID int64) (int64, error) {
	return repository.enqueue(ctx, orderapp.RefundEffectBridgeArgs{RefundID: refundID})
}

func (repository *FinancialRepository) EnqueuePaymentReconcile(ctx context.Context, commandID int64) (int64, error) {
	return repository.enqueue(ctx, orderapp.PaymentReconcileArgs{CommandID: commandID})
}

func (repository *FinancialRepository) EnqueueRefundReconcile(ctx context.Context, refundID int64) (int64, error) {
	return repository.enqueue(ctx, orderapp.RefundReconcileArgs{RefundID: refundID})
}

func (repository *FinancialRepository) enqueue(ctx context.Context, args interface{ Kind() string }) (int64, error) {
	if repository == nil || repository.client == nil || args == nil {
		return 0, orderport.ErrSettlementUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, args, string(platformjobqueue.QueueCritical))
	if err != nil || jobID < 1 {
		return 0, errors.Join(orderport.ErrSettlementUnavailable, err)
	}
	return jobID, nil
}

func (*FinancialRepository) GetPaymentCommandByOrder(ctx context.Context, orderID orderport.ID) (orderport.PaymentCommand, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.PaymentCommand{}, err
	}
	row, err := q.GetPE01PaymentCommandByOrder(ctx, int64(orderID))
	return mapPayment(row), settlementError(err)
}

func (*FinancialRepository) LockPaymentCommand(ctx context.Context, commandID int64) (orderport.PaymentCommand, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.PaymentCommand{}, err
	}
	row, err := q.LockPE01PaymentCommandByID(ctx, commandID)
	return mapPayment(row), settlementError(err)
}

func (*FinancialRepository) BindPaymentEffect(ctx context.Context, command orderport.PaymentCommand, effectID string, at time.Time) (orderport.PaymentCommand, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.PaymentCommand{}, err
	}
	id, err := parseEffectID(effectID)
	if err != nil || id < 1 {
		return orderport.PaymentCommand{}, orderport.ErrSettlementUnavailable
	}
	row, err := q.BindPE01PaymentEffect(ctx, orderdb.BindPE01PaymentEffectParams{ExternalEffectID: int8Value(id), UpdatedAt: pgTime(at), CommandID: command.ID, ExpectedVersion: command.Version})
	return mapPayment(row), settlementError(err)
}

func (*FinancialRepository) CompletePaymentEffect(ctx context.Context, command orderport.PaymentCommand, state orderport.EffectState, receipt [32]byte, at time.Time) (orderport.PaymentCommand, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.PaymentCommand{}, err
	}
	params := orderdb.CompletePE01PrepayParams{State: string(state), UpdatedAt: pgTime(at), CommandID: command.ID, ExpectedVersion: command.Version}
	if state == orderport.EffectExecuted {
		params.ProviderPrepayDigest = receipt[:]
	}
	row, err := q.CompletePE01Prepay(ctx, params)
	return mapPayment(row), settlementError(err)
}

func (*FinancialRepository) LockOrderByMerchantNo(ctx context.Context, reference string) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.LockPE01OrderByMerchantNo(ctx, reference)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	return fullOrder(row.ID, row.MerchantOrderNo, row.PlatformTransactionNo, row.CustomerID, row.ProductID, row.ProductVersion, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.PaymentIdentityDigest, row.SettledAmountMinor, row.RefundedAmountMinor, row.SettlementReceiptDigest, row.Version, row.CreatedAt, row.UpdatedAt), nil
}

func (*FinancialRepository) LockOrderByID(ctx context.Context, id orderport.ID) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.LockPE01OrderByID(ctx, int64(id))
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	return fullOrder(row.ID, row.MerchantOrderNo, row.PlatformTransactionNo, row.CustomerID, row.ProductID, row.ProductVersion, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.PaymentIdentityDigest, row.SettledAmountMinor, row.RefundedAmountMinor, row.SettlementReceiptDigest, row.Version, row.CreatedAt, row.UpdatedAt), nil
}

func (*FinancialRepository) GetSelfScoped(ctx context.Context, reference string, identity [32]byte) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.GetPE01OrderSelfScoped(ctx, orderdb.GetPE01OrderSelfScopedParams{MerchantOrderNo: reference, PaymentIdentityDigest: identity[:]})
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	return shortOrder(row.ID, row.MerchantOrderNo, row.CustomerID, row.ProductID, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.CreatedAt, row.Version, row.ProductVersion.Int64, identity), nil
}

func (*FinancialRepository) MarkOrderAwaitingPayment(ctx context.Context, current orderapp.FinancialOrderRecord, at time.Time) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.MarkPE01OrderAwaitingPayment(ctx, orderdb.MarkPE01OrderAwaitingPaymentParams{OrderID: int64(current.ID), ExpectedVersion: current.Version, UpdatedAt: pgTime(at)})
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	result := shortOrder(row.ID, row.MerchantOrderNo, row.CustomerID, row.ProductID, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.CreatedAt, row.Version, current.ProductVersion, current.PaymentIdentityDigest)
	result.UpdatedAt = at
	return result, nil
}

func (*FinancialRepository) ApplyPaymentSettlement(ctx context.Context, current orderapp.FinancialOrderRecord, providerRef string, digest [32]byte, at time.Time) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.ApplyPE01PaymentSettlement(ctx, orderdb.ApplyPE01PaymentSettlementParams{ProviderTransactionRef: providerRef, SettlementReceiptDigest: digest[:], PaidAt: pgTime(at), OrderID: int64(current.ID), AmountMinor: current.AmountMinor, Currency: current.Currency, ExpectedVersion: current.Version})
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	result := shortOrder(row.ID, row.MerchantOrderNo, row.CustomerID, row.ProductID, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.CreatedAt, row.Version, current.ProductVersion, current.PaymentIdentityDigest)
	result.ProviderTransactionRef, result.SettledAmountMinor, result.SettlementDigest, result.UpdatedAt = providerRef, result.AmountMinor, digest, at
	return result, nil
}

func (*FinancialRepository) ReserveCallback(ctx context.Context, kind string, eventDigest, payloadDigest [32]byte, orderID orderport.ID, refundID int64, at time.Time) (orderapp.CallbackReceipt, bool, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.CallbackReceipt{}, false, err
	}
	params := orderdb.ReservePE01CallbackReceiptParams{CallbackKind: kind, ProviderEventDigest: eventDigest[:], PayloadDigest: payloadDigest[:], OrderID: int64(orderID), ReceivedAt: pgTime(at)}
	if refundID > 0 {
		params.RefundID = int8Value(refundID)
	}
	row, err := q.ReservePE01CallbackReceipt(ctx, params)
	if err == nil {
		return mapCallback(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return orderapp.CallbackReceipt{}, false, settlementError(err)
	}
	existing, getErr := q.GetPE01CallbackReceipt(ctx, eventDigest[:])
	return mapCallback(existing), false, settlementError(getErr)
}

func (*FinancialRepository) CompleteCallback(ctx context.Context, id int64, outcome string, digest [32]byte, at time.Time) (orderapp.CallbackReceipt, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.CallbackReceipt{}, err
	}
	row, err := q.CompletePE01CallbackReceipt(ctx, orderdb.CompletePE01CallbackReceiptParams{ReceiptID: id, Outcome: textValue(outcome), ResultDigest: digest[:], CompletedAt: pgTime(at)})
	return mapCallback(row), settlementError(err)
}

func (*FinancialRepository) CountReservedRefundAmount(ctx context.Context, orderID orderport.ID) (int64, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return 0, err
	}
	value, err := q.CountPE01ReservedRefundAmount(ctx, int64(orderID))
	return value, settlementError(err)
}

func (*FinancialRepository) CreateRefund(ctx context.Context, order orderapp.FinancialOrderRecord, command orderport.RefundCommandV2, number string, source, target, payload, policy [32]byte, at time.Time) (orderport.RefundV2, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, err
	}
	row, err := q.CreatePE01Refund(ctx, orderdb.CreatePE01RefundParams{OrderID: int64(order.ID), OutRefundNo: number, AmountMinor: command.AmountMinor, Reason: command.Reason, SourceRefDigest: source[:], TargetRefDigest: target[:], PayloadDigest: payload[:], PolicyVersionDigest: policy[:], CreatedAt: pgTime(at)})
	return mapRefund(row), settlementError(err)
}

func (*FinancialRepository) LockRefundByOutRefundNo(ctx context.Context, number string) (orderport.RefundV2, [32]byte, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, [32]byte{}, err
	}
	row, err := q.LockPE01RefundByOutRefundNo(ctx, number)
	return mapRefund(row), digestValue(row.PayloadDigest), settlementError(err)
}

func (*FinancialRepository) LockRefundByID(ctx context.Context, refundID int64) (orderport.RefundV2, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, err
	}
	row, err := q.LockPE01RefundByID(ctx, refundID)
	return mapRefund(row), settlementError(err)
}

func (*FinancialRepository) BindRefundEffect(ctx context.Context, refund orderport.RefundV2, effectID string, at time.Time) (orderport.RefundV2, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, err
	}
	id, err := parseEffectID(effectID)
	if err != nil || id < 1 {
		return orderport.RefundV2{}, orderport.ErrSettlementUnavailable
	}
	row, err := q.BindPE01RefundEffect(ctx, orderdb.BindPE01RefundEffectParams{ExternalEffectID: int8Value(id), UpdatedAt: pgTime(at), RefundID: refund.ID, ExpectedVersion: refund.Version})
	return mapRefund(row), settlementError(err)
}

func (*FinancialRepository) CompleteRefundEffect(ctx context.Context, refund orderport.RefundV2, state orderport.EffectState, at time.Time) (orderport.RefundV2, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, err
	}
	row, err := q.MarkPE01RefundEffectResult(ctx, orderdb.MarkPE01RefundEffectResultParams{State: string(state), UpdatedAt: pgTime(at), RefundID: refund.ID, ExpectedVersion: refund.Version})
	return mapRefund(row), settlementError(err)
}

func (*FinancialRepository) ApplyRefundSettlement(ctx context.Context, refund orderport.RefundV2, providerDigest, settlementDigest [32]byte, at time.Time) (orderport.RefundV2, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderport.RefundV2{}, err
	}
	row, err := q.ApplyPE01RefundSettlement(ctx, orderdb.ApplyPE01RefundSettlementParams{RefundID: refund.ID, ProviderRefundDigest: providerDigest[:], SettlementReceiptDigest: settlementDigest[:], SettledAt: pgTime(at), AmountMinor: refund.AmountMinor, Currency: refund.Currency, ExpectedVersion: refund.Version})
	return mapRefund(row), settlementError(err)
}

func (*FinancialRepository) AddSettledRefundToOrder(ctx context.Context, order orderapp.FinancialOrderRecord, amount int64, at time.Time) (orderapp.FinancialOrderRecord, error) {
	q, err := settlementQueries(ctx)
	if err != nil {
		return orderapp.FinancialOrderRecord{}, err
	}
	row, err := q.AddPE01SettledRefundToOrder(ctx, orderdb.AddPE01SettledRefundToOrderParams{OrderID: int64(order.ID), AmountMinor: amount, ExpectedVersion: order.Version, SettledAt: pgTime(at)})
	if err != nil {
		return orderapp.FinancialOrderRecord{}, settlementError(err)
	}
	result := shortOrder(row.ID, row.MerchantOrderNo, row.CustomerID, row.ProductID, row.ProductKind, row.AmountMinor, row.Currency, row.Status, row.CreatedAt, row.Version, order.ProductVersion, order.PaymentIdentityDigest)
	result.SettledAmountMinor, result.RefundedAmountMinor, result.SettlementDigest, result.UpdatedAt = order.SettledAmountMinor, order.RefundedAmountMinor+amount, order.SettlementDigest, at
	return result, nil
}

func settlementQueries(ctx context.Context) (*orderdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return orderdb.New(tx), nil
}

func shortOrder(id int64, orderNo string, customerID, productID pgtype.Int8, kind pgtype.Text, amount int64, currency, state string, created pgtype.Timestamptz, version, productVersion int64, identity [32]byte) orderapp.FinancialOrderRecord {
	return orderapp.FinancialOrderRecord{ID: orderport.ID(id), MerchantOrderNo: orderNo, CustomerID: customerID.Int64, ProductID: productID.Int64, ProductVersion: productVersion, ProductKind: orderport.ProductKind(kind.String), AmountMinor: amount, Currency: currency, State: orderport.FinancialState(state), PaymentIdentityDigest: identity, Version: version, CreatedAt: created.Time.UTC(), UpdatedAt: created.Time.UTC()}
}

func fullOrder(id int64, orderNo, providerRef string, customerID, productID, productVersion pgtype.Int8, kind pgtype.Text, amount int64, currency, state string, identity []byte, settled, refunded int64, settlement []byte, version int64, created, updated pgtype.Timestamptz) orderapp.FinancialOrderRecord {
	result := shortOrder(id, orderNo, customerID, productID, kind, amount, currency, state, created, version, productVersion.Int64, digestValue(identity))
	result.ProviderTransactionRef, result.SettledAmountMinor, result.RefundedAmountMinor, result.SettlementDigest, result.UpdatedAt = providerRef, settled, refunded, digestValue(settlement), updated.Time.UTC()
	return result
}

func mapPayment(row orderdb.OrderPaymentCommand) orderport.PaymentCommand {
	effectID := ""
	if row.ExternalEffectID.Valid {
		effectID = "eer_" + strconv.FormatInt(row.ExternalEffectID.Int64, 10)
	}
	return orderport.PaymentCommand{ID: row.ID, OrderID: orderport.ID(row.OrderID), SourceRefDigest: digestValue(row.SourceRefDigest), TargetRefDigest: digestValue(row.TargetRefDigest), PayloadDigest: digestValue(row.PayloadDigest), PolicyVersionDigest: digestValue(row.PolicyVersionDigest), ExternalEffectID: effectID, State: orderport.EffectState(row.State), Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func mapRefund(row orderdb.OrderFinancialRefund) orderport.RefundV2 {
	effectID := ""
	if row.ExternalEffectID.Valid {
		effectID = "eer_" + strconv.FormatInt(row.ExternalEffectID.Int64, 10)
	}
	reasonDigest := sha256.Sum256([]byte("pe01/refund-reason/v1\x00" + row.Reason))
	return orderport.RefundV2{ID: row.ID, OrderID: orderport.ID(row.OrderID), OutRefundNo: row.OutRefundNo, AmountMinor: row.AmountMinor, Currency: row.Currency, ReasonDigest: reasonDigest, SourceRefDigest: digestValue(row.SourceRefDigest), TargetRefDigest: digestValue(row.TargetRefDigest), PayloadDigest: digestValue(row.PayloadDigest), PolicyDigest: digestValue(row.PolicyVersionDigest), ExternalEffectID: effectID, State: orderport.EffectState(row.State), Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
}

func mapCallback(row orderdb.OrderProviderCallbackReceipt) orderapp.CallbackReceipt {
	return orderapp.CallbackReceipt{ID: row.ID, Kind: row.CallbackKind, EventDigest: digestValue(row.ProviderEventDigest), PayloadDigest: digestValue(row.PayloadDigest), OrderID: orderport.ID(row.OrderID), RefundID: row.RefundID.Int64, Outcome: row.Outcome.String, ResultDigest: digestValue(row.ResultDigest), State: row.State}
}

func digestValue(value []byte) [32]byte  { var result [32]byte; copy(result[:], value); return result }
func int8Value(value int64) pgtype.Int8  { return pgtype.Int8{Int64: value, Valid: true} }
func textValue(value string) pgtype.Text { return pgtype.Text{String: value, Valid: true} }

func settlementError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return orderapp.ErrSettlementRowNotFound()
	}
	return errors.Join(orderport.ErrSettlementUnavailable, err)
}

func parseEffectID(value string) (int64, error) {
	if len(value) <= len("eer_") || value[:len("eer_")] != "eer_" {
		return 0, orderport.ErrSettlementUnavailable
	}
	return strconv.ParseInt(value[len("eer_"):], 10, 64)
}

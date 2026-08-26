package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderdb "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store/generated"
	platformjobqueue "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/jobqueue"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type CommerceRefundRepository struct {
	client *platformjobqueue.InsertOnlyClient
}

var _ orderapp.PE01RefundReferenceResolver = (*CommerceRefundRepository)(nil)
var _ orderapp.WeChatShopRefundStore = (*CommerceRefundRepository)(nil)

func NewCommerceRefundRepository(pool *pgxpool.Pool) (*CommerceRefundRepository, error) {
	client, err := platformjobqueue.NewInsertOnlyClient(pool)
	if err != nil {
		return nil, errors.Join(orderport.ErrCommerceRefundUnavailable, err)
	}
	return &CommerceRefundRepository{client: client}, nil
}

func (*CommerceRefundRepository) FindPE01RefundOrderIDs(ctx context.Context, reference string) ([]orderport.ID, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.FindPE01RefundOrderCandidates(ctx, reference)
	if err != nil {
		return nil, commerceRefundStoreError(err)
	}
	ids := make([]orderport.ID, len(rows))
	for index, id := range rows {
		ids[index] = orderport.ID(id)
	}
	return ids, nil
}

func (*CommerceRefundRepository) FindWeChatShopRefundOrder(ctx context.Context, reference string) (orderapp.WeChatShopOrderRecord, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderapp.WeChatShopOrderRecord{}, err
	}
	rows, err := q.FindWeChatShopRefundOrderCandidates(ctx, reference)
	if err != nil {
		return orderapp.WeChatShopOrderRecord{}, commerceRefundStoreError(err)
	}
	if len(rows) == 0 {
		return orderapp.WeChatShopOrderRecord{}, orderport.ErrCommerceRefundNotFound
	}
	if len(rows) != 1 {
		return orderapp.WeChatShopOrderRecord{}, orderport.ErrCommerceRefundConflict
	}
	row := rows[0]
	return orderapp.WeChatShopOrderRecord{ID: orderport.ID(row.ID), MerchantOrderNo: row.MerchantOrderNo, PlatformTransactionNo: row.PlatformTransactionNo, AmountMinor: row.AmountMinor, Currency: row.Currency, State: row.Status}, nil
}

func (*CommerceRefundRepository) GetWeChatShopRefundMaterial(ctx context.Context, providerOrderID string) (orderport.WeChatShopOrderMaterial, bool, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, err
	}
	row, err := q.GetWeChatShopRefundMaterial(ctx, providerOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return orderport.WeChatShopOrderMaterial{}, false, nil
	}
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, commerceRefundStoreError(err)
	}
	lines, err := q.ListWeChatShopRefundMaterialLines(ctx, row.ID)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, commerceRefundStoreError(err)
	}
	material, err := mapWeChatShopOrderMaterial(row, lines)
	if err != nil {
		return orderport.WeChatShopOrderMaterial{}, false, commerceRefundStoreError(err)
	}
	return material, true, nil
}

func (repository *CommerceRefundRepository) EnqueueWeChatShopMaterialSync(ctx context.Context, providerOrderID string, at time.Time) (int64, error) {
	q, err := commerceRefundQueries(ctx)
	if repository == nil || repository.client == nil || err != nil {
		return 0, commerceRefundStoreError(err)
	}
	request, err := q.ReserveWeChatShopMaterialSync(ctx, orderdb.ReserveWeChatShopMaterialSyncParams{ProviderOrderID: providerOrderID, RequestedAt: pgTime(at)})
	if errors.Is(err, pgx.ErrNoRows) {
		request, err = q.GetWeChatShopMaterialSync(ctx, providerOrderID)
		if err != nil || request.State != "queued" || !request.RiverJobID.Valid || request.RiverJobID.Int64 < 1 {
			return 0, commerceRefundStoreError(err)
		}
		return request.RiverJobID.Int64, nil
	}
	if err != nil || request.State != "reserved" || request.Generation < 1 {
		return 0, commerceRefundStoreError(err)
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, orderapp.WeChatShopMaterialSyncArgs{ProviderOrderID: providerOrderID}, string(platformjobqueue.QueueSync))
	if err != nil || jobID < 1 {
		return 0, errors.Join(orderport.ErrWeChatShopMaterialUnavailable, err)
	}
	queued, err := q.MarkWeChatShopMaterialSyncQueued(ctx, orderdb.MarkWeChatShopMaterialSyncQueuedParams{RiverJobID: pgtype.Int8{Int64: jobID, Valid: true}, ProviderOrderID: providerOrderID, Generation: request.Generation})
	if err != nil || queued.State != "queued" || !queued.RiverJobID.Valid || queued.RiverJobID.Int64 != jobID {
		return 0, commerceRefundConflictError(err)
	}
	return jobID, nil
}

func (*CommerceRefundRepository) CountWeChatShopReservedRefundAmount(ctx context.Context, orderID orderport.ID) (int64, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return 0, err
	}
	value, err := q.CountWeChatShopReservedRefundAmount(ctx, int64(orderID))
	return value, commerceRefundStoreError(err)
}

func (*CommerceRefundRepository) CountWeChatShopReservedRefundLineCount(ctx context.Context, orderID orderport.ID, productID, skuID string) (int64, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return 0, err
	}
	value, err := q.CountWeChatShopReservedRefundLineCount(ctx, orderdb.CountWeChatShopReservedRefundLineCountParams{OrderID: int64(orderID), ProductID: productID, SkuID: skuID})
	return value, commerceRefundStoreError(err)
}

func (*CommerceRefundRepository) GetWeChatShopRefundByCommand(ctx context.Context, actor int64, key [32]byte) (orderport.WeChatShopRefund, [32]byte, bool, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, [32]byte{}, false, err
	}
	row, err := q.GetWeChatShopRefundByCommand(ctx, orderdb.GetWeChatShopRefundByCommandParams{ActorID: actor, CommandKeyDigest: key[:]})
	if errors.Is(err, pgx.ErrNoRows) {
		return orderport.WeChatShopRefund{}, [32]byte{}, false, nil
	}
	return mapWeChatShopRefund(row), digestValue(row.CommandPayloadDigest), err == nil, commerceRefundStoreError(err)
}

func (*CommerceRefundRepository) CreateWeChatShopRefund(ctx context.Context, reservation orderapp.WeChatShopRefundReservation) (orderport.WeChatShopRefund, bool, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, false, err
	}
	params := orderdb.CreateWeChatShopRefundParams{
		OrderID: int64(reservation.Order.ID), ActorID: reservation.Command.Actor,
		MerchantOrderNo: reservation.Order.MerchantOrderNo, OutRefundNo: reservation.OutRefundNo,
		ProviderOrderID:        pgtype.Text{String: reservation.Material.ProviderOrderID, Valid: true},
		ProductID:              pgtype.Text{String: reservation.Command.ProductID, Valid: true},
		SkuID:                  pgtype.Text{String: reservation.Command.SKUID, Valid: true},
		RefundCount:            pgtype.Int8{Int64: reservation.Command.Count, Valid: true},
		UnitPriceMinor:         pgtype.Int8{Int64: reservation.Line.RealPriceMinor, Valid: true},
		ReasonCode:             pgtype.Text{String: reservation.Command.ReasonCode, Valid: true},
		MaterialEvidenceDigest: reservation.Material.EvidenceDigest[:],
		AmountMinor:            reservation.Command.AmountMinor, Currency: reservation.Order.Currency,
		ReasonDigest: reservation.ReasonDigest[:], TransactionDigest: reservation.TransactionDigest[:],
		CommandKeyDigest: reservation.CommandKeyDigest[:], CommandPayloadDigest: reservation.CommandPayloadDigest[:],
		SourceRefDigest: reservation.SourceRefDigest[:], TargetRefDigest: reservation.TargetRefDigest[:],
		PayloadDigest: reservation.PayloadDigest[:], PolicyVersionDigest: reservation.PolicyDigest[:],
		CreatedAt: pgTime(reservation.CreatedAt),
	}
	row, err := q.CreateWeChatShopRefund(ctx, params)
	if err == nil {
		return mapWeChatShopRefund(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return orderport.WeChatShopRefund{}, false, commerceRefundStoreError(err)
	}
	existing, getErr := q.GetWeChatShopRefundByCommand(ctx, orderdb.GetWeChatShopRefundByCommandParams{ActorID: reservation.Command.Actor, CommandKeyDigest: reservation.CommandKeyDigest[:]})
	if getErr != nil {
		return orderport.WeChatShopRefund{}, false, commerceRefundStoreError(getErr)
	}
	if !bytes.Equal(existing.CommandPayloadDigest, reservation.CommandPayloadDigest[:]) {
		return orderport.WeChatShopRefund{}, false, orderport.ErrCommerceRefundConflict
	}
	return mapWeChatShopRefund(existing), false, nil
}

func (repository *CommerceRefundRepository) EnqueueWeChatShopRefund(ctx context.Context, refundID int64) (int64, error) {
	if repository == nil || repository.client == nil || refundID < 1 {
		return 0, orderport.ErrCommerceRefundUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, orderapp.WeChatShopRefundArgs{RefundID: refundID}, string(platformjobqueue.QueueCritical))
	if err != nil || jobID < 1 {
		return 0, errors.Join(orderport.ErrCommerceRefundUnavailable, err)
	}
	return jobID, nil
}

func (*CommerceRefundRepository) LockWeChatShopRefundByID(ctx context.Context, refundID int64) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.LockWeChatShopRefundByID(ctx, refundID)
	return mapWeChatShopRefund(row), commerceRefundStoreError(err)
}

func (*CommerceRefundRepository) LockWeChatShopRefundByAfterSaleID(ctx context.Context, number string) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.LockWeChatShopRefundByAfterSaleID(ctx, pgtype.Text{String: number, Valid: true})
	return mapWeChatShopRefund(row), commerceRefundStoreError(err)
}

func (*CommerceRefundRepository) StartWeChatShopRefundExecution(ctx context.Context, refund orderport.WeChatShopRefund, job orderport.WeChatShopExecutionJob, at time.Time) (orderport.WeChatShopRefund, orderapp.WeChatShopRefundAttempt, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, orderapp.WeChatShopRefundAttempt{}, err
	}
	row, err := q.StartWeChatShopRefundExecution(ctx, orderdb.StartWeChatShopRefundExecutionParams{StartedAt: pgTime(at), RefundID: refund.ID, ExpectedVersion: refund.Version})
	if err != nil {
		return orderport.WeChatShopRefund{}, orderapp.WeChatShopRefundAttempt{}, commerceRefundConflictError(err)
	}
	updated := mapWeChatShopRefund(row)
	attemptRow, err := q.InsertWeChatShopRefundAttempt(ctx, orderdb.InsertWeChatShopRefundAttemptParams{RefundID: updated.ID, AttemptNo: updated.AttemptCount, RiverJobID: job.RiverJobID, RiverAttempt: job.RiverAttempt, ArgsDigest: job.ArgsDigest[:], RequestDigest: updated.PayloadDigest[:], StartedAt: pgTime(at)})
	if err != nil {
		return orderport.WeChatShopRefund{}, orderapp.WeChatShopRefundAttempt{}, commerceRefundStoreError(err)
	}
	return updated, mapWeChatShopAttempt(attemptRow), nil
}

func (repository *CommerceRefundRepository) RecoverWeChatShopRefundExecution(ctx context.Context, refund orderport.WeChatShopRefund, at time.Time) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.LockIncompleteWeChatShopRefundAttempt(ctx, refund.ID)
	if err != nil {
		return orderport.WeChatShopRefund{}, commerceRefundConflictError(err)
	}
	return repository.CompleteWeChatShopRefundExecution(ctx, refund, mapWeChatShopAttempt(row), orderport.WeChatShopProviderOutcomeUnknown, [32]byte{}, "", at)
}

func (*CommerceRefundRepository) CompleteWeChatShopRefundExecution(ctx context.Context, refund orderport.WeChatShopRefund, attempt orderapp.WeChatShopRefundAttempt, outcome orderport.WeChatShopProviderCompletion, evidence [32]byte, afterSaleID string, at time.Time) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	var optionalEvidence []byte
	if !zeroDigest(evidence) {
		optionalEvidence = evidence[:]
	}
	if _, err = q.CompleteWeChatShopRefundAttempt(ctx, orderdb.CompleteWeChatShopRefundAttemptParams{Outcome: pgtype.Text{String: string(outcome), Valid: true}, EvidenceDigest: optionalEvidence, CompletedAt: pgTime(at), AttemptID: attempt.ID}); err != nil {
		return orderport.WeChatShopRefund{}, commerceRefundConflictError(err)
	}
	params := orderdb.CompleteWeChatShopRefundExecutionParams{State: string(shopStateForOutcome(outcome)), CompletedAt: pgTime(at), RefundID: refund.ID, ExpectedVersion: refund.Version}
	if outcome == orderport.WeChatShopProviderAccepted {
		params.ProviderAcceptanceDigest = evidence[:]
		params.ProviderAftersaleID = pgtype.Text{String: afterSaleID, Valid: true}
	}
	row, err := q.CompleteWeChatShopRefundExecution(ctx, params)
	return mapWeChatShopRefund(row), commerceRefundConflictError(err)
}

func (repository *CommerceRefundRepository) EnqueueWeChatShopRefundReconciliation(ctx context.Context, refundID int64) (int64, error) {
	if repository == nil || repository.client == nil || refundID < 1 {
		return 0, orderport.ErrCommerceRefundUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	jobID, err := repository.client.InsertTx(ctx, tx, orderapp.WeChatShopRefundReconcileArgs{RefundID: refundID}, string(platformjobqueue.QueueCritical))
	if err != nil || jobID < 1 {
		return 0, errors.Join(orderport.ErrCommerceRefundUnavailable, err)
	}
	return jobID, nil
}

func (*CommerceRefundRepository) ReserveWeChatShopRefundCallback(ctx context.Context, refund orderport.WeChatShopRefund, command orderport.WeChatShopRefundCallbackCommand) (orderapp.WeChatShopCallbackReceipt, bool, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderapp.WeChatShopCallbackReceipt{}, false, err
	}
	afterSaleDigest := digestString("wechat-shop/aftersale-id/v1", command.AfterSaleID)
	row, err := q.ReserveWeChatShopRefundCallback(ctx, orderdb.ReserveWeChatShopRefundCallbackParams{RefundID: refund.ID, ProviderEventDigest: command.ProviderEventDigest[:], PayloadDigest: command.PayloadDigest[:], ProviderRefundDigest: afterSaleDigest[:], ProviderAftersaleID: pgtype.Text{String: command.AfterSaleID, Valid: true}, ProviderStatus: pgtype.Text{String: command.ProviderStatus, Valid: true}, ReceivedAt: pgTime(command.OccurredAt)})
	if err == nil {
		return mapWeChatShopCallback(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return orderapp.WeChatShopCallbackReceipt{}, false, commerceRefundStoreError(err)
	}
	existing, getErr := q.GetWeChatShopRefundCallback(ctx, command.ProviderEventDigest[:])
	return mapWeChatShopCallback(existing), false, commerceRefundStoreError(getErr)
}

func (*CommerceRefundRepository) CompleteWeChatShopRefundCallback(ctx context.Context, receipt orderapp.WeChatShopCallbackReceipt, outcome string, digest [32]byte, riverJobID int64, at time.Time) (orderapp.WeChatShopCallbackReceipt, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderapp.WeChatShopCallbackReceipt{}, err
	}
	row, err := q.CompleteWeChatShopRefundCallback(ctx, orderdb.CompleteWeChatShopRefundCallbackParams{Outcome: pgtype.Text{String: outcome, Valid: true}, ResultDigest: digest[:], RiverJobID: pgtype.Int8{Int64: riverJobID, Valid: true}, CompletedAt: pgTime(at), CallbackID: receipt.ID})
	return mapWeChatShopCallback(row), commerceRefundConflictError(err)
}

func (*CommerceRefundRepository) ApplyWeChatShopRefundSettlement(ctx context.Context, refund orderport.WeChatShopRefund, providerDigest, settlementDigest [32]byte, at time.Time) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.ApplyWeChatShopRefundSettlement(ctx, orderdb.ApplyWeChatShopRefundSettlementParams{ProviderRefundDigest: providerDigest[:], SettlementReceiptDigest: settlementDigest[:], SettledAt: pgTime(at), RefundID: refund.ID, AmountMinor: refund.AmountMinor, Currency: refund.Currency, ExpectedVersion: refund.Version})
	return mapWeChatShopRefund(row), commerceRefundConflictError(err)
}

func (*CommerceRefundRepository) MarkWeChatShopRefundFinalFailed(ctx context.Context, refund orderport.WeChatShopRefund, at time.Time) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.MarkWeChatShopRefundFinalFailed(ctx, orderdb.MarkWeChatShopRefundFinalFailedParams{UpdatedAt: pgTime(at), RefundID: refund.ID, ExpectedVersion: refund.Version})
	return mapWeChatShopRefund(row), commerceRefundConflictError(err)
}

func (*CommerceRefundRepository) RecordWeChatShopRefundQuery(ctx context.Context, refund orderport.WeChatShopRefund, query orderport.WeChatShopRefundQueryResult, outcome string, at time.Time) error {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return err
	}
	var providerDigest []byte
	if !zeroDigest(query.ProviderRefundDigest) {
		providerDigest = query.ProviderRefundDigest[:]
	}
	params := orderdb.InsertWeChatShopRefundQueryParams{RefundID: refund.ID, EvidenceDigest: query.EvidenceDigest[:], ProviderRefundDigest: providerDigest, AmountMinor: query.AmountMinor, Currency: query.Currency, Outcome: outcome, RecordedAt: pgTime(at)}
	row, err := q.InsertWeChatShopRefundQuery(ctx, params)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return commerceRefundStoreError(err)
	}
	existing, getErr := q.GetWeChatShopRefundQuery(ctx, orderdb.GetWeChatShopRefundQueryParams{RefundID: refund.ID, EvidenceDigest: query.EvidenceDigest[:]})
	if getErr != nil || existing.Outcome != outcome || existing.AmountMinor != query.AmountMinor || existing.Currency != query.Currency || !bytes.Equal(existing.ProviderRefundDigest, providerDigest) {
		return orderport.ErrCommerceRefundConflict
	}
	_ = row
	return nil
}

func commerceRefundQueries(ctx context.Context) (*orderdb.Queries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, errors.Join(orderport.ErrCommerceRefundUnavailable, err)
	}
	return orderdb.New(tx), nil
}

func mapWeChatShopRefund(row orderdb.OrderWechatShopRefund) orderport.WeChatShopRefund {
	return orderport.WeChatShopRefund{
		ID: row.ID, OrderID: orderport.ID(row.OrderID), ContractVersion: row.ContractVersion, MerchantOrderNo: row.MerchantOrderNo,
		ProviderOrderID: row.ProviderOrderID.String, ProductID: row.ProductID.String, SKUID: row.SkuID.String,
		RefundCount: row.RefundCount.Int64, UnitPriceMinor: row.UnitPriceMinor.Int64, ReasonCode: row.ReasonCode.String,
		MaterialEvidenceDigest: digestValue(row.MaterialEvidenceDigest), ProviderAfterSaleID: row.ProviderAftersaleID.String,
		OutRefundNo: row.OutRefundNo, AmountMinor: row.AmountMinor, Currency: row.Currency,
		ReasonDigest: digestValue(row.ReasonDigest), TransactionDigest: digestValue(row.TransactionDigest),
		SourceRefDigest: digestValue(row.SourceRefDigest), TargetRefDigest: digestValue(row.TargetRefDigest),
		PayloadDigest: digestValue(row.PayloadDigest), PolicyDigest: digestValue(row.PolicyVersionDigest),
		ProviderAcceptanceDigest: digestValue(row.ProviderAcceptanceDigest), ProviderRefundDigest: digestValue(row.ProviderRefundDigest),
		SettlementDigest: digestValue(row.SettlementReceiptDigest), State: orderport.WeChatShopRefundState(row.State),
		AttemptCount: row.AttemptCount, Version: row.Version, CreatedAt: row.CreatedAt.Time.UTC(),
		UpdatedAt: row.UpdatedAt.Time.UTC(), SettledAt: timeValue(row.SettledAt),
	}
}

func mapWeChatShopAttempt(row orderdb.OrderWechatShopRefundAttempt) orderapp.WeChatShopRefundAttempt {
	return orderapp.WeChatShopRefundAttempt{ID: row.ID, RefundID: row.RefundID, AttemptNo: row.AttemptNo, RequestDigest: digestValue(row.RequestDigest), Outcome: orderport.WeChatShopProviderCompletion(row.Outcome.String), EvidenceDigest: digestValue(row.EvidenceDigest)}
}

func mapWeChatShopCallback(row orderdb.OrderWechatShopRefundCallback) orderapp.WeChatShopCallbackReceipt {
	return orderapp.WeChatShopCallbackReceipt{ID: row.ID, RefundID: row.RefundID, ProviderEventDigest: digestValue(row.ProviderEventDigest), PayloadDigest: digestValue(row.PayloadDigest), ProviderRefundDigest: digestValue(row.ProviderRefundDigest), ProviderAfterSaleID: row.ProviderAftersaleID.String, ProviderStatus: row.ProviderStatus.String, RiverJobID: row.RiverJobID.Int64, Outcome: row.Outcome.String, ResultDigest: digestValue(row.ResultDigest), State: row.State}
}

func digestString(domain, value string) [32]byte {
	return sha256.Sum256([]byte(domain + "\x00" + value))
}

func shopStateForOutcome(outcome orderport.WeChatShopProviderCompletion) orderport.WeChatShopRefundState {
	switch outcome {
	case orderport.WeChatShopProviderAccepted:
		return orderport.WeChatShopRefundProviderAccepted
	case orderport.WeChatShopProviderFinalFailed:
		return orderport.WeChatShopRefundFinalFailed
	default:
		return orderport.WeChatShopRefundOutcomeUnknown
	}
}

func commerceRefundStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return orderport.ErrCommerceRefundNotFound
	}
	return errors.Join(orderport.ErrCommerceRefundUnavailable, err)
}

func commerceRefundConflictError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return orderport.ErrCommerceRefundConflict
	}
	return commerceRefundStoreError(err)
}

func zeroDigest(value [32]byte) bool { return value == [32]byte{} }

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

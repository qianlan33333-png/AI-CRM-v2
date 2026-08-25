package store

import (
	"bytes"
	"context"
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

func (*CommerceRefundRepository) CountWeChatShopReservedRefundAmount(ctx context.Context, orderID orderport.ID) (int64, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return 0, err
	}
	value, err := q.CountWeChatShopReservedRefundAmount(ctx, int64(orderID))
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
		AmountMinor: reservation.Command.AmountMinor, Currency: reservation.Order.Currency,
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

func (*CommerceRefundRepository) LockWeChatShopRefundByOutRefundNo(ctx context.Context, number string) (orderport.WeChatShopRefund, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderport.WeChatShopRefund{}, err
	}
	row, err := q.LockWeChatShopRefundByOutRefundNo(ctx, number)
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

func (*CommerceRefundRepository) CompleteWeChatShopRefundExecution(ctx context.Context, refund orderport.WeChatShopRefund, attempt orderapp.WeChatShopRefundAttempt, outcome orderport.WeChatShopProviderCompletion, evidence [32]byte, at time.Time) (orderport.WeChatShopRefund, error) {
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
	}
	row, err := q.CompleteWeChatShopRefundExecution(ctx, params)
	return mapWeChatShopRefund(row), commerceRefundConflictError(err)
}

func (*CommerceRefundRepository) ReserveWeChatShopRefundCallback(ctx context.Context, refund orderport.WeChatShopRefund, command orderport.WeChatShopRefundCallbackCommand) (orderapp.WeChatShopCallbackReceipt, bool, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderapp.WeChatShopCallbackReceipt{}, false, err
	}
	row, err := q.ReserveWeChatShopRefundCallback(ctx, orderdb.ReserveWeChatShopRefundCallbackParams{RefundID: refund.ID, ProviderEventDigest: command.ProviderEventDigest[:], PayloadDigest: command.PayloadDigest[:], ProviderRefundDigest: command.ProviderRefundDigest[:], ReceivedAt: pgTime(command.OccurredAt)})
	if err == nil {
		return mapWeChatShopCallback(row), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return orderapp.WeChatShopCallbackReceipt{}, false, commerceRefundStoreError(err)
	}
	existing, getErr := q.GetWeChatShopRefundCallback(ctx, command.ProviderEventDigest[:])
	return mapWeChatShopCallback(existing), false, commerceRefundStoreError(getErr)
}

func (*CommerceRefundRepository) CompleteWeChatShopRefundCallback(ctx context.Context, receipt orderapp.WeChatShopCallbackReceipt, outcome string, digest [32]byte, at time.Time) (orderapp.WeChatShopCallbackReceipt, error) {
	q, err := commerceRefundQueries(ctx)
	if err != nil {
		return orderapp.WeChatShopCallbackReceipt{}, err
	}
	row, err := q.CompleteWeChatShopRefundCallback(ctx, orderdb.CompleteWeChatShopRefundCallbackParams{Outcome: pgtype.Text{String: outcome, Valid: true}, ResultDigest: digest[:], CompletedAt: pgTime(at), CallbackID: receipt.ID})
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
		ID: row.ID, OrderID: orderport.ID(row.OrderID), MerchantOrderNo: row.MerchantOrderNo,
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
	return orderapp.WeChatShopCallbackReceipt{ID: row.ID, RefundID: row.RefundID, ProviderEventDigest: digestValue(row.ProviderEventDigest), PayloadDigest: digestValue(row.PayloadDigest), ProviderRefundDigest: digestValue(row.ProviderRefundDigest), Outcome: row.Outcome.String, ResultDigest: digestValue(row.ResultDigest), State: row.State}
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

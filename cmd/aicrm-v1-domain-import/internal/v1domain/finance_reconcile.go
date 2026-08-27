package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

func verifyFinanceTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	id, err := financeTargetID(row)
	if err != nil || ctx == nil || tx == nil {
		return "", ErrConflict
	}
	table := *row.TargetTable
	verified := false
	switch table {
	case "order_list_projections":
		var order orderport.Record
		order, err = readFinanceOrder(ctx, tx, id)
		verified = err == nil && financeOrderMatchesTarget(order, row.TargetDigest)
	case "order_historical_refunds":
		var refund orderport.HistoricalRefund
		refund, err = readFinanceRefund(ctx, tx, id)
		if err == nil && containsTarget(targets, "order_list_projections", strconv.FormatInt(int64(refund.OrderID), 10)) {
			var order orderport.Record
			order, err = readFinanceOrder(ctx, tx, int64(refund.OrderID))
			verified = err == nil && financeRefundMatchesTarget(refund, order, targets, row.TargetDigest)
		}
	}
	if err != nil || !verified {
		return "", targetVerificationError(table, *row.TargetID, err)
	}
	return table + ":" + *row.TargetID + ":v1_history:" + hex.EncodeToString(row.TargetDigest), nil
}

func financeTargetID(row reconciliationRow) (int64, error) {
	if row.TargetDomain == nil || *row.TargetDomain != "order" || row.TargetTable == nil || row.TargetID == nil || len(row.TargetDigest) != sha256.Size {
		return 0, ErrConflict
	}
	if !(row.TableID == "public/wechat_pay_orders" && *row.TargetTable == "order_list_projections") &&
		!(row.TableID == "public/wechat_pay_refunds" && *row.TargetTable == "order_historical_refunds") {
		return 0, ErrConflict
	}
	return positiveID(*row.TargetID)
}

func financeOrderMatchesTarget(order orderport.Record, expected []byte) bool {
	digest := orderapp.HistoricalOrderTargetDigest(order)
	return order.ID > 0 && order.RecordOrigin == orderport.RecordOriginV1History && order.Provider == "wechat" && order.Currency == "CNY" &&
		len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func financeRefundMatchesTarget(refund orderport.HistoricalRefund, order orderport.Record, targets map[string]map[string]struct{}, expected []byte) bool {
	if refund.ID < 1 || refund.OrderID != order.ID || order.ID < 1 || order.RecordOrigin != orderport.RecordOriginV1History || order.Provider != "wechat" ||
		order.Currency != "CNY" || refund.Currency != order.Currency || refund.OrderAmountMinor != order.AmountMinor ||
		!containsTarget(targets, "order_list_projections", strconv.FormatInt(int64(order.ID), 10)) ||
		(refund.TransactionID != "" && refund.TransactionID != order.PlatformTransactionNo) {
		return false
	}
	digest := orderapp.HistoricalRefundTargetDigest(refund)
	return len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

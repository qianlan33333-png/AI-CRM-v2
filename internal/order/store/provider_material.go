package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

// ProviderMaterialReader is Order-owned. It exposes only the canonical facts
// needed to prove a queued PE01 Provider request still matches its Order.
type ProviderMaterialReader struct{}

type PE01PrepayProviderMaterial struct {
	MerchantOrderNo       string
	CustomerID            int64
	ProductName           string
	AmountMinor           int64
	Currency              string
	PaymentIdentityDigest [32]byte
}

type PE01RefundProviderMaterial struct {
	MerchantOrderNo     string
	OriginalAmountMinor int64
	Currency            string
	OutRefundNo         string
	RefundAmountMinor   int64
	Reason              string
}

func NewProviderMaterialReader() *ProviderMaterialReader { return &ProviderMaterialReader{} }

func (*ProviderMaterialReader) ReadPE01Prepay(ctx context.Context, merchantOrderNo string) (PE01PrepayProviderMaterial, bool, error) {
	queries, err := transactionQueries(ctx)
	if err != nil || !validProviderMaterialReference(merchantOrderNo) {
		return PE01PrepayProviderMaterial{}, false, orderport.ErrSettlementUnavailable
	}
	row, err := queries.ReadPE01PrepayProviderMaterial(ctx, merchantOrderNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return PE01PrepayProviderMaterial{}, false, nil
	}
	if err != nil || !row.CustomerID.Valid || row.CustomerID.Int64 < 1 || row.AmountMinor < 1 || row.Currency != "CNY" || strings.TrimSpace(row.ProductNameSnapshot) != row.ProductNameSnapshot || row.ProductNameSnapshot == "" || len(row.ProductNameSnapshot) > 127 || len(row.PaymentIdentityDigest) != 32 {
		if err != nil {
			return PE01PrepayProviderMaterial{}, false, errors.Join(orderport.ErrSettlementUnavailable, err)
		}
		return PE01PrepayProviderMaterial{}, false, orderport.ErrSettlementUnavailable
	}
	return PE01PrepayProviderMaterial{MerchantOrderNo: row.MerchantOrderNo, CustomerID: row.CustomerID.Int64, ProductName: row.ProductNameSnapshot, AmountMinor: row.AmountMinor, Currency: row.Currency, PaymentIdentityDigest: digestValue(row.PaymentIdentityDigest)}, true, nil
}

func (*ProviderMaterialReader) ReadPE01Refund(ctx context.Context, outRefundNo string) (PE01RefundProviderMaterial, bool, error) {
	queries, err := transactionQueries(ctx)
	if err != nil || !validProviderMaterialReference(outRefundNo) {
		return PE01RefundProviderMaterial{}, false, orderport.ErrSettlementUnavailable
	}
	row, err := queries.ReadPE01RefundProviderMaterial(ctx, outRefundNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return PE01RefundProviderMaterial{}, false, nil
	}
	if err != nil || row.OriginalAmountMinor < 1 || row.RefundAmountMinor < 1 || row.RefundAmountMinor > row.OriginalAmountMinor || row.Currency != "CNY" || strings.TrimSpace(row.Reason) != row.Reason || row.Reason == "" || len(row.Reason) > 80 {
		if err != nil {
			return PE01RefundProviderMaterial{}, false, errors.Join(orderport.ErrSettlementUnavailable, err)
		}
		return PE01RefundProviderMaterial{}, false, orderport.ErrSettlementUnavailable
	}
	return PE01RefundProviderMaterial{MerchantOrderNo: row.MerchantOrderNo, OriginalAmountMinor: row.OriginalAmountMinor, Currency: row.Currency, OutRefundNo: row.OutRefundNo, RefundAmountMinor: row.RefundAmountMinor, Reason: row.Reason}, true, nil
}

func validProviderMaterialReference(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}

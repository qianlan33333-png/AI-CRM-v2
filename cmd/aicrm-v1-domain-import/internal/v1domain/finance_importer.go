package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1finance"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
)

const (
	financeOrdersTableID  = "public/wechat_pay_orders"
	financeRefundsTableID = "public/wechat_pay_refunds"
)

type FinanceReferenceResolver interface {
	ResolveHistoricalOrderReferences(context.Context, v1finance.OrderFact) (customerID, productID *int64, err error)
}

type financeImportJournal interface {
	orderport.HistoricalImportJournal
	validateArchiveRun(string) error
	LoadTerminal(context.Context, string, [sha256.Size]byte) (TerminalReceipt, bool, error)
	Record(context.Context, string, TerminalReceipt) error
}

type FinanceImportResult struct {
	ImportedOrders, ImportedRefunds       int
	QuarantinedOrders, QuarantinedRefunds int
	ReplayedOrders, ReplayedRefunds       int
}

// FinanceImporter imports only read-only V1 financial history. It neither
// creates payment/refund commands nor records Provider/effect/queue outcomes.
type FinanceImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   *orderapp.HistoricalImportService
	journal  financeImportJournal
	resolver FinanceReferenceResolver
}

func NewFinanceImporter(archive ArchiveSource, uow UnitOfWork, writer *orderapp.HistoricalImportService, journal financeImportJournal, resolver FinanceReferenceResolver) (*FinanceImporter, error) {
	if archive == nil || uow == nil || writer == nil || journal == nil || resolver == nil {
		return nil, ErrInvalidScope
	}
	return &FinanceImporter{archive: archive, uow: uow, writer: writer, journal: journal, resolver: resolver}, nil
}

type financeArchiveRow struct {
	archive         v1archive.ArchivedRow
	fieldDigest     [sha256.Size]byte
	redactionReason string
}

func (importer *FinanceImporter) Import(ctx context.Context, archiveRunID string) (FinanceImportResult, error) {
	if importer == nil || archiveRunID == "" {
		return FinanceImportResult{}, ErrInvalidScope
	}
	if err := importer.journal.validateArchiveRun(archiveRunID); err != nil {
		return FinanceImportResult{}, err
	}
	orders, err := importer.readRows(ctx, archiveRunID, financeOrdersTableID, "id", "coupon_claim_id")
	if err != nil {
		return FinanceImportResult{}, err
	}
	refunds, err := importer.readRows(ctx, archiveRunID, financeRefundsTableID, "id", "order_id")
	if err != nil {
		return FinanceImportResult{}, err
	}
	orderPayloads, refundPayloads := make([]json.RawMessage, len(orders)), make([]json.RawMessage, len(refunds))
	for index := range orders {
		orderPayloads[index] = orders[index].archive.Payload
	}
	for index := range refunds {
		refundPayloads[index] = refunds[index].archive.Payload
	}
	history := v1finance.AdaptHistory(orderPayloads, refundPayloads)
	if len(history.Orders) != len(orders) || len(history.Refunds) != len(refunds) {
		return FinanceImportResult{}, ErrConflict
	}

	result := FinanceImportResult{}
	orderTargets := make(map[int64]int64, len(orders))
	for index, decision := range history.Orders {
		if err := importer.importOrder(ctx, orders[index], decision, orderTargets, &result); err != nil {
			return FinanceImportResult{}, err
		}
	}
	for index, decision := range history.Refunds {
		if err := importer.importRefund(ctx, refunds[index], decision, orderTargets, &result); err != nil {
			return FinanceImportResult{}, err
		}
	}
	return result, nil
}

func (importer *FinanceImporter) readRows(ctx context.Context, archiveRunID, tableID string, excluded ...string) ([]financeArchiveRow, error) {
	rows := make([]financeArchiveRow, 0)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 ||
			row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		fieldDigest, err := financeSourceFieldDigest(row.Payload, excluded...)
		if err != nil {
			return fmt.Errorf("canonicalize archived %s row %d: %w", tableID, row.SourceOrdinal, err)
		}
		rows = append(rows, financeArchiveRow{archive: row, fieldDigest: fieldDigest, redactionReason: financeRedactionReason(tableID, row)})
		return nil
	})
	return rows, err
}

func (importer *FinanceImporter) importOrder(ctx context.Context, row financeArchiveRow, decision v1finance.OrderResult, targets map[int64]int64, result *FinanceImportResult) error {
	if row.redactionReason != "" {
		return importer.quarantine(ctx, financeOrderKind, row, row.redactionReason, result)
	}
	if decision.Disposition != v1finance.DispositionCandidate || decision.Fact == nil {
		return importer.quarantine(ctx, financeOrderKind, row, decision.Reason, result)
	}
	customerID, productID, err := importer.resolver.ResolveHistoricalOrderReferences(ctx, *decision.Fact)
	if err != nil {
		return err
	}
	if invalidHistoricalReferenceID(customerID) || invalidHistoricalReferenceID(productID) {
		return ErrConflict
	}
	fieldDigest := financeMappedOrderFieldDigest(row.fieldDigest, customerID, productID)
	writeResult, err := importer.writer.ImportOrder(ctx, orderport.HistoricalOrderRecord{
		Fact:  financeHistoricalFact(row, fieldDigest),
		Order: financeOrderRecord(*decision.Fact, customerID, productID),
	})
	if errors.Is(err, orderport.ErrHistoricalInput) {
		return importer.quarantineWithDigest(ctx, financeOrderKind, row, fieldDigest, "order_target_invalid", result)
	}
	if err != nil {
		return err
	}
	targets[decision.Fact.SourceID] = writeResult.TargetID
	result.ImportedOrders++
	if writeResult.Replayed {
		result.ReplayedOrders++
	}
	return nil
}

func (importer *FinanceImporter) importRefund(ctx context.Context, row financeArchiveRow, decision v1finance.RefundResult, targets map[int64]int64, result *FinanceImportResult) error {
	if row.redactionReason != "" {
		return importer.quarantine(ctx, financeRefundKind, row, row.redactionReason, result)
	}
	if decision.Disposition != v1finance.DispositionCandidate || decision.Fact == nil {
		return importer.quarantine(ctx, financeRefundKind, row, decision.Reason, result)
	}
	orderID, found := targets[decision.Fact.OrderSourceID]
	if !found || orderID < 1 {
		return importer.quarantine(ctx, financeRefundKind, row, "refund_parent_order_unavailable", result)
	}
	fieldDigest := financeMappedRefundFieldDigest(row.fieldDigest, orderID)
	writeResult, err := importer.writer.ImportRefund(ctx, orderport.HistoricalRefundRecord{
		Fact:   financeHistoricalFact(row, fieldDigest),
		Refund: financeRefundRecord(*decision.Fact, orderID),
	})
	if errors.Is(err, orderport.ErrHistoricalInput) {
		return importer.quarantineWithDigest(ctx, financeRefundKind, row, fieldDigest, "refund_target_invalid", result)
	}
	if err != nil {
		return err
	}
	result.ImportedRefunds++
	if writeResult.Replayed {
		result.ReplayedRefunds++
	}
	return nil
}

func (importer *FinanceImporter) quarantine(ctx context.Context, kind string, row financeArchiveRow, reason string, result *FinanceImportResult) error {
	return importer.quarantineWithDigest(ctx, kind, row, row.fieldDigest, reason, result)
}

func (importer *FinanceImporter) quarantineWithDigest(ctx context.Context, kind string, row financeArchiveRow, fieldDigest [sha256.Size]byte, reason string, result *FinanceImportResult) error {
	if reason == "" {
		return ErrConflict
	}
	want := TerminalReceipt{SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC, Disposition: "quarantine", Reason: reason, Metadata: fieldDigestMetadata(fieldDigest)}
	replayed := false
	if err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		found, exists, err := importer.journal.LoadTerminal(tx, kind, row.archive.SourceKeyHMAC)
		if err != nil {
			return err
		}
		if exists {
			if !sameFinanceTerminal(found, want) {
				return ErrConflict
			}
			replayed = true
		}
		return importer.journal.Record(tx, kind, want)
	}); err != nil {
		return err
	}
	if kind == financeOrderKind {
		result.QuarantinedOrders++
		if replayed {
			result.ReplayedOrders++
		}
	} else {
		result.QuarantinedRefunds++
		if replayed {
			result.ReplayedRefunds++
		}
	}
	return nil
}

func financeRedactionReason(tableID string, row v1archive.ArchivedRow) string {
	var fields []string
	switch tableID {
	case financeOrdersTableID:
		fields = []string{"id", "out_trade_no", "product_code", "product_name", "amount_total", "currency", "unionid", "status", "trade_state", "transaction_id", "paid_at", "created_at", "updated_at", "payer_name_snapshot", "refunded_amount_total", "refund_status"}
	case financeRefundsTableID:
		fields = []string{"id", "order_id", "out_trade_no", "transaction_id", "out_refund_no", "refund_id", "reason", "refund_amount_total", "order_amount_total", "currency", "status", "created_at", "updated_at"}
	default:
		return ""
	}
	for _, field := range fields {
		if v1archive.IsRedacted(row, field) {
			if tableID == financeOrdersTableID {
				return "order_business_field_redacted"
			}
			return "refund_business_field_redacted"
		}
	}
	return ""
}

func financeHistoricalFact(row financeArchiveRow, fieldDigest [sha256.Size]byte) orderport.HistoricalFact {
	return orderport.HistoricalFact{SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC, FieldDigest: fieldDigest}
}

func financeOrderRecord(source v1finance.OrderFact, customerID, productID *int64) orderport.Record {
	identityKind, identityValue := "", ""
	if source.UnionID != "" {
		identityKind, identityValue = "unionid", source.UnionID
	}
	return orderport.Record{
		RecordOrigin: orderport.RecordOriginV1History, Provider: "wechat", ProviderLabel: "微信支付（V1历史）",
		MerchantOrderNo: source.OrderNumber, PlatformTransactionNo: source.TransactionID,
		CustomerID: customerID, PayerNameSnapshot: source.PayerNameSnapshot,
		IdentityKind: identityKind, IdentityValue: identityValue,
		ProductID: productID, ProductCode: source.Product.Value, ProductNameSnapshot: source.ProductName,
		AmountMinor: source.AmountMinor, Currency: source.Currency, Status: source.Status, StatusLabel: "V1历史/未重新核验",
		DetailURL: "/api/admin/wechat-pay/orders/" + url.PathEscape(source.OrderNumber), CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}
}

func financeRefundRecord(source v1finance.RefundFact, orderID int64) orderport.HistoricalRefund {
	return orderport.HistoricalRefund{
		OrderID: orderport.ID(orderID), SourceRefundID: source.SourceID, RefundNumber: source.RefundNumber,
		ProviderRefundID: source.ProviderRefund, TransactionID: source.TransactionID, Status: source.Status,
		AmountMinor: source.AmountMinor, OrderAmountMinor: source.OrderAmount, Currency: source.Currency, Reason: source.Reason,
		CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
	}
}

func financeSourceFieldDigest(payload []byte, excluded ...string) ([sha256.Size]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return [sha256.Size]byte{}, ErrConflict
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return [sha256.Size]byte{}, ErrConflict
	}
	for _, field := range excluded {
		delete(fields, field)
	}
	canonical, err := json.Marshal(fields)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(append([]byte("v1-finance-source-field/v1\x00"), canonical...)), nil
}

func financeMappedOrderFieldDigest(source [sha256.Size]byte, customerID, productID *int64) [sha256.Size]byte {
	return financeMappedFieldDigest("orders", source, customerID, productID)
}

func financeMappedRefundFieldDigest(source [sha256.Size]byte, orderID int64) [sha256.Size]byte {
	return financeMappedFieldDigest("refunds", source, &orderID, nil)
}

func financeMappedFieldDigest(kind string, source [sha256.Size]byte, first, second *int64) [sha256.Size]byte {
	encoded, _ := json.Marshal(struct {
		Kind   string `json:"kind"`
		Source string `json:"source_field_digest"`
		First  *int64 `json:"first_target_id"`
		Second *int64 `json:"second_target_id"`
	}{Kind: kind, Source: fmt.Sprintf("%x", source), First: first, Second: second})
	return sha256.Sum256(append([]byte("v1-finance-mapped-field/v1\x00"), encoded...))
}

func invalidHistoricalReferenceID(value *int64) bool { return value != nil && *value < 1 }

func sameFinanceTerminal(found, want TerminalReceipt) bool {
	foundDigest, foundErr := receiptFieldDigest(found.Metadata)
	wantDigest, wantErr := receiptFieldDigest(want.Metadata)
	return foundErr == nil && wantErr == nil && found.SourceKeyDigest == want.SourceKeyDigest && found.PayloadDigest == want.PayloadDigest &&
		found.Disposition == want.Disposition && found.Reason == want.Reason && found.TargetID == want.TargetID && found.TargetDigest == want.TargetDigest && foundDigest == wantDigest
}

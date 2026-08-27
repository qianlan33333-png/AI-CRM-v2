package main

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

type servicePeriodSourceReference struct {
	row   v1archive.ArchivedRow
	value string
}

type servicePeriodReferenceResolver struct {
	finance                      *financeReferenceResolver
	products, orders             map[int64]servicePeriodSourceReference
	productJournal, orderJournal *v1domain.Journal
	orderReader                  orderport.HistoricalImportStore
}

func newServicePeriodReferenceResolver(ctx context.Context, archive v1domain.ArchiveSource, uow *platformstore.UnitOfWork, run string, dm01Run int64, key []byte) (*servicePeriodReferenceResolver, error) {
	if archive == nil {
		return nil, v1domain.ErrInvalidScope
	}
	finance, err := newFinanceReferenceResolver(ctx, uow, run, dm01Run, key)
	if err != nil {
		return nil, err
	}
	if err = uow.Within(ctx, func(ctx context.Context) error { return v1domain.VerifyServicePeriodFinancePrerequisite(ctx, run) }); err != nil {
		return nil, err
	}
	r := &servicePeriodReferenceResolver{finance: finance, products: map[int64]servicePeriodSourceReference{}, orders: map[int64]servicePeriodSourceReference{}, orderReader: orderstore.NewRepository()}
	r.productJournal, err = v1domain.NewJournal(v1domain.Scope{ImportVersion: staticImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/wechat_pay_products", TargetDomain: "product", TargetTable: "products"})
	if err != nil {
		return nil, err
	}
	r.orderJournal, err = v1domain.NewJournal(v1domain.Scope{ImportVersion: financeImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/wechat_pay_orders", TargetDomain: "order", TargetTable: "order_list_projections"})
	if err != nil {
		return nil, err
	}
	for _, table := range []struct {
		name, field string
		index       map[int64]servicePeriodSourceReference
	}{
		{"public/wechat_pay_products", "product_code", r.products}, {"public/wechat_pay_orders", "out_trade_no", r.orders},
	} {
		err = archive.EachTableRow(ctx, run, table.name, func(row v1archive.ArchivedRow) error {
			id, value, err := servicePeriodSourceReferenceFields(row, table.name, table.field)
			if err != nil {
				return err
			}
			if _, found := table.index[id]; found {
				return v1domain.ErrConflict
			}
			table.index[id] = servicePeriodSourceReference{row: row, value: value}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func servicePeriodSourceReferenceFields(row v1archive.ArchivedRow, table, field string) (int64, string, error) {
	var values map[string]json.RawMessage
	var id int64
	var value string
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || v1archive.IsRedacted(row, "id") || v1archive.IsRedacted(row, field) ||
		json.Unmarshal(row.Payload, &values) != nil || json.Unmarshal(values["id"], &id) != nil || id < 1 || json.Unmarshal(values[field], &value) != nil || value == "" {
		return 0, "", v1domain.ErrConflict
	}
	return id, value, nil
}

func servicePeriodReferenceTerminal(ctx context.Context, journal *v1domain.Journal, source servicePeriodSourceReference) (v1domain.TerminalReceipt, int64, error) {
	receipt, found, err := journal.LoadTerminal(ctx, v1domain.SourceIdentifier(source.row.SourceKeyHMAC))
	if err != nil {
		return receipt, 0, err
	}
	if !found {
		return receipt, 0, nil
	}
	if receipt.PayloadDigest != source.row.PayloadHMAC {
		return receipt, 0, v1domain.ErrConflict
	}
	if receipt.Disposition != "import" {
		return receipt, 0, nil
	}
	id, err := strconv.ParseInt(receipt.TargetID, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != receipt.TargetID {
		return receipt, 0, v1domain.ErrConflict
	}
	return receipt, id, nil
}

func (r *servicePeriodReferenceResolver) ResolveServicePeriodProduct(ctx context.Context, sourceID int64) (int64, error) {
	source, found := r.products[sourceID]
	if !found {
		return 0, nil
	}
	_, id, err := servicePeriodReferenceTerminal(ctx, r.productJournal, source)
	if err != nil || id == 0 {
		return 0, err
	}
	actual, err := r.finance.product(ctx, source.value)
	if err != nil {
		return 0, err
	}
	if actual == nil || *actual != id {
		return 0, v1domain.ErrConflict
	}
	return id, nil
}

func (r *servicePeriodReferenceResolver) ResolveServicePeriodCustomer(ctx context.Context, unionID string) (*int64, error) {
	if unionID == "" {
		return nil, nil
	}
	return r.finance.customer(ctx, unionID)
}

func (r *servicePeriodReferenceResolver) ResolveServicePeriodOrder(ctx context.Context, sourceID int64, outTradeNo string) (*int64, error) {
	source, found := r.orders[sourceID]
	if !found {
		return nil, nil
	}
	receipt, id, err := servicePeriodReferenceTerminal(ctx, r.orderJournal, source)
	if err != nil || id == 0 {
		return nil, err
	}
	order, err := r.orderReader.GetHistoricalOrder(ctx, orderport.ID(id))
	if err != nil {
		return nil, err
	}
	if !servicePeriodOrderReferenceMatches(order, receipt, source.value, outTradeNo) {
		return nil, v1domain.ErrConflict
	}
	return &id, nil
}

// Historical renewal/adjustment references may point to a different product.
// Preserve the proven source order identity; do not impose current sale rules.
func servicePeriodOrderReferenceMatches(order orderport.Record, receipt v1domain.TerminalReceipt, sourceNo, outTradeNo string) bool {
	return order.ID > 0 && strconv.FormatInt(int64(order.ID), 10) == receipt.TargetID && order.RecordOrigin == orderport.RecordOriginV1History &&
		order.MerchantOrderNo == sourceNo && (outTradeNo == "" || outTradeNo == sourceNo) &&
		orderapp.HistoricalOrderTargetDigest(order) == receipt.TargetDigest
}

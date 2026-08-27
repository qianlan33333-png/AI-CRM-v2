package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	couponapp "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/app"
	couponstore "github.com/qianlan33333-png/AI-CRM-v2/internal/coupon/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func importCoupon(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actor, dm01RunID int64, key []byte) (v1domain.CouponImportResult, error) {
	references, err := newServicePeriodReferenceResolver(ctx, archive, uow, runID, dm01RunID, key)
	if err != nil {
		return v1domain.CouponImportResult{}, err
	}
	journals := map[string]*v1domain.Journal{}
	for kind, tables := range map[string][2]string{
		"definitions": {"public/commerce_coupons", "coupons"},
		"bindings":    {"public/commerce_coupon_product_bindings", "coupon_targets"},
		"claims":      {"public/commerce_coupon_claims", "coupon_v1_history_claims"},
		"redemptions": {"public/commerce_coupon_redemptions", "coupon_v1_history_redemptions"},
	} {
		journals[kind], err = v1domain.NewJournal(v1domain.Scope{ImportVersion: couponImportVersion, ArchiveRunID: runID,
			AdapterID: v1archive.DefaultAdapterID, TableID: tables[0], TargetDomain: "coupon", TargetTable: tables[1]})
		if err != nil {
			return v1domain.CouponImportResult{}, err
		}
	}
	journal, err := v1domain.NewCouponHistoryJournal(journals["definitions"], journals["claims"], journals["redemptions"])
	if err != nil {
		return v1domain.CouponImportResult{}, err
	}
	writer, err := couponapp.NewHistoricalWriter(couponstore.NewRepository(), journal)
	if err != nil {
		return v1domain.CouponImportResult{}, err
	}
	importer, err := v1domain.NewCouponImporter(archive, uow, writer, &couponReferenceResolver{references}, journals, actor)
	if err != nil {
		return v1domain.CouponImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

// Reuse the verified source-ID/receipt mapping; never treat a V1 ID as a V2 FK.
type couponReferenceResolver struct {
	references *servicePeriodReferenceResolver
}

func (r *couponReferenceResolver) ResolveCouponProduct(ctx context.Context, sourceID, discount int64) (int64, error) {
	id, err := r.references.ResolveServicePeriodProduct(ctx, sourceID)
	if err != nil || id == 0 {
		return 0, err
	}
	product, err := r.references.finance.products.ReadProduct(ctx, productport.ID(id))
	if err != nil {
		return 0, err
	}
	if !historicalCouponProductEligible(product, discount) {
		return 0, nil
	}
	return id, nil
}

func historicalCouponProductEligible(product productport.Product, discount int64) bool {
	return product.ID > 0 && product.Currency == "CNY" && discount > 0 && discount < product.PriceMinor
}

func (r *couponReferenceResolver) ResolveCouponCustomer(ctx context.Context, unionID string) (*int64, error) {
	return r.references.ResolveServicePeriodCustomer(ctx, unionID)
}

func (r *couponReferenceResolver) ResolveCouponOrder(ctx context.Context, sourceID int64, outTradeNo string) (*int64, error) {
	return r.references.ResolveServicePeriodOrder(ctx, sourceID, outTradeNo)
}

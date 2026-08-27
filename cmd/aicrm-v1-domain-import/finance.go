package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderapp "github.com/qianlan33333-png/AI-CRM-v2/internal/order/app"
	orderstore "github.com/qianlan33333-png/AI-CRM-v2/internal/order/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func importFinance(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, dm01RunID int64, key []byte) (v1domain.FinanceImportResult, error) {
	resolver, err := newFinanceReferenceResolver(ctx, uow, runID, dm01RunID, key)
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	orders, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: financeImportVersion, ArchiveRunID: runID,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/wechat_pay_orders", TargetDomain: "order", TargetTable: "order_list_projections"})
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	refunds, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: financeImportVersion, ArchiveRunID: runID,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/wechat_pay_refunds", TargetDomain: "order", TargetTable: "order_historical_refunds"})
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	journal, err := v1domain.NewFinanceJournal(orders, refunds)
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	writer, err := orderapp.NewHistoricalImportService(uow, orderstore.NewRepository(), journal)
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	importer, err := v1domain.NewFinanceImporter(archive, uow, writer, journal, resolver)
	if err != nil {
		return v1domain.FinanceImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

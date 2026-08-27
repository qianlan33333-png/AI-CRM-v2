package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

func importServicePeriod(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, dm01RunID int64, key []byte) (v1domain.ServicePeriodImportResult, error) {
	resolver, err := newServicePeriodReferenceResolver(ctx, archive, uow, runID, dm01RunID, key)
	if err != nil {
		return v1domain.ServicePeriodImportResult{}, err
	}
	journals := map[string]*v1domain.Journal{}
	for source, target := range map[string]string{
		"public/service_period_products":     "product_service_period_history",
		"public/service_period_entitlements": "product_service_period_entitlement_history",
		"public/service_period_events":       "product_service_period_event_history",
	} {
		journals[source], err = v1domain.NewJournal(v1domain.Scope{ImportVersion: servicePeriodImportVersion, ArchiveRunID: runID,
			AdapterID: v1archive.DefaultAdapterID, TableID: source, TargetDomain: "product", TargetTable: target})
		if err != nil {
			return v1domain.ServicePeriodImportResult{}, err
		}
	}
	journal, err := v1domain.NewServicePeriodHistoryJournal(journals["public/service_period_products"], journals["public/service_period_entitlements"], journals["public/service_period_events"])
	if err != nil {
		return v1domain.ServicePeriodImportResult{}, err
	}
	writer, err := productapp.NewServicePeriodHistoryWriter(productstore.NewServicePeriodHistoryStore(), journal)
	if err != nil {
		return v1domain.ServicePeriodImportResult{}, err
	}
	importer, err := v1domain.NewServicePeriodImporter(archive, uow, writer, resolver, journals)
	if err != nil {
		return v1domain.ServicePeriodImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

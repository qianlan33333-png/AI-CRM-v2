package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

type finalEditableProjectionResult struct {
	Products         v1domain.EditableProductProjectionResult             `json:"products"`
	ServicePeriods   productstore.HistoricalServicePeriodProjectionResult `json:"service_periods"`
	ProductMaterials productstore.HistoricalProductMaterialClearResult    `json:"product_materials"`
	Audiences        segmentapp.AudienceEditableProjectionResult          `json:"audiences"`
	AutomationAgents automationstore.V1EditableAgentProjectionResult      `json:"automation_agents"`
}

func projectFinalEditableBusiness(ctx context.Context, pool *pgxpool.Pool, archive *v1archive.PostgresArchiveReader, archiveRunID string, actorID int64, at time.Time) (finalEditableProjectionResult, error) {
	if ctx == nil || pool == nil || archive == nil || archiveRunID == "" || actorID < 1 || at.IsZero() || at.Location() != time.UTC {
		return finalEditableProjectionResult{}, fmt.Errorf("invalid final editable projection scope")
	}
	preflight, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return finalEditableProjectionResult{}, err
	}
	defer preflight.Rollback(ctx) //nolint:errcheck
	if err = verifyReconciledArchive(ctx, preflight, archiveRunID); err != nil {
		return finalEditableProjectionResult{}, err
	}
	for _, version := range []string{staticImportVersion, servicePeriodImportVersion, v1domain.AudienceHistoryImportVersion, staticTailHistoryImportVersion, automationHistoryImportVersion} {
		if _, err = loadReconciliationCounts(ctx, preflight, archiveRunID, version); err != nil {
			return finalEditableProjectionResult{}, fmt.Errorf("editable projection requires reconciled scope %s: %w", version, err)
		}
	}
	if err = preflight.Commit(ctx); err != nil {
		return finalEditableProjectionResult{}, err
	}
	uow := platformstore.NewUnitOfWork(pool)
	journal, err := newStaticJournal(archiveRunID, "public/wechat_pay_products", "product", "products")
	if err != nil {
		return finalEditableProjectionResult{}, err
	}
	writer, err := product.NewHistoricalStaticProductWriter(productstore.NewHistoricalStaticProductStore(), journal)
	if err != nil {
		return finalEditableProjectionResult{}, err
	}
	importer, err := v1domain.NewProductImporter(archive, uow, writer, journal, actorID)
	if err != nil {
		return finalEditableProjectionResult{}, err
	}
	result := finalEditableProjectionResult{}
	if result.Products, err = importer.ProjectEditable(ctx, archiveRunID, at); err != nil {
		return finalEditableProjectionResult{}, err
	}
	if err = uow.Within(ctx, func(bound context.Context) error {
		result.ServicePeriods, err = productstore.NewHistoricalStaticProductStore().ProjectHistoricalServicePeriodProducts(bound, at)
		return err
	}); err != nil {
		return finalEditableProjectionResult{}, err
	}
	if err = uow.Within(ctx, func(bound context.Context) error {
		result.ProductMaterials, err = productstore.NewHistoricalStaticProductStore().ClearHistoricalEditableProductMaterials(bound, at)
		return err
	}); err != nil {
		return finalEditableProjectionResult{}, err
	}
	projector := segmentapp.NewAudienceEditableProjectionService(uow, segmentstore.NewAudienceEditableProjectionStore())
	if result.Audiences, err = projector.Project(ctx, actorID, at); err != nil {
		return finalEditableProjectionResult{}, err
	}
	editableAgents, err := loadEditableAutomationAgents(ctx, archive, uow, archiveRunID)
	if err != nil {
		return finalEditableProjectionResult{}, err
	}
	seeds := make([]automationstore.V1EditableAgentProjection, len(editableAgents))
	for index, item := range editableAgents {
		seeds[index] = automationstore.V1EditableAgentProjection{
			SourceAgentID: item.SourceAgentID, SourceConfigID: item.SourceConfigID, SourcePromptID: item.SourcePromptID,
			AgentName: item.AgentName, AgentCode: item.AgentCode,
			DraftRolePrompt: item.DraftRole, DraftTaskPrompt: item.DraftTask,
			PublishedRolePrompt: item.PublishedRole, PublishedTaskPrompt: item.PublishedTask,
			DraftVersion: item.DraftVersion, PublishedVersion: item.PublishedVersion,
			LegacyConfiguration: item.LegacyConfiguration, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		}
	}
	if err = uow.Within(ctx, func(bound context.Context) error {
		result.AutomationAgents, err = automationstore.ProjectV1EditableAgents(bound, seeds, actorID, at)
		return err
	}); err != nil {
		return finalEditableProjectionResult{}, err
	}
	return result, nil
}

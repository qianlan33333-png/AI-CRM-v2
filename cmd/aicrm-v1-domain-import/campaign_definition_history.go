package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const campaignDefinitionHistoryDomain = "campaign-definition-history"
const campaignDefinitionHistoryImportVersion = "v1-campaign-definition-history-a1"

func importCampaignDefinitionHistory(ctx context.Context, pool *pgxpool.Pool, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, sourceKey []byte) (v1domain.CampaignDefinitionHistoryImportResult, error) {
	definitions, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: campaignDefinitionHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/campaigns", TargetDomain: "campaign", TargetTable: "campaign_v1_definition_history"})
	if err != nil {
		return v1domain.CampaignDefinitionHistoryImportResult{}, err
	}
	steps, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: campaignDefinitionHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/campaign_steps", TargetDomain: "campaign", TargetTable: "campaign_v1_definition_step_history"})
	if err != nil {
		return v1domain.CampaignDefinitionHistoryImportResult{}, err
	}
	journal, err := v1domain.NewCampaignDefinitionHistoryJournal(definitions, steps)
	if err != nil {
		return v1domain.CampaignDefinitionHistoryImportResult{}, err
	}
	selector, err := v1domain.NewCampaignDefinitionSelector(archive, v1domain.NewCampaignDefinitionReceiptReader(pool))
	if err != nil {
		return v1domain.CampaignDefinitionHistoryImportResult{}, err
	}
	writer := campaignapp.NewCampaignDefinitionHistoryWriter(campaignstore.NewCampaignDefinitionHistoryStore(), journal)
	parent := v1domain.NewCampaignDefinitionCurrentResolver(run, campaignstore.NewCampaignDefinitionHistoryReader(pool))
	importer, err := v1domain.NewCampaignDefinitionHistoryImporter(selector, uow, writer, parent, journal, sourceKey)
	if err != nil {
		return v1domain.CampaignDefinitionHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const campaignHistoryImportVersion = "v1-campaign-history-a1"

func importCampaignHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, dm01RunID int64, key []byte) (v1domain.CampaignHistoryImportResult, error) {
	references, err := newChannelCustomerResolver(ctx, uow, dm01RunID, key)
	if err != nil {
		return v1domain.CampaignHistoryImportResult{}, err
	}
	journals, err := newCampaignHistoryJournals(runID)
	if err != nil {
		return v1domain.CampaignHistoryImportResult{}, err
	}
	journal, err := v1domain.NewCampaignHistoryJournal(journals["public/campaign_segments"], journals["public/campaign_members"], journals["public/cloud_broadcast_plans"], journals["public/cloud_broadcast_plan_recipients"], journals["public/cloud_broadcast_plan_recipient_messages"])
	if err != nil {
		return v1domain.CampaignHistoryImportResult{}, err
	}
	writer := campaignapp.NewCampaignHistoryWriter(campaignstore.NewCampaignHistoryStore(), journal)
	importer, err := v1domain.NewCampaignHistoryImporter(archive, uow, writer, &campaignHistoryCustomerResolver{references}, journals)
	if err != nil {
		return v1domain.CampaignHistoryImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func newCampaignHistoryJournals(runID string) (map[string]*v1domain.Journal, error) {
	journals := make(map[string]*v1domain.Journal, 5)
	for source, target := range map[string]string{
		"public/campaign_segments":                       "campaign_v1_history_segments",
		"public/campaign_members":                        "campaign_v1_history_members",
		"public/cloud_broadcast_plans":                   "campaign_v1_history_broadcast_plans",
		"public/cloud_broadcast_plan_recipients":         "campaign_v1_history_broadcast_recipients",
		"public/cloud_broadcast_plan_recipient_messages": "campaign_v1_history_broadcast_messages",
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: campaignHistoryImportVersion, ArchiveRunID: runID,
			AdapterID: v1archive.DefaultAdapterID, TableID: source, TargetDomain: "campaign", TargetTable: target})
		if err != nil {
			return nil, err
		}
		journals[source] = journal
	}
	return journals, nil
}

// Reuse the caller-bound DM01 customer proof; no source ID becomes a V2 FK.
type campaignHistoryCustomerResolver struct{ references *channelCustomerResolver }

func (r *campaignHistoryCustomerResolver) ResolveHistoricalCampaignCustomer(ctx context.Context, unionID string) (*int64, error) {
	return r.references.ResolveHistoricalChannelCustomer(ctx, unionID)
}

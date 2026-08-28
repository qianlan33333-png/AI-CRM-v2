package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const marketingStateHistoryImportVersion = "v1-marketing-state-history-a1"

func newMarketingStateHistoryJournal(run string) (*v1domain.MarketingStateHistoryJournal, error) {
	var journals [4]*v1domain.Journal
	for index, mapping := range [][2]string{
		{"public/customer_marketing_state_current", "segment_v1_marketing_state_snapshots"},
		{"public/customer_marketing_state_history", "segment_v1_marketing_state_changes"},
		{"public/customer_value_segment_current", "segment_v1_value_segment_snapshots"},
		{"public/customer_value_segment_history", "segment_v1_value_segment_changes"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: marketingStateHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "segment", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[index] = journal
	}
	return v1domain.NewMarketingStateHistoryJournal(journals[0], journals[1], journals[2], journals[3])
}

func importMarketingStateHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.MarketingStateHistoryImportResult, error) {
	journal, err := newMarketingStateHistoryJournal(run)
	if err != nil {
		return v1domain.MarketingStateHistoryImportResult{}, err
	}
	writer, err := segmentapp.NewMarketingStateHistoryWriter(segmentstore.NewMarketingStateHistoryStore(), journal)
	if err != nil {
		return v1domain.MarketingStateHistoryImportResult{}, err
	}
	importer, err := v1domain.NewMarketingStateHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return v1domain.MarketingStateHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

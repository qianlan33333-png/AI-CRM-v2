package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const legacyMarketingHistoryImportVersion = "v1-legacy-marketing-history-a1"

func newLegacyMarketingHistoryJournal(run string) (*v1domain.LegacyMarketingHistoryJournal, error) {
	var journals [2]*v1domain.Journal
	for index, mapping := range [][2]string{
		{"public/marketing_state_current", "segment_v1_legacy_marketing_states"},
		{"public/marketing_value_segment_current", "segment_v1_legacy_marketing_values"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: legacyMarketingHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "segment", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[index] = journal
	}
	return v1domain.NewLegacyMarketingHistoryJournal(journals[0], journals[1])
}

func importLegacyMarketingHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, sourceHMACKey []byte) (v1domain.LegacyMarketingHistoryImportResult, error) {
	journal, err := newLegacyMarketingHistoryJournal(run)
	if err != nil {
		return v1domain.LegacyMarketingHistoryImportResult{}, err
	}
	writer, err := segmentapp.NewLegacyMarketingHistoryWriter(segmentstore.NewLegacyMarketingHistoryStore(), journal)
	if err != nil {
		return v1domain.LegacyMarketingHistoryImportResult{}, err
	}
	importer, err := v1domain.NewLegacyMarketingHistoryImporter(archive, uow, writer, journal, sourceHMACKey)
	if err != nil {
		return v1domain.LegacyMarketingHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

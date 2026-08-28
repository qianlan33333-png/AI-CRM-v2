package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationstore "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	radarapp "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	radarstore "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
)

func importRadarClickHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, dm01Key, sourceKey []byte) (v1domain.RadarClickHistoryImportResult, error) {
	journal, err := newRadarClickHistoryJournal(run)
	if err != nil {
		return v1domain.RadarClickHistoryImportResult{}, err
	}
	writer, err := radarapp.NewRadarClickHistoryWriter(radarstore.NewRadarClickHistoryStore(), journal)
	if err != nil {
		return v1domain.RadarClickHistoryImportResult{}, err
	}
	refs, err := newRadarClickHistoryReferences(ctx, uow, run, dm01Run, dm01Key, sourceKey, radarstore.NewPostgresRepository())
	if err != nil {
		return v1domain.RadarClickHistoryImportResult{}, err
	}
	importer, err := v1domain.NewRadarClickHistoryImporter(archive, uow, writer, refs, journal)
	if err != nil {
		return v1domain.RadarClickHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func importMarketingConfigHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.MarketingConfigHistoryImportResult, error) {
	journal, err := newMarketingConfigHistoryJournal(run)
	if err != nil {
		return v1domain.MarketingConfigHistoryImportResult{}, err
	}
	writer, err := automationapp.NewMarketingConfigHistoryWriter(automationstore.NewMarketingConfigHistoryStore(), journal)
	if err != nil {
		return v1domain.MarketingConfigHistoryImportResult{}, err
	}
	importer, err := v1domain.NewMarketingConfigHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return v1domain.MarketingConfigHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

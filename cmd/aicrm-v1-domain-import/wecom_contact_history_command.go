package main

import (
	"context"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func importWeComContactHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string) (WeComContactHistoryImportResult, error) {
	journal, err := newWeComContactHistoryJournal(runID)
	if err != nil {
		return WeComContactHistoryImportResult{}, err
	}
	writer, err := contactapp.NewWeComContactHistoryWriter(contactstore.NewWeComContactHistoryStore(), journal)
	if err != nil {
		return WeComContactHistoryImportResult{}, err
	}
	importer, err := NewWeComContactHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return WeComContactHistoryImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

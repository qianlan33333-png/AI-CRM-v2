package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const broadcastJobHistoryImportVersion = "v1-broadcast-job-history-a1"

func importBroadcastJobHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.BroadcastJobHistoryImportResult, error) {
	journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: broadcastJobHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/broadcast_jobs", TargetDomain: "outbound", TargetTable: "outbound_v1_broadcast_job_history"})
	if err != nil {
		return v1domain.BroadcastJobHistoryImportResult{}, err
	}
	writer := outboundapp.NewBroadcastJobHistoryWriter(outboundstore.NewBroadcastJobHistoryStore(), journal)
	importer, err := v1domain.NewBroadcastJobHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return v1domain.BroadcastJobHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

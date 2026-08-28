package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const outboundTaskHistoryImportVersion = "v1-outbound-task-history-a1"

func importOutboundTaskHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, sourceHMACKey []byte) (v1domain.OutboundTaskHistoryImportResult, error) {
	if err := uow.Within(ctx, func(tx context.Context) error {
		return v1domain.VerifyOutboundTaskHistoryPrerequisite(tx, run)
	}); err != nil {
		return v1domain.OutboundTaskHistoryImportResult{}, err
	}
	journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: outboundTaskHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: "public/outbound_tasks", TargetDomain: "outbound", TargetTable: "outbound_v1_task_history"})
	if err != nil {
		return v1domain.OutboundTaskHistoryImportResult{}, err
	}
	writer := outboundapp.NewOutboundTaskHistoryWriter(outboundstore.NewOutboundTaskHistoryStore(), journal)
	importer, err := v1domain.NewOutboundTaskHistoryImporter(archive, uow, writer, journal, sourceHMACKey)
	if err != nil {
		return v1domain.OutboundTaskHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

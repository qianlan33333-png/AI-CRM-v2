package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

const messageHistoryImportVersion = "v1-message-history-a1"

type messageHistoryReferences struct{ customer *channelCustomerResolver }

func (r *messageHistoryReferences) ResolveHistoricalMessageCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return r.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}

func importMessageHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, key []byte) (v1domain.MessageHistoryImportResult, error) {
	resolver, err := newChannelCustomerResolver(ctx, uow, dm01Run, key)
	if err != nil {
		return v1domain.MessageHistoryImportResult{}, err
	}
	journal, err := newMessageHistoryJournal(run)
	if err != nil {
		return v1domain.MessageHistoryImportResult{}, err
	}
	writer := wecomapp.NewMessageHistoryWriter(wecomstore.NewMessageHistoryStore(), journal)
	importer, err := v1domain.NewMessageHistoryImporter(archive, uow, writer, &messageHistoryReferences{customer: resolver}, journal)
	if err != nil {
		return v1domain.MessageHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func newMessageHistoryJournal(run string) (*v1domain.Journal, error) {
	return v1domain.NewJournal(v1domain.Scope{ImportVersion: messageHistoryImportVersion, ArchiveRunID: run,
		AdapterID: v1archive.DefaultAdapterID, TableID: "public/archived_messages", TargetDomain: "wecom", TargetTable: "wecom_v1_message_history"})
}

package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const contactHistoryImportVersion = "v1-contact-history-a1"

type contactHistoryReferences struct{ customer *channelCustomerResolver }

func (r *contactHistoryReferences) ResolveHistoricalContactCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return r.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}

func importContactHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, key []byte) (v1domain.ContactHistoryImportResult, error) {
	resolver, err := newChannelCustomerResolver(ctx, uow, dm01Run, key)
	if err != nil {
		return v1domain.ContactHistoryImportResult{}, err
	}
	journal, err := newContactHistoryJournal(run)
	if err != nil {
		return v1domain.ContactHistoryImportResult{}, err
	}
	writer := contactapp.NewContactHistoryWriter(contactstore.NewContactHistoryStore(), journal)
	importer, err := v1domain.NewContactHistoryImporter(archive, uow, writer, &contactHistoryReferences{customer: resolver}, journal)
	if err != nil {
		return v1domain.ContactHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func newContactHistoryJournal(run string) (*v1domain.ContactHistoryJournal, error) {
	var journals [4]*v1domain.Journal
	for i, mapping := range [][2]string{
		{"public/sidebar_customer_profile_fields", "contact_v1_sidebar_profile_history"},
		{"public/owner_migration_results", "contact_v1_owner_migration_result_history"},
		{"public/owner_migration_import_sessions", "contact_v1_owner_migration_context_archive"},
		{"public/owner_migration_previews", "contact_v1_owner_migration_context_archive"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: contactHistoryImportVersion, ArchiveRunID: run,
			AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "contact", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[i] = journal
	}
	return v1domain.NewContactHistoryJournal(journals[0], journals[1], journals[2], journals[3])
}

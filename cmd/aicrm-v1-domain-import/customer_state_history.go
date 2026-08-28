package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const customerStateHistoryImportVersion = "v1-customer-state-history-a1"

func newCustomerStateHistoryJournal(run string) (*v1domain.CustomerStateHistoryJournal, error) {
	var journals [3]*v1domain.Journal
	for index, mapping := range [][2]string{
		{"public/class_user_status_current", "contact_v1_customer_status_snapshots"},
		{"public/class_user_status_history", "contact_v1_customer_status_changes"},
		{"public/class_term_tag_mapping", "contact_v1_class_term_tag_history"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: customerStateHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "contact", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[index] = journal
	}
	return v1domain.NewCustomerStateHistoryJournal(journals[0], journals[1], journals[2])
}

func importCustomerStateHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.CustomerStateHistoryImportResult, error) {
	journal, err := newCustomerStateHistoryJournal(run)
	if err != nil {
		return v1domain.CustomerStateHistoryImportResult{}, err
	}
	writer, err := contactapp.NewCustomerStateHistoryWriter(contactstore.NewCustomerStateHistoryStore(), journal)
	if err != nil {
		return v1domain.CustomerStateHistoryImportResult{}, err
	}
	importer, err := v1domain.NewCustomerStateHistoryImporter(archive, uow, writer, journal)
	if err != nil {
		return v1domain.CustomerStateHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

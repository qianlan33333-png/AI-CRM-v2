package main

import (
	"context"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const profileCatalogHistoryImportVersion = "v1-profile-catalog-history-a1"

func newProfileCatalogHistoryJournals(run string) (map[string]*v1domain.Journal, error) {
	journals := map[string]*v1domain.Journal{}
	for _, spec := range []struct{ source, domain, target string }{
		{v1profilecatalog.ProfileTemplatesTableID, "segment", v1profilecatalog.ProfileTemplatesTargetTable},
		{v1profilecatalog.ProfileCategoriesTableID, "segment", v1profilecatalog.ProfileCategoriesTargetTable},
		{v1profilecatalog.ProfileOptionMappingsTableID, "segment", v1profilecatalog.ProfileOptionMappingsTargetTable},
		{v1profilecatalog.SignupTagRulesTableID, "contact", v1profilecatalog.SignupTagRulesTargetTable},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: profileCatalogHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: spec.source, TargetDomain: spec.domain, TargetTable: spec.target})
		if err != nil {
			return nil, err
		}
		journals[spec.source] = journal
	}
	return journals, nil
}
func importProfileCatalogHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string) (v1domain.ProfileCatalogHistoryImportResult, error) {
	journals, err := newProfileCatalogHistoryJournals(run)
	if err != nil {
		return v1domain.ProfileCatalogHistoryImportResult{}, err
	}
	profiles, err := v1domain.NewProfileCatalogHistoryJournal(journals[v1profilecatalog.ProfileTemplatesTableID], journals[v1profilecatalog.ProfileCategoriesTableID], journals[v1profilecatalog.ProfileOptionMappingsTableID])
	if err != nil {
		return v1domain.ProfileCatalogHistoryImportResult{}, err
	}
	tags, err := v1domain.NewSignupTagHistoryJournal(journals[v1profilecatalog.SignupTagRulesTableID])
	if err != nil {
		return v1domain.ProfileCatalogHistoryImportResult{}, err
	}
	writer, err := v1profilecatalog.NewWriter(segmentapp.NewProfileCatalogHistoryService(segmentstore.NewProfileCatalogHistoryStore(), profiles), contactapp.NewSignupTagHistoryService(contactstore.NewSignupTagHistoryStore(), tags))
	if err != nil {
		return v1domain.ProfileCatalogHistoryImportResult{}, err
	}
	importer, err := v1domain.NewProfileCatalogHistoryImporter(archive, uow, writer, profileCatalogHistoryTxReader{}, journals)
	if err != nil {
		return v1domain.ProfileCatalogHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

// These reads see the same caller transaction as the owner writes and receipts.
type profileCatalogHistoryTxReader struct{}

func (profileCatalogHistoryTxReader) ReadTemplate(ctx context.Context, id int64) (segmentport.HistoricalProfileTemplate, error) {
	return segmentstore.NewProfileCatalogHistoryStore().GetHistoricalProfileTemplate(ctx, id)
}
func (profileCatalogHistoryTxReader) ReadCategory(ctx context.Context, id int64) (segmentport.HistoricalProfileCategory, error) {
	return segmentstore.NewProfileCatalogHistoryStore().GetHistoricalProfileCategory(ctx, id)
}
func (profileCatalogHistoryTxReader) ReadOptionMapping(ctx context.Context, id int64) (segmentport.HistoricalProfileOptionMapping, error) {
	return segmentstore.NewProfileCatalogHistoryStore().GetHistoricalProfileOptionMapping(ctx, id)
}
func (profileCatalogHistoryTxReader) ReadSignupTagRule(ctx context.Context, id int64) (contactport.HistoricalSignupTagRule, error) {
	return contactstore.NewSignupTagHistoryStore().GetHistoricalSignupTagRule(ctx, id)
}

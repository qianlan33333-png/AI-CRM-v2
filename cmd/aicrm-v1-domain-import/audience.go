package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

func importAudienceHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, actor, dm01Run int64, key []byte) (v1domain.AudienceHistoryImportResult, error) {
	resolver, err := newAudienceHistoryReferences(ctx, uow, dm01Run, key)
	if err != nil {
		return v1domain.AudienceHistoryImportResult{}, err
	}
	journals, err := newAudienceHistoryJournals(run)
	if err != nil {
		return v1domain.AudienceHistoryImportResult{}, err
	}
	journal, err := v1domain.NewAudienceHistoryJournal(journals)
	if err != nil {
		return v1domain.AudienceHistoryImportResult{}, err
	}
	writer := segmentapp.NewAudienceHistoryWriter(segmentstore.NewAudienceHistoryStore(), journal)
	importer, err := v1domain.NewAudienceHistoryImporter(archive, uow, writer, resolver, journals, actor)
	if err != nil {
		return v1domain.AudienceHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

func newAudienceHistoryJournals(run string) (map[string]*v1domain.Journal, error) {
	journals := make(map[string]*v1domain.Journal, 8)
	for _, spec := range []struct{ source, target string }{
		{v1audiencehistory.PackageGroupsTableID, "segment_v1_audience_groups"},
		{v1audiencehistory.PackagesTableID, "segment_v1_audience_packages"},
		{v1audiencehistory.PackageVersionsTableID, "segment_v1_audience_versions"},
		{v1audiencehistory.PackageSendersTableID, "segment_v1_audience_senders"},
		{v1audiencehistory.RulesTableID, "segment_v1_audience_rules"},
		{v1audiencehistory.RuleVersionsTableID, "segment_v1_audience_rule_versions"},
		{v1audiencehistory.SegmentsTableID, "segment_v1_definitions"},
		{v1audiencehistory.AudienceMembersTableID, "segment_v1_audience_members"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: v1domain.AudienceHistoryImportVersion,
			ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: spec.source, TargetDomain: "segment", TargetTable: spec.target})
		if err != nil {
			return nil, err
		}
		journals[spec.source] = journal
	}
	return journals, nil
}

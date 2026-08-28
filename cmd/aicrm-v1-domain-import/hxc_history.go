package main

import (
	"context"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcstore "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const hxcHistoryImportVersion = "v1-hxc-history-a1"

type hxcHistoryReferences struct{ customer *channelCustomerResolver }

func (r *hxcHistoryReferences) ResolveHXCHistoryCustomer(ctx context.Context, unionID string) (*int64, error) {
	if r == nil || r.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return r.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}
func newHXCHistoryJournal(run string) (*v1domain.HXCHistoryJournal, error) {
	var journals [8]*v1domain.Journal
	for i, mapping := range [][2]string{
		{v1hxchistory.DashboardMetaTableID, "hxc_v1_dashboard_refresh_history"},
		{v1hxchistory.DashboardSnapshotTableID, "hxc_v1_dashboard_observations"},
		{v1hxchistory.ActivationStatusTableID, "hxc_v1_activation_observations"},
		{v1hxchistory.HuangxiaocanActivationID, "hxc_v1_activation_observations"},
		{v1hxchistory.ExperienceLeadsTableID, "hxc_v1_experience_lead_history"},
		{v1hxchistory.ImportBatchesTableID, "hxc_v1_import_batch_history"},
		{v1hxchistory.SendRecordsTableID, "hxc_v1_runtime_archive"},
		{v1hxchistory.SendConfigTableID, "hxc_v1_runtime_archive"},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{ImportVersion: hxcHistoryImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: mapping[0], TargetDomain: "hxc", TargetTable: mapping[1]})
		if err != nil {
			return nil, err
		}
		journals[i] = journal
	}
	return v1domain.NewHXCHistoryJournal(journals[0], journals[1], journals[2], journals[3], journals[4], journals[5], journals[6], journals[7])
}
func importHXCHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, key []byte) (v1domain.HXCHistoryImportResult, error) {
	resolver, err := newChannelCustomerResolver(ctx, uow, dm01Run, key)
	if err != nil {
		return v1domain.HXCHistoryImportResult{}, err
	}
	journal, err := newHXCHistoryJournal(run)
	if err != nil {
		return v1domain.HXCHistoryImportResult{}, err
	}
	writer, err := hxcapp.NewHXCHistoryWriter(hxcstore.NewHXCHistoryStore(), journal)
	if err != nil {
		return v1domain.HXCHistoryImportResult{}, err
	}
	importer, err := v1domain.NewHXCHistoryImporter(archive, uow, writer, &hxcHistoryReferences{customer: resolver}, journal)
	if err != nil {
		return v1domain.HXCHistoryImportResult{}, err
	}
	return importer.Import(ctx, run)
}

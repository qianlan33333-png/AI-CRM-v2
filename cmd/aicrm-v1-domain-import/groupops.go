package main

import (
	"context"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1groupops"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const (
	groupOpsHistoryImportVersion = "v1-groupops-a1"
	groupOpsHistoryDomain        = "groupops"
	groupOpsRuntimeArchiveTarget = "group_ops_v1_runtime_archive"
)

func importGroupOps(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, runID string, actorID, dm01RunID int64, dm01Key []byte) (v1domain.GroupOpsImportResult, error) {
	resolver, err := newGroupOpsStaffResolver(ctx, uow, dm01RunID, dm01Key)
	if err != nil {
		return v1domain.GroupOpsImportResult{}, err
	}
	journals, err := newGroupOpsJournals(runID)
	if err != nil {
		return v1domain.GroupOpsImportResult{}, err
	}
	historyJournal, err := v1domain.NewGroupOpsHistoryJournal(
		journals[v1groupops.PlansTableID],
		journals[v1groupops.GroupChatsTableID],
		journals[v1groupops.GroupSnapshotsTableID],
		journals[v1groupops.PlanGroupsTableID],
		journals[v1groupops.PlanNodesTableID],
	)
	if err != nil {
		return v1domain.GroupOpsImportResult{}, err
	}
	writer, err := groupopsapp.NewHistoricalWriter(groupopsstore.NewRepository(), historyJournal)
	if err != nil {
		return v1domain.GroupOpsImportResult{}, err
	}
	importer, err := v1domain.NewGroupOpsImporter(archive, uow, writer, resolver, journals, actorID)
	if err != nil {
		return v1domain.GroupOpsImportResult{}, err
	}
	return importer.Import(ctx, runID)
}

func newGroupOpsJournals(runID string) (map[string]*v1domain.Journal, error) {
	values := make(map[string]*v1domain.Journal, 11)
	for _, spec := range []struct {
		table, target string
	}{
		{v1groupops.PlansTableID, "group_ops_plans"},
		{v1groupops.GroupChatsTableID, "group_ops_v1_history_directory"},
		{v1groupops.GroupSnapshotsTableID, "group_ops_v1_history_directory"},
		{v1groupops.PlanGroupsTableID, "group_ops_v1_history_groups"},
		{v1groupops.PlanNodesTableID, "group_ops_v1_history_nodes"},
		{"public/automation_group_ops_effect_dependency", groupOpsRuntimeArchiveTarget},
		{"public/automation_group_ops_effect_graph", groupOpsRuntimeArchiveTarget},
		{"public/automation_group_ops_effect_material", groupOpsRuntimeArchiveTarget},
		{"public/automation_group_ops_execution_log", groupOpsRuntimeArchiveTarget},
		{"public/automation_group_ops_trigger_event", groupOpsRuntimeArchiveTarget},
		{"public/automation_group_ops_webhook_events", groupOpsRuntimeArchiveTarget},
	} {
		journal, err := v1domain.NewJournal(v1domain.Scope{
			ImportVersion: groupOpsHistoryImportVersion,
			ArchiveRunID:  runID,
			AdapterID:     v1archive.DefaultAdapterID,
			TableID:       spec.table,
			TargetDomain:  groupOpsHistoryDomain,
			TargetTable:   spec.target,
		})
		if err != nil {
			return nil, err
		}
		values[spec.table] = journal
	}
	return values, nil
}

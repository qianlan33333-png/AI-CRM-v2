package v1domain

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestGroupOpsReconcileTablePolicyIsClosed(t *testing.T) {
	if len(groupOpsReconciledTables) != 11 {
		t.Fatalf("table count = %d", len(groupOpsReconciledTables))
	}
	for _, tableID := range []string{groupOpsPlansTable, groupOpsGroupChatsTable, groupOpsSnapshotsTable, groupOpsGroupsTable, groupOpsNodesTable} {
		for _, disposition := range []string{"import", "quarantine"} {
			if err := validateGroupOpsDisposition(tableID, disposition); err != nil {
				t.Fatalf("formal %s/%s = %v", tableID, disposition, err)
			}
		}
		if err := validateGroupOpsDisposition(tableID, "archive"); !errors.Is(err, ErrConflict) {
			t.Fatalf("formal archive %s = %v", tableID, err)
		}
	}
	for _, tableID := range groupOpsReconciledTables[5:] {
		if err := validateGroupOpsDisposition(tableID, "archive"); err != nil {
			t.Fatalf("runtime archive %s = %v", tableID, err)
		}
		for _, disposition := range []string{"import", "quarantine"} {
			if err := validateGroupOpsDisposition(tableID, disposition); !errors.Is(err, ErrConflict) {
				t.Fatalf("runtime %s/%s = %v", tableID, disposition, err)
			}
		}
	}
	if err := validateGroupOpsDisposition("public/other", "archive"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown table = %v", err)
	}
}

func TestGroupOpsTargetIDBindsExactSourceTargetPairs(t *testing.T) {
	for _, pair := range [][2]string{
		{groupOpsPlansTable, "group_ops_plans"},
		{groupOpsGroupChatsTable, "group_ops_v1_history_directory"},
		{groupOpsSnapshotsTable, "group_ops_v1_history_directory"},
		{groupOpsGroupsTable, "group_ops_v1_history_groups"},
		{groupOpsNodesTable, "group_ops_v1_history_nodes"},
	} {
		row := groupOpsReconcileRow(pair[0], pair[1], "17", groupOpsReconcileDigest(1))
		if id, err := groupOpsTargetID(row); err != nil || id != 17 {
			t.Fatalf("pair %v = %d/%v", pair, id, err)
		}
		wrong := row
		wrong.TableID = "public/automation_group_ops_execution_log"
		if _, err := groupOpsTargetID(wrong); !errors.Is(err, ErrConflict) {
			t.Fatalf("runtime source accepted: %v", err)
		}
	}
}

func TestGroupOpsTargetDigestsRequireFullReadOnlyFacts(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	owner := int64(9)
	plan := groupopsport.HistoricalPlan{Plan: groupopsport.Plan{ID: 41, Name: "V1 archived plan", Status: groupopsport.PlanArchived, Revision: 1, CreatedBy: 7, UpdatedBy: 7, CreatedAt: stamp, UpdatedAt: stamp},
		SourcePlanID: 30, SourceCode: "legacy-30", PlanType: "sop", OriginalStatus: "active", OwnerStaffID: &owner}
	planDigest := groupopsapp.HistoricalPlanTargetDigest(plan)
	if !groupOpsPlanMatchesTarget(plan, planDigest[:]) {
		t.Fatal("exact plan rejected")
	}
	changedPlan := plan
	changedPlan.SourceCode = "legacy-31"
	if groupOpsPlanMatchesTarget(changedPlan, planDigest[:]) {
		t.Fatal("plan source lineage drift accepted")
	}

	count := int32(9)
	directory := groupopsport.HistoricalDirectory{ID: 51, SourceKind: "group_chats", SourceID: groupOpsInt64Pointer(10), ChatReference: "chat-legacy", MemberCount: &count, OriginalStatus: "active", RecordedAt: stamp}
	directoryDigest := groupopsapp.HistoricalDirectoryTargetDigest(directory)
	if !groupOpsDirectoryMatchesTarget(groupOpsGroupChatsTable, directory, directoryDigest[:]) {
		t.Fatal("exact group chat rejected")
	}
	if groupOpsDirectoryMatchesTarget(groupOpsSnapshotsTable, directory, directoryDigest[:]) {
		t.Fatal("group chat accepted as snapshot")
	}
	internal, external := int32(3), int32(6)
	snapshot := groupopsport.HistoricalDirectory{ID: 52, SourceKind: "wecom_group_chat_snapshots", ChatReference: "chat-legacy", InternalMemberCount: &internal, ExternalMemberCount: &external, OriginalStatus: "normal", RecordedAt: stamp}
	snapshotDigest := groupopsapp.HistoricalDirectoryTargetDigest(snapshot)
	if !groupOpsDirectoryMatchesTarget(groupOpsSnapshotsTable, snapshot, snapshotDigest[:]) {
		t.Fatal("exact snapshot rejected")
	}

	group := groupopsport.HistoricalGroup{ID: 61, SourceGroupID: 40, SourcePlanID: plan.SourcePlanID, PlanID: plan.ID, ChatReference: "chat-legacy", DisplayName: "legacy group", InternalMemberCount: internal, ExternalMemberCount: external, OriginalStatus: "active", CreatedAt: stamp}
	groupDigest := groupopsapp.HistoricalGroupTargetDigest(group)
	if !groupOpsGroupMatchesTarget(group, groupDigest[:]) {
		t.Fatal("exact group rejected")
	}
	changedGroup := group
	changedGroup.SourcePlanID++
	if groupOpsGroupMatchesTarget(changedGroup, groupDigest[:]) {
		t.Fatal("group parent lineage drift accepted")
	}

	node := groupopsport.HistoricalNode{ID: 71, SourceNodeID: 50, SourcePlanID: plan.SourcePlanID, PlanID: plan.ID, DayIndex: 2, TriggerTime: "09:30", SortOrder: 1, OriginalStatus: "active", ContentPackage: []byte(`{"kind":"legacy_package","amount":9007199254740993}`), CreatedAt: stamp, UpdatedAt: stamp}
	nodeDigest := groupopsapp.HistoricalNodeTargetDigest(node)
	if !groupOpsNodeMatchesTarget(node, nodeDigest[:]) {
		t.Fatal("exact node rejected")
	}
	changedNode := node
	changedNode.ContentPackage = []byte(`{"kind":"legacy_package","amount":9007199254740992}`)
	if groupOpsNodeMatchesTarget(changedNode, nodeDigest[:]) {
		t.Fatal("node JSON fact drift accepted")
	}
}

func groupOpsReconcileRow(sourceTable, targetTable, targetID string, digest [sha256.Size]byte) reconciliationRow {
	domain := groupOpsTargetDomain
	return reconciliationRow{TableID: sourceTable, TargetDomain: &domain, TargetTable: &targetTable, TargetID: &targetID, TargetDigest: digest[:]}
}

func groupOpsReconcileDigest(first byte) (digest [sha256.Size]byte) {
	digest[0] = first
	return digest
}

func groupOpsInt64Pointer(value int64) *int64 { return &value }

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
)

var groupOpsReconciledTables = []string{
	groupOpsPlansTable,
	groupOpsGroupChatsTable,
	groupOpsSnapshotsTable,
	groupOpsGroupsTable,
	groupOpsNodesTable,
	"public/automation_group_ops_effect_dependency",
	"public/automation_group_ops_effect_graph",
	"public/automation_group_ops_effect_material",
	"public/automation_group_ops_execution_log",
	"public/automation_group_ops_trigger_event",
	"public/automation_group_ops_webhook_events",
}

// validateGroupOpsDisposition keeps runnable V1 records as archive-only facts.
// Formal rows are either imported as read-only targets or quarantined; they may
// never be disguised as ordinary archive terminals.
func validateGroupOpsDisposition(tableID, disposition string) error {
	switch tableID {
	case groupOpsPlansTable, groupOpsGroupChatsTable, groupOpsSnapshotsTable, groupOpsGroupsTable, groupOpsNodesTable:
		if disposition == "import" || disposition == "quarantine" {
			return nil
		}
	case "public/automation_group_ops_effect_dependency", "public/automation_group_ops_effect_graph", "public/automation_group_ops_effect_material",
		"public/automation_group_ops_execution_log", "public/automation_group_ops_trigger_event", "public/automation_group_ops_webhook_events":
		if disposition == "archive" {
			return nil
		}
	}
	return ErrConflict
}

// verifyGroupOpsTarget reads the Group Ops-owned immutable target with SQLc,
// checks the full owner digest, and proves children point to a plan imported
// in this same reconciliation batch. It has no Provider or runtime reads.
func verifyGroupOpsTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	id, err := groupOpsTargetID(row)
	if err != nil || ctx == nil || tx == nil {
		return "", ErrConflict
	}
	q := groupopsdb.New(tx)
	table := *row.TargetTable
	verified := false
	switch table {
	case "group_ops_plans":
		var value groupopsport.HistoricalPlan
		var rowValue groupopsdb.GetGroupOpsHistoricalPlanRow
		rowValue, err = q.GetGroupOpsHistoricalPlan(ctx, id)
		if err == nil {
			value, err = groupOpsPlanFromRow(rowValue)
		}
		verified = err == nil && groupOpsPlanMatchesTarget(value, row.TargetDigest)
	case "group_ops_v1_history_directory":
		var value groupopsport.HistoricalDirectory
		var rowValue groupopsdb.GroupOpsV1HistoryDirectory
		rowValue, err = q.GetGroupOpsHistoricalDirectory(ctx, id)
		if err == nil {
			value, err = groupOpsDirectoryFromRow(rowValue)
		}
		verified = err == nil && groupOpsDirectoryMatchesTarget(row.TableID, value, row.TargetDigest)
	case "group_ops_v1_history_groups":
		var value groupopsport.HistoricalGroup
		var rowValue groupopsdb.GroupOpsV1HistoryGroup
		rowValue, err = q.GetGroupOpsHistoricalGroup(ctx, id)
		if err == nil {
			value, err = groupOpsGroupFromRow(rowValue)
		}
		if err == nil {
			verified, err = groupOpsHistoricalParentMatches(ctx, q, value.PlanID, value.SourcePlanID, targets)
			verified = verified && err == nil && groupOpsGroupMatchesTarget(value, row.TargetDigest)
		}
	case "group_ops_v1_history_nodes":
		var value groupopsport.HistoricalNode
		var rowValue groupopsdb.GroupOpsV1HistoryNode
		rowValue, err = q.GetGroupOpsHistoricalNode(ctx, id)
		if err == nil {
			value, err = groupOpsNodeFromRow(rowValue)
		}
		if err == nil {
			verified, err = groupOpsHistoricalParentMatches(ctx, q, value.PlanID, value.SourcePlanID, targets)
			verified = verified && err == nil && groupOpsNodeMatchesTarget(value, row.TargetDigest)
		}
	}
	if err != nil || !verified {
		return "", targetVerificationError(table, *row.TargetID, err)
	}
	return table + ":" + *row.TargetID + ":v1_history:" + hex.EncodeToString(row.TargetDigest), nil
}

func groupOpsTargetID(row reconciliationRow) (int64, error) {
	if row.TargetDomain == nil || *row.TargetDomain != groupOpsTargetDomain || row.TargetTable == nil || row.TargetID == nil || len(row.TargetDigest) != sha256.Size {
		return 0, ErrConflict
	}
	valid := (row.TableID == groupOpsPlansTable && *row.TargetTable == "group_ops_plans") ||
		((row.TableID == groupOpsGroupChatsTable || row.TableID == groupOpsSnapshotsTable) && *row.TargetTable == "group_ops_v1_history_directory") ||
		(row.TableID == groupOpsGroupsTable && *row.TargetTable == "group_ops_v1_history_groups") ||
		(row.TableID == groupOpsNodesTable && *row.TargetTable == "group_ops_v1_history_nodes")
	if !valid {
		return 0, ErrConflict
	}
	return positiveID(*row.TargetID)
}

func groupOpsHistoricalParentMatches(ctx context.Context, q *groupopsdb.Queries, planID, sourcePlanID int64, targets map[string]map[string]struct{}) (bool, error) {
	if planID < 1 || sourcePlanID < 1 || !containsTarget(targets, "group_ops_plans", strconv.FormatInt(planID, 10)) {
		return false, nil
	}
	row, err := q.GetGroupOpsHistoricalPlan(ctx, planID)
	if err != nil {
		return false, err
	}
	parent, err := groupOpsPlanFromRow(row)
	if err != nil {
		return false, err
	}
	return parent.ID == planID && parent.SourcePlanID == sourcePlanID && parent.Status == groupopsport.PlanArchived && parent.Revision == 1, nil
}

func groupOpsPlanMatchesTarget(value groupopsport.HistoricalPlan, expected []byte) bool {
	digest := groupopsapp.HistoricalPlanTargetDigest(value)
	return value.ID > 0 && value.Status == groupopsport.PlanArchived && value.Revision == 1 && value.SourcePlanID > 0 &&
		value.CreatedBy > 0 && value.UpdatedBy > 0 && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() &&
		len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func groupOpsDirectoryMatchesTarget(sourceTable string, value groupopsport.HistoricalDirectory, expected []byte) bool {
	digest := groupopsapp.HistoricalDirectoryTargetDigest(value)
	validSource := sourceTable == groupOpsGroupChatsTable && value.SourceKind == "group_chats" && value.SourceID != nil && *value.SourceID > 0 &&
		value.MemberCount != nil && *value.MemberCount >= 0 && value.InternalMemberCount == nil && value.ExternalMemberCount == nil && value.OwnerName == nil ||
		sourceTable == groupOpsSnapshotsTable && value.SourceKind == "wecom_group_chat_snapshots" && value.SourceID == nil && value.MemberCount == nil &&
			value.InternalMemberCount != nil && *value.InternalMemberCount >= 0 && value.ExternalMemberCount != nil && *value.ExternalMemberCount >= 0
	return value.ID > 0 && value.ChatReference != "" && value.OriginalStatus != "" && !value.RecordedAt.IsZero() && validSource &&
		len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func groupOpsGroupMatchesTarget(value groupopsport.HistoricalGroup, expected []byte) bool {
	digest := groupopsapp.HistoricalGroupTargetDigest(value)
	return value.ID > 0 && value.SourceGroupID > 0 && value.SourcePlanID > 0 && value.PlanID > 0 && value.ChatReference != "" &&
		value.DisplayName != "" && value.OriginalStatus != "" && value.InternalMemberCount >= 0 && value.ExternalMemberCount >= 0 && !value.CreatedAt.IsZero() &&
		len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func groupOpsNodeMatchesTarget(value groupopsport.HistoricalNode, expected []byte) bool {
	digest := groupopsapp.HistoricalNodeTargetDigest(value)
	return value.ID > 0 && value.SourceNodeID > 0 && value.SourcePlanID > 0 && value.PlanID > 0 && value.DayIndex >= 0 && value.SortOrder >= 0 &&
		value.TriggerTime != "" && value.OriginalStatus != "" && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() &&
		len(value.ContentPackage) > 0 && len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func groupOpsPlanFromRow(row groupopsdb.GetGroupOpsHistoricalPlanRow) (groupopsport.HistoricalPlan, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return groupopsport.HistoricalPlan{}, ErrConflict
	}
	return groupopsport.HistoricalPlan{Plan: groupopsport.Plan{ID: row.ID, Name: row.Name, Status: groupopsport.PlanStatus(row.Status), Revision: row.Revision,
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, SourcePlanID: row.SourcePlanID,
		SourceCode: row.SourceCode, PlanType: row.PlanType, OriginalStatus: row.OriginalStatus, OwnerStaffID: groupOpsOptionalInt64(row.OwnerStaffID), ArchivedAt: groupOpsOptionalTime(row.ArchivedAt)}, nil
}

func groupOpsDirectoryFromRow(row groupopsdb.GroupOpsV1HistoryDirectory) (groupopsport.HistoricalDirectory, error) {
	if !row.RecordedAt.Valid {
		return groupopsport.HistoricalDirectory{}, ErrConflict
	}
	return groupopsport.HistoricalDirectory{ID: row.ID, SourceKind: row.SourceKind, SourceID: groupOpsOptionalInt64(row.SourceID), ChatReference: row.ChatReference,
		DisplayName: groupOpsOptionalText(row.DisplayName), OwnerStaffID: groupOpsOptionalInt64(row.OwnerStaffID), OwnerName: groupOpsOptionalText(row.OwnerName),
		MemberCount: groupOpsOptionalInt32(row.MemberCount), InternalMemberCount: groupOpsOptionalInt32(row.InternalMemberCount), ExternalMemberCount: groupOpsOptionalInt32(row.ExternalMemberCount),
		OriginalStatus: row.OriginalStatus, RecordedAt: row.RecordedAt.Time}, nil
}

func groupOpsGroupFromRow(row groupopsdb.GroupOpsV1HistoryGroup) (groupopsport.HistoricalGroup, error) {
	if !row.CreatedAt.Valid {
		return groupopsport.HistoricalGroup{}, ErrConflict
	}
	return groupopsport.HistoricalGroup{ID: row.ID, SourceGroupID: row.SourceGroupID, SourcePlanID: row.SourcePlanID, PlanID: row.PlanID,
		ChatReference: row.ChatReference, DisplayName: row.DisplayName, OwnerStaffID: groupOpsOptionalInt64(row.OwnerStaffID), InternalMemberCount: row.InternalMemberCount,
		ExternalMemberCount: row.ExternalMemberCount, OriginalStatus: row.OriginalStatus, CreatedAt: row.CreatedAt.Time, RemovedAt: groupOpsOptionalTime(row.RemovedAt)}, nil
}

func groupOpsNodeFromRow(row groupopsdb.GroupOpsV1HistoryNode) (groupopsport.HistoricalNode, error) {
	if !row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return groupopsport.HistoricalNode{}, ErrConflict
	}
	return groupopsport.HistoricalNode{ID: row.ID, SourceNodeID: row.SourceNodeID, SourcePlanID: row.SourcePlanID, PlanID: row.PlanID, DayIndex: row.DayIndex,
		TriggerTime: row.TriggerTime, SortOrder: row.SortOrder, OriginalStatus: row.OriginalStatus, ContentPackage: append([]byte(nil), row.ContentPackage...),
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}, nil
}

func groupOpsOptionalInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func groupOpsOptionalInt32(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}

func groupOpsOptionalText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func groupOpsOptionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

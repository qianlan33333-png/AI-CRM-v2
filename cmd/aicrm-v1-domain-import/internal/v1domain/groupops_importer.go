package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1groupops"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	groupOpsDomain = "groupops"

	groupOpsDirectoryTarget = "group_ops_v1_history_directory"
	groupOpsGroupsTarget    = "group_ops_v1_history_groups"
	groupOpsNodesTarget     = "group_ops_v1_history_nodes"

	groupOpsEffectDependencyTableID = "public/automation_group_ops_effect_dependency"
	groupOpsEffectGraphTableID      = "public/automation_group_ops_effect_graph"
	groupOpsEffectMaterialTableID   = "public/automation_group_ops_effect_material"
	groupOpsExecutionLogTableID     = "public/automation_group_ops_execution_log"
	groupOpsTriggerEventTableID     = "public/automation_group_ops_trigger_event"
	groupOpsWebhookEventsTableID    = "public/automation_group_ops_webhook_events"
)

var groupOpsRuntimeTableIDs = []string{
	groupOpsEffectDependencyTableID,
	groupOpsEffectGraphTableID,
	groupOpsEffectMaterialTableID,
	groupOpsExecutionLogTableID,
	groupOpsTriggerEventTableID,
	groupOpsWebhookEventsTableID,
}

// GroupOpsHistoryWriter is the owner-owned historical write boundary. Its
// methods are called inside the caller-owned transaction only.
type GroupOpsHistoryWriter interface {
	ImportPlan(context.Context, string, [sha256.Size]byte, groupopsport.HistoricalPlan) (groupopsport.HistoricalReceipt, error)
	ImportDirectory(context.Context, string, [sha256.Size]byte, groupopsport.HistoricalDirectory) (groupopsport.HistoricalReceipt, error)
	ImportGroup(context.Context, string, [sha256.Size]byte, groupopsport.HistoricalGroup) (groupopsport.HistoricalReceipt, error)
	ImportNode(context.Context, string, [sha256.Size]byte, groupopsport.HistoricalNode) (groupopsport.HistoricalReceipt, error)
}

// GroupOpsStaffResolver returns only a verified V2 staff reference. V1
// userids never become V2 IDs directly; an unresolved reference stays nil.
type GroupOpsStaffResolver interface {
	ResolveGroupOpsStaff(context.Context, string) (*int64, error)
}

type GroupOpsTableResult struct {
	Imported, Archived, Quarantined, Replayed int
}

type GroupOpsImportResult struct {
	Tables map[string]GroupOpsTableResult
}

type GroupOpsImporter struct {
	archive      ArchiveSource
	uow          UnitOfWork
	writer       GroupOpsHistoryWriter
	resolver     GroupOpsStaffResolver
	journals     map[string]groupOpsTerminalJournal
	archiveRunID string
	actorID      int64
}

func NewGroupOpsImporter(archive ArchiveSource, uow UnitOfWork, writer GroupOpsHistoryWriter, resolver GroupOpsStaffResolver, journals map[string]*Journal, actorID int64) (*GroupOpsImporter, error) {
	if !validGroupOpsImportJournals(journals) || archive == nil || uow == nil || writer == nil || resolver == nil || actorID < 1 {
		return nil, ErrInvalidScope
	}
	terminals := make(map[string]groupOpsTerminalJournal, len(journals))
	for tableID, journal := range journals {
		terminals[tableID] = journal
	}
	return newGroupOpsImporter(archive, uow, writer, resolver, terminals, journals[v1groupops.PlansTableID].scope.ArchiveRunID, actorID)
}

// newGroupOpsImporter keeps the production constructor bound to concrete
// scoped Journals while allowing isolated importer tests to use fakes.
func newGroupOpsImporter(archive ArchiveSource, uow UnitOfWork, writer GroupOpsHistoryWriter, resolver GroupOpsStaffResolver, journals map[string]groupOpsTerminalJournal, archiveRunID string, actorID int64) (*GroupOpsImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || archiveRunID == "" || actorID < 1 || len(journals) != len(groupOpsTableIDs()) {
		return nil, ErrInvalidScope
	}
	for _, tableID := range groupOpsTableIDs() {
		if journals[tableID] == nil {
			return nil, ErrInvalidScope
		}
	}
	return &GroupOpsImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journals: journals, archiveRunID: archiveRunID, actorID: actorID}, nil
}

func validGroupOpsImportJournals(journals map[string]*Journal) bool {
	if len(journals) != len(groupOpsTableIDs()) {
		return false
	}
	plans := journals[v1groupops.PlansTableID]
	if !validGroupOpsJournalScope(plans, v1groupops.PlansTableID, "group_ops_plans") {
		return false
	}
	for _, tableID := range []string{v1groupops.GroupChatsTableID, v1groupops.GroupSnapshotsTableID} {
		if !validGroupOpsJournalScope(journals[tableID], tableID, groupOpsDirectoryTarget) || journals[tableID].scope.ArchiveRunID != plans.scope.ArchiveRunID {
			return false
		}
	}
	for tableID, target := range map[string]string{v1groupops.PlanGroupsTableID: groupOpsGroupsTarget, v1groupops.PlanNodesTableID: groupOpsNodesTarget} {
		if !validGroupOpsJournalScope(journals[tableID], tableID, target) || journals[tableID].scope.ArchiveRunID != plans.scope.ArchiveRunID {
			return false
		}
	}
	for _, tableID := range groupOpsRuntimeTableIDs {
		journal := journals[tableID]
		if journal == nil || !journal.scope.valid() || journal.scope.ImportVersion != groupOpsImportVersion || journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != tableID || journal.scope.TargetDomain != groupOpsDomain || journal.scope.ArchiveRunID != plans.scope.ArchiveRunID {
			return false
		}
	}
	return true
}

func (importer *GroupOpsImporter) Import(ctx context.Context, archiveRunID string) (GroupOpsImportResult, error) {
	if importer == nil || ctx == nil || archiveRunID == "" || importer.archiveRunID != archiveRunID {
		return GroupOpsImportResult{}, ErrInvalidScope
	}
	rows, err := importer.readBusinessRows(ctx, archiveRunID)
	if err != nil {
		return GroupOpsImportResult{}, err
	}
	runtimeRows, err := importer.readRuntimeRows(ctx, archiveRunID)
	if err != nil {
		return GroupOpsImportResult{}, err
	}
	history := v1groupops.AdaptHistory(groupOpsPayloads(rows.groupChats), groupOpsPayloads(rows.snapshots), groupOpsPayloads(rows.plans), groupOpsPayloads(rows.planGroups), groupOpsPayloads(rows.planNodes))
	if len(history.GroupChats) != len(rows.groupChats) || len(history.Snapshots) != len(rows.snapshots) || len(history.Plans) != len(rows.plans) || len(history.PlanGroups) != len(rows.planGroups) || len(history.PlanNodes) != len(rows.planNodes) {
		return GroupOpsImportResult{}, ErrConflict
	}
	result := GroupOpsImportResult{Tables: make(map[string]GroupOpsTableResult, len(groupOpsTableIDs()))}
	for _, tableID := range groupOpsTableIDs() {
		result.Tables[tableID] = GroupOpsTableResult{}
	}
	for index, decision := range history.GroupChats {
		if err := importer.importGroupChat(ctx, rows.groupChats[index], decision, &result); err != nil {
			return GroupOpsImportResult{}, err
		}
	}
	for index, decision := range history.Snapshots {
		if err := importer.importSnapshot(ctx, rows.snapshots[index], decision, &result); err != nil {
			return GroupOpsImportResult{}, err
		}
	}
	planTargets := make(map[int64]int64, len(history.Plans))
	for index, decision := range history.Plans {
		if err := importer.importPlan(ctx, rows.plans[index], decision, planTargets, &result); err != nil {
			return GroupOpsImportResult{}, err
		}
	}
	for index, decision := range history.PlanGroups {
		if err := importer.importPlanGroup(ctx, rows.planGroups[index], decision, planTargets, &result); err != nil {
			return GroupOpsImportResult{}, err
		}
	}
	for index, decision := range history.PlanNodes {
		if err := importer.importPlanNode(ctx, rows.planNodes[index], decision, planTargets, &result); err != nil {
			return GroupOpsImportResult{}, err
		}
	}
	for tableID, tableRows := range runtimeRows {
		for _, row := range tableRows {
			if err := importer.recordDecision(ctx, tableID, row, "archive", "group_ops_runtime_history_only", &result); err != nil {
				return GroupOpsImportResult{}, err
			}
		}
	}
	return result, nil
}

type groupOpsBusinessRows struct {
	groupChats, snapshots, plans, planGroups, planNodes []groupOpsArchiveRow
}

type groupOpsArchiveRow struct {
	archive         v1archive.ArchivedRow
	redactionReason string
}

func (importer *GroupOpsImporter) readBusinessRows(ctx context.Context, archiveRunID string) (groupOpsBusinessRows, error) {
	var result groupOpsBusinessRows
	var err error
	if result.groupChats, err = importer.readRows(ctx, archiveRunID, v1groupops.GroupChatsTableID, true); err != nil {
		return groupOpsBusinessRows{}, err
	}
	if result.snapshots, err = importer.readRows(ctx, archiveRunID, v1groupops.GroupSnapshotsTableID, true); err != nil {
		return groupOpsBusinessRows{}, err
	}
	if result.plans, err = importer.readRows(ctx, archiveRunID, v1groupops.PlansTableID, true); err != nil {
		return groupOpsBusinessRows{}, err
	}
	if result.planGroups, err = importer.readRows(ctx, archiveRunID, v1groupops.PlanGroupsTableID, true); err != nil {
		return groupOpsBusinessRows{}, err
	}
	if result.planNodes, err = importer.readRows(ctx, archiveRunID, v1groupops.PlanNodesTableID, true); err != nil {
		return groupOpsBusinessRows{}, err
	}
	return result, nil
}

func (importer *GroupOpsImporter) readRuntimeRows(ctx context.Context, archiveRunID string) (map[string][]groupOpsArchiveRow, error) {
	result := make(map[string][]groupOpsArchiveRow, len(groupOpsRuntimeTableIDs))
	for _, tableID := range groupOpsRuntimeTableIDs {
		rows, err := importer.readRows(ctx, archiveRunID, tableID, false)
		if err != nil {
			return nil, err
		}
		result[tableID] = rows
	}
	return result, nil
}

func (importer *GroupOpsImporter) readRows(ctx context.Context, archiveRunID, tableID string, classify bool) ([]groupOpsArchiveRow, error) {
	rows := make([]groupOpsArchiveRow, 0)
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != tableID || row.SourceOrdinal < 1 || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return ErrConflict
		}
		reason := ""
		if classify && groupOpsRequiredFieldRedacted(tableID, row) {
			reason = "group_ops_history_business_field_redacted"
		}
		rows = append(rows, groupOpsArchiveRow{archive: row, redactionReason: reason})
		return nil
	})
	return rows, err
}

func groupOpsPayloads(rows []groupOpsArchiveRow) []json.RawMessage {
	values := make([]json.RawMessage, len(rows))
	for index := range rows {
		values[index] = rows[index].archive.Payload
	}
	return values
}

func groupOpsRequiredFieldRedacted(tableID string, row v1archive.ArchivedRow) bool {
	fields := map[string][]string{
		v1groupops.GroupChatsTableID:     {"id", "chat_id", "member_count", "status", "updated_at"},
		v1groupops.GroupSnapshotsTableID: {"chat_id", "group_name", "owner_userid", "owner_name", "internal_member_count", "external_member_count", "synced_at", "status"},
		v1groupops.PlansTableID:          {"id", "plan_code", "plan_name", "plan_type", "owner_userid", "status", "created_at", "updated_at"},
		v1groupops.PlanGroupsTableID:     {"id", "plan_id", "chat_id", "group_name_snapshot", "owner_userid_snapshot", "internal_member_count_snapshot", "external_member_count_snapshot", "status", "created_at"},
		v1groupops.PlanNodesTableID:      {"id", "plan_id", "day_index", "trigger_time_label", "sort_order", "status", "created_at", "updated_at", "content_package_json"},
	}
	for _, field := range fields[tableID] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func (importer *GroupOpsImporter) importGroupChat(ctx context.Context, row groupOpsArchiveRow, decision v1groupops.GroupChatResult, result *GroupOpsImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, v1groupops.GroupChatsTableID, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1groupops.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, v1groupops.GroupChatsTableID, row, "quarantine", groupOpsReason(decision.Reason), result)
	}
	fact := *decision.Fact
	return importer.importDirectory(ctx, v1groupops.GroupChatsTableID, row, fact.OwnerUserID, groupopsport.HistoricalDirectory{
		SourceKind: "group_chats", SourceID: int64Pointer(fact.SourceID), ChatReference: fact.ChatID, DisplayName: cloneStringPointer(fact.GroupName),
		MemberCount: int32Pointer(fact.MemberCount), OriginalStatus: fact.OriginalStatus, RecordedAt: fact.UpdatedAt,
	}, result)
}

func (importer *GroupOpsImporter) importSnapshot(ctx context.Context, row groupOpsArchiveRow, decision v1groupops.GroupSnapshotResult, result *GroupOpsImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, v1groupops.GroupSnapshotsTableID, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1groupops.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, v1groupops.GroupSnapshotsTableID, row, "quarantine", groupOpsReason(decision.Reason), result)
	}
	fact := *decision.Fact
	return importer.importDirectory(ctx, v1groupops.GroupSnapshotsTableID, row, &fact.OwnerUserID, groupopsport.HistoricalDirectory{
		SourceKind: "wecom_group_chat_snapshots", ChatReference: fact.ChatID, DisplayName: stringPointer(fact.GroupName), OwnerName: stringPointer(fact.OwnerName),
		InternalMemberCount: int32Pointer(fact.InternalMemberCount), ExternalMemberCount: int32Pointer(fact.ExternalMemberCount), OriginalStatus: fact.OriginalStatus, RecordedAt: fact.SyncedAt,
	}, result)
}

func (importer *GroupOpsImporter) importDirectory(ctx context.Context, tableID string, row groupOpsArchiveRow, sourceOwner *string, value groupopsport.HistoricalDirectory, result *GroupOpsImportResult) error {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		ownerID, err := importer.resolveStaff(tx, sourceOwner)
		if err != nil {
			return err
		}
		value.OwnerStaffID = ownerID
		receipt, err := importer.writer.ImportDirectory(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, value)
		if err != nil {
			return err
		}
		if !sameGroupOpsReceipt(receipt, row.archive) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if errors.Is(err, groupopsport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, tableID, row, "quarantine", "group_ops_directory_target_invalid", result)
	}
	if err != nil {
		return err
	}
	result.increment(tableID, "import", replayed)
	return nil
}

func (importer *GroupOpsImporter) importPlan(ctx context.Context, row groupOpsArchiveRow, decision v1groupops.PlanResult, targets map[int64]int64, result *GroupOpsImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, v1groupops.PlansTableID, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1groupops.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, v1groupops.PlansTableID, row, "quarantine", groupOpsReason(decision.Reason), result)
	}
	fact := *decision.Fact
	var targetID int64
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		targetID, replayed = 0, false
		ownerID, err := importer.resolveStaff(tx, &fact.SourceOwnerUserID)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.ImportPlan(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, groupopsport.HistoricalPlan{
			Plan:         groupopsport.Plan{Name: fact.PlanName, Status: groupopsport.PlanArchived, Revision: 1, CreatedBy: importer.actorID, UpdatedBy: importer.actorID, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt},
			SourcePlanID: fact.SourceID, SourceCode: fact.PlanCode, PlanType: fact.PlanType, OriginalStatus: fact.OriginalStatus, OwnerStaffID: ownerID, ArchivedAt: cloneTimePointer(fact.ArchivedAt),
		})
		if err != nil {
			return err
		}
		if !sameGroupOpsReceipt(receipt, row.archive) {
			return ErrConflict
		}
		targetID, replayed = receipt.TargetID, receipt.Replayed
		return nil
	})
	if errors.Is(err, groupopsport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, v1groupops.PlansTableID, row, "quarantine", "group_ops_plan_target_invalid", result)
	}
	if err != nil {
		return err
	}
	targets[fact.SourceID] = targetID
	result.increment(v1groupops.PlansTableID, "import", replayed)
	return nil
}

func (importer *GroupOpsImporter) importPlanGroup(ctx context.Context, row groupOpsArchiveRow, decision v1groupops.PlanGroupResult, targets map[int64]int64, result *GroupOpsImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, v1groupops.PlanGroupsTableID, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1groupops.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, v1groupops.PlanGroupsTableID, row, "quarantine", groupOpsReason(decision.Reason), result)
	}
	fact := *decision.Fact
	planID, found := targets[fact.PlanSourceID]
	if !found || planID < 1 {
		return importer.recordDecision(ctx, v1groupops.PlanGroupsTableID, row, "quarantine", "group_ops_plan_parent_unavailable", result)
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		ownerID, err := importer.resolveStaff(tx, &fact.OwnerUserID)
		if err != nil {
			return err
		}
		receipt, err := importer.writer.ImportGroup(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, groupopsport.HistoricalGroup{
			SourceGroupID: fact.SourceID, SourcePlanID: fact.PlanSourceID, PlanID: planID, ChatReference: fact.ChatID, DisplayName: fact.GroupName,
			OwnerStaffID: ownerID, InternalMemberCount: fact.InternalMemberCountSnapshot, ExternalMemberCount: fact.ExternalMemberCountSnapshot,
			OriginalStatus: fact.OriginalStatus, CreatedAt: fact.CreatedAt, RemovedAt: cloneTimePointer(fact.RemovedAt),
		})
		if err != nil {
			return err
		}
		if !sameGroupOpsReceipt(receipt, row.archive) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if errors.Is(err, groupopsport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, v1groupops.PlanGroupsTableID, row, "quarantine", "group_ops_group_target_invalid", result)
	}
	if err != nil {
		return err
	}
	result.increment(v1groupops.PlanGroupsTableID, "import", replayed)
	return nil
}

func (importer *GroupOpsImporter) importPlanNode(ctx context.Context, row groupOpsArchiveRow, decision v1groupops.PlanNodeResult, targets map[int64]int64, result *GroupOpsImportResult) error {
	if row.redactionReason != "" {
		return importer.recordDecision(ctx, v1groupops.PlanNodesTableID, row, "quarantine", row.redactionReason, result)
	}
	if decision.Disposition != v1groupops.DispositionCandidate || decision.Fact == nil {
		return importer.recordDecision(ctx, v1groupops.PlanNodesTableID, row, "quarantine", groupOpsReason(decision.Reason), result)
	}
	fact := *decision.Fact
	planID, found := targets[fact.PlanSourceID]
	if !found || planID < 1 {
		return importer.recordDecision(ctx, v1groupops.PlanNodesTableID, row, "quarantine", "group_ops_plan_parent_unavailable", result)
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		receipt, err := importer.writer.ImportNode(tx, SourceIdentifier(row.archive.SourceKeyHMAC), row.archive.PayloadHMAC, groupopsport.HistoricalNode{
			SourceNodeID: fact.SourceID, SourcePlanID: fact.PlanSourceID, PlanID: planID, DayIndex: fact.DayIndex, TriggerTime: fact.TriggerTime,
			SortOrder: fact.SortOrder, OriginalStatus: fact.OriginalStatus, ContentPackage: append(json.RawMessage(nil), fact.ContentPackage...), CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt,
		})
		if err != nil {
			return err
		}
		if !sameGroupOpsReceipt(receipt, row.archive) {
			return ErrConflict
		}
		replayed = receipt.Replayed
		return nil
	})
	if errors.Is(err, groupopsport.ErrHistoryInvalid) {
		return importer.recordDecision(ctx, v1groupops.PlanNodesTableID, row, "quarantine", "group_ops_node_target_invalid", result)
	}
	if err != nil {
		return err
	}
	result.increment(v1groupops.PlanNodesTableID, "import", replayed)
	return nil
}

func (importer *GroupOpsImporter) recordDecision(ctx context.Context, tableID string, row groupOpsArchiveRow, disposition, reason string, result *GroupOpsImportResult) error {
	if (disposition != "archive" && disposition != "quarantine") || reason == "" {
		return ErrConflict
	}
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed = false
		journal := importer.journals[tableID]
		existing, found, err := journal.LoadTerminal(tx, SourceIdentifier(row.archive.SourceKeyHMAC))
		if err != nil {
			return err
		}
		want := TerminalReceipt{SourceKeyDigest: row.archive.SourceKeyHMAC, PayloadDigest: row.archive.PayloadHMAC, Disposition: disposition, Reason: reason}
		if found {
			if !sameGroupOpsTerminal(existing, want) {
				return ErrConflict
			}
			replayed = true
		}
		return journal.Record(tx, want)
	})
	if err != nil {
		return err
	}
	result.increment(tableID, disposition, replayed)
	return nil
}

func (importer *GroupOpsImporter) resolveStaff(ctx context.Context, source *string) (*int64, error) {
	if source == nil || *source == "" {
		return nil, nil
	}
	staffID, err := importer.resolver.ResolveGroupOpsStaff(ctx, *source)
	if err != nil || staffID == nil {
		return staffID, err
	}
	if *staffID < 1 {
		return nil, ErrConflict
	}
	return staffID, nil
}

func sameGroupOpsReceipt(receipt groupopsport.HistoricalReceipt, row v1archive.ArchivedRow) bool {
	return receipt.SourceIdentifier == SourceIdentifier(row.SourceKeyHMAC) && receipt.PayloadDigest == row.PayloadHMAC && receipt.TargetID > 0 && receipt.TargetDigest != ([sha256.Size]byte{})
}

func sameGroupOpsTerminal(found, want TerminalReceipt) bool {
	return found.SourceKeyDigest == want.SourceKeyDigest && found.PayloadDigest == want.PayloadDigest && found.Disposition == want.Disposition &&
		found.Reason == want.Reason && found.TargetID == "" && found.TargetDigest == ([sha256.Size]byte{}) && len(found.Metadata) == 0
}

func (result *GroupOpsImportResult) increment(tableID, disposition string, replayed bool) {
	value := result.Tables[tableID]
	switch disposition {
	case "import":
		value.Imported++
	case "archive":
		value.Archived++
	case "quarantine":
		value.Quarantined++
	}
	if replayed {
		value.Replayed++
	}
	result.Tables[tableID] = value
}

func groupOpsReason(reason string) string {
	if reason == "" {
		return "group_ops_history_shape_invalid"
	}
	return reason
}

func groupOpsTableIDs() []string {
	return append([]string{v1groupops.GroupChatsTableID, v1groupops.GroupSnapshotsTableID, v1groupops.PlansTableID, v1groupops.PlanGroupsTableID, v1groupops.PlanNodesTableID}, groupOpsRuntimeTableIDs...)
}

func int64Pointer(value int64) *int64    { return &value }
func int32Pointer(value int32) *int32    { return &value }
func stringPointer(value string) *string { return &value }

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	return stringPointer(*value)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

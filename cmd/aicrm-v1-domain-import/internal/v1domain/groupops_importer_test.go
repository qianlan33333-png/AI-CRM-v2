package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1groupops"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestGroupOpsImporterPreservesHistoryWithoutRuntimeEffects(t *testing.T) {
	importer, writer, journals := groupOpsImporterFixture(t)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, tableID := range []string{v1groupops.GroupChatsTableID, v1groupops.GroupSnapshotsTableID, v1groupops.PlansTableID, v1groupops.PlanGroupsTableID, v1groupops.PlanNodesTableID} {
		if got := result.Tables[tableID]; got != (GroupOpsTableResult{Imported: 1}) {
			t.Fatalf("table %s result=%+v", tableID, got)
		}
	}
	for _, tableID := range groupOpsRuntimeTableIDs {
		if got := result.Tables[tableID]; got != (GroupOpsTableResult{Archived: 1}) {
			t.Fatalf("runtime table %s result=%+v", tableID, got)
		}
	}
	if len(writer.plans) != 1 || len(writer.directories) != 2 || len(writer.groups) != 1 || len(writer.nodes) != 1 {
		t.Fatalf("unexpected owner writes: %+v", writer)
	}
	plan := writer.plans[0]
	if plan.Status != groupopsport.PlanArchived || plan.Revision != 1 || plan.CreatedBy != 99 || plan.UpdatedBy != 99 || plan.SourcePlanID != 30 || plan.OwnerStaffID == nil || *plan.OwnerStaffID != 77 {
		t.Fatalf("unsafe historical plan: %+v", plan)
	}
	if writer.directories[0].SourceKind != "group_chats" || writer.directories[1].SourceKind != "wecom_group_chat_snapshots" || writer.directories[0].OwnerStaffID != nil || writer.directories[1].OwnerStaffID == nil || *writer.directories[1].OwnerStaffID != 77 {
		t.Fatalf("directory source facts were merged or owner mapping invented: %+v", writer.directories)
	}
	if group := writer.groups[0]; group.PlanID != writer.planTargetID || group.SourcePlanID != 30 || group.OwnerStaffID == nil || *group.OwnerStaffID != 77 {
		t.Fatalf("historical group lost parent mapping: %+v", group)
	}
	if node := writer.nodes[0]; node.PlanID != writer.planTargetID || node.DayIndex != 2 || node.TriggerTime != "09:30" || string(node.ContentPackage) != `{"kind":"legacy_package","reference":"opaque"}` {
		t.Fatalf("historical node was translated into runtime behavior: %+v", node)
	}
	if writer.runtimeCalls != 0 || writer.nonTransactionCall || writer.resolverOutsideTransaction {
		t.Fatal("writer/resolver left caller transaction or ran runtime behavior")
	}
	for _, journal := range journals {
		for _, terminal := range journal.receipts {
			if terminal.Metadata != nil {
				t.Fatal("terminal receipt stored source metadata")
			}
		}
	}

	replay, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	for _, tableID := range groupOpsTableIDs() {
		if got := replay.Tables[tableID]; got.Replayed != 1 || got.Imported+got.Archived != 1 {
			t.Fatalf("table %s replay=%+v", tableID, got)
		}
	}
	if len(writer.plans) != 1 || len(writer.directories) != 2 || len(writer.groups) != 1 || len(writer.nodes) != 1 {
		t.Fatal("replay wrote duplicate Group Ops history")
	}
}

func TestGroupOpsImporterQuarantinesPlanAndDependentsWhenPlanCannotWrite(t *testing.T) {
	importer, writer, journals := groupOpsImporterFixture(t)
	writer.errors["plan"] = groupopsport.ErrHistoryInvalid
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables[v1groupops.PlansTableID] != (GroupOpsTableResult{Quarantined: 1}) || result.Tables[v1groupops.PlanGroupsTableID] != (GroupOpsTableResult{Quarantined: 1}) || result.Tables[v1groupops.PlanNodesTableID] != (GroupOpsTableResult{Quarantined: 1}) {
		t.Fatalf("plan isolation result=%+v", result.Tables)
	}
	if len(writer.plans) != 0 || len(writer.groups) != 0 || len(writer.nodes) != 0 {
		t.Fatalf("failed plan wrote dependent history: %+v", writer)
	}
	if got := onlyTerminal(t, journals[v1groupops.PlansTableID]); got.Reason != "group_ops_plan_target_invalid" {
		t.Fatalf("plan terminal=%+v", got)
	}
	if got := onlyTerminal(t, journals[v1groupops.PlanGroupsTableID]); got.Reason != "group_ops_plan_parent_unavailable" {
		t.Fatalf("group terminal=%+v", got)
	}
	if got := onlyTerminal(t, journals[v1groupops.PlanNodesTableID]); got.Reason != "group_ops_plan_parent_unavailable" {
		t.Fatalf("node terminal=%+v", got)
	}
}

func TestGroupOpsImporterRejectsRedactionAndBadArchiveIdentity(t *testing.T) {
	importer, writer, journals := groupOpsImporterFixture(t)
	archive := importer.archive.(*groupOpsArchiveFake)
	archive.rows[v1groupops.PlansTableID][0].RedactedFields = []string{"plan_name"}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables[v1groupops.PlansTableID] != (GroupOpsTableResult{Quarantined: 1}) || len(writer.plans) != 0 {
		t.Fatalf("redacted plan result=%+v writer=%+v", result.Tables[v1groupops.PlansTableID], writer.plans)
	}
	if got := onlyTerminal(t, journals[v1groupops.PlansTableID]); got.Reason != "group_ops_history_business_field_redacted" {
		t.Fatalf("redacted plan terminal=%+v", got)
	}

	broken, brokenWriter, brokenJournals := groupOpsImporterFixture(t)
	brokenArchive := broken.archive.(*groupOpsArchiveFake)
	brokenArchive.rows[v1groupops.GroupChatsTableID][0].SourceKeyHMAC = [sha256.Size]byte{}
	if _, err := broken.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || brokenWriter.writeCount() != 0 || terminalCount(brokenJournals) != 0 {
		t.Fatalf("bad archive identity err=%v writes=%d terminals=%d", err, brokenWriter.writeCount(), terminalCount(brokenJournals))
	}
}

func TestNewGroupOpsImporterRequiresAllScopedJournals(t *testing.T) {
	writer := &groupOpsWriterFake{errors: map[string]error{}, receipts: map[string]groupopsport.HistoricalReceipt{}}
	resolver := groupOpsResolverFake{staff: map[string]*int64{}, outside: &writer.resolverOutsideTransaction}
	journals := make(map[string]*Journal, len(groupOpsTableIDs()))
	for _, tableID := range groupOpsTableIDs() {
		target := "group_ops_v1_runtime_archive"
		switch tableID {
		case v1groupops.PlansTableID:
			target = "group_ops_plans"
		case v1groupops.GroupChatsTableID, v1groupops.GroupSnapshotsTableID:
			target = groupOpsDirectoryTarget
		case v1groupops.PlanGroupsTableID:
			target = groupOpsGroupsTarget
		case v1groupops.PlanNodesTableID:
			target = groupOpsNodesTarget
		}
		journal, err := NewJournal(Scope{ImportVersion: groupOpsImportVersion, ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: tableID, TargetDomain: groupOpsDomain, TargetTable: target})
		if err != nil {
			t.Fatal(err)
		}
		journals[tableID] = journal
	}
	if _, err := NewGroupOpsImporter(&groupOpsArchiveFake{}, groupOpsUOW{}, writer, resolver, journals, 99); err != nil {
		t.Fatal(err)
	}
	delete(journals, groupOpsWebhookEventsTableID)
	if _, err := NewGroupOpsImporter(&groupOpsArchiveFake{}, groupOpsUOW{}, writer, resolver, journals, 99); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("missing runtime terminal accepted: %v", err)
	}
}

type groupOpsArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *groupOpsArchiveFake) EachTableRow(_ context.Context, runID, tableID string, callback func(v1archive.ArchivedRow) error) error {
	if runID != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range archive.rows[tableID] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type groupOpsTxKey struct{}

type groupOpsUOW struct{}

func (groupOpsUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, groupOpsTxKey{}, true))
}

type groupOpsJournalFake struct{ receipts map[string]TerminalReceipt }

func (journal *groupOpsJournalFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(groupOpsTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing Group Ops transaction")
	}
	receipt, found := journal.receipts[source]
	return receipt, found, nil
}

func (journal *groupOpsJournalFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(groupOpsTxKey{}) != true {
		return errors.New("missing Group Ops transaction")
	}
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if existing, found := journal.receipts[key]; found && !sameGroupOpsTerminal(existing, receipt) {
		return ErrConflict
	}
	journal.receipts[key] = receipt
	return nil
}

type groupOpsWriterFake struct {
	plans                      []groupopsport.HistoricalPlan
	directories                []groupopsport.HistoricalDirectory
	groups                     []groupopsport.HistoricalGroup
	nodes                      []groupopsport.HistoricalNode
	errors                     map[string]error
	receipts                   map[string]groupopsport.HistoricalReceipt
	planTargetID               int64
	runtimeCalls               int
	nonTransactionCall         bool
	resolverOutsideTransaction bool
}

func (writer *groupOpsWriterFake) ImportPlan(ctx context.Context, source string, payload [sha256.Size]byte, value groupopsport.HistoricalPlan) (groupopsport.HistoricalReceipt, error) {
	if err := writer.inside(ctx); err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if err := writer.errors["plan"]; err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if receipt, found := writer.receipts["plan/"+source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.plans = append(writer.plans, value)
	receipt := writer.receipt("plan/"+source, source, payload)
	writer.planTargetID = receipt.TargetID
	return receipt, nil
}

func (writer *groupOpsWriterFake) ImportDirectory(ctx context.Context, source string, payload [sha256.Size]byte, value groupopsport.HistoricalDirectory) (groupopsport.HistoricalReceipt, error) {
	if err := writer.inside(ctx); err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if err := writer.errors["directory"]; err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if receipt, found := writer.receipts["directory/"+source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.directories = append(writer.directories, value)
	return writer.receipt("directory/"+source, source, payload), nil
}

func (writer *groupOpsWriterFake) ImportGroup(ctx context.Context, source string, payload [sha256.Size]byte, value groupopsport.HistoricalGroup) (groupopsport.HistoricalReceipt, error) {
	if err := writer.inside(ctx); err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if err := writer.errors["group"]; err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if receipt, found := writer.receipts["group/"+source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.groups = append(writer.groups, value)
	return writer.receipt("group/"+source, source, payload), nil
}

func (writer *groupOpsWriterFake) ImportNode(ctx context.Context, source string, payload [sha256.Size]byte, value groupopsport.HistoricalNode) (groupopsport.HistoricalReceipt, error) {
	if err := writer.inside(ctx); err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if err := writer.errors["node"]; err != nil {
		return groupopsport.HistoricalReceipt{}, err
	}
	if receipt, found := writer.receipts["node/"+source]; found {
		receipt.Replayed = true
		return receipt, nil
	}
	writer.nodes = append(writer.nodes, value)
	return writer.receipt("node/"+source, source, payload), nil
}

func (writer *groupOpsWriterFake) inside(ctx context.Context) error {
	if ctx.Value(groupOpsTxKey{}) != true {
		writer.nonTransactionCall = true
		return errors.New("missing Group Ops transaction")
	}
	return nil
}

func (writer *groupOpsWriterFake) receipt(key, source string, payload [sha256.Size]byte) groupopsport.HistoricalReceipt {
	receipt := groupopsport.HistoricalReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: int64(700 + len(writer.receipts)), TargetDigest: sha256.Sum256([]byte(key))}
	writer.receipts[key] = receipt
	return receipt
}

func (writer *groupOpsWriterFake) writeCount() int {
	return len(writer.plans) + len(writer.directories) + len(writer.groups) + len(writer.nodes)
}

type groupOpsResolverFake struct {
	staff   map[string]*int64
	outside *bool
}

func (resolver groupOpsResolverFake) ResolveGroupOpsStaff(ctx context.Context, source string) (*int64, error) {
	if ctx.Value(groupOpsTxKey{}) != true {
		*resolver.outside = true
		return nil, errors.New("missing Group Ops transaction")
	}
	value := resolver.staff[source]
	if value == nil {
		return nil, nil
	}
	copy := *value
	return &copy, nil
}

func groupOpsImporterFixture(t *testing.T) (*GroupOpsImporter, *groupOpsWriterFake, map[string]*groupOpsJournalFake) {
	t.Helper()
	archive := &groupOpsArchiveFake{rows: groupOpsArchiveRows(t)}
	writer := &groupOpsWriterFake{errors: map[string]error{}, receipts: map[string]groupopsport.HistoricalReceipt{}}
	staffID := int64(77)
	resolver := groupOpsResolverFake{staff: map[string]*int64{"owner-known": &staffID}, outside: &writer.resolverOutsideTransaction}
	journals := make(map[string]*groupOpsJournalFake, len(groupOpsTableIDs()))
	terminals := make(map[string]groupOpsTerminalJournal, len(groupOpsTableIDs()))
	for _, tableID := range groupOpsTableIDs() {
		journal := &groupOpsJournalFake{receipts: map[string]TerminalReceipt{}}
		journals[tableID], terminals[tableID] = journal, journal
	}
	importer, err := newGroupOpsImporter(archive, groupOpsUOW{}, writer, resolver, terminals, "archive-run", 99)
	if err != nil {
		t.Fatal(err)
	}
	return importer, writer, journals
}

func groupOpsArchiveRows(t *testing.T) map[string][]v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.FixedZone("v1-source", 8*60*60))
	rows := map[string][]v1archive.ArchivedRow{
		v1groupops.GroupChatsTableID:     {groupOpsArchivedRow(t, v1groupops.GroupChatsTableID, 10, map[string]any{"id": int64(10), "chat_id": "chat-ref", "group_name": nil, "owner_userid": nil, "member_count": int32(9), "status": "active", "updated_at": stamp})},
		v1groupops.GroupSnapshotsTableID: {groupOpsArchivedRow(t, v1groupops.GroupSnapshotsTableID, 20, map[string]any{"chat_id": "chat-ref", "group_name": "Historic Group", "owner_userid": "owner-known", "owner_name": "Owner", "internal_member_count": int32(3), "external_member_count": int32(6), "status": "normal", "synced_at": stamp})},
		v1groupops.PlansTableID:          {groupOpsArchivedRow(t, v1groupops.PlansTableID, 30, map[string]any{"id": int64(30), "plan_code": "plan-30", "plan_name": "Historic Plan", "plan_type": "sop", "owner_userid": "owner-known", "status": "active", "created_at": stamp, "updated_at": stamp, "archived_at": nil})},
		v1groupops.PlanGroupsTableID:     {groupOpsArchivedRow(t, v1groupops.PlanGroupsTableID, 40, map[string]any{"id": int64(40), "plan_id": int64(30), "chat_id": "chat-ref", "group_name_snapshot": "Historic Group", "owner_userid_snapshot": "owner-known", "internal_member_count_snapshot": int32(3), "external_member_count_snapshot": int32(6), "status": "active", "created_at": stamp, "removed_at": nil})},
		v1groupops.PlanNodesTableID:      {groupOpsArchivedRow(t, v1groupops.PlanNodesTableID, 50, map[string]any{"id": int64(50), "plan_id": int64(30), "day_index": int32(2), "trigger_time_label": "09:30", "sort_order": int32(1), "status": "active", "created_at": stamp, "updated_at": stamp, "content_package_json": map[string]any{"kind": "legacy_package", "reference": "opaque"}})},
	}
	for index, tableID := range groupOpsRuntimeTableIDs {
		rows[tableID] = []v1archive.ArchivedRow{groupOpsArchivedRow(t, tableID, int64(index+1), map[string]any{"id": index + 1})}
	}
	return rows
}

func groupOpsArchivedRow(t *testing.T, tableID string, ordinal int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", tableID, ordinal))), PayloadHMAC: sha256.Sum256(payload), Payload: payload}
}

func onlyTerminal(t *testing.T, journal *groupOpsJournalFake) TerminalReceipt {
	t.Helper()
	if len(journal.receipts) != 1 {
		t.Fatalf("receipts=%+v", journal.receipts)
	}
	for _, receipt := range journal.receipts {
		return receipt
	}
	return TerminalReceipt{}
}

func terminalCount(journals map[string]*groupOpsJournalFake) int {
	count := 0
	for _, journal := range journals {
		count += len(journal.receipts)
	}
	return count
}

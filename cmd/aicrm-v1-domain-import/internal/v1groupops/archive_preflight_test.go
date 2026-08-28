package v1groupops

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var groupOpsArchiveRun = flag.String("groupops-archive-run", "", "optional reconciled V2 archive run for read-only Group Ops preflight")

func TestGroupOpsRequiredFieldRedactionQuarantinesOnlyUsedFacts(t *testing.T) {
	chat := v1archive.ArchivedRow{RedactedFields: []string{"chat_id"}}
	if !groupOpsRequiredFieldRedacted(GroupChatsTableID, chat) {
		t.Fatal("redacted Group Ops chat reference was accepted")
	}
	plan := v1archive.ArchivedRow{RedactedFields: []string{"webhook_key", "signature_secret_hash"}}
	if groupOpsRequiredFieldRedacted(PlansTableID, plan) {
		t.Fatal("archived-only plan credential blocked non-executable candidate")
	}
}

// TestReconciledGroupOpsArchivePreflight is opt-in and read-only. It verifies
// source identity, five-table conservation, and candidate classification
// without opening any target transaction or logging source values.
func TestReconciledGroupOpsArchivePreflight(t *testing.T) {
	if *groupOpsArchiveRun == "" {
		t.Skip("supply -groupops-archive-run and V2 archive environment for read-only Group Ops preflight")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive reader")
	}
	defer archive.Close()
	groupChats, err := readGroupOpsTable(ctx, archive, *groupOpsArchiveRun, GroupChatsTableID)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := readGroupOpsTable(ctx, archive, *groupOpsArchiveRun, GroupSnapshotsTableID)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := readGroupOpsTable(ctx, archive, *groupOpsArchiveRun, PlansTableID)
	if err != nil {
		t.Fatal(err)
	}
	planGroups, err := readGroupOpsTable(ctx, archive, *groupOpsArchiveRun, PlanGroupsTableID)
	if err != nil {
		t.Fatal(err)
	}
	planNodes, err := readGroupOpsTable(ctx, archive, *groupOpsArchiveRun, PlanNodesTableID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groupChats) != 36 || len(snapshots) != 17 || len(plans) != 12 || len(planGroups) != 14 || len(planNodes) != 3 {
		t.Fatal("unexpected Group Ops archive table counts")
	}
	history := AdaptHistory(groupChats, snapshots, plans, planGroups, planNodes)
	if len(history.GroupChats) != len(groupChats) || len(history.Snapshots) != len(snapshots) || len(history.Plans) != len(plans) || len(history.PlanGroups) != len(planGroups) || len(history.PlanNodes) != len(planNodes) {
		t.Fatal("Group Ops archive row conservation failed")
	}
	logGroupOpsPreflight(t, history)
}

func readGroupOpsTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID, table string) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, 0)
	err := archive.EachTableRow(ctx, runID, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal < 1 || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return errors.New("invalid Group Ops archive row identity")
		}
		if groupOpsRequiredFieldRedacted(table, row) {
			result = append(result, json.RawMessage(`{}`))
			return nil
		}
		result = append(result, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read Group Ops archive table: %w", err)
	}
	return result, nil
}

func groupOpsRequiredFieldRedacted(table string, row v1archive.ArchivedRow) bool {
	fields := map[string][]string{
		GroupChatsTableID:     {"id", "chat_id", "member_count", "status", "updated_at"},
		GroupSnapshotsTableID: {"chat_id", "group_name", "owner_userid", "owner_name", "internal_member_count", "external_member_count", "synced_at", "status"},
		PlansTableID:          {"id", "plan_code", "plan_name", "plan_type", "owner_userid", "status", "created_at", "updated_at"},
		PlanGroupsTableID:     {"id", "plan_id", "chat_id", "group_name_snapshot", "owner_userid_snapshot", "internal_member_count_snapshot", "external_member_count_snapshot", "status", "created_at"},
		PlanNodesTableID:      {"id", "plan_id", "day_index", "trigger_time_label", "sort_order", "status", "created_at", "updated_at", "content_package_json"},
	}
	for _, field := range fields[table] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func logGroupOpsPreflight(t *testing.T, history History) {
	groupCandidates, groupQuarantined := count(history.GroupChats, func(value GroupChatResult) Disposition { return value.Disposition })
	snapshotCandidates, snapshotQuarantined := count(history.Snapshots, func(value GroupSnapshotResult) Disposition { return value.Disposition })
	planCandidates, planQuarantined := count(history.Plans, func(value PlanResult) Disposition { return value.Disposition })
	groupCandidatesPlan, groupQuarantinedPlan := count(history.PlanGroups, func(value PlanGroupResult) Disposition { return value.Disposition })
	nodeCandidates, nodeQuarantined := count(history.PlanNodes, func(value PlanNodeResult) Disposition { return value.Disposition })
	t.Logf("read-only Group Ops preflight: group_chats=candidate:%d quarantine:%d snapshots=candidate:%d quarantine:%d plans=archived_candidate:%d quarantine:%d plan_groups=candidate:%d quarantine:%d plan_nodes=historical_candidate:%d quarantine:%d", groupCandidates, groupQuarantined, snapshotCandidates, snapshotQuarantined, planCandidates, planQuarantined, groupCandidatesPlan, groupQuarantinedPlan, nodeCandidates, nodeQuarantined)
}

func count[T any](values []T, disposition func(T) Disposition) (candidate, quarantine int) {
	for _, value := range values {
		if disposition(value) == DispositionCandidate {
			candidate++
		} else {
			quarantine++
		}
	}
	return
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsdb "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var groupOpsHistoryTestDatabaseURL = flag.String("groupops-history-test-database-url", "", "PostgreSQL URL for Group Ops V1 history rollback test")

func TestHistoricalRowsPreserveNullsAndJSONBSemantics(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	plan, err := groupOpsHistoricalPlan(41, "V1 archived plan", string(groupopsport.PlanArchived), 1, 7, 7, historicalStamp(stamp), historicalStamp(stamp), 30, "legacy-30", "sop", "active", pgtype.Int8{}, pgtype.Timestamptz{})
	if err != nil || plan.Plan.Status != groupopsport.PlanArchived || plan.OwnerStaffID != nil || plan.ArchivedAt != nil || !plan.CreatedAt.Equal(stamp) {
		t.Fatalf("plan = %#v, %v", plan, err)
	}

	chat, err := groupOpsHistoricalDirectory(groupopsdb.GroupOpsV1HistoryDirectory{ID: 51, SourceKind: historicalDirectoryGroupChats,
		SourceID: pgtype.Int8{Int64: 10, Valid: true}, ChatReference: "chat-ref", DisplayName: pgtype.Text{}, OwnerStaffID: pgtype.Int8{}, OwnerName: pgtype.Text{},
		MemberCount: pgtype.Int4{Int32: 9, Valid: true}, OriginalStatus: "active", RecordedAt: historicalStamp(stamp)})
	if err != nil || chat.SourceID == nil || *chat.SourceID != 10 || chat.DisplayName != nil || chat.OwnerName != nil || chat.InternalMemberCount != nil || chat.ExternalMemberCount != nil || !chat.RecordedAt.Equal(stamp) {
		t.Fatalf("chat = %#v, %v", chat, err)
	}

	snapshotName := "snapshot"
	snapshot, err := groupOpsHistoricalDirectory(groupopsdb.GroupOpsV1HistoryDirectory{ID: 52, SourceKind: historicalDirectorySnapshots,
		ChatReference: "chat-ref", DisplayName: pgtype.Text{String: snapshotName, Valid: true}, OwnerStaffID: pgtype.Int8{}, OwnerName: pgtype.Text{},
		InternalMemberCount: pgtype.Int4{Int32: 3, Valid: true}, ExternalMemberCount: pgtype.Int4{Int32: 6, Valid: true}, OriginalStatus: "normal", RecordedAt: historicalStamp(stamp)})
	if err != nil || snapshot.SourceID != nil || snapshot.MemberCount != nil || snapshot.DisplayName == nil || *snapshot.DisplayName != snapshotName || snapshot.InternalMemberCount == nil || snapshot.ExternalMemberCount == nil {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}

	group, err := groupOpsHistoricalGroup(groupopsdb.GroupOpsV1HistoryGroup{ID: 56, SourceGroupID: 40, SourcePlanID: 30, PlanID: 41,
		ChatReference: "chat-ref", DisplayName: "legacy group", OwnerStaffID: pgtype.Int8{}, InternalMemberCount: 3, ExternalMemberCount: 6,
		OriginalStatus: "active", CreatedAt: historicalStamp(stamp), RemovedAt: pgtype.Timestamptz{}})
	if err != nil || group.OwnerStaffID != nil || group.RemovedAt != nil || !group.CreatedAt.Equal(stamp) {
		t.Fatalf("group = %#v, %v", group, err)
	}

	node, err := groupOpsHistoricalNode(groupopsdb.GroupOpsV1HistoryNode{ID: 61, SourceNodeID: 50, SourcePlanID: 30, PlanID: 41, DayIndex: 2,
		TriggerTime: "09:30", SortOrder: 1, OriginalStatus: "active", ContentPackage: []byte(`{"reference":"opaque","kind":"legacy_package"}`), CreatedAt: historicalStamp(stamp), UpdatedAt: historicalStamp(stamp)})
	if err != nil || node.ID != 61 || !sameJSON(node.ContentPackage, []byte(`{"kind":"legacy_package","reference":"opaque"}`)) {
		t.Fatalf("node = %#v, %v", node, err)
	}
}

func TestHistoricalStorePostgreSQLRoundTripRollback(t *testing.T) {
	if *groupOpsHistoryTestDatabaseURL == "" {
		t.Skip("-groupops-history-test-database-url is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *groupOpsHistoryTestDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	repository := NewRepository()
	uow := platformstore.NewUnitOfWork(pool)
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 123456000, time.FixedZone("source", 8*60*60))
	key := fmt.Sprintf("groupops-v1-history-%d", time.Now().UnixNano())
	var beforePlans, beforeDirectory, createdPlanID int64
	err = uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return err
		}
		beforePlans, err = q.CountGroupOpsHistoricalPlans(tx)
		if err != nil {
			return err
		}
		beforeDirectory, err = q.CountGroupOpsHistoricalDirectory(tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Within(ctx, func(tx context.Context) error {
		plan, err := repository.CreateHistoricalPlan(tx, groupopsport.HistoricalPlan{Plan: groupopsport.Plan{Name: key, Status: groupopsport.PlanArchived, Revision: 1, CreatedBy: 41, UpdatedBy: 41, CreatedAt: stamp, UpdatedAt: stamp},
			SourcePlanID: 30, SourceCode: "legacy-30", PlanType: "sop", OriginalStatus: "active"})
		if err != nil {
			return fmt.Errorf("create plan: %w", err)
		}
		createdPlanID = plan.ID
		chat, err := repository.CreateHistoricalDirectory(tx, groupopsport.HistoricalDirectory{SourceKind: historicalDirectoryGroupChats, SourceID: int64Pointer(10), ChatReference: "chat-legacy", MemberCount: int32Pointer(9), OriginalStatus: "active", RecordedAt: stamp})
		if err != nil {
			return fmt.Errorf("create group chat: %w", err)
		}
		snapshot, err := repository.CreateHistoricalDirectory(tx, groupopsport.HistoricalDirectory{SourceKind: historicalDirectorySnapshots, ChatReference: "chat-legacy", InternalMemberCount: int32Pointer(3), ExternalMemberCount: int32Pointer(6), OriginalStatus: "normal", RecordedAt: stamp})
		if err != nil {
			return fmt.Errorf("create snapshot: %w", err)
		}
		group, err := repository.CreateHistoricalGroup(tx, groupopsport.HistoricalGroup{SourceGroupID: 40, SourcePlanID: 30, PlanID: plan.ID, ChatReference: "chat-legacy", DisplayName: "legacy group", InternalMemberCount: 3, ExternalMemberCount: 6, OriginalStatus: "active", CreatedAt: stamp})
		if err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		node, err := repository.CreateHistoricalNode(tx, groupopsport.HistoricalNode{SourceNodeID: 50, SourcePlanID: 30, PlanID: plan.ID, DayIndex: 2, TriggerTime: "09:30", SortOrder: 1, OriginalStatus: "active", ContentPackage: json.RawMessage(`{"kind":"legacy_package","reference":"opaque"}`), CreatedAt: stamp, UpdatedAt: stamp})
		if err != nil {
			return fmt.Errorf("create node: %w", err)
		}
		if stored, err := repository.GetHistoricalPlan(tx, plan.ID); err != nil || stored.Plan.Status != groupopsport.PlanArchived || !stored.CreatedAt.Equal(stamp) {
			return fmt.Errorf("get plan: %#v %w", stored, err)
		}
		if stored, err := repository.GetHistoricalDirectory(tx, chat.ID); err != nil || stored.SourceKind != historicalDirectoryGroupChats || stored.SourceID == nil || *stored.SourceID != 10 {
			return fmt.Errorf("get chat: %#v %w", stored, err)
		}
		if stored, err := repository.GetHistoricalDirectory(tx, snapshot.ID); err != nil || stored.SourceKind != historicalDirectorySnapshots || stored.SourceID != nil {
			return fmt.Errorf("get snapshot: %#v %w", stored, err)
		}
		if stored, err := repository.GetHistoricalGroup(tx, group.ID); err != nil || stored.PlanID != plan.ID {
			return fmt.Errorf("get group: %#v %w", stored, err)
		}
		if stored, err := repository.GetHistoricalNode(tx, node.ID); err != nil || !sameJSON(stored.ContentPackage, node.ContentPackage) {
			return fmt.Errorf("get node: %#v %w", stored, err)
		}

		reader := NewHistoricalReader(groupOpsHistoryContextUOW{ctx: tx})
		if values, total, err := reader.ListHistoricalPlans(ctx, 1000, 0); err != nil || total != beforePlans+1 || !hasHistoricalPlan(values, plan.ID) {
			return fmt.Errorf("list plans: %#v/%d/%w", values, total, err)
		}
		if values, total, err := reader.ListHistoricalDirectory(ctx, 1000, 0); err != nil || total != beforeDirectory+2 || !hasHistoricalDirectory(values, chat.ID) || !hasHistoricalDirectory(values, snapshot.ID) {
			return fmt.Errorf("list directory: %#v/%d/%w", values, total, err)
		}
		if values, total, err := reader.ListHistoricalGroups(ctx, plan.ID, 10, 0); err != nil || len(values) != 1 || total != 1 || values[0].ID != group.ID {
			return fmt.Errorf("list groups: %#v/%d/%w", values, total, err)
		}
		if values, total, err := reader.ListHistoricalNodes(ctx, plan.ID, 10, 0); err != nil || len(values) != 1 || total != 1 || values[0].ID != node.ID {
			return fmt.Errorf("list nodes: %#v/%d/%w", values, total, err)
		}

		q, err := queries(tx)
		if err != nil {
			return err
		}
		current, err := q.GetGroupOpsPlan(tx, plan.ID)
		if err != nil || current.Status != string(groupopsport.PlanArchived) {
			return fmt.Errorf("current archived plan: %#v %w", current, err)
		}
		if nodes, err := q.ListGroupOpsPlanNodes(tx, plan.ID); err != nil || len(nodes) != 0 {
			return fmt.Errorf("current nodes: %#v %w", nodes, err)
		}
		if groups, err := q.ListGroupOpsPlanGroupAssets(tx, plan.ID); err != nil || len(groups) != 0 {
			return fmt.Errorf("current groups: %#v %w", groups, err)
		}
		if webhook, err := q.GetGroupOpsPlanWebhookDescriptor(tx, plan.ID); err != nil || webhook != "" {
			return fmt.Errorf("empty webhook: %q %w", webhook, err)
		}
		return errGroupOpsHistoryRollback
	})
	if !errors.Is(err, errGroupOpsHistoryRollback) {
		t.Fatalf("round trip = %v", err)
	}

	err = uow.Within(ctx, func(tx context.Context) error {
		q, err := queries(tx)
		if err != nil {
			return err
		}
		plans, err := q.CountGroupOpsHistoricalPlans(tx)
		if err != nil {
			return err
		}
		directory, err := q.CountGroupOpsHistoricalDirectory(tx)
		if err != nil {
			return err
		}
		groups, err := q.CountGroupOpsHistoricalGroups(tx, createdPlanID)
		if err != nil {
			return err
		}
		nodes, err := q.CountGroupOpsHistoricalNodes(tx, createdPlanID)
		if err != nil {
			return err
		}
		if plans != beforePlans || directory != beforeDirectory || groups != 0 || nodes != 0 {
			return fmt.Errorf("rollback counts %d/%d/%d/%d", plans, directory, groups, nodes)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("rollback verification = %v", err)
	}
}

type groupOpsHistoryContextUOW struct{ ctx context.Context }

func (uow groupOpsHistoryContextUOW) Within(_ context.Context, callback func(context.Context) error) error {
	return callback(uow.ctx)
}

var errGroupOpsHistoryRollback = errors.New("rollback Group Ops history")

func historicalStamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func int64Pointer(value int64) *int64 { return &value }

func int32Pointer(value int32) *int32 { return &value }

func hasHistoricalPlan(values []groupopsport.HistoricalPlan, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func hasHistoricalDirectory(values []groupopsport.HistoricalDirectory, id int64) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func sameJSON(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

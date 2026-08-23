package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestRepositoryPostgreSQLAtomicRoundTripAndRollback(t *testing.T) {
	databaseURL := os.Getenv("AICRM_GROUP_OPS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AICRM_GROUP_OPS_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var version string
	if err = pool.QueryRow(ctx, "SHOW server_version_num").Scan(&version); err != nil || version != "160014" {
		t.Fatalf("PostgreSQL version=%q err=%v", version, err)
	}

	repository := NewRepository()
	events := &groupOpsIntegrationEventAppender{}
	uow := platformstore.NewUnitOfWork(pool)
	service := groupopsapp.NewService(groupOpsIntegrationUOW{}, repository, groupOpsIntegrationStaff{}, events)
	key := fmt.Sprintf("group-ops-store-integration-%d", time.Now().UnixNano())
	staffUserID := key + "-staff"
	activeStaffID, err := contactfixture.CreateStaffWithDetails(ctx, pool, staffUserID, "Group Ops integration staff", true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = contactfixture.DeleteStaff(context.Background(), pool, activeStaffID) }()
	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		detail, txErr := service.Create(tx, groupopsport.CreatePlanCommand{Name: "Integration", Actor: 41, IdempotencyKey: key + "-create"})
		if txErr != nil {
			return fmt.Errorf("create: %w", txErr)
		}
		if detail.Plan.ID < 1 || detail.Plan.Status != groupopsport.PlanDraft || detail.ProviderExecutionEligible || detail.RealExternalCallExecuted {
			return errors.New("create readback is invalid")
		}
		detail, txErr = service.AddMember(tx, groupopsport.MemberCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, StaffID: activeStaffID, Actor: 41, IdempotencyKey: key + "-member"})
		if txErr != nil {
			return fmt.Errorf("add member: %w", txErr)
		}
		if len(detail.Members) != 1 {
			return errors.New("member readback is invalid")
		}
		detail, txErr = service.AddGroupAsset(tx, groupopsport.GroupAssetCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, AssetRef: "local-group-9", Actor: 41, IdempotencyKey: key + "-asset"})
		if txErr != nil {
			return fmt.Errorf("add asset: %w", txErr)
		}
		if len(detail.GroupAssets) != 1 || detail.GroupAssets[0].ID < 1 {
			return errors.New("asset readback is invalid")
		}
		detail, txErr = service.AddNode(tx, groupopsport.NodeCreateCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Position: 1, Kind: groupopsport.NodeMessage, MessageText: "stored local content", Actor: 41, IdempotencyKey: key + "-node"})
		if txErr != nil {
			return fmt.Errorf("add node: %w", txErr)
		}
		if len(detail.Nodes) != 1 || detail.Nodes[0].ID < 1 {
			return errors.New("node readback is invalid")
		}
		preview, txErr := service.Preview(tx, detail.Plan.ID)
		if txErr != nil {
			return fmt.Errorf("preview: %w", txErr)
		}
		if !preview.Valid || preview.ProviderExecutionEligible || preview.RealExternalCallExecuted {
			return errors.New("preview is invalid")
		}
		detail, txErr = service.Activate(tx, groupopsport.TransitionCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Actor: 41, IdempotencyKey: key + "-activate"})
		if txErr != nil {
			return fmt.Errorf("activate: %w", txErr)
		}
		if detail.Plan.Status != groupopsport.PlanActive {
			return errors.New("activate readback is invalid")
		}
		var plans, receipts int
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_plans").Scan(&plans); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_operation_receipts WHERE state = 'completed'").Scan(&receipts); txErr != nil {
			return txErr
		}
		if plans != 1 || receipts != 5 || events.count != 5 {
			return groupopsapp.ErrUnavailable
		}
		return errGroupOpsRepositoryRollback
	})
	if !errors.Is(err, errGroupOpsRepositoryRollback) {
		t.Fatalf("round trip=%v", err)
	}

	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		var plans, receipts, eventRows int
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_plans").Scan(&plans); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_operation_receipts").Scan(&receipts); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM event_log WHERE idempotency_key LIKE $1", key+"%").Scan(&eventRows); txErr != nil {
			return txErr
		}
		if plans != 0 || receipts != 0 || eventRows != 0 {
			return groupopsapp.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		t.Fatalf("rollback verification=%v", err)
	}
}

type groupOpsIntegrationUOW struct{}

func (groupOpsIntegrationUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type groupOpsIntegrationStaff struct{}

func (groupOpsIntegrationStaff) IsActiveStaff(context.Context, int64) (bool, error) { return true, nil }

type groupOpsIntegrationEventAppender struct{ count int }

var _ eventport.Appender = (*groupOpsIntegrationEventAppender)(nil)

func (events *groupOpsIntegrationEventAppender) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	events.count++
	return eventport.EventID(events.count), nil
}

var errGroupOpsRepositoryRollback = errors.New("rollback group ops repository integration")

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	contactfixture "github.com/qianlan33333-png/AI-CRM-v2/acceptance/contactfixture"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
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
	events := groupOpsIntegrationEventAppender{}
	uow := platformstore.NewUnitOfWork(pool)
	service := groupopsapp.NewService(groupOpsIntegrationUOW{}, repository, contactstore.NewStaffDirectoryRepository(pool), events)
	key := fmt.Sprintf("group-ops-store-integration-%d", time.Now().UnixNano())
	activeStaffID, err := contactfixture.CreateStaffRecord(ctx, pool, key+"-staff", "Group Ops integration staff", true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if fixtureErr := contactfixture.DeleteStaff(context.Background(), pool, activeStaffID); fixtureErr != nil {
			t.Errorf("delete active staff fixture: %v", fixtureErr)
		}
	}()
	assertActiveStaffShareLock(t, ctx, pool, uow, activeStaffID)
	err = uow.Within(ctx, func(tx context.Context) error {
		db, txErr := platformstore.TxFromContext(tx)
		if txErr != nil {
			return txErr
		}
		active, txErr := contactstore.NewStaffDirectoryRepository(pool).IsActiveStaff(tx, activeStaffID)
		if txErr != nil {
			return fmt.Errorf("active staff lock: %w", txErr)
		}
		if !active {
			return errors.New("active staff lock returned false")
		}
		var shareHeld bool
		if txErr = db.QueryRow(tx, `SELECT EXISTS (
  SELECT 1 FROM pg_locks
  WHERE pid = pg_backend_pid() AND relation = 'staff'::regclass AND mode = 'RowShareLock'
)`).Scan(&shareHeld); txErr != nil {
			return txErr
		}
		if !shareHeld {
			return errors.New("active staff reader did not hold share lock")
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
		var plans, receipts, eventRows int
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_plans").Scan(&plans); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM group_ops_operation_receipts WHERE state = 'completed'").Scan(&receipts); txErr != nil {
			return txErr
		}
		if txErr = db.QueryRow(tx, "SELECT count(*) FROM event_log WHERE idempotency_key LIKE $1", key+"%").Scan(&eventRows); txErr != nil {
			return txErr
		}
		if plans != 1 || receipts != 5 || eventRows != 5 {
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

func assertActiveStaffShareLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, uow *platformstore.UnitOfWork, staffID int64) {
	t.Helper()
	reader := contactstore.NewStaffDirectoryRepository(pool)
	err := uow.Within(ctx, func(tx context.Context) error {
		active, err := reader.IsActiveStaff(tx, staffID)
		if err != nil || !active {
			return fmt.Errorf("active staff reader: active=%t err=%w", active, err)
		}
		updateCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err = contactfixture.SetStaffActive(updateCtx, pool, staffID, false)
		if isLockTimeout(err) {
			return nil
		}
		return fmt.Errorf("deactivation unexpectedly passed share lock: %w", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = contactfixture.SetStaffActive(ctx, pool, staffID, false); err != nil {
		t.Fatalf("deactivation after reader commit: %v", err)
	}
	if err = contactfixture.SetStaffActive(ctx, pool, staffID, true); err != nil {
		t.Fatalf("restore active staff: %v", err)
	}
}

func isLockTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014"
}

type groupOpsIntegrationUOW struct{}

func (groupOpsIntegrationUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type groupOpsIntegrationEventAppender struct{}

var _ eventport.Appender = groupOpsIntegrationEventAppender{}

func (groupOpsIntegrationEventAppender) Append(ctx context.Context, event eventport.Event) (eventport.EventID, error) {
	db, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	var id int64
	err = db.QueryRow(ctx, `INSERT INTO event_log (event_type, payload, occurred_at, idempotency_key)
VALUES ($1, $2::jsonb, $3, $4)
RETURNING id`, event.Type, event.Payload, event.OccurredAt, event.IdempotencyKey).Scan(&id)
	if err != nil {
		return 0, err
	}
	return eventport.EventID(id), nil
}

var errGroupOpsRepositoryRollback = errors.New("rollback group ops repository integration")

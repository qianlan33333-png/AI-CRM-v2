package groupops_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
	groupopsstore "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

func TestRuntimeRepositoryPostgreSQLBindsSnapshotsToEERAtomically(t *testing.T) {
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
	uow := platformstore.NewUnitOfWork(pool)
	repository := groupopsstore.NewRepository()
	planService := groupopsapp.NewService(uow, repository, runtimeStaff{}, runtimeEvents{})
	effectsRepository := eerstore.NewRepository(pool, uow)
	effects, err := eer.NewService(effectsRepository)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := groupopsapp.NewRuntimeService(uow, repository, repository, effects, runtimeDirectory{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("group-ops-runtime-integration-%d", time.Now().UnixNano())
	detail, err := planService.Create(ctx, groupopsport.CreatePlanCommand{Name: "Runtime Integration", Actor: 41, IdempotencyKey: key + "-create"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = planService.AddMember(ctx, groupopsport.MemberCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, StaffID: 7, Actor: 41, IdempotencyKey: key + "-member"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = planService.AddGroupAsset(ctx, groupopsport.GroupAssetCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, AssetRef: "chat:runtime", Actor: 41, IdempotencyKey: key + "-group"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = planService.AddNode(ctx, groupopsport.NodeCreateCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Position: 1, Kind: groupopsport.NodeMessage, MessageText: "immutable content", MaterialRef: "material:runtime", Actor: 41, IdempotencyKey: key + "-node"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = planService.Activate(ctx, groupopsport.TransitionCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Actor: 41, IdempotencyKey: key + "-activate"})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := runtime.AcceptPlan(ctx, groupopsport.AcceptPlanCommand{PlanID: detail.Plan.ID, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:integration", IdempotencyKey: key + "-broadcast"})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accepted != 1 || summary.ProviderAccepted != 0 || summary.DeliveryProven != 0 || summary.ProviderExecutionEligible || summary.RealExternalCallExecuted {
		t.Fatalf("unsafe runtime summary: %+v", summary)
	}
	var owner, kind, state, content, material string
	err = pool.QueryRow(ctx, `
		SELECT ee.owner, ee.kind, ge.state, ge.content_snapshot::text, ge.material_snapshot::text
		FROM group_ops_executions ge
		JOIN external_effects ee ON ee.id = ge.external_effect_id
		WHERE ge.plan_id = $1`, detail.Plan.ID).Scan(&owner, &kind, &state, &content, &material)
	if err != nil {
		t.Fatal(err)
	}
	if owner != "group_ops" || kind != "group_ops_broadcast" || state != "accepted" || content != `{"message_text": "immutable content"}` || material != `{"reference": "material:runtime"}` {
		t.Fatalf("runtime snapshot binding owner=%q kind=%q state=%q content=%q material=%q", owner, kind, state, content, material)
	}
}

type runtimeStaff struct{}

func (runtimeStaff) IsActiveStaff(context.Context, int64) (bool, error) { return true, nil }

type runtimeDirectory struct{}

func (runtimeDirectory) ListEligibleStaff(context.Context) ([]contactport.StaffDirectoryEntry, error) {
	return []contactport.StaffDirectoryEntry{}, nil
}

type runtimeEvents struct{}

func (runtimeEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 1, nil
}

package groupops_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	eerport "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	eerstore "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/store"
	groupopsapp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/app"
	groupopshttp "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/http"
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
	runtime, err := groupopsapp.NewRuntimeService(uow, repository, repository, effects, runtimeDirectory{}, nil, runtimeSender{}, runtimeJobs{})
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
	detail, err = planService.AddNode(ctx, groupopsport.NodeCreateCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Position: 1, Kind: groupopsport.NodeMessage, MessageText: "immutable content", Actor: 41, IdempotencyKey: key + "-node"})
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
	if summary.Accepted != 1 || summary.ProviderAccepted != 0 || summary.DeliveryProven != 0 || !summary.ProviderExecutionEligible || summary.RealExternalCallExecuted {
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
	if owner != "group_ops" || kind != "group_ops_broadcast" || state != "accepted" || content != `{"node_kind": "message", "message_text": "immutable content", "schema_version": 1}` || material != `{"node_kind": "message", "reference": "", "schema_version": 1}` {
		t.Fatalf("runtime snapshot binding owner=%q kind=%q state=%q content=%q material=%q", owner, kind, state, content, material)
	}
}

func TestRuntimeHTTPPostgreSQLReceiptReplayConflictAndRollback(t *testing.T) {
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
	runtime, err := groupopsapp.NewRuntimeService(uow, repository, repository, effects, runtimeDirectory{}, nil, runtimeSender{}, runtimeJobs{})
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("group-ops-runtime-http-%d", time.Now().UnixNano())
	detail := createActiveRuntimePlan(t, ctx, planService, key)
	callRunDue := func(key string, service *groupopsapp.RuntimeService) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, groupopshttp.PlansPath+fmt.Sprintf("/%d/run-due", detail.Plan.ID), nil)
		request.Header.Set("Idempotency-Key", key)
		request = request.WithContext(runtimeAuthorizedContext(request.Context()))
		response := httptest.NewRecorder()
		groupopshttp.NewWithRuntime(planService, service, nil).RunDue(response, request)
		return response
	}
	protocols := runtimeProtocol{}
	callBroadcast := func(idempotencyKey string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, groupopshttp.BroadcastPath, strings.NewReader(fmt.Sprintf(`{"plan_id":%d}`, detail.Plan.ID)))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		response := httptest.NewRecorder()
		groupopshttp.NewWithRuntime(planService, runtime, protocols).Broadcast(response, request)
		return response
	}
	callWebhook := func(idempotencyKey string) *httptest.ResponseRecorder {
		path := strings.Replace(groupopshttp.WebhookPath, "{webhook_key}", detail.WebhookDescriptor.Reference, 1)
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"event":"local"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		response := httptest.NewRecorder()
		groupopshttp.NewWithRuntime(planService, runtime, protocols).Webhook(response, request)
		return response
	}
	if response := callRunDue(key+"-due", runtime); response.Code != http.StatusAccepted {
		t.Fatalf("first status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callRunDue(key+"-due", runtime); response.Code != http.StatusAccepted {
		t.Fatalf("replay status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callBroadcast(key + "-broadcast"); response.Code != http.StatusAccepted {
		t.Fatalf("broadcast status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callBroadcast(key + "-broadcast"); response.Code != http.StatusAccepted {
		t.Fatalf("broadcast replay status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callWebhook(key + "-webhook"); response.Code != http.StatusAccepted {
		t.Fatalf("webhook status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callWebhook(key + "-webhook"); response.Code != http.StatusAccepted {
		t.Fatalf("webhook replay status=%d body=%s", response.Code, response.Body.String())
	}
	var runs, executions, receipts, effectsBefore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_runs WHERE plan_id = $1`, detail.Plan.ID).Scan(&runs); err != nil || runs != 3 {
		t.Fatalf("runs=%d err=%v", runs, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_executions WHERE plan_id = $1`, detail.Plan.ID).Scan(&executions); err != nil || executions != 3 {
		t.Fatalf("executions=%d err=%v", executions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_operation_receipts WHERE operation = 'runtime_run_due' AND actor_scope = 'admin:7' AND state = 'completed'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("receipts=%d err=%v", receipts, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE group_ops_plans SET revision = revision + 1, updated_at = now() WHERE id = $1`, detail.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if response := callRunDue(key+"-due", runtime); response.Code != http.StatusConflict {
		t.Fatalf("changed snapshot status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callBroadcast(key + "-broadcast"); response.Code != http.StatusConflict {
		t.Fatalf("changed broadcast status=%d body=%s", response.Code, response.Body.String())
	}
	if response := callWebhook(key + "-webhook"); response.Code != http.StatusConflict {
		t.Fatalf("changed webhook status=%d body=%s", response.Code, response.Body.String())
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_runs WHERE plan_id = $1`, detail.Plan.ID).Scan(&runs); err != nil || runs != 3 {
		t.Fatalf("conflict runs=%d err=%v", runs, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM external_effects WHERE owner = 'group_ops'`).Scan(&effectsBefore); err != nil || effectsBefore < 3 {
		t.Fatalf("effects=%d err=%v", effectsBefore, err)
	}
	expectedEffects := effectsBefore

	failingDetail := createActiveRuntimePlan(t, ctx, planService, key+"-rollback")
	failingRuntime, err := groupopsapp.NewRuntimeService(uow, repository, repository, runtimeFailingEffects{}, runtimeDirectory{}, nil, runtimeSender{}, runtimeJobs{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, groupopshttp.PlansPath+fmt.Sprintf("/%d/run-due", failingDetail.Plan.ID), nil)
	request.Header.Set("Idempotency-Key", key+"-rollback-due")
	request = request.WithContext(runtimeAuthorizedContext(request.Context()))
	response := httptest.NewRecorder()
	groupopshttp.NewWithRuntime(planService, failingRuntime, nil).RunDue(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("rollback status=%d body=%s", response.Code, response.Body.String())
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_runs WHERE plan_id = $1`, failingDetail.Plan.ID).Scan(&runs); err != nil || runs != 0 {
		t.Fatalf("rollback runs=%d err=%v", runs, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM external_effects WHERE owner = 'group_ops'`).Scan(&effectsBefore); err != nil || effectsBefore != expectedEffects {
		t.Fatalf("rollback effects=%d err=%v", effectsBefore, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_operation_receipts WHERE operation = 'runtime_run_due' AND actor_scope = 'admin:7' AND state = 'completed'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("rollback receipts=%d err=%v", receipts, err)
	}
	request = httptest.NewRequest(http.MethodPost, groupopshttp.PlansPath+fmt.Sprintf("/%d/run-due", failingDetail.Plan.ID), nil)
	request.Header.Set("Idempotency-Key", key+"-rollback-due")
	request = request.WithContext(runtimeAuthorizedContext(request.Context()))
	response = httptest.NewRecorder()
	groupopshttp.NewWithRuntime(planService, runtime, nil).RunDue(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("rollback retry status=%d body=%s", response.Code, response.Body.String())
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM group_ops_runs WHERE plan_id = $1`, failingDetail.Plan.ID).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("rollback retry runs=%d err=%v", runs, err)
	}
}

func createActiveRuntimePlan(t *testing.T, ctx context.Context, service *groupopsapp.Service, key string) groupopsport.Detail {
	t.Helper()
	detail, err := service.Create(ctx, groupopsport.CreatePlanCommand{Name: "Runtime HTTP " + key[len(key)-12:], Actor: 41, IdempotencyKey: key + "-create"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.AddMember(ctx, groupopsport.MemberCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, StaffID: 7, Actor: 41, IdempotencyKey: key + "-member"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.AddGroupAsset(ctx, groupopsport.GroupAssetCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, AssetRef: "chat:http-" + key[len(key)-8:], Actor: 41, IdempotencyKey: key + "-group"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.AddNode(ctx, groupopsport.NodeCreateCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Position: 1, Kind: groupopsport.NodeMessage, MessageText: "http receipt", Actor: 41, IdempotencyKey: key + "-node"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.PutWebhookDescriptor(ctx, groupopsport.WebhookDescriptorCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Reference: "hook-" + key[len(key)-8:], Actor: 41, IdempotencyKey: key + "-webhook-descriptor"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Activate(ctx, groupopsport.TransitionCommand{PlanID: detail.Plan.ID, ExpectedRevision: detail.Plan.Revision, Actor: 41, IdempotencyKey: key + "-activate"})
	if err != nil {
		t.Fatal(err)
	}
	return detail
}

func runtimeAuthorizedContext(ctx context.Context) context.Context {
	ctx = authport.WithAuthenticatedSession(ctx, authport.Principal{AdminUserID: 7, Role: authport.RoleOps}, authport.SessionRef("runtime-http"))
	ctx, _ = authport.WithAuthorization(ctx, authport.Authorization{Capability: authport.CapabilityOperationsManage, Scope: authport.ScopeGlobal})
	return ctx
}

type runtimeFailingEffects struct{}

type runtimeProtocol struct{}

func (runtimeProtocol) Authenticate(context.Context, *http.Request, string, string, []byte) (groupopshttp.ProtocolPrincipal, error) {
	return groupopshttp.ProtocolPrincipal{ID: "runtime-http-event-0001"}, nil
}

func (runtimeFailingEffects) Accept(context.Context, eerport.AcceptCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, errors.New("local EER test failure")
}

func (runtimeFailingEffects) Queue(context.Context, eerport.QueueCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, errors.New("unexpected queue")
}

func (runtimeFailingEffects) Reconcile(context.Context, eerport.ReconcileCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, errors.New("unexpected reconcile")
}

type runtimeStaff struct{}

func (runtimeStaff) IsActiveStaff(context.Context, int64) (bool, error) { return true, nil }

type runtimeDirectory struct{}

func (runtimeDirectory) ListEligibleStaff(context.Context) ([]contactport.StaffDirectoryEntry, error) {
	return []contactport.StaffDirectoryEntry{}, nil
}

type runtimeSender struct{}

func (runtimeSender) ResolveExecutionSender(context.Context, string) (string, bool, error) {
	return "staff-7", true, nil
}

type runtimeJobs struct{}

func (runtimeJobs) Insert(_ context.Context, args groupopsapp.GroupOpsDispatchJobArgs, generation int64, scheduled time.Time) (eerport.RiverJobLink, error) {
	return eerport.RiverJobLink{JobID: 1, Generation: generation, Queue: "outbound", ArgsDigest: eerport.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000"), ScheduledAt: scheduled}, nil
}

type runtimeEvents struct{}

func (runtimeEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 1, nil
}

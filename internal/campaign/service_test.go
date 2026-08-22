package campaign

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalLifecycleCASIdempotencyAndAudit(t *testing.T) {
	svc, store := testService(t, testCampaign("spring", ApprovalDraft, RuntimeIdle, 1))
	add, err := svc.AddStep(context.Background(), StepCreateCommand{CampaignCode: "spring", ExpectedVersion: 1, DelayMinutes: 30, Content: "你好", Actor: Actor{ID: 7}, IdempotencyKey: "key-add-00000001"})
	if err != nil || add.Campaign.Version != 2 || add.Command != nil || add.Plan != nil || !add.LocalProjection || add.RealSend || add.RuntimeExecuted {
		t.Fatalf("add=%+v err=%v", add, err)
	}
	updated, err := svc.UpdateStep(context.Background(), StepUpdateCommand{CampaignCode: "spring", StepIndex: 1, ExpectedVersion: 2, DelayMinutes: ptr(int32(60)), Actor: Actor{ID: 7}, IdempotencyKey: "key-update-000001"})
	if err != nil || updated.Campaign.Version != 3 {
		t.Fatalf("update=%+v err=%v", updated, err)
	}
	approved, err := svc.Approve(context.Background(), VersionedCommand{CampaignCode: "spring", ExpectedVersion: 3, Actor: Actor{ID: 7}, IdempotencyKey: "key-approve-00001"})
	if err != nil || approved.Campaign.ApprovalStatus != ApprovalApproved || approved.Command != nil {
		t.Fatalf("approve=%+v err=%v", approved, err)
	}
	started, err := svc.Start(context.Background(), VersionedCommand{CampaignCode: "spring", ExpectedVersion: 4, Actor: Actor{ID: 7}, IdempotencyKey: "key-start-0000001"})
	if err != nil || started.Campaign.RuntimeStatus != RuntimePlanned || started.Plan == nil || started.Command == nil || started.Command.RealSend || started.Command.RuntimeExecuted || started.Command.Operation != CommandStart {
		t.Fatalf("start=%+v err=%v", started, err)
	}
	replay, err := svc.Start(context.Background(), VersionedCommand{CampaignCode: "spring", ExpectedVersion: 4, Actor: Actor{ID: 7}, IdempotencyKey: "key-start-0000001"})
	if err != nil || replay.Command == nil || replay.Command.ID != started.Command.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	paused, err := svc.Pause(context.Background(), VersionedCommand{CampaignCode: "spring", ExpectedVersion: 5, Actor: Actor{ID: 7}, IdempotencyKey: "key-pause-0000001"})
	if err != nil || paused.Campaign.RuntimeStatus != RuntimePaused || paused.Campaign.Version != 6 {
		t.Fatalf("pause=%+v err=%v", paused, err)
	}
	if got := store.AuditEvents(); len(got) != 5 || got[3].Type != "cloud_campaign.started" {
		t.Fatalf("audit=%+v", got)
	}
}

func TestDraftOnlyDeleteAndAuditRollback(t *testing.T) {
	svc, store := testService(t, testCampaign("draft", ApprovalDraft, RuntimeIdle, 1), testCampaign("approved", ApprovalApproved, RuntimeIdle, 1))
	if _, err := svc.Delete(context.Background(), VersionedCommand{CampaignCode: "approved", ExpectedVersion: 1, Actor: Actor{ID: 9}, IdempotencyKey: "key-delete-000001"}); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("delete approved err=%v", err)
	}
	store.FailAudit(true)
	if _, err := svc.AddStep(context.Background(), StepCreateCommand{CampaignCode: "draft", ExpectedVersion: 1, DelayMinutes: 0, Content: "content", Actor: Actor{ID: 9}, IdempotencyKey: "key-add-rollback01"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("audit failure err=%v", err)
	}
	store.FailAudit(false)
	detail, err := svc.Detail(context.Background(), "draft")
	if err != nil || detail.Campaign.Version != 1 || len(detail.Steps) != 0 {
		t.Fatalf("rollback detail=%+v err=%v", detail, err)
	}
	deleted, err := svc.Delete(context.Background(), VersionedCommand{CampaignCode: "draft", ExpectedVersion: 1, Actor: Actor{ID: 9}, IdempotencyKey: "key-delete-000002"})
	if err != nil || !deleted.Deleted {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	if _, err = svc.Detail(context.Background(), "draft"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("detail after delete err=%v", err)
	}
}

func TestBatchStartSkipsUnstartableWithoutExternalExecution(t *testing.T) {
	svc, store := testService(t, testCampaign("a", ApprovalApproved, RuntimeIdle, 1), testCampaign("b", ApprovalDraft, RuntimeIdle, 1))
	store.SeedSteps("a", []Step{{Index: 1, DelayMinutes: 0, Content: "one"}})
	store.SeedSteps("b", []Step{{Index: 1, DelayMinutes: 0, Content: "two"}})
	result, err := svc.BatchStart(context.Background(), BatchStartCommand{Items: []BatchStartItem{{CampaignCode: "a", ExpectedVersion: 1}, {CampaignCode: "b", ExpectedVersion: 1}}, Actor: Actor{ID: 5}, IdempotencyKey: "key-batch-start01"})
	if err != nil || len(result.Started) != 1 || len(result.Skipped) != 1 || len(result.Failed) != 0 || result.Started[0].Command == nil || result.Started[0].Command.Operation != CommandBatchStart || result.Started[0].Command.RealSend || result.Started[0].Command.RuntimeExecuted {
		t.Fatalf("batch=%+v err=%v", result, err)
	}
	if _, err := svc.BatchStart(context.Background(), BatchStartCommand{Items: []BatchStartItem{{CampaignCode: "a", ExpectedVersion: 1}, {CampaignCode: "b", ExpectedVersion: 1}}, Actor: Actor{ID: 5}, IdempotencyKey: "key-batch-start01"}); err != nil {
		t.Fatalf("batch replay err=%v", err)
	}
}

func TestStartRejectsMalformedLocalPlanCommandFromAdapter(t *testing.T) {
	cases := map[string]func(Plan, Command) (Plan, Command){
		"zero IDs":               func(plan Plan, command Command) (Plan, Command) { plan.ID = 0; command.ID = 0; return plan, command },
		"wrong plan campaign":    func(plan Plan, command Command) (Plan, Command) { plan.CampaignCode = "other"; return plan, command },
		"wrong command campaign": func(plan Plan, command Command) (Plan, Command) { command.CampaignCode = "other"; return plan, command },
		"wrong version":          func(plan Plan, command Command) (Plan, Command) { plan.CampaignVersion++; return plan, command },
		"wrong step count":       func(plan Plan, command Command) (Plan, Command) { plan.StepCount++; return plan, command },
		"wrong plan timestamp": func(plan Plan, command Command) (Plan, Command) {
			plan.CreatedAt = plan.CreatedAt.Add(time.Second)
			return plan, command
		},
		"wrong command timestamp": func(plan Plan, command Command) (Plan, Command) {
			command.CreatedAt = command.CreatedAt.Add(time.Second)
			return plan, command
		},
		"wrong plan link": func(plan Plan, command Command) (Plan, Command) { command.PlanID++; return plan, command },
		"wrong operation": func(plan Plan, command Command) (Plan, Command) {
			if command.Operation == CommandStart {
				command.Operation = CommandBatchStart
			} else {
				command.Operation = CommandStart
			}
			return plan, command
		},
		"external flags": func(plan Plan, command Command) (Plan, Command) {
			command.RealSend = true
			command.RuntimeExecuted = true
			return plan, command
		},
	}
	for name, alter := range cases {
		t.Run(name+" single", func(t *testing.T) { assertMalformedStartRejected(t, alter, false) })
		t.Run(name+" batch", func(t *testing.T) { assertMalformedStartRejected(t, alter, true) })
	}
}

type malformedPlanCommandStore struct {
	*MemoryStore
	alter func(Plan, Command) (Plan, Command)
}

func (store *malformedPlanCommandStore) CreateLocalPlanAndCommand(ctx context.Context, campaign Campaign, stepCount int32, operation CommandOperation, now time.Time) (Plan, Command, error) {
	plan, command, err := store.MemoryStore.CreateLocalPlanAndCommand(ctx, campaign, stepCount, operation, now)
	if err != nil {
		return plan, command, err
	}
	plan, command = store.alter(plan, command)
	return plan, command, nil
}

func assertMalformedStartRejected(t *testing.T, alter func(Plan, Command) (Plan, Command), batch bool) {
	t.Helper()
	seed := testCampaign("spring", ApprovalApproved, RuntimeIdle, 1)
	backing := NewMemoryStore(seed)
	backing.SeedSteps("spring", []Step{{Index: 1, DelayMinutes: 0, Content: "one"}})
	store := &malformedPlanCommandStore{MemoryStore: backing, alter: alter}
	svc, err := NewService(store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC) }
	if batch {
		_, err = svc.BatchStart(context.Background(), BatchStartCommand{Items: []BatchStartItem{{CampaignCode: "spring", ExpectedVersion: 1}}, Actor: Actor{ID: 8}, IdempotencyKey: "key-malformed-bat"})
	} else {
		_, err = svc.Start(context.Background(), VersionedCommand{CampaignCode: "spring", ExpectedVersion: 1, Actor: Actor{ID: 8}, IdempotencyKey: "key-malformed-one"})
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("start err=%v", err)
	}
	detail, err := svc.Detail(context.Background(), "spring")
	if err != nil || detail.Campaign.Version != 1 || detail.Campaign.RuntimeStatus != RuntimeIdle || len(backing.AuditEvents()) != 0 {
		t.Fatalf("rollback detail=%+v err=%v audit=%+v", detail, err, backing.AuditEvents())
	}
}

func testService(t *testing.T, seed ...Campaign) (*Service, *MemoryStore) {
	t.Helper()
	store := NewMemoryStore(seed...)
	svc, err := NewService(store, store, store)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 22, 1, 2, 3, 0, time.UTC) }
	return svc, store
}
func testCampaign(code string, approval ApprovalStatus, runtime RuntimeStatus, version int64) Campaign {
	now := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	return Campaign{Code: code, Name: "Campaign " + code, ApprovalStatus: approval, RuntimeStatus: runtime, Version: version, CreatedBy: 1, UpdatedBy: 1, CreatedAt: now, UpdatedAt: now}
}
func ptr[T any](value T) *T { return &value }

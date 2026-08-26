package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	groupopsport "github.com/qianlan33333-png/AI-CRM-v2/internal/groupops/port"
)

func TestRuntimeRunDueBindsImmutableContentAndMaterialSnapshotsToEER(t *testing.T) {
	service, runtime, effects := newRuntimeFixture(t)
	preview, err := service.PreviewRunDue(context.Background(), 91)
	if err != nil || preview.DueExecutionCount != 2 || preview.NextDueAt == nil || !preview.NextDueAt.Equal(time.Date(2026, 8, 25, 8, 15, 0, 0, time.UTC)) {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	summary, err := service.RunDue(context.Background(), groupopsport.RunDueCommand{PlanID: 91, ActorID: 7, IdempotencyKey: "group-ops-run-due-0001"})
	if err != nil || summary.Accepted != 2 || len(summary.Executions) != 2 || effects.accepts != 2 {
		t.Fatalf("summary=%+v err=%v accepts=%d", summary, err, effects.accepts)
	}
	for _, draft := range runtime.drafts {
		if string(draft.ContentSnapshot) != `{"schema_version":1,"node_kind":"message","message_text":"first"}` || string(draft.MaterialSnapshot) != `{"schema_version":1,"node_kind":"message","reference":""}` || draft.ExternalEffectID == "" {
			t.Fatalf("draft=%+v", draft)
		}
	}
	if !summary.ProviderExecutionEligible || summary.RealExternalCallExecuted || summary.ProviderAccepted != 0 || summary.DeliveryProven != 0 {
		t.Fatalf("unsafe summary=%+v", summary)
	}

	replayed, err := service.RunDue(context.Background(), groupopsport.RunDueCommand{PlanID: 91, ActorID: 7, IdempotencyKey: "group-ops-run-due-0001"})
	if err != nil || replayed.Run.ID != summary.Run.ID || effects.accepts != 2 || len(runtime.drafts) != 2 {
		t.Fatalf("replay=%+v err=%v accepts=%d drafts=%d", replayed, err, effects.accepts, len(runtime.drafts))
	}
}

func TestRuntimeBroadcastUsesIdempotencyAndDoesNotQueueProvider(t *testing.T) {
	service, runtime, effects := newRuntimeFixture(t)
	command := groupopsport.AcceptPlanCommand{PlanID: 91, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:group-broadcast", IdempotencyKey: "group-ops-broadcast-0001"}
	first, err := service.AcceptPlan(context.Background(), command)
	if err != nil || first.Accepted != 4 || effects.accepts != 4 {
		t.Fatalf("first=%+v err=%v accepts=%d", first, err, effects.accepts)
	}
	replayed, err := service.AcceptPlan(context.Background(), command)
	if err != nil || replayed.Run.ID != first.Run.ID || effects.accepts != 4 {
		t.Fatalf("replayed=%+v err=%v accepts=%d", replayed, err, effects.accepts)
	}
	command.IdempotencyKey = "group-ops-broadcast-0002"
	second, err := service.AcceptPlan(context.Background(), command)
	if err != nil || second.Run.ID == first.Run.ID || effects.accepts != 8 || len(runtime.drafts) != 8 {
		t.Fatalf("second=%+v err=%v accepts=%d drafts=%d", second, err, effects.accepts, len(runtime.drafts))
	}
}

func TestRuntimeReceiptRejectsChangedPlanSnapshotWithoutNewRunOrEffect(t *testing.T) {
	service, runtime, effects := newRuntimeFixture(t)
	command := groupopsport.AcceptPlanCommand{PlanID: 91, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:group-broadcast", IdempotencyKey: "group-ops-broadcast-receipt-01"}
	first, err := service.AcceptPlan(context.Background(), command)
	if err != nil || first.Accepted != 4 || effects.accepts != 4 {
		t.Fatalf("first=%+v err=%v accepts=%d", first, err, effects.accepts)
	}
	plans := service.plans.(*testStore)
	detail := plans.details[91]
	detail.Plan.Revision++
	detail.Nodes[0].MessageText = "changed"
	plans.details[91] = detail
	_, err = service.AcceptPlan(context.Background(), command)
	if !errors.Is(err, ErrConflict) || effects.accepts != 4 || len(runtime.runs) != 1 || len(runtime.drafts) != 4 {
		t.Fatalf("err=%v accepts=%d runs=%d drafts=%d", err, effects.accepts, len(runtime.runs), len(runtime.drafts))
	}
}

func TestRuntimeManualReconcilePersistsMatchingEERAndExecutionFacts(t *testing.T) {
	service, _, effects := newRuntimeFixture(t)
	summary, err := service.RunDue(context.Background(), groupopsport.RunDueCommand{PlanID: 91, ActorID: 7, IdempotencyKey: "group-ops-run-due-0002"})
	if err != nil {
		t.Fatal(err)
	}
	execution := summary.Executions[0]
	execution, err = service.ProjectExecutionOutcome(context.Background(), groupopsport.ExecutionOutcomeCommand{ExecutionID: execution.ID, State: groupopsport.ExecutionOutcomeUnknown, AttemptCount: 1})
	if err != nil || execution.State != groupopsport.ExecutionOutcomeUnknown || execution.ProviderAccepted || execution.ProviderReceiptPresent {
		t.Fatalf("unknown=%+v err=%v", execution, err)
	}
	reconciled, err := service.ManualReconcile(context.Background(), groupopsport.ManualReconcileCommand{
		ExecutionID: execution.ID, ActorID: 7, IdempotencyKey: "group-ops-reconcile-0001", Generation: 2, Fence: 3,
		LeaseExpiresAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC), EvidenceDigest: string(runtimeDigest("evidence", "official-provider-receipt")), DeliveryProven: true,
	})
	if err != nil || reconciled.State != groupopsport.ExecutionReconciled || reconciled.ProviderAccepted || reconciled.DeliveryProven || !reconciled.ReconciliationEvidencePresent || effects.reconciles != 1 {
		t.Fatalf("reconciled=%+v err=%v reconciles=%d", reconciled, err, effects.reconciles)
	}
	_, err = service.ManualReconcile(context.Background(), groupopsport.ManualReconcileCommand{
		ExecutionID: execution.ID, ActorID: 7, IdempotencyKey: "group-ops-reconcile-0002", Generation: 2, Fence: 3,
		LeaseExpiresAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC), EvidenceDigest: string(runtimeDigest("evidence", "official-provider-receipt")), DeliveryProven: false,
	})
	if !errors.Is(err, ErrStateConflict) || effects.reconciles != 1 {
		t.Fatalf("second reconcile err=%v reconciles=%d", err, effects.reconciles)
	}
	if _, err = service.ManualReconcile(context.Background(), groupopsport.ManualReconcileCommand{ExecutionID: execution.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid reconcile err=%v", err)
	}
}

func TestRuntimeManualReconcileOnlyVerifierCanEstablishDelivery(t *testing.T) {
	service, _, _ := newRuntimeFixture(t, evidenceVerifierStub{delivery: true})
	summary, err := service.RunDue(context.Background(), groupopsport.RunDueCommand{PlanID: 91, ActorID: 7, IdempotencyKey: "group-ops-run-due-verified"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := service.ProjectExecutionOutcome(context.Background(), groupopsport.ExecutionOutcomeCommand{ExecutionID: summary.Executions[0].ID, State: groupopsport.ExecutionOutcomeUnknown, AttemptCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := service.ManualReconcile(context.Background(), groupopsport.ManualReconcileCommand{ExecutionID: execution.ID, ActorID: 7, IdempotencyKey: "group-ops-reconcile-verified", Generation: 2, Fence: 3, LeaseExpiresAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC), EvidenceDigest: string(runtimeDigest("evidence", "verified")), DeliveryProven: false})
	if err != nil || !reconciled.ProviderAccepted || !reconciled.DeliveryProven {
		t.Fatalf("reconciled=%+v err=%v", reconciled, err)
	}
}

func TestRuntimeProjectsProviderAcceptanceAndDeliveryAsSeparateFacts(t *testing.T) {
	service, _, _ := newRuntimeFixture(t)
	summary, err := service.RunDue(context.Background(), groupopsport.RunDueCommand{PlanID: 91, ActorID: 7, IdempotencyKey: "group-ops-run-due-0003"})
	if err != nil {
		t.Fatal(err)
	}
	receipt := string(runtimeDigest("provider-receipt", "accepted"))
	accepted, err := service.ProjectExecutionOutcome(context.Background(), groupopsport.ExecutionOutcomeCommand{ExecutionID: summary.Executions[0].ID, State: groupopsport.ExecutionProviderAccepted, ProviderAccepted: true, ProviderReceiptDigest: receipt, AttemptCount: 1})
	if err != nil || !accepted.ProviderAccepted || accepted.DeliveryProven {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	delivered, err := service.ProjectExecutionOutcome(context.Background(), groupopsport.ExecutionOutcomeCommand{ExecutionID: accepted.ID, State: groupopsport.ExecutionDeliveryProven, ProviderAccepted: true, DeliveryProven: true, ProviderReceiptDigest: receipt, AttemptCount: 1})
	if err != nil || !delivered.ProviderAccepted || !delivered.DeliveryProven {
		t.Fatalf("delivered=%+v err=%v", delivered, err)
	}
}

func TestRuntimeGroupRefreshIsDisabledByDefault(t *testing.T) {
	service, _, _ := newRuntimeFixture(t)
	_, err := service.RefreshGroups(context.Background(), groupopsport.GroupRefreshCommand{OwnerStaffID: 7, ActorID: 7, Limit: 50, IdempotencyKey: "group-ops-group-sync-0001"})
	if !errors.Is(err, ErrProviderDisabled) {
		t.Fatalf("err=%v", err)
	}
	members, err := service.ListOperationMembers(context.Background(), 100)
	if err != nil || members.Scope != "group_ops" || len(members.Items) != 1 || members.Items[0].SenderUserID != "staff-7" || !members.ProviderExecutionEligible || members.RealExternalCallExecuted {
		t.Fatalf("members=%+v err=%v", members, err)
	}
}

func TestAcceptPlanFailsClosedWhenDispatchProviderIsDisabled(t *testing.T) {
	service, _, effects := newRuntimeFixture(t)
	service.SetDispatchEnabled(false)
	preview, previewErr := service.PreviewRunDue(context.Background(), 91)
	if previewErr != nil || preview.ProviderExecutionEligible || preview.RealExternalCallExecuted {
		t.Fatalf("preview=%+v err=%v", preview, previewErr)
	}
	_, err := service.AcceptPlan(context.Background(), groupopsport.AcceptPlanCommand{PlanID: 91, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:fixture", IdempotencyKey: "group-ops-disabled-0001"})
	if !errors.Is(err, ErrProviderDisabled) || effects.accepts != 0 {
		t.Fatalf("err=%v accepts=%d", err, effects.accepts)
	}
}

func TestMaterialExecutionWaitsUntilContinuationAndSevenDayDelayDoesNotFreezeEarly(t *testing.T) {
	service, runtime, effects := newRuntimeFixture(t)
	plans := service.plans.(*testStore)
	detail := plans.details[91]
	detail.Nodes = []groupopsport.Node{
		{ID: 21, Position: 1, Kind: groupopsport.NodeDelay, DelayMinutes: 10080},
		{ID: 22, Position: 2, Kind: groupopsport.NodeMessage, MaterialPlan: groupopsport.MaterialPlan{References: []groupopsport.MaterialReference{{Kind: "image", ID: 41}}}},
	}
	plans.details[91] = detail
	materials := &runtimeMaterialBoundary{}
	continuations := &runtimeContinuationJobFixture{}
	service.materials, service.continuations = materials, continuations

	command := groupopsport.AcceptPlanCommand{PlanID: 91, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:material", IdempotencyKey: "group-ops-material-0001"}
	summary, err := service.AcceptPlan(context.Background(), command)
	if err != nil || summary.Accepted != 0 || summary.MaterialPending != 2 || len(summary.PendingIntents) != 2 || effects.accepts != 0 || materials.freezes != 0 || materials.captures != 2 {
		t.Fatalf("summary=%+v err=%v effects=%d materials=%+v", summary, err, effects.accepts, materials)
	}
	var wantContinuation time.Time
	for _, intent := range runtime.intents {
		wantContinuation = intent.ScheduledFor.Add(-materialPreparationLead)
		break
	}
	if len(continuations.scheduled) != 2 || !continuations.scheduled[0].Equal(wantContinuation) {
		t.Fatalf("continuations=%v want=%v", continuations.scheduled, wantContinuation)
	}
	replay, err := service.AcceptPlan(context.Background(), command)
	if err != nil || replay.MaterialPending != 2 || materials.captures != 2 || effects.accepts != 0 {
		t.Fatalf("replay=%+v err=%v captures=%d effects=%d", replay, err, materials.captures, effects.accepts)
	}

	for key := range runtime.intents {
		intent := runtime.intents[key]
		lateNow := intent.ScheduledFor.Add(2 * time.Hour)
		service.now = func() time.Time { return lateNow }
		materials.now = lateNow
		materials.readyUntil = lateNow.Add(2 * time.Hour)
		first, continueErr := service.ContinueMaterialIntent(context.Background(), eer.Digest(runtimeDigestFromBytes(key)))
		if continueErr != nil || first.Execution.ID < 1 || first.Pending || effects.accepts != 1 || !materials.requiredThrough.Equal(lateNow.Add(materialDispatchGrace)) {
			t.Fatalf("first=%+v err=%v effects=%d required=%v", first, continueErr, effects.accepts, materials.requiredThrough)
		}
		second, continueErr := service.ContinueMaterialIntent(context.Background(), eer.Digest(runtimeDigestFromBytes(key)))
		if continueErr != nil || second.Execution.ID != first.Execution.ID || effects.accepts != 1 {
			t.Fatalf("second=%+v err=%v effects=%d", second, continueErr, effects.accepts)
		}
		break
	}
}

func TestLegacyMaterialReferenceIsExplicitPreviewBlockerBeforeEffects(t *testing.T) {
	service, _, effects := newRuntimeFixture(t)
	plans := service.plans.(*testStore)
	detail := plans.details[91]
	detail.Nodes[0].MaterialRef = "legacy:material"
	plans.details[91] = detail
	preview, err := service.PreviewRunDue(context.Background(), 91)
	if err != nil || len(preview.Blockers) != 1 || preview.Blockers[0] != "legacy_material_reference_unsupported" || effects.accepts != 0 {
		t.Fatalf("preview=%+v err=%v effects=%d", preview, err, effects.accepts)
	}
	_, err = service.AcceptPlan(context.Background(), groupopsport.AcceptPlanCommand{PlanID: 91, Trigger: groupopsport.RunTriggerBroadcast, AcceptedBy: "service:legacy", IdempotencyKey: "group-ops-legacy-0001"})
	if !errors.Is(err, ErrStateConflict) || effects.accepts != 0 {
		t.Fatalf("err=%v effects=%d", err, effects.accepts)
	}
}

func TestRefreshGroupsRequiresCompleteSnapshotBeforeReplacingOwnerProjection(t *testing.T) {
	service, runtime, _ := newRuntimeFixture(t)
	source := &runtimeDirectorySource{snapshot: groupopsport.GroupDirectorySnapshot{Items: []groupopsport.GroupDirectoryItem{{ChatReference: "chat-1", OwnerStaffID: 7, DisplayName: "Group 1"}}, Complete: true}}
	service.groups = source
	if _, err := service.RefreshGroups(context.Background(), groupopsport.GroupRefreshCommand{OwnerStaffID: 7, ActorID: 7, Limit: 200, IdempotencyKey: "groupops-full-0001"}); err != nil || source.limit != completeDirectorySnapshotLimit || runtime.directoryReplacements != 1 || len(runtime.directoryItems) != 1 {
		t.Fatalf("err=%v source=%+v runtime=%+v", err, source, runtime)
	}
	source.snapshot.Complete = false
	if _, err := service.RefreshGroups(context.Background(), groupopsport.GroupRefreshCommand{OwnerStaffID: 7, ActorID: 7, Limit: 200, IdempotencyKey: "groupops-partial-0001"}); !errors.Is(err, ErrUnavailable) || runtime.directoryReplacements != 1 {
		t.Fatalf("partial err=%v replacements=%d", err, runtime.directoryReplacements)
	}
}

func newRuntimeFixture(t *testing.T, evidence ...groupopsport.ReconciliationEvidenceVerifier) (*RuntimeService, *runtimeStoreFixture, *runtimeEffectsFixture) {
	t.Helper()
	anchor := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	plans := &testStore{details: map[int64]groupopsport.Detail{91: {
		Plan:        groupopsport.Plan{ID: 91, Name: "runtime", Status: groupopsport.PlanActive, Revision: 4, CreatedBy: 7, UpdatedBy: 7, CreatedAt: anchor.Add(-time.Hour), UpdatedAt: anchor},
		Members:     []groupopsport.Member{{StaffID: 7}},
		GroupAssets: []groupopsport.GroupAsset{{ID: 1, AssetRef: "chat:a"}, {ID: 2, AssetRef: "chat:b"}},
		Nodes: []groupopsport.Node{
			{ID: 11, Position: 1, Kind: groupopsport.NodeMessage, MessageText: "first", MaterialPlan: groupopsport.MaterialPlan{References: []groupopsport.MaterialReference{}}},
			{ID: 12, Position: 2, Kind: groupopsport.NodeDelay, DelayMinutes: 15},
			{ID: 13, Position: 3, Kind: groupopsport.NodeMessage, MessageText: "second"},
		},
		WebhookDescriptor: descriptor("daily-course"), Safety: groupopsport.LocalSafety(),
	}}, receipts: map[string]Receipt{}}
	runtime := &runtimeStoreFixture{runs: map[string]groupopsport.Run{}, executions: map[int64]groupopsport.Execution{}, intents: map[[sha256.Size]byte]ExecutionIntentRecord{}, webhookPlan: 91}
	effects := &runtimeEffectsFixture{}
	service, err := NewRuntimeService(testUOW{}, plans, runtime, effects, runtimeStaffFixture{}, nil, runtimeSenderFixture{}, runtimeJobFixture{}, evidence...)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return anchor.Add(10 * time.Minute) }
	return service, runtime, effects
}

type evidenceVerifierStub struct{ delivery bool }

type runtimeSenderFixture struct{}

func (runtimeSenderFixture) ResolveExecutionSender(_ context.Context, target string) (string, bool, error) {
	return "staff-" + target[len(target)-1:], true, nil
}

type runtimeJobFixture struct{}

func (runtimeJobFixture) Insert(_ context.Context, args GroupOpsDispatchJobArgs, generation int64, scheduled time.Time) (eer.RiverJobLink, error) {
	return eer.RiverJobLink{JobID: 1, Generation: generation, Queue: "outbound", ArgsDigest: runtimeDigest("job", args.EffectID), ScheduledAt: scheduled}, nil
}

type runtimeContinuationJobFixture struct{ scheduled []time.Time }

func (fixture *runtimeContinuationJobFixture) Insert(_ context.Context, args GroupOpsMaterialContinuationJobArgs, generation int64, scheduled time.Time) (eer.RiverJobLink, error) {
	fixture.scheduled = append(fixture.scheduled, scheduled)
	return eer.RiverJobLink{JobID: int64(len(fixture.scheduled)), Generation: generation, Queue: "outbound", ArgsDigest: runtimeDigest("continuation", args.ExecutionKeyDigest), ScheduledAt: scheduled}, nil
}

type runtimeMaterialBoundary struct {
	captures, freezes int
	now, readyUntil   time.Time
	requiredThrough   time.Time
}

func (boundary *runtimeMaterialBoundary) CaptureAndPrepare(_ context.Context, plan groupopsport.MaterialPlan, _ time.Time) (groupopsport.MaterialSourceSnapshot, error) {
	boundary.captures++
	refs := make([]groupopsport.CapturedMaterialReference, len(plan.References))
	for index, ref := range plan.References {
		refs[index] = groupopsport.CapturedMaterialReference{Kind: ref.Kind, ID: ref.ID, SourceDigest: string(runtimeDigest("source", ref.Kind, fmt.Sprint(ref.ID)))}
	}
	return groupopsport.MaterialSourceSnapshot{References: refs}, nil
}

func (boundary *runtimeMaterialBoundary) FreezePrepared(_ context.Context, _ json.RawMessage, requiredThrough time.Time) (groupopsport.PreparedMaterial, error) {
	boundary.freezes++
	boundary.requiredThrough = requiredThrough
	snapshot := json.RawMessage(`{"schema_version":2,"node_kind":"message","attachments":[{"msgtype":"image","media_id":"provider-image"}]}`)
	return groupopsport.PreparedMaterial{Snapshot: snapshot, Digest: string(runtimeDigest("group-ops-material", string(snapshot))), ReadyUntil: boundary.readyUntil}, nil
}

type runtimeDirectorySource struct {
	snapshot groupopsport.GroupDirectorySnapshot
	limit    int32
}

func (source *runtimeDirectorySource) ListOwnedGroups(_ context.Context, _ int64, limit int32) (groupopsport.GroupDirectorySnapshot, error) {
	source.limit = limit
	return source.snapshot, nil
}

func (*runtimeDirectorySource) RefreshOperationMembers(context.Context, int32) ([]groupopsport.OperationMember, error) {
	return nil, errors.New("unexpected")
}

func (stub evidenceVerifierStub) VerifyReconciliationEvidence(_ context.Context, evidence groupopsport.ReconciliationEvidence) (groupopsport.ReconciliationEvidenceResult, error) {
	if evidence.ExecutionID < 1 || evidence.ExternalEffectID == "" || evidence.EvidenceDigest == "" {
		return groupopsport.ReconciliationEvidenceResult{}, errors.New("invalid evidence")
	}
	return groupopsport.ReconciliationEvidenceResult{DeliveryProven: stub.delivery, EvidenceDigest: evidence.EvidenceDigest}, nil
}

type runtimeStaffFixture struct{}

func (runtimeStaffFixture) ListEligibleStaff(context.Context) ([]contactport.StaffDirectoryEntry, error) {
	return []contactport.StaffDirectoryEntry{{WeComUserID: "staff-7", DisplayName: "Staff 7", UpdatedAt: time.Now()}}, nil
}

type runtimeEffectsFixture struct {
	accepts    int
	reconciles int
	effects    map[string]string
}

func (fixture *runtimeEffectsFixture) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	if fixture.effects == nil {
		fixture.effects = map[string]string{}
	}
	key := string(command.CommandDigest())
	id := fixture.effects[key]
	if id == "" {
		fixture.accepts++
		id = fmt.Sprintf("eer_%d", fixture.accepts)
		fixture.effects[key] = id
	}
	now := time.Now().UTC()
	return eer.Projection{ID: id, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateAccepted, Generation: 1, UpdatedAt: now}, eer.OperationReceipt{ID: "accept", EffectID: id, CommandDigest: command.CommandDigest(), State: eer.StateAccepted, CompletedAt: now}, nil
}

func (*runtimeEffectsFixture) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	now := time.Now().UTC()
	return eer.Projection{ID: command.EffectID, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateQueued, Generation: command.Job.Generation, UpdatedAt: now}, eer.OperationReceipt{ID: "queue", EffectID: command.EffectID, CommandDigest: command.CommandDigest(), State: eer.StateQueued, CompletedAt: now}, nil
}

func (fixture *runtimeEffectsFixture) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.reconciles++
	now := time.Now().UTC()
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerGroupOps, Kind: eer.KindGroupOpsBroadcast, State: eer.StateReconciled, AttemptCount: 1, Generation: command.Lease.Generation, UpdatedAt: now}, eer.OperationReceipt{ID: "reconcile", EffectID: command.Lease.EffectID, CommandDigest: command.CommandDigest(), State: eer.StateReconciled, CompletedAt: now}, nil
}

type runtimeStoreFixture struct {
	runs                  map[string]groupopsport.Run
	executions            map[int64]groupopsport.Execution
	drafts                []ExecutionDraft
	intents               map[[sha256.Size]byte]ExecutionIntentRecord
	webhookPlan           int64
	directoryReplacements int
	directoryItems        []groupopsport.GroupDirectoryItem
}

func (fixture *runtimeStoreFixture) InsertExecutionIntent(_ context.Context, draft ExecutionIntentDraft) (ExecutionIntentRecord, error) {
	if value, ok := fixture.intents[draft.ExecutionKeyDigest]; ok {
		return value, nil
	}
	value := ExecutionIntentRecord{ExecutionIntentDraft: draft, ID: int64(len(fixture.intents) + 1), State: groupopsport.ExecutionIntentMaterialPending}
	fixture.intents[draft.ExecutionKeyDigest] = value
	return value, nil
}

func (fixture *runtimeStoreFixture) LockExecutionIntent(_ context.Context, key [sha256.Size]byte) (ExecutionIntentRecord, error) {
	value, ok := fixture.intents[key]
	if !ok {
		return ExecutionIntentRecord{}, ErrNotFound
	}
	return value, nil
}

func (fixture *runtimeStoreFixture) MarkExecutionIntentReady(_ context.Context, id int64, now time.Time) (ExecutionIntentRecord, error) {
	for key, value := range fixture.intents {
		if value.ID == id {
			value.State = groupopsport.ExecutionIntentReadyToAccept
			fixture.intents[key] = value
			return value, nil
		}
	}
	return ExecutionIntentRecord{}, ErrNotFound
}

func (fixture *runtimeStoreFixture) AcceptExecutionIntent(_ context.Context, id, executionID int64, now time.Time) (ExecutionIntentRecord, error) {
	for key, value := range fixture.intents {
		if value.ID == id {
			value.State, value.ExecutionID = groupopsport.ExecutionIntentAccepted, executionID
			fixture.intents[key] = value
			return value, nil
		}
	}
	return ExecutionIntentRecord{}, ErrNotFound
}

func (fixture *runtimeStoreFixture) FailExecutionIntent(_ context.Context, id int64, code string, now time.Time) (ExecutionIntentRecord, error) {
	for key, value := range fixture.intents {
		if value.ID == id {
			value.State, value.FailureCode = groupopsport.ExecutionIntentFinalFailed, code
			fixture.intents[key] = value
			return value, nil
		}
	}
	return ExecutionIntentRecord{}, ErrNotFound
}

func (fixture *runtimeStoreFixture) ListExecutionKeys(_ context.Context, planID, revision int64) ([]ExecutionKey, error) {
	result := []ExecutionKey{}
	for _, execution := range fixture.executions {
		run := fixture.runByID(execution.RunID)
		if execution.PlanID == planID && execution.PlanRevision == revision && run.Trigger == groupopsport.RunTriggerDue {
			result = append(result, ExecutionKey{NodeID: execution.NodeID, TargetReference: execution.TargetReference})
		}
	}
	return result, nil
}

func (fixture *runtimeStoreFixture) ReserveRun(_ context.Context, reservation RunReservation) (groupopsport.Run, error) {
	key := string(reservation.SourceKeyDigest[:])
	if run, ok := fixture.runs[key]; ok {
		return run, nil
	}
	run := groupopsport.Run{ID: int64(len(fixture.runs) + 1), PlanID: reservation.PlanID, Trigger: reservation.Trigger, PlanRevision: reservation.PlanRevision, ScheduledFor: reservation.ScheduledFor, AcceptedAt: reservation.AcceptedAt, AcceptedBy: reservation.AcceptedBy}
	fixture.runs[key] = run
	return run, nil
}

func (fixture *runtimeStoreFixture) InsertExecution(_ context.Context, draft ExecutionDraft) (groupopsport.Execution, error) {
	for _, current := range fixture.drafts {
		if current.ExecutionKeyDigest == draft.ExecutionKeyDigest {
			return fixture.executions[executionIDForDraft(fixture.drafts, current)], nil
		}
	}
	fixture.drafts = append(fixture.drafts, draft)
	id := int64(len(fixture.drafts))
	execution := groupopsport.Execution{ID: id, RunID: draft.RunID, PlanID: draft.PlanID, PlanRevision: draft.PlanRevision, NodeID: draft.NodeID, NodePosition: draft.NodePosition, TargetReference: draft.TargetReference, TargetDigest: draft.TargetDigest, ContentDigest: draft.ContentDigest, MaterialDigest: draft.MaterialDigest, ExternalEffectID: draft.ExternalEffectID, State: groupopsport.ExecutionAccepted, CreatedAt: draft.CreatedAt, UpdatedAt: draft.CreatedAt}
	fixture.executions[id] = execution
	return execution, nil
}

func (fixture *runtimeStoreFixture) ReadRunSummary(_ context.Context, runID int64) (groupopsport.RunSummary, error) {
	result := groupopsport.RunSummary{Run: fixture.runByID(runID), Executions: []groupopsport.Execution{}, RuntimeSafety: groupopsport.DisabledRuntimeSafety()}
	for _, execution := range fixture.executions {
		if execution.RunID == runID {
			result.Executions = append(result.Executions, execution)
			switch execution.State {
			case groupopsport.ExecutionAccepted:
				result.Accepted++
			case groupopsport.ExecutionProviderAccepted:
				result.ProviderAccepted++
			case groupopsport.ExecutionDeliveryProven:
				result.DeliveryProven++
			case groupopsport.ExecutionOutcomeUnknown:
				result.OutcomeUnknown++
			case groupopsport.ExecutionReconciled:
				result.Reconciled++
			case groupopsport.ExecutionFinalFailed:
				result.FinalFailed++
			}
		}
	}
	for _, intent := range fixture.intents {
		if intent.RunID == runID && (intent.State == groupopsport.ExecutionIntentMaterialPending || intent.State == groupopsport.ExecutionIntentReadyToAccept) {
			result.MaterialPending++
			result.PendingIntents = append(result.PendingIntents, groupopsport.ExecutionIntent{ID: intent.ID, NodeID: intent.NodeID, NodePosition: intent.NodePosition, TargetReference: intent.TargetReference, ScheduledFor: intent.ScheduledFor, State: groupopsport.ExecutionIntentMaterialPending})
		}
	}
	return result, nil
}

func (fixture *runtimeStoreFixture) ListExecutions(_ context.Context, planID int64, _, _ int32) ([]groupopsport.Execution, int64, error) {
	items := []groupopsport.Execution{}
	for _, execution := range fixture.executions {
		if execution.PlanID == planID {
			items = append(items, execution)
		}
	}
	return items, int64(len(items)), nil
}

func (fixture *runtimeStoreFixture) GetExecution(_ context.Context, id int64) (groupopsport.Execution, error) {
	value, ok := fixture.executions[id]
	if !ok {
		return groupopsport.Execution{}, ErrNotFound
	}
	return value, nil
}

func (fixture *runtimeStoreFixture) ReconcileExecution(_ context.Context, id int64, _ string, delivery bool, now time.Time) (groupopsport.Execution, error) {
	value := fixture.executions[id]
	value.State, value.ProviderAccepted, value.DeliveryProven, value.ReconciliationEvidencePresent, value.UpdatedAt = groupopsport.ExecutionReconciled, value.ProviderAccepted || delivery, delivery, true, now
	fixture.executions[id] = value
	return value, nil
}

func (fixture *runtimeStoreFixture) RecordExecutionOutcome(_ context.Context, id int64, state groupopsport.ExecutionState, providerAccepted, deliveryProven bool, receipt string, attempts int32, now time.Time) (groupopsport.Execution, error) {
	value, ok := fixture.executions[id]
	if !ok {
		return groupopsport.Execution{}, ErrNotFound
	}
	value.State, value.ProviderAccepted, value.DeliveryProven = state, providerAccepted, deliveryProven
	value.ProviderReceiptPresent, value.AttemptCount, value.UpdatedAt = receipt != "", attempts, now
	fixture.executions[id] = value
	return value, nil
}
func (fixture *runtimeStoreFixture) FindPlanByWebhookReference(context.Context, string) (int64, error) {
	return fixture.webhookPlan, nil
}
func (*runtimeStoreFixture) ListDirectoryGroups(context.Context, int64, int32, int32) ([]groupopsport.GroupDirectoryItem, int64, error) {
	return []groupopsport.GroupDirectoryItem{}, 0, nil
}
func (fixture *runtimeStoreFixture) ReplaceDirectoryGroups(_ context.Context, _ int64, items []groupopsport.GroupDirectoryItem, _ time.Time) error {
	fixture.directoryReplacements++
	fixture.directoryItems = append([]groupopsport.GroupDirectoryItem{}, items...)
	return nil
}
func (*runtimeStoreFixture) RecordDirectoryRefresh(context.Context, string, int64, int64, [sha256.Size]byte, string, int32, bool, time.Time) error {
	return nil
}

func (fixture *runtimeStoreFixture) runByID(id int64) groupopsport.Run {
	for _, run := range fixture.runs {
		if run.ID == id {
			return run
		}
	}
	return groupopsport.Run{}
}

func executionIDForDraft(values []ExecutionDraft, want ExecutionDraft) int64 {
	for index, value := range values {
		if value.ExecutionKeyDigest == want.ExecutionKeyDigest {
			return int64(index + 1)
		}
	}
	return 0
}

func runtimeDigestFromBytes(value [sha256.Size]byte) string {
	return "sha256:" + hex.EncodeToString(value[:])
}

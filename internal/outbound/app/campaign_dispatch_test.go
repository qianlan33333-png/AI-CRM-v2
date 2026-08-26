package app

import (
	"context"
	"errors"
	"testing"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type campaignDispatchFixture struct {
	bindings                                    []outboundport.CampaignDispatchBinding
	effects                                     map[string]string
	runtimeAccepts, runtimeQueues, enqueueCalls int
	eligibility                                 map[int64]contactport.ContactEligibility
	eligibilityCalls                            int
	eligibilityErr                              error
	checkedIDs                                  []contactport.CustomerID
	candidates                                  []outboundport.CampaignDispatchCandidate
	receipt                                     *outboundport.CampaignDispatchReceipt
	audiencePackageID                           int64
	audienceSource                              bool
	lastEnvelope                                eer.EffectEnvelope
	inUOW                                       bool
	recovery                                    CampaignDispatchAttemptRecovery
	recoveryErr                                 error
	recorded                                    []outboundport.CampaignDispatchProviderAttemptReceipt
	recordErr                                   error
	claimCalls, runAttemptCalls                 int
	recoverCalls, completeRecordedCalls         int
	reconcileCalls                              int
	providerReceiptSavedBeforeEERCompletion     bool
	audienceEvidence                            outboundport.CampaignDispatchReconciliationEvidence
	audienceEvidencePresent                     bool
}

func (fixture *campaignDispatchFixture) Within(ctx context.Context, callback func(context.Context) error) error {
	previous := fixture.inUOW
	fixture.inUOW = true
	defer func() { fixture.inUOW = previous }()
	return callback(ctx)
}
func (*campaignDispatchFixture) LockCampaignHandoffForDispatch(context.Context, string, string) (int64, error) {
	return 19, nil
}
func (*campaignDispatchFixture) ReadCampaignHandoffForDispatch(context.Context, string, string) (int64, error) {
	return 19, nil
}
func (fixture *campaignDispatchFixture) LoadCampaignDispatchByEffect(_ context.Context, effectID string) (outboundport.CampaignDispatchBinding, error) {
	for _, binding := range fixture.bindings {
		if binding.ExternalEffectID == effectID {
			return binding, nil
		}
	}
	return outboundport.CampaignDispatchBinding{}, errors.New("binding not found")
}
func (*campaignDispatchFixture) LoadCampaignDispatchProviderRequest(context.Context, string) (outboundport.CampaignDispatchProviderRequest, error) {
	return outboundport.CampaignDispatchProviderRequest{}, errors.New("not used")
}
func (fixture *campaignDispatchFixture) AudiencePackageForCampaignHandoff(context.Context, int64) (int64, bool, error) {
	return fixture.audiencePackageID, fixture.audienceSource, nil
}
func (fixture *campaignDispatchFixture) ListCampaignDispatchCandidates(context.Context, int64) ([]outboundport.CampaignDispatchCandidate, error) {
	if fixture.candidates != nil {
		return append([]outboundport.CampaignDispatchCandidate(nil), fixture.candidates...), nil
	}
	return []outboundport.CampaignDispatchCandidate{{CustomerID: 2, StepIndex: 1, Content: "hello"}, {CustomerID: 7, StepIndex: 1, Content: "hello"}}, nil
}
func (fixture *campaignDispatchFixture) CheckContactEligibility(_ context.Context, check contactport.ContactEligibilityCheck) ([]contactport.ContactEligibility, error) {
	fixture.eligibilityCalls++
	fixture.checkedIDs = append([]contactport.CustomerID(nil), check.CustomerIDs...)
	if fixture.eligibilityErr != nil {
		return nil, fixture.eligibilityErr
	}
	if check.Checkpoint != contactport.ContactEligibilityDispatch || check.EvaluatedAt.IsZero() {
		return nil, contactport.ErrInvalidContactEligibility
	}
	result := make([]contactport.ContactEligibility, len(check.CustomerIDs))
	for index, customerID := range check.CustomerIDs {
		decision, present := fixture.eligibility[int64(customerID)]
		if !present {
			decision = contactport.ContactEligibility{CustomerID: customerID, CustomerActive: true, Eligible: true, Exclusion: contactport.ContactEligibilityExclusionNone}
		}
		result[index] = decision
	}
	return result, nil
}
func (fixture *campaignDispatchFixture) ReserveCampaignDispatchReceipt(_ context.Context, actorID, handoffID int64, key, payload [32]byte, result outbound.CampaignDispatchSummary) (outboundport.CampaignDispatchReceipt, error) {
	receipt := outboundport.CampaignDispatchReceipt{ID: 1, ActorID: actorID, HandoffID: handoffID, KeyDigest: key, PayloadDigest: payload, Result: result}
	fixture.receipt = &receipt
	return receipt, nil
}
func (fixture *campaignDispatchFixture) LoadCampaignDispatchReceipt(_ context.Context, actorID int64, key [32]byte) (outboundport.CampaignDispatchReceipt, bool, error) {
	if fixture.receipt == nil || fixture.receipt.ActorID != actorID || fixture.receipt.KeyDigest != key {
		return outboundport.CampaignDispatchReceipt{}, false, nil
	}
	return *fixture.receipt, true, nil
}
func (fixture *campaignDispatchFixture) InsertCampaignDispatchBinding(_ context.Context, binding outboundport.CampaignDispatchBinding) (outboundport.CampaignDispatchBinding, error) {
	for _, current := range fixture.bindings {
		if current.HandoffID == binding.HandoffID && current.CustomerID == binding.CustomerID && current.StepIndex == binding.StepIndex {
			return current, nil
		}
	}
	binding.ID = int64(len(fixture.bindings) + 1)
	fixture.bindings = append(fixture.bindings, binding)
	return binding, nil
}
func (fixture *campaignDispatchFixture) UpdateCampaignDispatchState(_ context.Context, effectID string, state outbound.CampaignDispatchState) error {
	for index := range fixture.bindings {
		if fixture.bindings[index].ExternalEffectID == effectID {
			fixture.bindings[index].State = state
			return nil
		}
	}
	return errors.New("binding not found")
}
func (fixture *campaignDispatchFixture) ReadCampaignDispatchSummary(_ context.Context, handoffID int64) (outbound.CampaignDispatchSummary, error) {
	result := outbound.CampaignDispatchSummary{HandoffID: handoffID, UpdatedAt: time.Now().UTC()}
	for _, binding := range fixture.bindings {
		switch binding.State {
		case outbound.CampaignDispatchBlocked:
			result.Blocked++
		case outbound.CampaignDispatchAccepted:
			result.Accepted++
		case outbound.CampaignDispatchQueued:
			result.Queued++
		case outbound.CampaignDispatchExecuted:
			result.Executed++
		case outbound.CampaignDispatchOutcomeUnknown:
			result.OutcomeUnknown++
		case outbound.CampaignDispatchReconciled:
			result.Reconciled++
		}
	}
	for _, receipt := range fixture.recorded {
		result.BusinessCallDispatched = result.BusinessCallDispatched || receipt.BusinessCallDispatched
		result.RealExternalCallExecuted = result.RealExternalCallExecuted || receipt.RealExternalCallExecuted
		result.DeliveryProven = result.DeliveryProven || receipt.DeliveryProven
	}
	return result, nil
}
func (fixture *campaignDispatchFixture) RecordCampaignProviderAttemptReceipt(_ context.Context, _ string, _ int32, receipt outboundport.CampaignDispatchProviderAttemptReceipt) error {
	if fixture.recordErr != nil {
		return fixture.recordErr
	}
	fixture.recorded = append(fixture.recorded, receipt)
	return nil
}
func (fixture *campaignDispatchFixture) LoadCampaignDispatchAttemptRecovery(context.Context, string) (CampaignDispatchAttemptRecovery, error) {
	return fixture.recovery, fixture.recoveryErr
}
func (fixture *campaignDispatchFixture) LoadAudienceCampaignDispatchReconciliationEvidence(context.Context, string) (outboundport.CampaignDispatchReconciliationEvidence, bool, error) {
	return fixture.audienceEvidence, fixture.audienceEvidencePresent, nil
}
func (fixture *campaignDispatchFixture) EnqueueCampaignDispatch(_ context.Context, effectID string) (eer.RiverJobLink, error) {
	fixture.enqueueCalls++
	return eer.RiverJobLink{JobID: int64(100 + fixture.enqueueCalls), Generation: 1, Queue: "outbound", ArgsDigest: campaignDispatchTestDigest("args", effectID), ScheduledAt: time.Now().UTC()}, nil
}
func (fixture *campaignDispatchFixture) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.lastEnvelope = command.Envelope
	if fixture.effects == nil {
		fixture.effects = make(map[string]string)
	}
	payloadDigest := string(command.Envelope.PayloadDigest())
	effectID := fixture.effects[payloadDigest]
	if effectID == "" {
		fixture.runtimeAccepts++
		effectID = string(rune('1' + fixture.runtimeAccepts - 1))
		fixture.effects[payloadDigest] = effectID
	}
	return eer.Projection{ID: effectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateAccepted, Generation: 1, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{ID: "a", EffectID: effectID, CommandDigest: command.CommandDigest(), State: eer.StateAccepted, CompletedAt: time.Now().UTC()}, nil
}

type audienceQualificationFixture struct {
	values []outboundport.AudienceDispatchTargetQualification
	err    error
	called bool
}

func (fixture *audienceQualificationFixture) QualifyAudienceDispatchTargets(_ context.Context, packageID int64, customerIDs []int64) ([]outboundport.AudienceDispatchTargetQualification, error) {
	fixture.called = true
	if packageID != 77 || len(customerIDs) != 2 || customerIDs[0] != 2 || customerIDs[1] != 7 {
		return nil, errors.New("unexpected audience qualification")
	}
	return fixture.values, fixture.err
}
func (fixture *campaignDispatchFixture) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.runtimeQueues++
	return eer.Projection{ID: command.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{ID: "q", EffectID: command.EffectID, CommandDigest: command.CommandDigest(), State: eer.StateQueued, CompletedAt: time.Now().UTC()}, nil
}
func (fixture *campaignDispatchFixture) Claim(_ context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	fixture.claimCalls++
	lease := eer.Lease{EffectID: command.EffectID, Generation: 2, Fence: 3, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	return lease, eer.Projection{ID: command.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now().UTC()}, nil
}
func (fixture *campaignDispatchFixture) RunAttempt(ctx context.Context, lease eer.Lease, adapter eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	fixture.runAttemptCalls++
	envelope, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, SourceRefDigest: campaignDispatchTestDigest("source", "run"), TargetRefDigest: campaignDispatchTestDigest("target", "run"), PayloadDigest: campaignDispatchTestDigest("payload", "run"), PolicyVersionHash: campaignDispatchTestDigest("policy", "run")})
	if err != nil {
		return eer.Projection{}, eer.OperationReceipt{}, err
	}
	result, runErr := adapter.Execute(ctx, envelope, eer.Attempt{Number: 1, Generation: lease.Generation, Fence: lease.Fence, StartedAt: time.Now().UTC()})
	fixture.providerReceiptSavedBeforeEERCompletion = len(fixture.recorded) == 1
	state := eer.State(result.Completion)
	if runErr != nil {
		state = eer.StateOutcomeUnknown
	}
	return eer.Projection{ID: lease.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: state, AttemptCount: 1, Generation: lease.Generation, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{ID: "complete", EffectID: lease.EffectID, CommandDigest: result.ReceiptDigest, State: state, CompletedAt: time.Now().UTC()}, runErr
}
func (fixture *campaignDispatchFixture) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.reconcileCalls++
	digest := command.CommandDigest()
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateReconciled, AttemptCount: fixture.recovery.AttemptCount, Generation: command.Lease.Generation, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{ID: "reconcile", EffectID: command.Lease.EffectID, CommandDigest: digest, State: eer.StateReconciled, CompletedAt: time.Now().UTC()}, nil
}
func (fixture *campaignDispatchFixture) RecoverAttemptedToUnknown(_ context.Context, command eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.recoverCalls++
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateOutcomeUnknown, AttemptCount: 1, Generation: command.Lease.Generation, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{}, nil
}
func (fixture *campaignDispatchFixture) CompleteRecordedAttempt(_ context.Context, command eer.CompleteRecordedAttemptCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.completeRecordedCalls++
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateExecuted, AttemptCount: command.Attempt.Number, Generation: command.Lease.Generation, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{}, nil
}

func TestCampaignDispatchExternalGateOffCreatesOnlyBlockedBindings(t *testing.T) {
	fixture := &campaignDispatchFixture{}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(false))
	if err != nil || summary.Blocked != 2 || summary.Queued != 0 || fixture.runtimeAccepts != 0 || fixture.enqueueCalls != 0 {
		t.Fatalf("summary=%+v err=%v accepts=%d enqueues=%d", summary, err, fixture.runtimeAccepts, fixture.enqueueCalls)
	}
}

func TestCampaignDispatchGatedQueueUsesEERAndNeverClaimsDelivery(t *testing.T) {
	fixture := &campaignDispatchFixture{}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || summary.Queued != 2 || summary.DeliveryProven || summary.RealExternalCallExecuted || fixture.runtimeAccepts != 2 || fixture.runtimeQueues != 2 || fixture.enqueueCalls != 2 {
		t.Fatalf("summary=%+v err=%v accepts/queues/enqueues=%d/%d/%d", summary, err, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls)
	}
}

func TestCampaignDispatchAudienceFreezesOnlyQualifiedRelationshipOwner(t *testing.T) {
	serviceFixture := &campaignDispatchFixture{audienceSource: true, audiencePackageID: 77}
	service, err := NewCampaignDispatchService(serviceFixture, serviceFixture, serviceFixture, serviceFixture, serviceFixture)
	if err != nil {
		t.Fatal(err)
	}
	qualifier := &audienceQualificationFixture{values: []outboundport.AudienceDispatchTargetQualification{
		{CustomerID: 2, Eligible: true, SenderUserID: "owner-2", ExternalUserID: "external-2"},
		{CustomerID: 7, Eligible: false, Exclusion: "sender_not_allowed"},
	}}
	service, err = service.WithAudienceQualification(qualifier, nil)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || !qualifier.called || summary.Queued != 1 || summary.Blocked != 1 || serviceFixture.runtimeAccepts != 1 || serviceFixture.enqueueCalls != 1 {
		t.Fatalf("summary=%+v err=%v qualifier=%t accepts=%d enqueues=%d", summary, err, qualifier.called, serviceFixture.runtimeAccepts, serviceFixture.enqueueCalls)
	}
	queued := serviceFixture.bindings[0]
	if queued.CustomerID != 2 || queued.SenderUserIDSnapshot != "owner-2" || queued.ExternalUserIDSnapshot != "external-2" || queued.PayloadDigest != outbound.AudienceCampaignDispatchPayloadDigest(19, 2, 1, "hello", "owner-2", "external-2") || string(serviceFixture.lastEnvelope.TargetRefDigest()) != outbound.AudienceCampaignDispatchRecipientDigest(2, "owner-2", "external-2") {
		t.Fatalf("queued=%+v envelope=%+v", queued, serviceFixture.lastEnvelope)
	}
	blocked := serviceFixture.bindings[1]
	if blocked.CustomerID != 7 || blocked.BlockReason != "sender_not_allowed" || blocked.ExternalEffectID != "" || blocked.SenderUserIDSnapshot != "" {
		t.Fatalf("blocked=%+v", blocked)
	}
}

func TestCampaignDispatchAudienceFailsClosedWithoutQualificationWiring(t *testing.T) {
	fixture := &campaignDispatchFixture{audienceSource: true, audiencePackageID: 77}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Dispatch(context.Background(), campaignDispatchTestCommand(true)); !errors.Is(err, outbound.ErrCampaignDispatchUnavailable) || len(fixture.bindings) != 0 || fixture.runtimeAccepts != 0 {
		t.Fatalf("err=%v bindings=%+v accepts=%d", err, fixture.bindings, fixture.runtimeAccepts)
	}
}

func TestCampaignDispatchReplayDoesNotInsertAnotherRiverJob(t *testing.T) {
	fixture := &campaignDispatchFixture{}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil {
		t.Fatal(err)
	}
	fixture.eligibility = map[int64]contactport.ContactEligibility{
		2: {CustomerID: 2, CustomerActive: true, Eligible: false, Exclusion: contactport.ContactEligibilityExclusionContactPolicy},
	}
	replayed, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || replayed.Queued != first.Queued || fixture.runtimeAccepts != 2 || fixture.runtimeQueues != 2 || fixture.enqueueCalls != 2 || fixture.eligibilityCalls != 1 {
		t.Fatalf("first=%+v replayed=%+v err=%v accepts/queues/enqueues/checks=%d/%d/%d/%d", first, replayed, err, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls, fixture.eligibilityCalls)
	}
}

func TestCampaignDispatchRechecksContactPolicyBeforeQueue(t *testing.T) {
	fixture := &campaignDispatchFixture{eligibility: map[int64]contactport.ContactEligibility{
		2: {CustomerID: 2, CustomerActive: true, Eligible: false, Exclusion: contactport.ContactEligibilityExclusionContactPolicy},
	}}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || summary.Blocked != 1 || summary.Queued != 1 || fixture.runtimeAccepts != 1 || fixture.runtimeQueues != 1 || fixture.enqueueCalls != 1 || fixture.eligibilityCalls != 1 {
		t.Fatalf("summary=%+v err=%v accepts/queues/enqueues/checks=%d/%d/%d/%d", summary, err, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls, fixture.eligibilityCalls)
	}
	if len(fixture.bindings) != 2 || fixture.bindings[0].State != outbound.CampaignDispatchBlocked || fixture.bindings[0].BlockReason != "contact_policy" {
		t.Fatalf("bindings=%+v", fixture.bindings)
	}
}

func TestCampaignDispatchEligibilityFailureRollsBackBeforeEffects(t *testing.T) {
	fixture := &campaignDispatchFixture{eligibilityErr: contactport.ErrContactEligibilityUnavailable}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Dispatch(context.Background(), campaignDispatchTestCommand(true)); !errors.Is(err, outbound.ErrCampaignDispatchUnavailable) {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if len(fixture.bindings) != 0 || fixture.runtimeAccepts != 0 || fixture.runtimeQueues != 0 || fixture.enqueueCalls != 0 {
		t.Fatalf("bindings=%+v accepts/queues/enqueues=%d/%d/%d", fixture.bindings, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls)
	}
}

func TestCampaignDispatchEligibilityDeduplicatesAndSortsCustomers(t *testing.T) {
	fixture := &campaignDispatchFixture{candidates: []outboundport.CampaignDispatchCandidate{
		{CustomerID: 7, StepIndex: 2, Content: "later"},
		{CustomerID: 2, StepIndex: 1, Content: "first"},
		{CustomerID: 7, StepIndex: 1, Content: "earlier"},
	}}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || summary.Queued != 3 || len(fixture.checkedIDs) != 2 || fixture.checkedIDs[0] != 2 || fixture.checkedIDs[1] != 7 {
		t.Fatalf("summary=%+v err=%v checked=%v", summary, err, fixture.checkedIDs)
	}
}

type campaignDispatchProviderAttemptAdapter struct{ calls int }

func (adapter *campaignDispatchProviderAttemptAdapter) Execute(context.Context, eer.EffectEnvelope, eer.Attempt) (eer.AdapterResult, error) {
	return eer.AdapterResult{}, errors.New("evidence path required")
}
func (adapter *campaignDispatchProviderAttemptAdapter) ExecuteWithCampaignDispatchProviderEvidence(_ context.Context, _ eer.EffectEnvelope, _ eer.Attempt, record func(outboundport.CampaignDispatchProviderAttemptReceipt) error) (eer.AdapterResult, error) {
	adapter.calls++
	result := eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: campaignDispatchTestDigest("provider", "receipt"), BusinessCallDispatched: true, RealExternalCallExecuted: true}
	evidence := outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(result.Completion), ReceiptDigest: result.ReceiptDigest, BusinessCallDispatched: true, RealExternalCallExecuted: true, ProviderMessageID: "msg-1", ProviderCode: "0", ProviderResultReceived: true}
	return result, record(evidence)
}

func TestCampaignDispatchRunEffectPersistsExactProviderReceiptBeforeEERCompletion(t *testing.T) {
	effectID := "eer_41"
	fixture := &campaignDispatchFixture{
		bindings: []outboundport.CampaignDispatchBinding{{HandoffID: 19, ExternalEffectID: effectID, State: outbound.CampaignDispatchQueued}},
		recovery: CampaignDispatchAttemptRecovery{EffectID: effectID, EffectState: eer.StateQueued},
	}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &campaignDispatchProviderAttemptAdapter{}
	if err = service.RunEffect(context.Background(), effectID, campaignDispatchTestDigest("worker", effectID), adapter); err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || !fixture.providerReceiptSavedBeforeEERCompletion || len(fixture.recorded) != 1 || fixture.recorded[0].ProviderMessageID != "msg-1" || fixture.bindings[0].State != outbound.CampaignDispatchExecuted {
		t.Fatalf("calls=%d saved-before=%t receipts=%+v binding=%+v", adapter.calls, fixture.providerReceiptSavedBeforeEERCompletion, fixture.recorded, fixture.bindings[0])
	}
}

func TestCampaignDispatchAttemptRecoveryNeverCallsProvider(t *testing.T) {
	effectID := "eer_41"
	expired := time.Now().UTC().Add(-time.Minute)
	for _, test := range []struct {
		name                      string
		receipt                   *outboundport.CampaignDispatchProviderAttemptReceipt
		wantState                 outbound.CampaignDispatchState
		wantComplete, wantUnknown int
	}{
		{name: "exact receipt completes recorded attempt", receipt: &outboundport.CampaignDispatchProviderAttemptReceipt{Completion: string(eer.CompletionExecuted), ReceiptDigest: campaignDispatchTestDigest("provider", "receipt"), BusinessCallDispatched: true, RealExternalCallExecuted: true, ProviderMessageID: "msg-1", ProviderResultReceived: true}, wantState: outbound.CampaignDispatchExecuted, wantComplete: 1},
		{name: "missing receipt becomes outcome unknown", wantState: outbound.CampaignDispatchOutcomeUnknown, wantUnknown: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			recovery := CampaignDispatchAttemptRecovery{EffectID: effectID, EffectState: eer.StateAttempted, AttemptCount: 1, Lease: eer.Lease{EffectID: effectID, Generation: 2, Fence: 3, ExpiresAt: expired}, Attempt: eer.Attempt{Number: 1, Generation: 2, Fence: 3, StartedAt: expired.Add(-time.Minute)}}
			if test.receipt != nil {
				recovery.Receipt, recovery.ReceiptExists = *test.receipt, true
			}
			fixture := &campaignDispatchFixture{bindings: []outboundport.CampaignDispatchBinding{{HandoffID: 19, ExternalEffectID: effectID, State: outbound.CampaignDispatchAttempted}}, recovery: recovery}
			service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
			if err != nil {
				t.Fatal(err)
			}
			adapter := &campaignDispatchProviderAttemptAdapter{}
			if err = service.RunEffect(context.Background(), effectID, campaignDispatchTestDigest("worker", effectID), adapter); err != nil {
				t.Fatal(err)
			}
			if adapter.calls != 0 || fixture.claimCalls != 0 || fixture.completeRecordedCalls != test.wantComplete || fixture.recoverCalls != test.wantUnknown || fixture.bindings[0].State != test.wantState {
				t.Fatalf("provider/claim/complete/unknown=%d/%d/%d/%d binding=%+v", adapter.calls, fixture.claimCalls, fixture.completeRecordedCalls, fixture.recoverCalls, fixture.bindings[0])
			}
		})
	}
}

type campaignDispatchEvidenceVerifier struct {
	fixture     *campaignDispatchFixture
	called      int
	calledInUOW bool
}

func (verifier *campaignDispatchEvidenceVerifier) VerifyAudienceCampaignDispatch(_ context.Context, evidence outboundport.CampaignDispatchReconciliationEvidence) (bool, eer.Digest, error) {
	verifier.called++
	verifier.calledInUOW = verifier.fixture.inUOW
	if evidence.ProviderMessageID != "msg-1" {
		return false, "", errors.New("wrong evidence")
	}
	return true, campaignDispatchTestDigest("verified", evidence.ProviderMessageID), nil
}

func TestCampaignDispatchExecutedDeliveryVerificationDoesNotReconcileEEROrHoldUOW(t *testing.T) {
	effectID := "eer_41"
	providerDigest := campaignDispatchTestDigest("provider", "receipt")
	fixture := &campaignDispatchFixture{
		bindings:                []outboundport.CampaignDispatchBinding{{HandoffID: 19, ExternalEffectID: effectID, State: outbound.CampaignDispatchExecuted}},
		recovery:                CampaignDispatchAttemptRecovery{EffectID: effectID, EffectState: eer.StateExecuted, AttemptCount: 1},
		audienceEvidencePresent: true,
		audienceEvidence:        outboundport.CampaignDispatchReconciliationEvidence{ExternalEffectID: effectID, ProviderMessageID: "msg-1", SenderUserID: "owner-1", ExternalUserID: "external-1", ProviderReceiptDigest: providerDigest, BusinessCallDispatched: true, RealExternalCallExecuted: true},
	}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &campaignDispatchEvidenceVerifier{fixture: fixture}
	service, err = service.WithAudienceQualification(&audienceQualificationFixture{}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.ManualReconcile(context.Background(), campaignDispatchReconcileTestCommand(effectID))
	if err != nil || verifier.called != 1 || verifier.calledInUOW || fixture.reconcileCalls != 0 || len(fixture.recorded) != 1 || !fixture.recorded[0].DeliveryProven || fixture.bindings[0].State != outbound.CampaignDispatchExecuted || !summary.DeliveryProven {
		t.Fatalf("summary=%+v err=%v verifier=%d/in-uow:%t reconcile=%d receipts=%+v binding=%+v", summary, err, verifier.called, verifier.calledInUOW, fixture.reconcileCalls, fixture.recorded, fixture.bindings[0])
	}
}

func TestCampaignDispatchOutcomeUnknownUsesEERReconciliationAfterProviderQuery(t *testing.T) {
	effectID := "eer_41"
	providerDigest := campaignDispatchTestDigest("provider", "receipt")
	fixture := &campaignDispatchFixture{
		bindings:                []outboundport.CampaignDispatchBinding{{HandoffID: 19, ExternalEffectID: effectID, State: outbound.CampaignDispatchOutcomeUnknown}},
		recovery:                CampaignDispatchAttemptRecovery{EffectID: effectID, EffectState: eer.StateOutcomeUnknown, AttemptCount: 1},
		audienceEvidencePresent: true,
		audienceEvidence:        outboundport.CampaignDispatchReconciliationEvidence{ExternalEffectID: effectID, ProviderMessageID: "msg-1", SenderUserID: "owner-1", ExternalUserID: "external-1", ProviderReceiptDigest: providerDigest, BusinessCallDispatched: true, RealExternalCallExecuted: true},
	}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &campaignDispatchEvidenceVerifier{fixture: fixture}
	service, err = service.WithAudienceQualification(&audienceQualificationFixture{}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.ManualReconcile(context.Background(), campaignDispatchReconcileTestCommand(effectID))
	if err != nil || verifier.called != 1 || verifier.calledInUOW || fixture.reconcileCalls != 1 || len(fixture.recorded) != 1 || fixture.bindings[0].State != outbound.CampaignDispatchReconciled || summary.Reconciled != 1 || !summary.DeliveryProven {
		t.Fatalf("summary=%+v err=%v verifier=%d/in-uow:%t reconcile=%d receipts=%+v binding=%+v", summary, err, verifier.called, verifier.calledInUOW, fixture.reconcileCalls, fixture.recorded, fixture.bindings[0])
	}
}

func campaignDispatchTestCommand(gate bool) CampaignDispatchCommand {
	return CampaignDispatchCommand{CampaignCode: "spring-campaign", PlanID: "ctp_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ActorID: 7, IdempotencyKey: "campaign-dispatch-key", ExternalGate: gate}
}
func campaignDispatchReconcileTestCommand(effectID string) CampaignDispatchReconcileCommand {
	return CampaignDispatchReconcileCommand{CampaignCode: "spring-campaign", PlanID: "ctp_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EffectID: effectID, ActorID: 7, IdempotencyKey: "campaign-reconcile-key", Generation: 2, Fence: 3, LeaseExpiresAt: time.Now().UTC().Add(-time.Minute), EvidenceDigest: string(campaignDispatchTestDigest("operator", effectID))}
}
func campaignDispatchTestDigest(label, value string) eer.Digest { return digest(label, value) }

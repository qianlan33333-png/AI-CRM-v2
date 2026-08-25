package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	outbound "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
)

type campaignDispatchFixture struct {
	bindings                                    []outboundport.CampaignDispatchBinding
	effects                                     map[string]string
	runtimeAccepts, runtimeQueues, enqueueCalls int
}

func (*campaignDispatchFixture) Within(ctx context.Context, callback func(context.Context) error) error {
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
func (*campaignDispatchFixture) ListCampaignDispatchCandidates(context.Context, int64) ([]outboundport.CampaignDispatchCandidate, error) {
	return []outboundport.CampaignDispatchCandidate{{CustomerID: 2, StepIndex: 1, Content: "hello"}, {CustomerID: 7, StepIndex: 1, Content: "hello"}}, nil
}
func (fixture *campaignDispatchFixture) ReserveCampaignDispatchReceipt(_ context.Context, actorID, handoffID int64, key, payload [32]byte, result outbound.CampaignDispatchSummary) (outboundport.CampaignDispatchReceipt, error) {
	return outboundport.CampaignDispatchReceipt{ID: 1, ActorID: actorID, HandoffID: handoffID, KeyDigest: key, PayloadDigest: payload, Result: result}, nil
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
		}
	}
	return result, nil
}
func (*campaignDispatchFixture) RecordCampaignProviderAttemptReceipt(context.Context, string, int32, string, eer.Digest) error {
	return nil
}
func (fixture *campaignDispatchFixture) EnqueueCampaignDispatch(_ context.Context, effectID string) (eer.RiverJobLink, error) {
	fixture.enqueueCalls++
	return eer.RiverJobLink{JobID: int64(100 + fixture.enqueueCalls), Generation: 1, Queue: "outbound", ArgsDigest: campaignDispatchTestDigest("args", effectID), ScheduledAt: time.Now().UTC()}, nil
}
func (fixture *campaignDispatchFixture) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
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
func (fixture *campaignDispatchFixture) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	fixture.runtimeQueues++
	return eer.Projection{ID: command.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateQueued, Generation: 2, UpdatedAt: time.Now().UTC()}, eer.OperationReceipt{ID: "q", EffectID: command.EffectID, CommandDigest: command.CommandDigest(), State: eer.StateQueued, CompletedAt: time.Now().UTC()}, nil
}
func (*campaignDispatchFixture) Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	return eer.Lease{}, eer.Projection{}, errors.New("not used")
}
func (*campaignDispatchFixture) RunAttempt(context.Context, eer.Lease, eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, errors.New("not used")
}
func (*campaignDispatchFixture) Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, errors.New("not used")
}

func TestCampaignDispatchExternalGateOffCreatesOnlyBlockedBindings(t *testing.T) {
	fixture := &campaignDispatchFixture{}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture)
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
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || summary.Queued != 2 || summary.DeliveryProven || summary.RealExternalCallExecuted || fixture.runtimeAccepts != 2 || fixture.runtimeQueues != 2 || fixture.enqueueCalls != 2 {
		t.Fatalf("summary=%+v err=%v accepts/queues/enqueues=%d/%d/%d", summary, err, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls)
	}
}

func TestCampaignDispatchReplayDoesNotInsertAnotherRiverJob(t *testing.T) {
	fixture := &campaignDispatchFixture{}
	service, err := NewCampaignDispatchService(fixture, fixture, fixture, fixture)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Dispatch(context.Background(), campaignDispatchTestCommand(true))
	if err != nil || replayed.Queued != first.Queued || fixture.runtimeAccepts != 2 || fixture.runtimeQueues != 2 || fixture.enqueueCalls != 2 {
		t.Fatalf("first=%+v replayed=%+v err=%v accepts/queues/enqueues=%d/%d/%d", first, replayed, err, fixture.runtimeAccepts, fixture.runtimeQueues, fixture.enqueueCalls)
	}
}

func campaignDispatchTestCommand(gate bool) CampaignDispatchCommand {
	return CampaignDispatchCommand{CampaignCode: "spring-campaign", PlanID: "ctp_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ActorID: 7, IdempotencyKey: "campaign-dispatch-key", ExternalGate: gate}
}
func campaignDispatchTestDigest(label, value string) eer.Digest { return digest(label, value) }

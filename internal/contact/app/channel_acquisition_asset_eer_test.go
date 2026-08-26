package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type channelAcquisitionAssetEERStub struct {
	now              time.Time
	projection       eer.Projection
	acceptCommand    eer.AcceptCommand
	queueCommand     eer.QueueCommand
	claimCommand     eer.ClaimCommand
	reconcileCommand eer.ReconcileCommand
	recoverCommand   eer.RecoverAttemptedCommand
	runEnvelope      eer.EffectEnvelope
	runAttempt       eer.Attempt
	runResult        eer.AdapterResult
	runCalls         int
	terminalOutcome  eer.TerminalOutcome
}

func (stub *channelAcquisitionAssetEERStub) Accept(_ context.Context, command eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.acceptCommand = command
	projection := stub.projection
	if projection.ID == "" {
		projection = eer.Projection{ID: "eer_ch02_1", Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateAccepted, Generation: 1, UpdatedAt: stub.now}
	}
	return projection, channelAcquisitionAssetEERReceipt("accept", projection, stub.now), nil
}

func (stub *channelAcquisitionAssetEERStub) Queue(_ context.Context, command eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.queueCommand = command
	projection := eer.Projection{ID: command.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateQueued, Generation: command.Job.Generation, UpdatedAt: stub.now}
	return projection, channelAcquisitionAssetEERReceipt("queue", projection, stub.now), nil
}

func (stub *channelAcquisitionAssetEERStub) Claim(_ context.Context, command eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	stub.claimCommand = command
	lease := eer.Lease{EffectID: command.EffectID, Generation: 2, Fence: 3, ExpiresAt: stub.now.Add(time.Minute)}
	return lease, eer.Projection{ID: command.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateQueued, Generation: lease.Generation, UpdatedAt: stub.now}, nil
}

func (stub *channelAcquisitionAssetEERStub) RunAttempt(ctx context.Context, lease eer.Lease, adapter eer.Adapter) (eer.Projection, eer.OperationReceipt, error) {
	stub.runCalls++
	result, err := adapter.Execute(ctx, stub.runEnvelope, stub.runAttempt)
	stub.runResult = result
	state := eer.StateOutcomeUnknown
	if err == nil {
		switch result.Completion {
		case eer.CompletionExecuted:
			state = eer.StateExecuted
		case eer.CompletionFinalFailed:
			state = eer.StateFinalFailed
		case eer.CompletionOutcomeUnknown:
			state = eer.StateOutcomeUnknown
		default:
			err = errors.New("invalid adapter result")
		}
	}
	projection := eer.Projection{ID: lease.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: state, Generation: lease.Generation, UpdatedAt: stub.now}
	return projection, channelAcquisitionAssetEERReceipt("attempt", projection, stub.now), err
}

func (stub *channelAcquisitionAssetEERStub) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.reconcileCommand = command
	projection := eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateReconciled, Generation: command.Lease.Generation, UpdatedAt: stub.now}
	return projection, channelAcquisitionAssetEERReceipt("reconcile", projection, stub.now), nil
}

func (stub *channelAcquisitionAssetEERStub) RecoverAttemptedToUnknown(_ context.Context, command eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	stub.recoverCommand = command
	projection := eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateOutcomeUnknown, Generation: command.Lease.Generation, UpdatedAt: stub.now}
	return projection, channelAcquisitionAssetEERReceipt("recover", projection, stub.now), nil
}

func (stub *channelAcquisitionAssetEERStub) CompleteRecordedAttempt(_ context.Context, command eer.CompleteRecordedAttemptCommand) (eer.Projection, eer.OperationReceipt, error) {
	projection := eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerContact, Kind: eer.KindContactAcquisitionAssetPublish, State: eer.StateExecuted, Generation: command.Lease.Generation, UpdatedAt: stub.now}
	return projection, channelAcquisitionAssetEERReceipt("complete-recorded", projection, stub.now), nil
}

func (stub *channelAcquisitionAssetEERStub) GetTerminalOutcome(context.Context, string) (eer.TerminalOutcome, error) {
	if stub.terminalOutcome.EffectID == "" {
		return eer.TerminalOutcome{}, eer.ErrNotFound
	}
	return stub.terminalOutcome, nil
}

func TestCH02EERTerminalReaderRejectsCrossFamilyRecovery(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	stub := &channelAcquisitionAssetEERStub{now: now, terminalOutcome: eer.TerminalOutcome{
		EffectID: "eer_ch02_1", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateOutcomeUnknown,
		ReceiptID: "eerop_1", ReceiptDigest: channelAcquisitionAssetEERDigest("terminal"), Generation: 2, Fence: 3, LeaseExpiresAt: now,
	}}
	runtime, err := NewChannelAcquisitionAssetEERRuntime(stub, stub)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = runtime.Terminal(context.Background(), "eer_ch02_1"); !errors.Is(err, ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("cross-family terminal err=%v", err)
	}
	stub.terminalOutcome.Owner, stub.terminalOutcome.Kind = eer.OwnerContact, eer.KindContactAcquisitionAssetPublish
	terminal, found, err := runtime.Terminal(context.Background(), "eer_ch02_1")
	if err != nil || !found || terminal.EffectID != "eer_ch02_1" || terminal.State != eer.StateOutcomeUnknown {
		t.Fatalf("terminal=%+v found=%v err=%v", terminal, found, err)
	}
}

func channelAcquisitionAssetEERReceipt(label string, projection eer.Projection, at time.Time) eer.OperationReceipt {
	return eer.OperationReceipt{ID: label + "-receipt", EffectID: projection.ID, CommandDigest: channelAcquisitionAssetDigest(label, projection.ID), State: projection.State, CompletedAt: at}
}

func TestCH02EERRuntimeFreezesContactAcquisitionAssetFamily(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	stub := &channelAcquisitionAssetEERStub{now: now}
	runtime, err := NewChannelAcquisitionAssetEERRuntime(stub, stub)
	if err != nil {
		t.Fatal(err)
	}
	spec := ChannelAcquisitionAssetEffectSpec{
		SourceRefDigest: channelAcquisitionAssetEERDigest("source"), TargetRefDigest: channelAcquisitionAssetEERDigest("target"),
		PayloadDigest: channelAcquisitionAssetEERDigest("payload"), PolicyVersionHash: channelAcquisitionAssetEERDigest("policy"),
	}
	projection, _, err := runtime.Accept(context.Background(), ChannelAcquisitionAssetEffectAcceptCommand{ReceiptKeyDigest: channelAcquisitionAssetEERDigest("receipt"), Spec: spec})
	if err != nil || projection.ID != "eer_ch02_1" || projection.State != eer.StateAccepted {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	if stub.acceptCommand.Envelope.Owner() != eer.OwnerContact || stub.acceptCommand.Envelope.Kind() != eer.KindContactAcquisitionAssetPublish ||
		stub.acceptCommand.Envelope.SourceRefDigest() != spec.SourceRefDigest || stub.acceptCommand.Envelope.TargetRefDigest() != spec.TargetRefDigest ||
		stub.acceptCommand.Envelope.PayloadDigest() != spec.PayloadDigest || stub.acceptCommand.Envelope.PolicyVersionHash() != spec.PolicyVersionHash {
		t.Fatalf("accepted envelope owner=%q kind=%q", stub.acceptCommand.Envelope.Owner(), stub.acceptCommand.Envelope.Kind())
	}
	if projection.EnvelopeFingerprint != stub.acceptCommand.Envelope.Fingerprint() || projection.EnvelopeFingerprint == spec.Fingerprint() {
		t.Fatalf("typed envelope fingerprint=%q actual=%q spec=%q", projection.EnvelopeFingerprint, stub.acceptCommand.Envelope.Fingerprint(), spec.Fingerprint())
	}

	stub.projection = eer.Projection{ID: "eer_wrong", Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage, State: eer.StateAccepted, Generation: 1, UpdatedAt: now}
	if _, _, err = runtime.Accept(context.Background(), ChannelAcquisitionAssetEffectAcceptCommand{ReceiptKeyDigest: channelAcquisitionAssetEERDigest("receipt-2"), Spec: spec}); !errors.Is(err, ErrChannelAcquisitionAssetUnavailable) {
		t.Fatalf("wrong family error=%v", err)
	}
}

func TestCH02EERRuntimeAdapterFailsClosedBeforeProviderCallbackForWrongFamily(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	wrongEnvelope, err := eer.NewEnvelope(eer.EnvelopeInput{
		Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMessage,
		SourceRefDigest: channelAcquisitionAssetEERDigest("source"), TargetRefDigest: channelAcquisitionAssetEERDigest("target"),
		PayloadDigest: channelAcquisitionAssetEERDigest("payload"), PolicyVersionHash: channelAcquisitionAssetEERDigest("policy"),
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := eer.Lease{EffectID: "eer_ch02_2", Generation: 2, Fence: 3, ExpiresAt: now.Add(time.Minute)}
	stub := &channelAcquisitionAssetEERStub{now: now, runEnvelope: wrongEnvelope, runAttempt: eer.Attempt{Number: 1, Generation: lease.Generation, Fence: lease.Fence, StartedAt: now}}
	runtime, err := NewChannelAcquisitionAssetEERRuntime(stub, stub)
	if err != nil {
		t.Fatal(err)
	}
	providerCalls := 0
	projection, _, err := runtime.RunAttempt(context.Background(), lease, func(context.Context) (eer.AdapterResult, error) {
		providerCalls++
		return eer.AdapterResult{Completion: eer.CompletionExecuted, ReceiptDigest: channelAcquisitionAssetEERDigest("provider")}, nil
	})
	if err == nil || projection.State != eer.StateOutcomeUnknown || providerCalls != 0 || stub.runCalls != 1 {
		t.Fatalf("projection=%+v err=%v provider=%d run=%d", projection, err, providerCalls, stub.runCalls)
	}
}

func TestCH02EERRuntimeRecoveryUsesExactLease(t *testing.T) {
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	stub := &channelAcquisitionAssetEERStub{now: now}
	runtime, err := NewChannelAcquisitionAssetEERRuntime(stub, stub)
	if err != nil {
		t.Fatal(err)
	}
	lease := eer.Lease{EffectID: "eer_ch02_3", Generation: 4, Fence: 5, ExpiresAt: now.Add(-time.Minute)}
	projection, _, err := runtime.RecoverAttempted(context.Background(), lease)
	if err != nil || projection.State != eer.StateOutcomeUnknown || stub.recoverCommand.Lease != lease {
		t.Fatalf("projection=%+v recovery=%+v err=%v", projection, stub.recoverCommand, err)
	}
}

func channelAcquisitionAssetEERDigest(seed string) eer.Digest {
	sum := sha256.Sum256([]byte(seed))
	return eer.Digest("sha256:" + hex.EncodeToString(sum[:]))
}

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
)

type outboundMediaReconcileFixture struct {
	control      OutboundMediaReconcileControl
	receipt      OutboundMediaReconciliationReceipt
	found        bool
	recorded     OutboundMediaReconciliationReceipt
	runtimeCalls int
	targetDigest string
	runtime      eer.ReconcileCommand
}

func (*outboundMediaReconcileFixture) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

func (f *outboundMediaReconcileFixture) LockOutboundMediaEffectForReconcile(_ context.Context, _ int64, targetDigest string) (OutboundMediaReconcileControl, error) {
	f.targetDigest = targetDigest
	return f.control, nil
}

func (f *outboundMediaReconcileFixture) ReadOutboundMediaReconciliationReceipt(context.Context, string) (OutboundMediaReconciliationReceipt, bool, error) {
	return f.receipt, f.found, nil
}

func (f *outboundMediaReconcileFixture) RecordOutboundMediaReconciliationReceipt(_ context.Context, receipt OutboundMediaReconciliationReceipt) error {
	f.recorded = receipt
	return nil
}

func (f *outboundMediaReconcileFixture) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	f.runtimeCalls++
	f.runtime = command
	return eer.Projection{ID: command.Lease.EffectID, Owner: eer.OwnerOutbound, Kind: eer.KindOutboundMedia, State: eer.StateReconciled}, eer.OperationReceipt{CommandDigest: eer.Digest(mediaEERDigest("outbound-media-eer-receipt"))}, nil
}

func TestOutboundMediaReconcilePersistsVerifiedReceiptWithoutRetry(t *testing.T) {
	command := outboundMediaReconcileTestCommand()
	fixture := &outboundMediaReconcileFixture{control: outboundMediaReconcileTestControl(string(eer.StateOutcomeUnknown))}
	result, err := NewOutboundMediaReconcileService(fixture, fixture, fixture).Reconcile(context.Background(), command)
	if err != nil || fixture.runtimeCalls != 1 || result.Replay || result.EffectID != "eer_7" || result.State != string(eer.StateReconciled) || fixture.targetDigest != mediaEERDigest("outbound-media-target", command.TargetRef) || fixture.runtime.Lease.Generation != command.Generation || fixture.runtime.Lease.Fence != command.Fence || fixture.runtime.EvidenceDigest != eer.Digest(command.EvidenceDigest) || fixture.recorded.EvidenceDigest != command.EvidenceDigest || fixture.recorded.ProviderAccepted != command.ProviderAccepted || fixture.recorded.DeliveryProven != command.DeliveryProven {
		t.Fatalf("result=%#v recorded=%#v runtime=%#v target=%s calls=%d err=%v", result, fixture.recorded, fixture.runtime, fixture.targetDigest, fixture.runtimeCalls, err)
	}
}

func TestOutboundMediaReconcileReplaysOnlyExactImmutableReceipt(t *testing.T) {
	command := outboundMediaReconcileTestCommand()
	fixture := &outboundMediaReconcileFixture{control: outboundMediaReconcileTestControl(string(eer.StateReconciled)), found: true}
	fixture.receipt = OutboundMediaReconciliationReceipt{EffectID: fixture.control.EffectID, Generation: command.Generation, Fence: command.Fence, LeaseExpiresAt: command.LeaseExpiresAt, EvidenceDigest: command.EvidenceDigest, ProviderAccepted: command.ProviderAccepted, DeliveryProven: command.DeliveryProven, EERReceiptDigest: mediaEERDigest("outbound-media-eer-receipt")}
	result, err := NewOutboundMediaReconcileService(fixture, fixture, fixture).Reconcile(context.Background(), command)
	if err != nil || !result.Replay || fixture.runtimeCalls != 0 || result.ProviderAccepted != command.ProviderAccepted || result.DeliveryProven != command.DeliveryProven {
		t.Fatalf("result=%#v calls=%d err=%v", result, fixture.runtimeCalls, err)
	}

	command.EvidenceDigest = mediaEERDigest("different-evidence")
	_, err = NewOutboundMediaReconcileService(fixture, fixture, fixture).Reconcile(context.Background(), command)
	if !errors.Is(err, ErrOutboundMediaReconcileConflict) || fixture.runtimeCalls != 0 {
		t.Fatalf("conflict err=%v calls=%d", err, fixture.runtimeCalls)
	}
}

func TestOutboundMediaReconcileRequiresUnknownAndConsistentVerification(t *testing.T) {
	command := outboundMediaReconcileTestCommand()
	fixture := &outboundMediaReconcileFixture{control: outboundMediaReconcileTestControl(string(eer.StateQueued))}
	_, err := NewOutboundMediaReconcileService(fixture, fixture, fixture).Reconcile(context.Background(), command)
	if !errors.Is(err, ErrOutboundMediaReconcileConflict) || fixture.runtimeCalls != 0 {
		t.Fatalf("state err=%v calls=%d", err, fixture.runtimeCalls)
	}
	command.DeliveryProven, command.ProviderAccepted = true, false
	_, err = NewOutboundMediaReconcileService(fixture, fixture, fixture).Reconcile(context.Background(), command)
	if !errors.Is(err, ErrOutboundMediaReconcileInvalid) {
		t.Fatalf("verification err=%v", err)
	}
}

func outboundMediaReconcileTestCommand() OutboundMediaReconcileCommand {
	return OutboundMediaReconcileCommand{ContentPackageID: 42, TargetRef: "external_contact_7", Generation: 3, Fence: 9, LeaseExpiresAt: time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC), EvidenceDigest: mediaEERDigest("outbound-media-verified-evidence"), ProviderAccepted: true, DeliveryProven: true}
}

func outboundMediaReconcileTestControl(state string) OutboundMediaReconcileControl {
	command := outboundMediaReconcileTestCommand()
	return OutboundMediaReconcileControl{EffectID: "eer_7", State: state, Generation: command.Generation, Fence: command.Fence, LeaseExpiresAt: command.LeaseExpiresAt}
}

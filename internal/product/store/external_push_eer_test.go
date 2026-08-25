package store

import (
	"context"
	"strings"
	"testing"
	"time"

	eerport "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects/port"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

func TestCommerceExternalPushEERAccepterCreatesOnlyLocalAcceptedFact(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	runtime := &commerceExternalPushEERRuntime{projection: eerport.Projection{
		ID: "eer_44", Owner: eerport.OwnerProduct, Kind: eerport.KindProductExternalPushTest,
		State: eerport.StateAccepted, Generation: 1, UpdatedAt: now,
	}}
	accepter := NewCommerceExternalPushEERAccepter(runtime)
	result, err := accepter.AcceptProductExternalPushTest(context.Background(), productapp.ProductExternalPushEffectCommand{
		ProductID: 9, ProductKind: productport.ExternalPushWeChatPay,
		ConfigurationDigest: [32]byte{1}, ReceiptKeyDigest: [32]byte{2},
	})
	if err != nil {
		t.Fatalf("AcceptProductExternalPushTest() error = %v", err)
	}
	if result.EffectID != "eer_44" || result.State != "accepted" || result.ProviderAccepted || result.DeliveryProven ||
		result.RealExternalCallExecuted || result.AutoRetryAllowed || !result.CreatedAt.Equal(now) {
		t.Fatalf("unexpected local acceptance result: %#v", result)
	}
	if runtime.command.Envelope.Owner() != eerport.OwnerProduct || runtime.command.Envelope.Kind() != eerport.KindProductExternalPushTest ||
		!strings.HasPrefix(string(runtime.command.ReceiptKeyDigest), "sha256:") {
		t.Fatalf("unexpected EER command: %#v", runtime.command)
	}
}

func TestProductExternalPushEffectIDRejectsNonOpaqueOrNumericLeak(t *testing.T) {
	for _, value := range []string{"", "44", "eer_0", "eer_01", "eer_x", "eer_-1", "eer_1x"} {
		if _, err := productExternalPushEffectID(value); err == nil {
			t.Fatalf("productExternalPushEffectID(%q) unexpectedly accepted", value)
		}
	}
	if got, err := productExternalPushEffectID("eer_44"); err != nil || got != 44 {
		t.Fatalf("productExternalPushEffectID() = %d, %v", got, err)
	}
}

type commerceExternalPushEERRuntime struct {
	command    eerport.AcceptCommand
	projection eerport.Projection
}

func (r *commerceExternalPushEERRuntime) Accept(_ context.Context, command eerport.AcceptCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	r.command = command
	return r.projection, eerport.OperationReceipt{ID: "eer_receipt_1", EffectID: r.projection.ID, State: r.projection.State, CompletedAt: r.projection.UpdatedAt}, nil
}
func (*commerceExternalPushEERRuntime) Queue(context.Context, eerport.QueueCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, productapp.ErrUnavailable
}
func (*commerceExternalPushEERRuntime) Claim(context.Context, eerport.ClaimCommand) (eerport.Lease, eerport.Projection, error) {
	return eerport.Lease{}, eerport.Projection{}, productapp.ErrUnavailable
}
func (*commerceExternalPushEERRuntime) RunAttempt(context.Context, eerport.Lease, eerport.Adapter) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, productapp.ErrUnavailable
}
func (*commerceExternalPushEERRuntime) Reconcile(context.Context, eerport.ReconcileCommand) (eerport.Projection, eerport.OperationReceipt, error) {
	return eerport.Projection{}, eerport.OperationReceipt{}, productapp.ErrUnavailable
}

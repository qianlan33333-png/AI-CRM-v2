package app

import (
	"context"
	"errors"
	"testing"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

func TestWC01GenericMutationsCannotBypassTypedWeComTagProjection(t *testing.T) {
	store := &weComTagGuardStore{projection: eer.Projection{ID: "eer_1", Owner: eer.OwnerWeCom, Kind: eer.KindWeComTagSync}}
	service, err := NewService(store, store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, mutate := range map[string]func() error{
		"cancel": func() error {
			_, _, err := service.Cancel(ctx, eer.CancelCommand{EffectID: "eer_1"})
			return err
		},
		"retry": func() error {
			_, _, err := service.Retry(ctx, eer.RetryCommand{EffectID: "eer_1"})
			return err
		},
		"reconcile": func() error {
			_, _, err := service.Reconcile(ctx, eer.ReconcileCommand{Lease: eer.Lease{EffectID: "eer_1"}})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := mutate(); !errors.Is(err, eer.ErrReconcileRequired) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	if store.mutations != 0 {
		t.Fatalf("generic runtime mutations=%d", store.mutations)
	}
}

type weComTagGuardStore struct {
	projection eer.Projection
	mutations  int
}

func (store *weComTagGuardStore) Get(context.Context, string) (eer.Projection, error) {
	return store.projection, nil
}
func (*weComTagGuardStore) List(context.Context, int32) ([]eer.Projection, error) {
	return nil, nil
}
func (*weComTagGuardStore) Diagnostics(context.Context) (eer.Diagnostics, error) {
	return eer.Diagnostics{}, nil
}
func (*weComTagGuardStore) Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*weComTagGuardStore) Queue(context.Context, eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*weComTagGuardStore) Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	return eer.Lease{}, eer.Projection{}, nil
}
func (store *weComTagGuardStore) Retry(context.Context, eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (store *weComTagGuardStore) Cancel(context.Context, eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*weComTagGuardStore) PersistAttempt(context.Context, eer.Lease) (eer.EffectEnvelope, eer.Attempt, eer.Projection, error) {
	return eer.EffectEnvelope{}, eer.Attempt{}, eer.Projection{}, nil
}
func (*weComTagGuardStore) CompleteAttempt(context.Context, eer.Lease, eer.Attempt, eer.AdapterResult) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (store *weComTagGuardStore) Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*weComTagGuardStore) RecoverAttemptedToUnknown(context.Context, eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}

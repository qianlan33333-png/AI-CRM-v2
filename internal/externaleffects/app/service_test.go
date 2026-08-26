package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

func TestGenericMutationsCannotBypassTypedDomainControl(t *testing.T) {
	typed := []struct {
		owner eer.Owner
		kind  eer.Kind
	}{
		{eer.OwnerOutbound, eer.KindOutboundMessage},
		{eer.OwnerOutbound, eer.KindOutboundMedia},
		{eer.OwnerContact, eer.KindContactAcquisitionAssetPublish},
		{eer.OwnerWeCom, eer.KindWeComTagSync},
		{eer.OwnerSurvey, eer.KindSurveyWebhook},
		{eer.OwnerOrder, eer.KindOrderPaymentPrepay},
		{eer.OwnerOrder, eer.KindOrderRefund},
		{eer.OwnerGroupOps, eer.KindGroupOpsBroadcast}, {eer.OwnerMedia, eer.KindMediaWeComUpload},
		{eer.OwnerProduct, eer.KindProductExternalPushTest},
	}
	digest := eer.Digest("sha256:" + strings.Repeat("a", 64))
	now := time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC)
	mutations := []struct {
		name  string
		state eer.State
		run   func(*Service) error
	}{
		{name: "cancel", state: eer.StateAccepted, run: func(service *Service) error {
			_, _, err := service.Cancel(context.Background(), eer.CancelCommand{EffectID: "eer_1", ReceiptKeyDigest: digest})
			return err
		}},
		{name: "retry", state: eer.StateRetryableFailed, run: func(service *Service) error {
			_, _, err := service.Retry(context.Background(), eer.RetryCommand{
				EffectID: "eer_1", ReceiptKeyDigest: digest,
				Job: eer.RiverJobLink{JobID: 42, Generation: 2, Queue: "external-effects", ArgsDigest: digest, ScheduledAt: now},
			})
			return err
		}},
		{name: "retry_outcome_unknown", state: eer.StateOutcomeUnknown, run: func(service *Service) error {
			_, _, err := service.Retry(context.Background(), eer.RetryCommand{
				EffectID: "eer_1", ReceiptKeyDigest: digest,
				Job: eer.RiverJobLink{JobID: 43, Generation: 2, Queue: "external-effects", ArgsDigest: digest, ScheduledAt: now},
			})
			return err
		}},
		{name: "reconcile", state: eer.StateOutcomeUnknown, run: func(service *Service) error {
			_, _, err := service.Reconcile(context.Background(), eer.ReconcileCommand{
				Lease:            eer.Lease{EffectID: "eer_1", Generation: 1, Fence: 1, ExpiresAt: now},
				ReceiptKeyDigest: digest, EvidenceDigest: digest,
			})
			return err
		}},
	}

	for _, pair := range typed {
		pair := pair
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(string(pair.owner)+"/"+string(pair.kind)+"/"+mutation.name, func(t *testing.T) {
				store := &typedControlGuardStore{projection: eer.Projection{
					ID: "eer_1", Owner: pair.owner, Kind: pair.kind, State: mutation.state,
				}}
				service, err := NewService(store, store)
				if err != nil {
					t.Fatal(err)
				}
				if err = mutation.run(service); !errors.Is(err, eer.ErrReconcileRequired) {
					t.Fatalf("err=%v", err)
				}
				if store.mutations != 0 {
					t.Fatalf("generic runtime mutations=%d", store.mutations)
				}
			})
		}
	}
}

func TestGenericControlRemainsAvailableForUnboundEffect(t *testing.T) {
	now := time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC)
	digest := eer.Digest("sha256:" + strings.Repeat("b", 64))
	store := &typedControlGuardStore{projection: eer.Projection{
		ID: "eer_2", Owner: eer.OwnerCampaign, Kind: eer.KindCampaignDispatch,
		State: eer.StateAccepted, Generation: 1, UpdatedAt: now,
	}}
	service, err := NewService(store, store)
	if err != nil {
		t.Fatal(err)
	}
	projection, _, err := service.Cancel(context.Background(), eer.CancelCommand{
		EffectID: "eer_2", ReceiptKeyDigest: digest,
	})
	if err != nil || projection.State != eer.StateCancelled || store.mutations != 1 {
		t.Fatalf("projection=%#v mutations=%d err=%v", projection, store.mutations, err)
	}
}

func TestGenericMutationFailsClosedOnReaderIdentityMismatch(t *testing.T) {
	store := &typedControlGuardStore{projection: eer.Projection{
		ID: "eer_2", Owner: eer.OwnerCampaign, Kind: eer.KindCampaignDispatch,
	}}
	service, err := NewService(store, store)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = service.Cancel(context.Background(), eer.CancelCommand{EffectID: "eer_1"})
	if !errors.Is(err, eer.ErrUnavailable) || store.mutations != 0 {
		t.Fatalf("mutations=%d err=%v", store.mutations, err)
	}
}

func TestGenericMutationFailsClosedOnIllegalEffectFamily(t *testing.T) {
	for _, projection := range []eer.Projection{
		{ID: "eer_1", Owner: eer.Owner("provider"), Kind: eer.KindOutboundMessage},
		{ID: "eer_1", Owner: eer.OwnerContact, Kind: eer.KindWeComTagSync},
	} {
		store := &typedControlGuardStore{projection: projection}
		service, err := NewService(store, store)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = service.Cancel(context.Background(), eer.CancelCommand{EffectID: "eer_1"})
		if !errors.Is(err, eer.ErrUnavailable) || store.mutations != 0 {
			t.Fatalf("projection=%+v mutations=%d err=%v", projection, store.mutations, err)
		}
	}
}

type typedControlGuardStore struct {
	projection eer.Projection
	mutations  int
}

func (store *typedControlGuardStore) Get(context.Context, string) (eer.Projection, error) {
	return store.projection, nil
}
func (*typedControlGuardStore) List(context.Context, int32) ([]eer.Projection, error) {
	return nil, nil
}
func (*typedControlGuardStore) Diagnostics(context.Context) (eer.Diagnostics, error) {
	return eer.Diagnostics{}, nil
}
func (*typedControlGuardStore) Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*typedControlGuardStore) Queue(context.Context, eer.QueueCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*typedControlGuardStore) Claim(context.Context, eer.ClaimCommand) (eer.Lease, eer.Projection, error) {
	return eer.Lease{}, eer.Projection{}, nil
}
func (store *typedControlGuardStore) Retry(context.Context, eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (store *typedControlGuardStore) Cancel(_ context.Context, command eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	projection := store.projection
	projection.State = eer.StateCancelled
	return projection, eer.OperationReceipt{
		ID: "receipt_1", EffectID: command.EffectID, CommandDigest: command.CommandDigest(),
		State: eer.StateCancelled, CompletedAt: projection.UpdatedAt,
	}, nil
}
func (*typedControlGuardStore) PersistAttempt(context.Context, eer.Lease) (eer.EffectEnvelope, eer.Attempt, eer.Projection, error) {
	return eer.EffectEnvelope{}, eer.Attempt{}, eer.Projection{}, nil
}
func (*typedControlGuardStore) CompleteAttempt(context.Context, eer.Lease, eer.Attempt, eer.AdapterResult) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (store *typedControlGuardStore) Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	store.mutations++
	return eer.Projection{}, eer.OperationReceipt{}, nil
}
func (*typedControlGuardStore) RecoverAttemptedToUnknown(context.Context, eer.RecoverAttemptedCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, nil
}

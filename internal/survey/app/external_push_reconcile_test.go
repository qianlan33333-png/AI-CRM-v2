package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

type inlineExternalPushUoW struct{}

func (inlineExternalPushUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type externalPushReconcileStoreStub struct {
	verify      ExternalPushBinding
	verifyErr   error
	recorded    ExternalPushBinding
	recordErr   error
	verifyCalls int
	recordCalls int
}

func (*externalPushReconcileStoreStub) BindExternalPush(context.Context, ExternalPushBinding) (ExternalPushBinding, error) {
	return ExternalPushBinding{}, ErrExternalPushUnavailable
}
func (*externalPushReconcileStoreStub) GetExternalPush(context.Context, surveyport.ID, int64) (ExternalPushBinding, error) {
	return ExternalPushBinding{}, ErrExternalPushUnavailable
}
func (s *externalPushReconcileStoreStub) VerifyExternalPushReconcile(_ context.Context, _ ExternalPushReconcileCommand) (ExternalPushBinding, error) {
	s.verifyCalls++
	return s.verify, s.verifyErr
}
func (s *externalPushReconcileStoreStub) RecordExternalPushReconcile(_ context.Context, _ ExternalPushReconcileCommand) (ExternalPushBinding, error) {
	s.recordCalls++
	return s.recorded, s.recordErr
}

type externalPushReconcileRuntimeStub struct {
	projection     eer.Projection
	err            error
	reconcileCalls int
	command        eer.ReconcileCommand
}

func (*externalPushReconcileRuntimeStub) Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error) {
	return eer.Projection{}, eer.OperationReceipt{}, ErrExternalPushUnavailable
}
func (s *externalPushReconcileRuntimeStub) Reconcile(_ context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	s.reconcileCalls++
	s.command = command
	return s.projection, eer.OperationReceipt{}, s.err
}

func TestExternalPushReconcileOnlyUsesVerifiedUnknownBindingAndRecordsSeparateReceiptFacts(t *testing.T) {
	lease := externalPushReconcileLease()
	store := &externalPushReconcileStoreStub{
		verify:   ExternalPushBinding{QuestionnaireID: 9, SubmissionID: 12, EffectID: lease.EffectID, State: eer.StateOutcomeUnknown},
		recorded: ExternalPushBinding{QuestionnaireID: 9, SubmissionID: 12, EffectID: lease.EffectID, State: eer.StateReconciled, ProviderAccepted: true, DeliveryProven: false},
	}
	runtime := &externalPushReconcileRuntimeStub{projection: eer.Projection{ID: lease.EffectID, Owner: eer.OwnerSurvey, Kind: eer.KindSurveyWebhook, State: eer.StateReconciled, Generation: lease.Generation, UpdatedAt: time.Now().UTC()}}
	service, err := NewExternalPushService(inlineExternalPushUoW{}, store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	evidence := sha256.Sum256([]byte("operator-verified-receipt"))
	result, err := service.Reconcile(context.Background(), ExternalPushReconcileCommand{
		QuestionnaireID: 9, SubmissionID: 12, Lease: lease, EvidenceDigest: evidence, ProviderAccepted: true, DeliveryProven: false,
		IdempotencyKey: "survey-external-push-reconcile-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != eer.StateReconciled || !result.ProviderAccepted || result.DeliveryProven || store.verifyCalls != 1 || store.recordCalls != 1 || runtime.reconcileCalls != 1 {
		t.Fatalf("result=%+v calls=%d/%d/%d", result, store.verifyCalls, store.recordCalls, runtime.reconcileCalls)
	}
	if runtime.command.Lease != lease || runtime.command.EvidenceDigest != pushDigest(evidence) || runtime.command.ReceiptKeyDigest == "" {
		t.Fatalf("runtime command=%+v", runtime.command)
	}
}

func TestExternalPushReconcileFailsClosedBeforeAnyAutomaticRetryPath(t *testing.T) {
	lease := externalPushReconcileLease()
	store := &externalPushReconcileStoreStub{verifyErr: ErrExternalPushReconcileRequired}
	runtime := &externalPushReconcileRuntimeStub{}
	service, err := NewExternalPushService(inlineExternalPushUoW{}, store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	evidence := sha256.Sum256([]byte("operator-verified-receipt"))
	_, err = service.Reconcile(context.Background(), ExternalPushReconcileCommand{QuestionnaireID: 9, SubmissionID: 12, Lease: lease, EvidenceDigest: evidence, IdempotencyKey: "survey-external-push-reconcile-key"})
	if !errors.Is(err, ErrExternalPushReconcileRequired) || store.recordCalls != 0 || runtime.reconcileCalls != 0 {
		t.Fatalf("err/calls=%v/%d/%d", err, store.recordCalls, runtime.reconcileCalls)
	}

	_, err = service.Reconcile(context.Background(), ExternalPushReconcileCommand{QuestionnaireID: 9, SubmissionID: 12, Lease: lease, IdempotencyKey: "survey-external-push-reconcile-key"})
	if !errors.Is(err, ErrExternalPushReconcileRequired) || store.verifyCalls != 1 {
		t.Fatalf("invalid evidence err/calls=%v/%d", err, store.verifyCalls)
	}
}

func TestExternalPushReconcileRejectsNonTerminalRuntimeResult(t *testing.T) {
	lease := externalPushReconcileLease()
	store := &externalPushReconcileStoreStub{verify: ExternalPushBinding{QuestionnaireID: 9, SubmissionID: 12, EffectID: lease.EffectID, State: eer.StateOutcomeUnknown}}
	runtime := &externalPushReconcileRuntimeStub{projection: eer.Projection{ID: lease.EffectID, Owner: eer.OwnerSurvey, Kind: eer.KindSurveyWebhook, State: eer.StateOutcomeUnknown, Generation: lease.Generation, UpdatedAt: time.Now().UTC()}}
	service, err := NewExternalPushService(inlineExternalPushUoW{}, store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	evidence := sha256.Sum256([]byte("operator-verified-receipt"))
	_, err = service.Reconcile(context.Background(), ExternalPushReconcileCommand{QuestionnaireID: 9, SubmissionID: 12, Lease: lease, EvidenceDigest: evidence, IdempotencyKey: "survey-external-push-reconcile-key"})
	if !errors.Is(err, ErrExternalPushReconcileRequired) || store.recordCalls != 0 || runtime.reconcileCalls != 1 {
		t.Fatalf("err/calls=%v/%d/%d", err, store.recordCalls, runtime.reconcileCalls)
	}
}

func externalPushReconcileLease() eer.Lease {
	return eer.Lease{EffectID: "eer_123", Generation: 2, Fence: 5, ExpiresAt: time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)}
}

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	surveyport "github.com/qianlan33333-png/AI-CRM-v2/internal/survey/port"
)

var (
	ErrExternalPushUnavailable       = errors.New("survey external push unavailable")
	ErrExternalPushNotFound          = errors.New("survey external push binding not found")
	ErrExternalPushReconcileRequired = errors.New("survey external push reconciliation required")
	ErrExternalPushReconcileConflict = errors.New("survey external push reconciliation conflict")
)

type ExternalPushBinding struct {
	ID, SubmissionID, CustomerID                                                 int64
	QuestionnaireID                                                              surveyport.ID
	EffectID, SourceRefDigest, TargetRefDigest, PayloadDigest, PolicyVersionHash string
	State                                                                        eer.State
	ProviderAccepted, DeliveryProven                                             bool
	CreatedAt                                                                    time.Time
}
type ExternalPushCommand struct {
	QuestionnaireID                                                    surveyport.ID
	SubmissionID, CustomerID                                           int64
	SourceRefDigest, TargetRefDigest, PayloadDigest, PolicyVersionHash [32]byte
	IdempotencyKey                                                     string
}
type ExternalPushReconcileCommand struct {
	QuestionnaireID                  surveyport.ID
	SubmissionID                     int64
	Lease                            eer.Lease
	EvidenceDigest                   [32]byte
	ProviderAccepted, DeliveryProven bool
	IdempotencyKey                   string
}
type ExternalPushStore interface {
	BindExternalPush(context.Context, ExternalPushBinding) (ExternalPushBinding, error)
	GetExternalPush(context.Context, surveyport.ID, int64) (ExternalPushBinding, error)
	VerifyExternalPushReconcile(context.Context, ExternalPushReconcileCommand) (ExternalPushBinding, error)
	RecordExternalPushReconcile(context.Context, ExternalPushReconcileCommand) (ExternalPushBinding, error)
}
type externalPushRuntime interface {
	Accept(context.Context, eer.AcceptCommand) (eer.Projection, eer.OperationReceipt, error)
	Reconcile(context.Context, eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error)
}
type ExternalPushService struct {
	uow     platformport.UnitOfWork
	store   ExternalPushStore
	runtime externalPushRuntime
}

type PublicExternalPushBinder struct{ Push *ExternalPushService }

func (b PublicExternalPushBinder) BindPublicSubmission(ctx context.Context, record PublicDefinitionRecord, submissionID int64, input surveyport.PublicSubmissionCommand, _ time.Time) error {
	if b.Push == nil || input.CanonicalCustomerID < 1 {
		return ErrH5IdentityRequired
	}
	payload := sha256.Sum256([]byte(fmt.Sprintf("survey-public\x00%d\x00%d", record.ID, submissionID)))
	_, err := b.Push.Accept(ctx, ExternalPushCommand{QuestionnaireID: record.View.ID, SubmissionID: submissionID, CustomerID: input.CanonicalCustomerID, SourceRefDigest: sha256.Sum256([]byte(fmt.Sprintf("submission\x00%d", submissionID))), TargetRefDigest: sha256.Sum256([]byte(record.View.Slug)), PayloadDigest: payload, PolicyVersionHash: sha256.Sum256([]byte("survey-webhook-v1")), IdempotencyKey: input.SubmissionKey})
	return err
}

func NewExternalPushService(uow platformport.UnitOfWork, store ExternalPushStore, runtime externalPushRuntime) (*ExternalPushService, error) {
	if uow == nil || store == nil || runtime == nil {
		return nil, ErrExternalPushUnavailable
	}
	return &ExternalPushService{uow, store, runtime}, nil
}
func (s *ExternalPushService) Accept(ctx context.Context, c ExternalPushCommand) (ExternalPushBinding, error) {
	if s == nil || c.QuestionnaireID < 1 || c.SubmissionID < 1 || c.CustomerID < 1 || len(c.IdempotencyKey) < 16 {
		return ExternalPushBinding{}, ErrExternalPushUnavailable
	}
	var out ExternalPushBinding
	err := s.uow.Within(ctx, func(tx context.Context) error {
		env, err := eer.NewEnvelope(eer.EnvelopeInput{Owner: eer.OwnerSurvey, Kind: eer.KindSurveyWebhook, SourceRefDigest: pushDigest(c.SourceRefDigest), TargetRefDigest: pushDigest(c.TargetRefDigest), PayloadDigest: pushDigest(c.PayloadDigest), PolicyVersionHash: pushDigest(c.PolicyVersionHash)})
		if err != nil {
			return err
		}
		p, _, err := s.runtime.Accept(tx, eer.AcceptCommand{ReceiptKeyDigest: pushTextDigest("survey/push/accept", c.IdempotencyKey), Envelope: env})
		if err != nil {
			return err
		}
		out, err = s.store.BindExternalPush(tx, ExternalPushBinding{QuestionnaireID: c.QuestionnaireID, SubmissionID: c.SubmissionID, CustomerID: c.CustomerID, EffectID: p.ID, SourceRefDigest: string(pushDigest(c.SourceRefDigest)), TargetRefDigest: string(pushDigest(c.TargetRefDigest)), PayloadDigest: string(pushDigest(c.PayloadDigest)), PolicyVersionHash: string(pushDigest(c.PolicyVersionHash)), State: p.State, CreatedAt: time.Now().UTC()})
		return err
	})
	if err != nil {
		return ExternalPushBinding{}, fmt.Errorf("%w: %v", ErrExternalPushUnavailable, err)
	}
	if out.EffectID == "" || out.State != eer.StateAccepted {
		return ExternalPushBinding{}, ErrExternalPushUnavailable
	}
	return out, nil
}
func (s *ExternalPushService) Detail(ctx context.Context, q surveyport.ID, submissionID int64) (ExternalPushBinding, error) {
	if s == nil || q < 1 || submissionID < 1 {
		return ExternalPushBinding{}, ErrExternalPushUnavailable
	}
	v, e := s.store.GetExternalPush(ctx, q, submissionID)
	if e != nil || v.QuestionnaireID != q || v.SubmissionID != submissionID || v.EffectID == "" {
		return ExternalPushBinding{}, ErrExternalPushUnavailable
	}
	return v, nil
}
func (s *ExternalPushService) Reconcile(ctx context.Context, c ExternalPushReconcileCommand) (ExternalPushBinding, error) {
	if s == nil || c.QuestionnaireID < 1 || c.SubmissionID < 1 || !c.LeaseFieldsValid() || c.EvidenceDigest == [32]byte{} || len(c.IdempotencyKey) < 16 {
		return ExternalPushBinding{}, ErrExternalPushReconcileRequired
	}
	var out ExternalPushBinding
	err := s.uow.Within(ctx, func(tx context.Context) error {
		binding, err := s.store.VerifyExternalPushReconcile(tx, c)
		if err != nil {
			return err
		}
		if binding.EffectID != c.Lease.EffectID {
			return ErrExternalPushReconcileRequired
		}
		projection, _, err := s.runtime.Reconcile(tx, eer.ReconcileCommand{
			Lease: c.Lease, ReceiptKeyDigest: pushTextDigest("survey/push/reconcile", c.IdempotencyKey), EvidenceDigest: pushDigest(c.EvidenceDigest),
		})
		if err != nil {
			return err
		}
		if projection.ID != binding.EffectID || projection.State != eer.StateReconciled {
			return ErrExternalPushReconcileRequired
		}
		out, err = s.store.RecordExternalPushReconcile(tx, c)
		return err
	})
	if err != nil {
		return ExternalPushBinding{}, err
	}
	if out.EffectID != c.Lease.EffectID || out.State != eer.StateReconciled {
		return ExternalPushBinding{}, ErrExternalPushReconcileRequired
	}
	return out, nil
}

func (c ExternalPushReconcileCommand) LeaseFieldsValid() bool {
	return c.Lease.EffectID != "" && c.Lease.Generation > 0 && c.Lease.Fence > 0 && !c.Lease.ExpiresAt.IsZero()
}
func pushDigest(v [32]byte) eer.Digest { return eer.Digest("sha256:" + hex.EncodeToString(v[:])) }
func pushTextDigest(label, value string) eer.Digest {
	v := sha256.Sum256([]byte(label + "\x00" + value))
	return pushDigest(v)
}

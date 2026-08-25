// Package app owns the operator-safe external-effects application boundary.
package app

import (
	"context"

	eer "github.com/qianlan33333-png/AI-CRM-v2/internal/externaleffects"
)

type Reader interface {
	List(context.Context, int32) ([]eer.Projection, error)
	Get(context.Context, string) (eer.Projection, error)
	Diagnostics(context.Context) (eer.Diagnostics, error)
}

type Service struct {
	runtime *eer.Service
	reader  Reader
}

func NewService(store eer.Store, reader Reader) (*Service, error) {
	runtime, err := eer.NewService(store)
	if err != nil || reader == nil {
		return nil, eer.ErrInvalidCommand
	}
	return &Service{runtime: runtime, reader: reader}, nil
}

func (s *Service) List(ctx context.Context, limit int32) ([]eer.Projection, error) {
	if s == nil || s.reader == nil || ctx == nil || limit < 1 || limit > 100 {
		return nil, eer.ErrInvalidCommand
	}
	return s.reader.List(ctx, limit)
}
func (s *Service) Detail(ctx context.Context, id string) (eer.Projection, error) {
	if s == nil || s.reader == nil || ctx == nil {
		return eer.Projection{}, eer.ErrInvalidCommand
	}
	return s.reader.Get(ctx, id)
}
func (s *Service) Diagnostics(ctx context.Context) (eer.Diagnostics, error) {
	if s == nil || s.reader == nil || ctx == nil {
		return eer.Diagnostics{}, eer.ErrInvalidCommand
	}
	return s.reader.Diagnostics(ctx)
}
func (s *Service) Cancel(ctx context.Context, command eer.CancelCommand) (eer.Projection, eer.OperationReceipt, error) {
	if s == nil || s.runtime == nil {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrUnavailable
	}
	if err := s.rejectTypedDomainMutation(ctx, command.EffectID); err != nil {
		return eer.Projection{}, eer.OperationReceipt{}, err
	}
	return s.runtime.Cancel(ctx, command)
}
func (s *Service) Retry(ctx context.Context, command eer.RetryCommand) (eer.Projection, eer.OperationReceipt, error) {
	if s == nil || s.runtime == nil {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrUnavailable
	}
	if err := s.rejectTypedDomainMutation(ctx, command.EffectID); err != nil {
		return eer.Projection{}, eer.OperationReceipt{}, err
	}
	return s.runtime.Retry(ctx, command)
}
func (s *Service) Reconcile(ctx context.Context, command eer.ReconcileCommand) (eer.Projection, eer.OperationReceipt, error) {
	if s == nil || s.runtime == nil {
		return eer.Projection{}, eer.OperationReceipt{}, eer.ErrUnavailable
	}
	if err := s.rejectTypedDomainMutation(ctx, command.Lease.EffectID); err != nil {
		return eer.Projection{}, eer.OperationReceipt{}, err
	}
	return s.runtime.Reconcile(ctx, command)
}

func (s *Service) rejectTypedDomainMutation(ctx context.Context, effectID string) error {
	if s == nil || s.reader == nil || ctx == nil || effectID == "" {
		return eer.ErrInvalidCommand
	}
	projection, err := s.reader.Get(ctx, effectID)
	if err != nil {
		return err
	}
	if projection.ID != effectID {
		return eer.ErrUnavailable
	}
	if typedDomainControlRequired(projection.Owner, projection.Kind) {
		return eer.ErrReconcileRequired
	}
	return nil
}

// typedDomainControlRequired freezes the effect families that keep a second,
// owner-domain state fact. Their controls must update EER, the typed fact, and
// any River job in the owning domain's UoW.
func typedDomainControlRequired(owner eer.Owner, kind eer.Kind) bool {
	switch owner {
	case eer.OwnerOutbound:
		return kind == eer.KindOutboundMessage || kind == eer.KindOutboundMedia
	case eer.OwnerWeCom:
		return kind == eer.KindWeComTagSync
	case eer.OwnerSurvey:
		return kind == eer.KindSurveyWebhook
	case eer.OwnerOrder:
		return kind == eer.KindOrderPaymentPrepay || kind == eer.KindOrderRefund
	case eer.OwnerGroupOps:
		return kind == eer.KindGroupOpsBroadcast
	case eer.OwnerProduct:
		return kind == eer.KindProductExternalPushTest
	default:
		return false
	}
}

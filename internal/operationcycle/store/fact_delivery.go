package store

import (
	"context"
	"errors"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

// FactDeliveryConsumer closes the local durable event-delivery receipt. It is
// deliberately a no-op projection: operation-cycle facts must not trigger a
// provider call, timer, or automated retry.
type FactDeliveryConsumer struct {
	uow        platformport.UnitOfWork
	deliveries eventport.DeliveryCompleter
}

var _ eventport.DeliverySubscriber = (*FactDeliveryConsumer)(nil)

func NewFactDeliveryConsumer(uow platformport.UnitOfWork, deliveries eventport.DeliveryCompleter) (*FactDeliveryConsumer, error) {
	if uow == nil || deliveries == nil {
		return nil, errors.New("operation-cycle fact delivery dependency is required")
	}
	return &FactDeliveryConsumer{uow: uow, deliveries: deliveries}, nil
}

func (*FactDeliveryConsumer) Consumer() string { return eventport.ConsumerOperationCycleFact }
func (*FactDeliveryConsumer) EventTypes() []string {
	return []string{eventport.EvOperationCycleFact}
}

func (consumer *FactDeliveryConsumer) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	if consumer == nil || consumer.uow == nil || consumer.deliveries == nil || claim.Record.ID <= 0 || claim.Record.Type != eventport.EvOperationCycleFact || claim.Consumer != eventport.ConsumerOperationCycleFact || claim.Owner == "" || claim.Status != eventport.DeliveryProcessing {
		return eventport.PoisonDelivery(errors.New("invalid operation-cycle fact delivery"))
	}
	return consumer.uow.Within(ctx, func(txCtx context.Context) error {
		return consumer.deliveries.Complete(txCtx, claim.Record.ID, claim.Consumer, claim.Owner)
	})
}

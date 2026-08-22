package campaign

import (
	"context"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type deliveryCompleter interface {
	Complete(context.Context, eventport.EventID, string, string) error
}
type FactDeliveryConsumer struct {
	uow        platformport.UnitOfWork
	deliveries deliveryCompleter
}

func NewFactDeliveryConsumer(uow platformport.UnitOfWork, deliveries deliveryCompleter) (*FactDeliveryConsumer, error) {
	if uow == nil || deliveries == nil {
		return nil, ErrUnavailable
	}
	return &FactDeliveryConsumer{uow, deliveries}, nil
}
func (*FactDeliveryConsumer) Consumer() string     { return eventport.ConsumerCloudCampaignFact }
func (*FactDeliveryConsumer) EventTypes() []string { return []string{eventport.EvCloudCampaignFact} }
func (c *FactDeliveryConsumer) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	if c == nil || claim.Record.ID < 1 || claim.Record.Type != eventport.EvCloudCampaignFact || claim.Consumer != eventport.ConsumerCloudCampaignFact || claim.Owner == "" || claim.Status != eventport.DeliveryProcessing {
		return ErrUnavailable
	}
	return c.uow.Within(ctx, func(tx context.Context) error {
		return c.deliveries.Complete(tx, claim.Record.ID, claim.Consumer, claim.Owner)
	})
}

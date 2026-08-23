package outbound

import (
	"context"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type campaignHandoffDeliveryCompleter interface {
	Complete(context.Context, eventport.EventID, string, string) error
}

// CampaignHandoffFactDeliveryConsumer has no domain repository or Provider
// dependency. It only completes the Events delivery receipt in a fresh UoW.
type CampaignHandoffFactDeliveryConsumer struct {
	uow        platformport.UnitOfWork
	deliveries campaignHandoffDeliveryCompleter
}

func NewCampaignHandoffFactDeliveryConsumer(uow platformport.UnitOfWork, deliveries campaignHandoffDeliveryCompleter) (*CampaignHandoffFactDeliveryConsumer, error) {
	if uow == nil || deliveries == nil {
		return nil, ErrCampaignHandoffUnavailable
	}
	return &CampaignHandoffFactDeliveryConsumer{uow: uow, deliveries: deliveries}, nil
}

func (*CampaignHandoffFactDeliveryConsumer) Consumer() string {
	return eventport.ConsumerOutboundCampaignHandoffFact
}

func (*CampaignHandoffFactDeliveryConsumer) EventTypes() []string {
	return []string{eventport.EvOutboundCampaignHandoffFact}
}

func (consumer *CampaignHandoffFactDeliveryConsumer) ConsumeDelivery(ctx context.Context, claim eventport.DeliveryClaim) error {
	if consumer == nil || consumer.uow == nil || consumer.deliveries == nil || claim.Record.ID < 1 ||
		claim.Record.Type != eventport.EvOutboundCampaignHandoffFact || claim.Consumer != eventport.ConsumerOutboundCampaignHandoffFact ||
		claim.Owner == "" || claim.Status != eventport.DeliveryProcessing {
		return ErrCampaignHandoffUnavailable
	}
	return consumer.uow.Within(ctx, func(tx context.Context) error {
		return consumer.deliveries.Complete(tx, claim.Record.ID, claim.Consumer, claim.Owner)
	})
}

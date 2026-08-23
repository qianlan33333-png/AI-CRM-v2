package campaign

import (
	"context"
	"errors"
	"testing"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type campaignFactDeliveryUoW struct{ calls int }

func (uow *campaignFactDeliveryUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

var _ platformport.UnitOfWork = (*campaignFactDeliveryUoW)(nil)

type campaignFactDeliveryCompleter struct{ calls int }

func (completer *campaignFactDeliveryCompleter) Complete(_ context.Context, eventID eventport.EventID, consumer, owner string) error {
	completer.calls++
	if eventID != 9 || consumer != eventport.ConsumerCloudCampaignFact || owner != "river:9" {
		return errors.New("unexpected local campaign delivery completion")
	}
	return nil
}

func TestFactDeliveryConsumerCompletesBoundCampaignFactWithoutExternalEffect(t *testing.T) {
	uow := &campaignFactDeliveryUoW{}
	completer := &campaignFactDeliveryCompleter{}
	consumer, err := NewFactDeliveryConsumer(uow, completer)
	if err != nil {
		t.Fatal(err)
	}
	if got := consumer.EventTypes(); len(got) != 1 || got[0] != eventport.EvCloudCampaignFact {
		t.Fatalf("event types=%v", got)
	}
	err = consumer.ConsumeDelivery(context.Background(), eventport.DeliveryClaim{
		Record:   eventport.Record{Event: eventport.Event{Type: eventport.EvCloudCampaignFact}, ID: 9},
		Consumer: eventport.ConsumerCloudCampaignFact, Owner: "river:9", Status: eventport.DeliveryProcessing,
	})
	if err != nil || uow.calls != 1 || completer.calls != 1 {
		t.Fatalf("err/uow/completion=%v/%d/%d", err, uow.calls, completer.calls)
	}
}

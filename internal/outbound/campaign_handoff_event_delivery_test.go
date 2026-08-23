package outbound

import (
	"context"
	"testing"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type handoffDeliveryUoW struct{ calls int }

func (uow *handoffDeliveryUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type handoffDeliveryCompleterStub struct{ calls int }

func (stub *handoffDeliveryCompleterStub) Complete(_ context.Context, id eventport.EventID, consumer, owner string) error {
	if id != 7 || consumer != eventport.ConsumerOutboundCampaignHandoffFact || owner != "river:7" {
		return ErrCampaignHandoffUnavailable
	}
	stub.calls++
	return nil
}

func TestCampaignHandoffFactDeliveryConsumerOnlyCompletesEventsReceipt(t *testing.T) {
	uow := &handoffDeliveryUoW{}
	completer := &handoffDeliveryCompleterStub{}
	consumer, err := NewCampaignHandoffFactDeliveryConsumer(uow, completer)
	if err != nil {
		t.Fatal(err)
	}
	err = consumer.ConsumeDelivery(context.Background(), eventport.DeliveryClaim{
		Record:   eventport.Record{ID: 7, Event: eventport.Event{Type: eventport.EvOutboundCampaignHandoffFact}},
		Consumer: eventport.ConsumerOutboundCampaignHandoffFact, Owner: "river:7", Status: eventport.DeliveryProcessing,
	})
	if err != nil || uow.calls != 1 || completer.calls != 1 {
		t.Fatalf("err=%v calls=%d/%d", err, uow.calls, completer.calls)
	}
}

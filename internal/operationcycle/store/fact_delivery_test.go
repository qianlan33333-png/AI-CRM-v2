package store

import (
	"context"
	"errors"
	"testing"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type factDeliveryUOW struct{ calls int }

func (uow *factDeliveryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

var _ platformport.UnitOfWork = (*factDeliveryUOW)(nil)

type factDeliveryCompleter struct{ calls int }

func (completer *factDeliveryCompleter) Complete(_ context.Context, eventID eventport.EventID, consumer, owner string) error {
	completer.calls++
	if eventID != 7 || consumer != eventport.ConsumerOperationCycleFact || owner != "river:7" {
		return errors.New("unexpected delivery completion")
	}
	return nil
}

func TestFactDeliveryConsumerCompletesOnlyTheLocalFactReceipt(t *testing.T) {
	uow := &factDeliveryUOW{}
	completer := &factDeliveryCompleter{}
	consumer, err := NewFactDeliveryConsumer(uow, completer)
	if err != nil {
		t.Fatal(err)
	}
	err = consumer.ConsumeDelivery(context.Background(), eventport.DeliveryClaim{
		Record:   eventport.Record{Event: eventport.Event{Type: eventport.EvOperationCycleFact}, ID: 7},
		Consumer: eventport.ConsumerOperationCycleFact, Owner: "river:7", Status: eventport.DeliveryProcessing,
	})
	if err != nil || uow.calls != 1 || completer.calls != 1 {
		t.Fatalf("err/uow/complete=%v/%d/%d", err, uow.calls, completer.calls)
	}
	if err = consumer.ConsumeDelivery(context.Background(), eventport.DeliveryClaim{Record: eventport.Record{Event: eventport.Event{Type: "wrong"}, ID: 7}}); !errors.Is(err, eventport.ErrDeliveryPoison) {
		t.Fatalf("wrong event error=%v, want poison", err)
	}
}

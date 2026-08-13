package callback

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type immediateCallbackUoW struct{}

func (immediateCallbackUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type deduplicatingCallbackAppender struct {
	events map[string]eventport.Event
}

func (appender *deduplicatingCallbackAppender) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if appender.events == nil {
		appender.events = make(map[string]eventport.Event)
	}
	if previous, found := appender.events[event.IdempotencyKey]; found {
		if !reflect.DeepEqual(previous, event) {
			return 0, eventport.ErrIdempotencyConflict
		}
		return 1, nil
	}
	appender.events[event.IdempotencyKey] = event
	return eventport.EventID(len(appender.events)), nil
}

func TestEventDispatcherPersistsOneStableFactForReplay(t *testing.T) {
	appender := &deduplicatingCallbackAppender{}
	dispatcher, err := NewEventDispatcher(immediateCallbackUoW{}, appender)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("<xml><CreateTime>1700000000</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[enter_agent]]></Event><FromUserName><![CDATA[user-1]]></FromUserName></xml>")
	for replay := 0; replay < 2; replay++ {
		if err := dispatcher.Dispatch(context.Background(), message); err != nil {
			t.Fatalf("Dispatch replay %d error = %v", replay, err)
		}
	}
	if len(appender.events) != 1 {
		t.Fatalf("durable event facts = %d, want 1", len(appender.events))
	}
	for _, event := range appender.events {
		if event.Type != acceptedCallbackEventType || event.OccurredAt != time.Unix(1700000000, 0).UTC() {
			t.Fatalf("event = %#v", event)
		}
		var payload callbackAuditPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Disposition != "accepted" || payload.Reason != "" || len(payload.EventSHA256) != 64 {
			t.Fatalf("audit payload = %s, %v", event.Payload, err)
		}
		if string(event.Payload) == string(message) {
			t.Fatal("audit payload retained raw callback message")
		}
	}
}

func TestEventDispatcherRejectsUnknownEventAfterAuditingIt(t *testing.T) {
	appender := &deduplicatingCallbackAppender{}
	dispatcher, err := NewEventDispatcher(immediateCallbackUoW{}, appender)
	if err != nil {
		t.Fatal(err)
	}
	err = dispatcher.Dispatch(context.Background(), []byte("<xml><CreateTime>1700000001</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[change_external_contact]]></Event></xml>"))
	if !errors.Is(err, ErrUnknownCallbackEvent) {
		t.Fatalf("Dispatch() error = %v, want ErrUnknownCallbackEvent", err)
	}
	if len(appender.events) != 1 {
		t.Fatalf("rejected audit facts = %d, want 1", len(appender.events))
	}
	for _, event := range appender.events {
		var payload callbackAuditPayload
		if event.Type != rejectedCallbackEventType || json.Unmarshal(event.Payload, &payload) != nil || payload.Disposition != "rejected" || payload.Reason != "unsupported_callback_event" {
			t.Fatalf("rejected event = %#v", event)
		}
	}
}

func TestEventDispatcherFailsClosedForInvalidDependenciesOrMessage(t *testing.T) {
	if dispatcher, err := NewEventDispatcher(nil, &deduplicatingCallbackAppender{}); dispatcher != nil || !errors.Is(err, ErrInvalidEventDispatcher) {
		t.Fatalf("NewEventDispatcher(nil) = %v, %v", dispatcher, err)
	}
	dispatcher, err := NewEventDispatcher(immediateCallbackUoW{}, &deduplicatingCallbackAppender{})
	if err != nil {
		t.Fatal(err)
	}
	if err = dispatcher.Dispatch(context.Background(), []byte("<xml><MsgType>event</MsgType><Event>enter_agent</Event></xml>")); !errors.Is(err, ErrUnknownCallbackEvent) {
		t.Fatalf("invalid Dispatch() error = %v", err)
	}
}

func TestAuditSubscriberAcceptsOnlyCallbackAuditTypes(t *testing.T) {
	subscriber := NewAuditSubscriber()
	if got := subscriber.EventTypes(); !reflect.DeepEqual(got, []string{acceptedCallbackEventType, rejectedCallbackEventType}) {
		t.Fatalf("EventTypes() = %v", got)
	}
	if err := subscriber.Consume(context.Background(), eventport.Record{}); err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
}

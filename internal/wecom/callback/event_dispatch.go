package callback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	acceptedCallbackEventType = "wecom.callback.accepted"
	rejectedCallbackEventType = "wecom.callback.rejected"
	acceptedCallbackEvent     = "enter_agent"
)

var (
	ErrInvalidEventDispatcher = errors.New("invalid WeCom callback event dispatcher")
	ErrUnknownCallbackEvent   = errors.New("unknown WeCom callback event")
)

// MessageDispatcher establishes the sole callback-to-events boundary. It may
// persist an audit fact but must not perform a provider call or a domain write.
type MessageDispatcher interface {
	Dispatch(context.Context, []byte) error
}

// EventDispatcher writes a de-identified callback receipt to the existing
// events outbox. The outbox unique idempotency key makes provider replay safe;
// downstream handling remains independently at-least-once.
type EventDispatcher struct {
	uow    platformport.UnitOfWork
	events eventport.Appender
}

func NewEventDispatcher(uow platformport.UnitOfWork, events eventport.Appender) (*EventDispatcher, error) {
	if nilLike(uow) || nilLike(events) {
		return nil, ErrInvalidEventDispatcher
	}
	return &EventDispatcher{uow: uow, events: events}, nil
}

func (dispatcher *EventDispatcher) Dispatch(ctx context.Context, message []byte) error {
	if dispatcher == nil || nilLike(dispatcher.uow) || nilLike(dispatcher.events) || ctx == nil {
		return ErrInvalidEventDispatcher
	}
	event, err := parseCallbackEvent(message)
	if err != nil {
		return err
	}

	digest := sha256.Sum256(message)
	digestText := hex.EncodeToString(digest[:])
	eventType := acceptedCallbackEventType
	payload := callbackAuditPayload{Disposition: "accepted", EventSHA256: digestText}
	result := error(nil)
	if event.MessageType != "event" || event.Event != acceptedCallbackEvent {
		eventType = rejectedCallbackEventType
		payload.Disposition = "rejected"
		payload.Reason = "unsupported_callback_event"
		result = ErrUnknownCallbackEvent
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode callback audit: %w", err)
	}
	if err = dispatcher.uow.Within(ctx, func(txCtx context.Context) error {
		_, appendErr := dispatcher.events.Append(txCtx, eventport.Event{
			Type:           eventType,
			Payload:        encoded,
			OccurredAt:     event.OccurredAt,
			IdempotencyKey: eventType + ":" + digestText,
		})
		return appendErr
	}); err != nil {
		return fmt.Errorf("append callback audit: %w", err)
	}
	return result
}

// AuditSubscriber closes this slice's delivery boundary without performing a
// contact, identity, sync, outbound, or provider action. event_log remains the
// durable audit record; future business consumers must be separate slices.
type AuditSubscriber struct{}

var _ eventport.Subscriber = (*AuditSubscriber)(nil)

func NewAuditSubscriber() *AuditSubscriber { return &AuditSubscriber{} }

func (*AuditSubscriber) EventTypes() []string {
	return []string{acceptedCallbackEventType, rejectedCallbackEventType}
}

func (*AuditSubscriber) Consume(_ context.Context, _ eventport.Record) error { return nil }

type parsedCallbackEvent struct {
	MessageType string
	Event       string
	OccurredAt  time.Time
}

type callbackAuditPayload struct {
	Disposition string `json:"disposition"`
	Reason      string `json:"reason,omitempty"`
	EventSHA256 string `json:"event_sha256"`
}

func parseCallbackEvent(message []byte) (parsedCallbackEvent, error) {
	var envelope struct {
		XMLName    xml.Name `xml:"xml"`
		CreateTime string   `xml:"CreateTime"`
		MsgType    string   `xml:"MsgType"`
		Event      string   `xml:"Event"`
	}
	decoder := xml.NewDecoder(strings.NewReader(string(message)))
	if err := decoder.Decode(&envelope); err != nil || envelope.XMLName.Local != "xml" {
		return parsedCallbackEvent{}, ErrUnknownCallbackEvent
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return parsedCallbackEvent{}, ErrUnknownCallbackEvent
	}
	createdAt, err := strconv.ParseInt(envelope.CreateTime, 10, 64)
	if err != nil || createdAt <= 0 || strings.TrimSpace(envelope.MsgType) == "" {
		return parsedCallbackEvent{}, ErrUnknownCallbackEvent
	}
	return parsedCallbackEvent{
		MessageType: envelope.MsgType,
		Event:       envelope.Event,
		OccurredAt:  time.Unix(createdAt, 0).UTC(),
	}, nil
}

func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reference := reflect.ValueOf(value)
	return (reference.Kind() == reflect.Chan || reference.Kind() == reflect.Func ||
		reference.Kind() == reflect.Interface || reference.Kind() == reflect.Map ||
		reference.Kind() == reflect.Pointer || reference.Kind() == reflect.Slice) && reference.IsNil()
}

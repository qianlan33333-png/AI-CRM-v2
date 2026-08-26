package legacyaudience

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

type inboundWebhookWorld struct {
	packages  map[int64]bool
	receipts  []InboundWebhookReceipt
	replays   map[[32]byte]InboundWebhookTransportReplay
	events    []LocalEvent
	failEvent bool
	nextID    int64
	inTx      bool
}

func newInboundWebhookWorld() *inboundWebhookWorld {
	return &inboundWebhookWorld{packages: map[int64]bool{42: true}, replays: make(map[[32]byte]InboundWebhookTransportReplay), nextID: 1}
}

func (world *inboundWebhookWorld) Within(ctx context.Context, operation func(context.Context) error) error {
	if world.inTx {
		return errors.New("nested transaction")
	}
	receipts := append([]InboundWebhookReceipt(nil), world.receipts...)
	replays := make(map[[32]byte]InboundWebhookTransportReplay, len(world.replays))
	for key, value := range world.replays {
		replays[key] = value
	}
	events := append([]LocalEvent(nil), world.events...)
	nextID := world.nextID
	world.inTx = true
	err := operation(ctx)
	world.inTx = false
	if err != nil {
		world.receipts, world.replays, world.events, world.nextID = receipts, replays, events, nextID
	}
	return err
}

func (world *inboundWebhookWorld) PackageExistsForInbound(_ context.Context, packageID int64) (bool, error) {
	if !world.inTx {
		return false, ErrUnavailable
	}
	return world.packages[packageID], nil
}

func (world *inboundWebhookWorld) ReserveInboundWebhook(_ context.Context, reservation InboundWebhookReservation) (InboundWebhookReceipt, bool, error) {
	if !world.inTx {
		return InboundWebhookReceipt{}, false, ErrUnavailable
	}
	for _, receipt := range world.receipts {
		if receipt.PackageID == reservation.PackageID && receipt.ExternalEventIDDigest == reservation.ExternalEventIDDigest {
			if receipt.PayloadDigest != reservation.PayloadDigest {
				return InboundWebhookReceipt{}, false, ErrIdempotencyConflict
			}
			if replay, found := world.replays[reservation.TransportEventIDDigest]; found && (replay.ReceiptID != receipt.ID || replay.PayloadDigest != reservation.PayloadDigest) {
				return InboundWebhookReceipt{}, false, ErrIdempotencyConflict
			}
			world.replays[reservation.TransportEventIDDigest] = InboundWebhookTransportReplay{ReceiptID: receipt.ID, PayloadDigest: reservation.PayloadDigest}
			return receipt, false, nil
		}
	}
	if _, found := world.replays[reservation.TransportEventIDDigest]; found {
		return InboundWebhookReceipt{}, false, ErrIdempotencyConflict
	}
	receipt := InboundWebhookReceipt{
		ID: world.nextID, PackageID: reservation.PackageID, State: InboundWebhookReceived,
		ExternalEventIDDigest: reservation.ExternalEventIDDigest, PayloadDigest: reservation.PayloadDigest,
		CreatedAt: reservation.CreatedAt,
	}
	world.nextID++
	world.receipts = append(world.receipts, receipt)
	world.replays[reservation.TransportEventIDDigest] = InboundWebhookTransportReplay{ReceiptID: receipt.ID, PayloadDigest: reservation.PayloadDigest}
	return receipt, true, nil
}

func (world *inboundWebhookWorld) Append(_ context.Context, event LocalEvent) error {
	if !world.inTx {
		return ErrUnavailable
	}
	if world.failEvent {
		return errors.New("event append failed")
	}
	world.events = append(world.events, event)
	return nil
}

func inboundWebhookInput() InboundWebhookInput {
	body := []byte(`{"external_event_id":"business-event-0001","status":"done","message":{"text":"ok"},"action":{}}`)
	return InboundWebhookInput{
		PackageID: 42, ClientID: AIAudienceWebhookClientID, TransportEventID: "transport-event-0001",
		ExternalEventID: "business-event-0001", Status: "done", Message: json.RawMessage(`{"text":"ok"}`),
		Action: json.RawMessage(`{}`), PayloadDigest: sha256.Sum256(body),
	}
}

func TestInboundWebhookServiceAcceptsAndReplaysWithoutDuplicateEvent(t *testing.T) {
	world := newInboundWebhookWorld()
	service, err := NewInboundWebhookService(world, world, world)
	if err != nil {
		t.Fatalf("NewInboundWebhookService: %v", err)
	}
	now := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	first, err := service.Accept(context.Background(), inboundWebhookInput())
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	replayInput := inboundWebhookInput()
	replayInput.TransportEventID = "transport-event-0002"
	replayed, err := service.Accept(context.Background(), replayInput)
	if err != nil {
		t.Fatalf("Accept replay: %v", err)
	}
	if first.Replayed || !replayed.Replayed || first.Receipt != replayed.Receipt || len(world.receipts) != 1 || len(world.replays) != 2 || len(world.events) != 1 {
		t.Fatalf("first=%+v replayed=%+v receipts=%d replays=%d events=%d", first, replayed, len(world.receipts), len(world.replays), len(world.events))
	}
	var eventPayload map[string]any
	if json.Unmarshal(world.events[0].Payload, &eventPayload) != nil || eventPayload["receipt_id"] != float64(first.Receipt.ID) || eventPayload["package_id"] != float64(42) {
		t.Fatalf("event payload=%s", world.events[0].Payload)
	}
}

func TestInboundWebhookServiceRejectsBothReplayMismatchClasses(t *testing.T) {
	world := newInboundWebhookWorld()
	service, _ := NewInboundWebhookService(world, world, world)
	service.now = func() time.Time { return time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC) }
	input := inboundWebhookInput()
	if _, err := service.Accept(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	changedPayload := input
	changedPayload.PayloadDigest = sha256.Sum256([]byte("changed"))
	if _, err := service.Accept(context.Background(), changedPayload); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("business replay mismatch err=%v", err)
	}
	changedIdentity := input
	changedIdentity.ExternalEventID = "business-event-0002"
	changedIdentity.PayloadDigest = sha256.Sum256([]byte("other"))
	if _, err := service.Accept(context.Background(), changedIdentity); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("transport replay mismatch err=%v", err)
	}
	if len(world.receipts) != 1 || len(world.events) != 1 {
		t.Fatalf("mismatch changed state receipts=%d events=%d", len(world.receipts), len(world.events))
	}
}

func TestInboundWebhookServiceRollsBackReceiptWhenEventAppendFails(t *testing.T) {
	world := newInboundWebhookWorld()
	world.failEvent = true
	service, _ := NewInboundWebhookService(world, world, world)
	service.now = time.Now
	if _, err := service.Accept(context.Background(), inboundWebhookInput()); err == nil {
		t.Fatal("expected failure")
	}
	if len(world.receipts) != 0 || len(world.replays) != 0 || len(world.events) != 0 {
		t.Fatalf("partial commit receipts=%d replays=%d events=%d", len(world.receipts), len(world.replays), len(world.events))
	}
}

func TestInboundWebhookServiceValidatesIdentityAndPayload(t *testing.T) {
	world := newInboundWebhookWorld()
	service, _ := NewInboundWebhookService(world, world, world)
	valid := inboundWebhookInput()
	tests := []InboundWebhookInput{
		{},
		func() InboundWebhookInput { value := valid; value.ClientID = "other"; return value }(),
		func() InboundWebhookInput { value := valid; value.TransportEventID = "short"; return value }(),
		func() InboundWebhookInput { value := valid; value.ExternalEventID = " "; return value }(),
		func() InboundWebhookInput { value := valid; id := int64(0); value.MemberEventID = &id; return value }(),
		func() InboundWebhookInput { value := valid; value.Message = json.RawMessage(`[]`); return value }(),
		func() InboundWebhookInput { value := valid; value.Action = json.RawMessage(`null`); return value }(),
	}
	for index, input := range tests {
		if _, err := service.Accept(context.Background(), input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("case %d err=%v", index, err)
		}
	}
	if !reflect.DeepEqual(world.receipts, []InboundWebhookReceipt(nil)) {
		t.Fatalf("invalid inputs wrote receipts: %+v", world.receipts)
	}
}

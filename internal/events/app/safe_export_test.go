package app

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

type internalEventSafeExportTestUOW struct{ rollbacks int }

func (uow *internalEventSafeExportTestUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	err := callback(ctx)
	if err != nil {
		uow.rollbacks++
	}
	return err
}

type internalEventSafeExportStoreStub struct {
	receipt       InternalEventSafeExportReceipt
	source        InternalEventSafeExportSourceSnapshot
	stored        InternalEventSafeExportStoredSnapshot
	fail          string
	createCalls   int
	completeCalls int
	readCalls     int
}

func (store *internalEventSafeExportStoreStub) ReserveInternalEventSafeExportReceipt(_ context.Context, actor int64, _ [32]byte, payload [32]byte, _ time.Time) (InternalEventSafeExportReceipt, bool, error) {
	if store.fail == "reserve" {
		return InternalEventSafeExportReceipt{}, false, errors.New("reserve failed")
	}
	if store.receipt.ID == 0 {
		store.receipt = InternalEventSafeExportReceipt{ID: 1, PayloadDigest: payload}
		return store.receipt, true, nil
	}
	if subtle.ConstantTimeCompare(store.receipt.PayloadDigest[:], payload[:]) != 1 {
		return InternalEventSafeExportReceipt{}, false, ErrInternalEventSafeExportConflict
	}
	return store.receipt, false, nil
}

func (store *internalEventSafeExportStoreStub) ReadInternalEventSafeExportSourceSnapshot(context.Context, InternalEventSafeExportFilter, int) (InternalEventSafeExportSourceSnapshot, error) {
	if store.fail == "source" {
		return InternalEventSafeExportSourceSnapshot{}, errors.New("source failed")
	}
	return store.source, nil
}

func (store *internalEventSafeExportStoreStub) CreateInternalEventSafeExport(_ context.Context, export InternalEventSafeExport, actor int64, filterDigest [32]byte, upperEventID int64, rowsDigest, resultDigest [32]byte, rows []InternalEventSafeExportRow) error {
	store.createCalls++
	if store.fail == "create" {
		return errors.New("create failed")
	}
	store.stored = InternalEventSafeExportStoredSnapshot{Export: export, ActorID: actor, FilterDigest: filterDigest, UpperEventID: upperEventID, DigestVersion: InternalEventSafeExportDigestVersion, RowsDigest: rowsDigest, ResultDigest: resultDigest, Rows: append([]InternalEventSafeExportRow(nil), rows...), ReceiptID: store.receipt.ID, ReceiptPayloadDigest: filterDigest, ReceiptResultDigest: resultDigest}
	return nil
}

func (store *internalEventSafeExportStoreStub) CompleteInternalEventSafeExportReceipt(_ context.Context, _ int64, _ string, resultDigest [32]byte, snapshot json.RawMessage, _ time.Time) (InternalEventSafeExportReceipt, error) {
	store.completeCalls++
	if store.fail == "complete" {
		return InternalEventSafeExportReceipt{}, errors.New("complete failed")
	}
	store.receipt.Completed = true
	store.receipt.ResultDigest = resultDigest
	store.receipt.ResultSnapshot = append(json.RawMessage(nil), snapshot...)
	store.stored.ReceiptResultDigest = resultDigest
	store.stored.ReceiptResultSnapshot = append(json.RawMessage(nil), snapshot...)
	return store.receipt, nil
}

func (store *internalEventSafeExportStoreStub) ReadInternalEventSafeExportSnapshot(context.Context, string, int64) (InternalEventSafeExportStoredSnapshot, error) {
	store.readCalls++
	if store.fail == "read" {
		return InternalEventSafeExportStoredSnapshot{}, errors.New("read failed")
	}
	return store.stored, nil
}

type internalEventSafeExportEventsStub struct {
	store  *internalEventSafeExportStoreStub
	events []eventport.Event
	err    error
}

func (events *internalEventSafeExportEventsStub) Append(_ context.Context, event eventport.Event) (eventport.EventID, error) {
	if events.err != nil {
		return 0, events.err
	}
	events.events = append(events.events, event)
	events.store.stored.AuditEventType = event.Type
	events.store.stored.AuditIdempotencyKey = event.IdempotencyKey
	events.store.stored.AuditOccurredAt = event.OccurredAt
	events.store.stored.AuditPayload = append(json.RawMessage(nil), event.Payload...)
	return 99, nil
}

func newInternalEventSafeExportTestService(store *internalEventSafeExportStoreStub, events *internalEventSafeExportEventsStub, uow *internalEventSafeExportTestUOW) *InternalEventSafeExportService {
	service := NewInternalEventSafeExportService(uow, store, events)
	service.now = func() time.Time { return time.Date(2026, time.August, 25, 7, 0, 0, 0, time.UTC) }
	return service
}

func validInternalEventSafeExportTestRow(stamp time.Time) InternalEventSafeExportRow {
	attempts := int32(0)
	return InternalEventSafeExportRow{EventID: 7, EventType: eventport.EvTagApplied, OccurredAt: stamp, Consumer: eventport.ConsumerAutomationTagTrigger, Status: string(eventport.DeliveryPending), AttemptCount: &attempts}
}

func TestInternalEventSafeExportCreateReplayAndTamperFailClosed(t *testing.T) {
	stamp := time.Date(2026, time.August, 25, 7, 1, 0, 0, time.UTC)
	store := &internalEventSafeExportStoreStub{source: InternalEventSafeExportSourceSnapshot{Watermark: stamp, UpperEventID: 7, Rows: []InternalEventSafeExportRow{validInternalEventSafeExportTestRow(stamp)}}}
	uow := &internalEventSafeExportTestUOW{}
	events := &internalEventSafeExportEventsStub{store: store}
	service := newInternalEventSafeExportTestService(store, events, uow)
	command := InternalEventSafeExportCreate{ActorID: 41, IdempotencyKey: "internal-event-safe-export-test-01"}
	first, err := service.Create(context.Background(), command)
	if err != nil || first.RecordCount != 1 || store.createCalls != 1 || store.completeCalls != 1 || len(events.events) != 1 {
		t.Fatalf("first=%+v create=%d complete=%d events=%d err=%v", first, store.createCalls, store.completeCalls, len(events.events), err)
	}
	replay, err := service.Create(context.Background(), command)
	if err != nil || replay != first || store.createCalls != 1 || store.completeCalls != 1 || len(events.events) != 1 || store.readCalls != 1 {
		t.Fatalf("replay=%+v first=%+v create=%d complete=%d events=%d reads=%d err=%v", replay, first, store.createCalls, store.completeCalls, len(events.events), store.readCalls, err)
	}
	originalAudit := append(json.RawMessage(nil), store.stored.AuditPayload...)
	store.stored.AuditPayload = json.RawMessage(`{"result_digest":"tampered"}`)
	if _, _, err := service.Download(context.Background(), first.ID, command.ActorID); !errors.Is(err, ErrInternalEventSafeExportUnavailable) {
		t.Fatalf("tampered audit err=%v", err)
	}
	store.stored.AuditPayload = originalAudit
	store.stored.Rows[0].EventType = eventport.EvOperationCycleFact
	if _, err := service.Get(context.Background(), first.ID, command.ActorID); !errors.Is(err, ErrInternalEventSafeExportUnavailable) {
		t.Fatalf("tampered row err=%v", err)
	}
}

func TestInternalEventSafeExportRejectsOverflowAndClosedProjectionDrift(t *testing.T) {
	stamp := time.Date(2026, time.August, 25, 7, 1, 0, 0, time.UTC)
	for name, rows := range map[string][]InternalEventSafeExportRow{
		"overflow":         make([]InternalEventSafeExportRow, InternalEventSafeExportMaximumRows+1),
		"unknown consumer": {{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Consumer: "unknown.consumer", Status: string(eventport.DeliveryPending), AttemptCount: func() *int32 { value := int32(0); return &value }()}},
		"wrong binding":    {{EventID: 1, EventType: eventport.EvTagApplied, OccurredAt: stamp, Consumer: eventport.ConsumerOperationCycleFact, Status: string(eventport.DeliveryPending), AttemptCount: func() *int32 { value := int32(0); return &value }()}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &internalEventSafeExportStoreStub{source: InternalEventSafeExportSourceSnapshot{Watermark: stamp, UpperEventID: 1, Rows: rows}}
			uow := &internalEventSafeExportTestUOW{}
			events := &internalEventSafeExportEventsStub{store: store}
			_, err := newInternalEventSafeExportTestService(store, events, uow).Create(context.Background(), InternalEventSafeExportCreate{ActorID: 41, IdempotencyKey: "internal-event-safe-export-test-02"})
			if err == nil || store.createCalls != 0 || store.completeCalls != 0 || len(events.events) != 0 || uow.rollbacks != 1 {
				t.Fatalf("create=%d complete=%d events=%d rollbacks=%d err=%v", store.createCalls, store.completeCalls, len(events.events), uow.rollbacks, err)
			}
		})
	}
}

func TestInternalEventSafeExportFailureStagesRollbackBeforeReceiptCompletion(t *testing.T) {
	stamp := time.Date(2026, time.August, 25, 7, 1, 0, 0, time.UTC)
	for _, stage := range []string{"reserve", "source", "create", "append", "complete"} {
		t.Run(stage, func(t *testing.T) {
			store := &internalEventSafeExportStoreStub{fail: stage, source: InternalEventSafeExportSourceSnapshot{Watermark: stamp, UpperEventID: 7, Rows: []InternalEventSafeExportRow{validInternalEventSafeExportTestRow(stamp)}}}
			uow := &internalEventSafeExportTestUOW{}
			events := &internalEventSafeExportEventsStub{store: store}
			if stage == "append" {
				store.fail = ""
				events.err = errors.New("append failed")
			}
			_, err := newInternalEventSafeExportTestService(store, events, uow).Create(context.Background(), InternalEventSafeExportCreate{ActorID: 41, IdempotencyKey: "internal-event-safe-export-test-03"})
			if !errors.Is(err, ErrInternalEventSafeExportUnavailable) || uow.rollbacks != 1 {
				t.Fatalf("stage=%s rollbacks=%d err=%v", stage, uow.rollbacks, err)
			}
			if stage != "complete" && store.completeCalls != 0 {
				t.Fatalf("stage=%s completed receipt", stage)
			}
		})
	}
}

func TestInternalEventSafeExportRowsDigestBindsOrderAndShape(t *testing.T) {
	stamp := time.Date(2026, time.August, 25, 7, 1, 0, 0, time.UTC)
	first := validInternalEventSafeExportTestRow(stamp)
	second := first
	second.EventID = 8
	one, err := internalEventSafeExportRowsDigest([]InternalEventSafeExportRow{first, second})
	if err != nil {
		t.Fatal(err)
	}
	two, err := internalEventSafeExportRowsDigest([]InternalEventSafeExportRow{second, first})
	if err != nil || subtle.ConstantTimeCompare(one[:], two[:]) == 1 {
		t.Fatalf("order digest one=%x two=%x err=%v", one, two, err)
	}
}

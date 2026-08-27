package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type historyWriterFixture struct {
	definition  productport.ServicePeriodHistoryDefinition
	entitlement productport.ServicePeriodHistoryEntitlement
	event       productport.ServicePeriodHistoryEvent
}

func newHistoryWriterFixture() historyWriterFixture {
	at := time.Date(2026, 8, 27, 10, 11, 12, 123456789, time.FixedZone("source", 8*3600))
	customer, order, entitlement := int64(7), int64(8), int64(102)
	return historyWriterFixture{
		definition: productport.ServicePeriodHistoryDefinition{SourceDefinitionID: 1, ProductID: 9,
			MembershipConfigID: " config ", MembershipConfigName: " 原名 ", DurationDays: -3, Deleted: true, CreatedAt: at, UpdatedAt: at},
		entitlement: productport.ServicePeriodHistoryEntitlement{SourceEntitlementID: 2, DefinitionID: 101,
			CustomerID: &customer, MembershipConfigID: "", Status: "expired", StartAt: at, EndAt: at.Add(-time.Hour),
			LastOrderID: &order, LastOutTradeNo: " trade ", RenewalCount: -2, CreatedAt: at, UpdatedAt: at},
		event: productport.ServicePeriodHistoryEvent{SourceEventID: 3, DefinitionID: 101, EntitlementID: &entitlement,
			CustomerID: &customer, OrderID: &order, EventID: " event ", EventType: "admin_adjusted", DurationDays: -7,
			OutTradeNo: " trade ", BeforeStartAt: &at, BeforeEndAt: &at, AfterStartAt: &at, AfterEndAt: &at, CreatedAt: at},
	}
}

func (f historyWriterFixture) run(writer *ServicePeriodHistoryWriter, ctx context.Context, kind, source string, payload [32]byte) (productport.ServicePeriodHistoryReceipt, error) {
	switch kind {
	case "definitions":
		return writer.ImportDefinition(ctx, source, payload, f.definition)
	case "entitlements":
		return writer.ImportEntitlement(ctx, source, payload, f.entitlement)
	default:
		return writer.ImportEvent(ctx, source, payload, f.event)
	}
}

type historyWriterStore struct {
	ctx                                  context.Context
	definition                           productport.ServicePeriodHistoryDefinition
	entitlement                          productport.ServicePeriodHistoryEntitlement
	event                                productport.ServicePeriodHistoryEvent
	creates, definitionReads, eventReads int
	createErr, getErr                    error
	badCreate                            bool
}

func (s *historyWriterStore) check(ctx context.Context) {
	if ctx != s.ctx {
		panic("writer changed caller transaction context")
	}
}

func (s *historyWriterStore) CreateServicePeriodHistoryDefinition(ctx context.Context, value productport.ServicePeriodHistoryDefinition) (productport.ServicePeriodHistoryDefinition, error) {
	s.check(ctx)
	s.creates++
	value.ID = 101
	if s.badCreate {
		value.ProductID++
	}
	s.definition = value
	return value, s.createErr
}
func (s *historyWriterStore) GetServicePeriodHistoryDefinition(ctx context.Context, _ int64) (productport.ServicePeriodHistoryDefinition, error) {
	s.check(ctx)
	s.definitionReads++
	return s.definition, s.getErr
}
func (s *historyWriterStore) CreateServicePeriodHistoryEntitlement(ctx context.Context, value productport.ServicePeriodHistoryEntitlement) (productport.ServicePeriodHistoryEntitlement, error) {
	s.check(ctx)
	s.creates++
	value.ID = 102
	if s.badCreate {
		value.RenewalCount++
	}
	s.entitlement = value
	return value, s.createErr
}
func (s *historyWriterStore) GetServicePeriodHistoryEntitlement(ctx context.Context, _ int64) (productport.ServicePeriodHistoryEntitlement, error) {
	s.check(ctx)
	return s.entitlement, s.getErr
}
func (s *historyWriterStore) CreateServicePeriodHistoryEvent(ctx context.Context, value productport.ServicePeriodHistoryEvent) (productport.ServicePeriodHistoryEvent, error) {
	s.check(ctx)
	s.creates++
	value.ID = 103
	if s.badCreate {
		value.EventType = "activated"
	}
	s.event = value
	return value, s.createErr
}
func (s *historyWriterStore) GetServicePeriodHistoryEvent(ctx context.Context, _ int64) (productport.ServicePeriodHistoryEvent, error) {
	s.check(ctx)
	s.eventReads++
	return s.event, s.getErr
}

type historyWriterJournal struct {
	ctx                context.Context
	receipts           map[string]productport.ServicePeriodHistoryReceipt
	loads, records     int
	loadErr, recordErr error
}

func (j *historyWriterJournal) LoadServicePeriodHistory(ctx context.Context, kind, source string) (productport.ServicePeriodHistoryReceipt, bool, error) {
	if ctx != j.ctx {
		panic("journal lost caller transaction context")
	}
	j.loads++
	receipt, found := j.receipts[kind+"/"+source]
	return receipt, found, j.loadErr
}
func (j *historyWriterJournal) RecordServicePeriodHistory(ctx context.Context, kind string, receipt productport.ServicePeriodHistoryReceipt) error {
	if ctx != j.ctx {
		panic("journal lost caller transaction context")
	}
	j.records++
	if j.recordErr != nil {
		return j.recordErr
	}
	j.receipts[kind+"/"+receipt.SourceIdentifier] = receipt
	return nil
}

func newHistoryWriterTest(t *testing.T) (*ServicePeriodHistoryWriter, *historyWriterStore, *historyWriterJournal, context.Context) {
	t.Helper()
	type transactionKey struct{}
	ctx := context.WithValue(context.Background(), transactionKey{}, "caller transaction")
	store := &historyWriterStore{ctx: ctx, entitlement: newHistoryWriterFixture().entitlement}
	store.entitlement.ID = 102
	journal := &historyWriterJournal{ctx: ctx, receipts: make(map[string]productport.ServicePeriodHistoryReceipt)}
	writer, err := NewServicePeriodHistoryWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	return writer, store, journal, ctx
}

func TestServicePeriodHistoryCreateAndActualReplay(t *testing.T) {
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		t.Run(kind, func(t *testing.T) {
			writer, store, journal, ctx := newHistoryWriterTest(t)
			fixture := newHistoryWriterFixture()
			first, err := fixture.run(writer, ctx, kind, "source", [32]byte{1})
			if err != nil || first.TargetID < 1 || first.TargetDigest == [32]byte{} || first.Replayed {
				t.Fatalf("create: %+v %v", first, err)
			}
			replay, err := fixture.run(writer, ctx, kind, "source", [32]byte{1})
			if err != nil || !replay.Replayed || replay.TargetID != first.TargetID || replay.TargetDigest != first.TargetDigest || store.creates != 1 || journal.records != 1 {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			// A receipt is not sufficient: the actual target must still match.
			switch kind {
			case "definitions":
				store.definition.MembershipConfigName += " changed"
			case "entitlements":
				store.entitlement.LastOutTradeNo += " changed"
			case "events":
				store.event.DurationDays++
			}
			if _, err = fixture.run(writer, ctx, kind, "source", [32]byte{1}); !errors.Is(err, productport.ErrServicePeriodHistoryConflict) {
				t.Fatalf("drift accepted: %v", err)
			}
		})
	}
}

func TestServicePeriodHistoryReplayRejectsReceiptAndPayloadDrift(t *testing.T) {
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		for _, field := range []string{"source", "payload", "digest", "id", "actual-id", "input", "request-payload"} {
			t.Run(kind+"/"+field, func(t *testing.T) {
				writer, store, journal, ctx := newHistoryWriterTest(t)
				fixture, payload := newHistoryWriterFixture(), [32]byte{1}
				if _, err := fixture.run(writer, ctx, kind, "source", payload); err != nil {
					t.Fatal(err)
				}
				receipt := journal.receipts[kind+"/source"]
				switch field {
				case "source":
					receipt.SourceIdentifier = "other"
				case "payload":
					receipt.PayloadDigest[0]++
				case "digest":
					receipt.TargetDigest[0]++
				case "id":
					receipt.TargetID = 0
				case "actual-id":
					store.definition.ID++
					store.entitlement.ID++
					store.event.ID++
				case "input":
					fixture.definition.ProductID++
					fixture.entitlement.RenewalCount++
					fixture.event.EventType = "activated"
				case "request-payload":
					payload[0]++
				}
				journal.receipts[kind+"/source"] = receipt
				got, err := fixture.run(writer, ctx, kind, "source", payload)
				if !errors.Is(err, productport.ErrServicePeriodHistoryConflict) || got != (productport.ServicePeriodHistoryReceipt{}) || journal.records != 1 || store.creates != 1 {
					t.Fatalf("accepted drift: %+v %v", got, err)
				}
			})
		}
	}
}

func TestServicePeriodHistoryPreservesSourceAndNormalizesMicroseconds(t *testing.T) {
	writer, store, _, ctx := newHistoryWriterTest(t)
	f := newHistoryWriterFixture()
	original := *f.event.BeforeStartAt
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		if _, err := f.run(writer, ctx, kind, "source", [32]byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if store.definition.MembershipConfigID != " config " || store.definition.MembershipConfigName != " 原名 " || store.definition.DurationDays != -3 ||
		store.entitlement.RenewalCount != -2 || store.entitlement.Status != "expired" || !store.entitlement.EndAt.Before(store.entitlement.StartAt) || store.event.DurationDays != -7 ||
		store.definition.CreatedAt.Location() != time.UTC || store.definition.CreatedAt.Nanosecond() != 123456000 || store.event.BeforeStartAt.Location() != time.UTC ||
		store.event.BeforeStartAt.Nanosecond() != 123456000 || *f.event.BeforeStartAt != original {
		t.Fatal("source text/state/negative adjustments changed or timestamp input mutated")
	}
	// Failed grants really can lack identity, entitlement, order and all dates.
	f.event.SourceEventID = 4
	f.event.EventType = "grant_failed_missing_unionid"
	f.event.EntitlementID, f.event.CustomerID, f.event.OrderID = nil, nil, nil
	f.event.BeforeStartAt, f.event.BeforeEndAt, f.event.AfterStartAt, f.event.AfterEndAt = nil, nil, nil, nil
	if _, err := f.run(writer, ctx, "events", "failure-source", [32]byte{2}); err != nil {
		t.Fatal(err)
	}
	if store.event.EntitlementID != nil || store.event.BeforeStartAt != nil || store.event.CustomerID != nil || store.event.OrderID != nil {
		t.Fatal("missing facts fabricated")
	}
	f.entitlement.CustomerID, f.entitlement.LastOrderID = nil, nil
	if _, err := f.run(writer, ctx, "entitlements", "unmapped-source", [32]byte{3}); err != nil {
		t.Fatal(err)
	}
}

func TestServicePeriodHistoryInvalidInputsDoNotAccessAdapters(t *testing.T) {
	zero := int64(0)
	invalid := []struct {
		name, kind string
		edit       func(*historyWriterFixture)
	}{
		{"definition target id", "definitions", func(f *historyWriterFixture) { f.definition.ID = 1 }},
		{"source definition id", "definitions", func(f *historyWriterFixture) { f.definition.SourceDefinitionID = 0 }},
		{"product id", "definitions", func(f *historyWriterFixture) { f.definition.ProductID = -1 }},
		{"created time", "definitions", func(f *historyWriterFixture) { f.definition.CreatedAt = time.Time{} }},
		{"updated time", "definitions", func(f *historyWriterFixture) { f.definition.UpdatedAt = f.definition.CreatedAt.Add(-time.Hour) }},
		{"nul text", "definitions", func(f *historyWriterFixture) { f.definition.MembershipConfigName = "a\x00b" }},
		{"invalid utf8", "definitions", func(f *historyWriterFixture) { f.definition.MembershipConfigID = string([]byte{255}) }},
		{"entitlement target id", "entitlements", func(f *historyWriterFixture) { f.entitlement.ID = 1 }},
		{"source entitlement id", "entitlements", func(f *historyWriterFixture) { f.entitlement.SourceEntitlementID = 0 }},
		{"definition id", "entitlements", func(f *historyWriterFixture) { f.entitlement.DefinitionID = 0 }},
		{"customer id", "entitlements", func(f *historyWriterFixture) { f.entitlement.CustomerID = &zero }},
		{"last order id", "entitlements", func(f *historyWriterFixture) { f.entitlement.LastOrderID = &zero }},
		{"empty status", "entitlements", func(f *historyWriterFixture) { f.entitlement.Status = "" }},
		{"end time", "entitlements", func(f *historyWriterFixture) { f.entitlement.EndAt = time.Time{} }},
		{"event target id", "events", func(f *historyWriterFixture) { f.event.ID = 1 }},
		{"source event id", "events", func(f *historyWriterFixture) { f.event.SourceEventID = 0 }},
		{"event definition id", "events", func(f *historyWriterFixture) { f.event.DefinitionID = 0 }},
		{"entitlement id", "events", func(f *historyWriterFixture) { f.event.EntitlementID = &zero }},
		{"order id", "events", func(f *historyWriterFixture) { f.event.OrderID = &zero }},
		{"event id", "events", func(f *historyWriterFixture) { f.event.EventID = "" }},
		{"event type", "events", func(f *historyWriterFixture) { f.event.EventType = "" }},
		{"optional time", "events", func(f *historyWriterFixture) { f.event.AfterEndAt = new(time.Time) }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			writer, store, journal, ctx := newHistoryWriterTest(t)
			fixture := newHistoryWriterFixture()
			tc.edit(&fixture)
			_, err := fixture.run(writer, ctx, tc.kind, "source", [32]byte{1})
			if !errors.Is(err, productport.ErrServicePeriodHistoryInvalid) || store.creates != 0 || journal.loads != 0 {
				t.Fatalf("invalid input reached adapters: %v", err)
			}
		})
	}
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		writer, store, journal, ctx := newHistoryWriterTest(t)
		for _, source := range []string{"", " source", "source\x00"} {
			if _, err := newHistoryWriterFixture().run(writer, ctx, kind, source, [32]byte{1}); !errors.Is(err, productport.ErrServicePeriodHistoryInvalid) {
				t.Fatal(err)
			}
		}
		if _, err := newHistoryWriterFixture().run(writer, ctx, kind, "source", [32]byte{}); !errors.Is(err, productport.ErrServicePeriodHistoryInvalid) {
			t.Fatal(err)
		}
		if store.creates != 0 || journal.loads != 0 {
			t.Fatal("invalid source reached adapters")
		}
	}
}

func TestServicePeriodHistoryErrorsReturnNoSuccessReceipt(t *testing.T) {
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		for _, point := range []string{"load", "create", "record", "replay-read", "bad-create"} {
			t.Run(kind+"/"+point, func(t *testing.T) {
				writer, store, journal, ctx := newHistoryWriterTest(t)
				f := newHistoryWriterFixture()
				privateErr := errors.New("private source payload must not escape")
				want := productport.ErrServicePeriodHistoryUnavailable
				switch point {
				case "load":
					journal.loadErr = privateErr
				case "create":
					store.createErr = privateErr
				case "record":
					journal.recordErr = privateErr
				case "bad-create":
					store.badCreate = true
					want = productport.ErrServicePeriodHistoryConflict
				case "replay-read":
					if _, err := f.run(writer, ctx, kind, "source", [32]byte{1}); err != nil {
						t.Fatal(err)
					}
					store.getErr = privateErr
				}
				got, err := f.run(writer, ctx, kind, "source", [32]byte{1})
				if err != want || got != (productport.ServicePeriodHistoryReceipt{}) {
					t.Fatalf("error leaked or returned success: %+v %v", got, err)
				}
				if point != "record" && point != "replay-read" && journal.records != 0 {
					t.Fatal("recorded failed target")
				}
			})
		}
	}
	writer, store, journal, ctx := newHistoryWriterTest(t)
	store.entitlement.DefinitionID++
	if _, err := newHistoryWriterFixture().run(writer, ctx, "events", "source", [32]byte{1}); err != productport.ErrServicePeriodHistoryConflict || store.creates != 0 || journal.records != 0 {
		t.Fatalf("wrong event parent accepted: %v", err)
	}
	for _, cause := range []error{productport.ErrServicePeriodHistoryConflict, productport.ErrServicePeriodHistoryInvalid} {
		if historyWriteError(fmt.Errorf("private details: %w", cause)) != cause {
			t.Fatal("known classification lost")
		}
	}
}

func TestServicePeriodHistoryMissingDependencies(t *testing.T) {
	var store *historyWriterStore
	var journal *historyWriterJournal
	if _, err := NewServicePeriodHistoryWriter(store, journal); err != productport.ErrServicePeriodHistoryUnavailable {
		t.Fatal(err)
	}
	writer, _, _, ctx := newHistoryWriterTest(t)
	for _, kind := range []string{"definitions", "entitlements", "events"} {
		if _, err := newHistoryWriterFixture().run(nil, ctx, kind, "source", [32]byte{1}); err != productport.ErrServicePeriodHistoryUnavailable {
			t.Fatal(err)
		}
		if _, err := newHistoryWriterFixture().run(writer, nil, kind, "source", [32]byte{1}); err != productport.ErrServicePeriodHistoryUnavailable {
			t.Fatal(err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := newHistoryWriterFixture().run(writer, cancelled, kind, "source", [32]byte{1}); err != productport.ErrServicePeriodHistoryUnavailable {
			t.Fatal(err)
		}
	}
}

func TestServicePeriodHistoryDigestsCoverEveryField(t *testing.T) {
	f := newHistoryWriterFixture()
	cases := []struct {
		record any
		digest func(any) [32]byte
	}{
		{f.definition, func(v any) [32]byte {
			return ServicePeriodHistoryDefinitionTargetDigest(v.(productport.ServicePeriodHistoryDefinition))
		}},
		{f.entitlement, func(v any) [32]byte {
			return ServicePeriodHistoryEntitlementTargetDigest(v.(productport.ServicePeriodHistoryEntitlement))
		}},
		{f.event, func(v any) [32]byte {
			return ServicePeriodHistoryEventTargetDigest(v.(productport.ServicePeriodHistoryEvent))
		}},
	}
	for _, tc := range cases {
		original := reflect.ValueOf(tc.record)
		baseline := tc.digest(tc.record)
		if baseline == [32]byte{} {
			t.Fatal("zero digest")
		}
		for index := 0; index < original.NumField(); index++ {
			changed := reflect.New(original.Type()).Elem()
			changed.Set(original)
			field := changed.Field(index)
			switch value := field.Interface().(type) {
			case int64:
				field.SetInt(value + 1)
			case int32:
				field.SetInt(int64(value) + 1)
			case string:
				field.SetString(value + " changed")
			case bool:
				field.SetBool(!value)
			case time.Time:
				field.Set(reflect.ValueOf(value.Add(time.Microsecond)))
			case *int64:
				next := *value + 1
				field.Set(reflect.ValueOf(&next))
			case *time.Time:
				next := value.Add(time.Microsecond)
				field.Set(reflect.ValueOf(&next))
			default:
				t.Fatalf("untested field type %T", value)
			}
			if tc.digest(changed.Interface()) == baseline {
				t.Fatalf("digest omitted %s.%s", original.Type().Name(), original.Type().Field(index).Name)
			}
			if field.Kind() == reflect.Pointer {
				field.SetZero()
				if tc.digest(changed.Interface()) == baseline {
					t.Fatal("digest omitted NULL")
				}
			}
		}
	}
	d := f.definition
	d.CreatedAt = historyMicro(d.CreatedAt)
	d.UpdatedAt = historyMicro(d.UpdatedAt)
	e := f.event
	normalized := historyMicro(*e.BeforeStartAt)
	e.BeforeStartAt = &normalized
	if ServicePeriodHistoryDefinitionTargetDigest(f.definition) != ServicePeriodHistoryDefinitionTargetDigest(d) || ServicePeriodHistoryEventTargetDigest(f.event) != ServicePeriodHistoryEventTargetDigest(e) {
		t.Fatal("equivalent PostgreSQL timestamps have different digest")
	}
}

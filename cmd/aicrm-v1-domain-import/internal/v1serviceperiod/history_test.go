package v1serviceperiod

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHistoryPreservesAllStaticFieldsWithoutRewritingStates(t *testing.T) {
	product, entitlement, event := historyFixtures()
	product.MembershipConfigID, product.MembershipConfigName = " config ", " 配置 "
	product.Deleted = true
	entitlement.UnionID, event.UnionID = " union ", " union "
	entitlement.ExternalUserIDSnapshot = " external snapshot "
	entitlement.Status, entitlement.LastOutTradeNo = "expired", " original-order "
	entitlement.MembershipConfigID = " historical config "
	event.EventType, event.EventID, event.OutTradeNo = "admin_adjusted", " original-event ", " original-order "
	event.DurationDays = -30
	history := AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if len(history.Products) != 1 || len(history.Entitlements) != 1 || len(history.Events) != 1 {
		t.Fatal("input rows were not conserved")
	}
	if history.Products[0].Disposition != DispositionCandidate || !reflect.DeepEqual(history.Products[0].Fact, &product) ||
		history.Entitlements[0].Disposition != DispositionCandidate || !reflect.DeepEqual(history.Entitlements[0].Fact, &entitlement) ||
		history.Events[0].Disposition != DispositionCandidate || !reflect.DeepEqual(history.Events[0].Fact, &event) {
		t.Fatal("static fields, whitespace, times or original state were changed")
	}
}

func TestHistoryPreservesObservedEventKindsIncludingFailedGrant(t *testing.T) {
	for _, kind := range []string{"activated", "admin_adjusted", "refunded", "grant_failed_missing_unionid", "renewed"} {
		t.Run(kind, func(t *testing.T) {
			product, entitlement, event := historyFixtures()
			event.EventType = kind
			if kind == "grant_failed_missing_unionid" {
				event.UnionID, event.OutTradeNo = "", ""
				event.EntitlementSourceID, event.OrderSourceID = nil, nil
				event.BeforeStartAt, event.BeforeEndAt, event.AfterStartAt, event.AfterEndAt = nil, nil, nil, nil
			}
			history := AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
			if history.Events[0].Disposition != DispositionCandidate || !reflect.DeepEqual(history.Events[0].Fact, &event) {
				t.Fatal("historical event was rewritten or an absent optional reference was fabricated")
			}
		})
	}
	product, entitlement, _ := historyFixtures()
	entitlement.LastOrderSourceID = nil
	entitlement.LastOutTradeNo = ""
	entitlement.Status = "active"
	history := AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, nil)
	if history.Entitlements[0].Disposition != DispositionCandidate || !reflect.DeepEqual(history.Entitlements[0].Fact, &entitlement) {
		t.Fatal("nullable last order was not preserved")
	}
}

func TestHistoryQuarantinesMissingOrConflictingSourceParents(t *testing.T) {
	product, entitlement, event := historyFixtures()
	withoutProduct := AdaptHistory(nil, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if withoutProduct.Entitlements[0].Reason != "entitlement_service_product_unresolved" || withoutProduct.Events[0].Reason != "service_event_product_unresolved" {
		t.Fatal("missing service-product parent was not isolated")
	}
	withoutEntitlement := AdaptHistory([]json.RawMessage{raw(t, product)}, nil, []json.RawMessage{raw(t, event)})
	if withoutEntitlement.Events[0].Reason != "service_event_entitlement_unresolved" {
		t.Fatal("explicit absent entitlement was not isolated")
	}
	wrongTrade := entitlement
	wrongTrade.TradeProductSourceID++
	history := AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, wrongTrade)}, nil)
	if history.Entitlements[0].Reason != "entitlement_product_reference_conflict" {
		t.Fatal("conflicting trade product was not isolated")
	}
	event.UnionID = "different-source-customer"
	history = AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if history.Events[0].Reason != "service_event_entitlement_reference_conflict" {
		t.Fatal("event linked to another source customer")
	}
}

func TestHistoryRejectsAmbiguousSourceParentsWithoutDroppingRows(t *testing.T) {
	product, entitlement, event := historyFixtures()
	history := AdaptHistory([]json.RawMessage{raw(t, product), raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if len(history.Products) != 2 || history.Products[0].Reason != "service_product_source_ambiguous" || history.Products[1].Reason != "service_product_source_ambiguous" ||
		history.Entitlements[0].Disposition != DispositionPending || history.Events[0].Disposition != DispositionPending {
		t.Fatal("ambiguous product was selected or source rows were dropped")
	}
	history = AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement), raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if len(history.Entitlements) != 2 || history.Entitlements[0].Reason != "entitlement_source_ambiguous" || history.Entitlements[1].Reason != "entitlement_source_ambiguous" ||
		history.Events[0].Reason != "service_event_entitlement_unresolved" {
		t.Fatal("ambiguous entitlement was selected or source rows were dropped")
	}
}

func TestHistoryRejectsMalformedJSONAndInvalidOptionalFacts(t *testing.T) {
	for _, payload := range []json.RawMessage{[]byte(`{`), []byte(`[]`), []byte(`{} {}`), []byte(`{"id":"1"}`)} {
		history := AdaptHistory([]json.RawMessage{payload}, []json.RawMessage{payload}, []json.RawMessage{payload})
		if history.Products[0].Disposition != DispositionInvalid || history.Entitlements[0].Disposition != DispositionInvalid || history.Events[0].Disposition != DispositionInvalid {
			t.Fatal("malformed source was not isolated")
		}
	}
	product, entitlement, event := historyFixtures()
	for _, mutate := range []func(*EventFact){
		func(value *EventFact) { value.SourceID = 0 },
		func(value *EventFact) { value.EventType = "" },
		func(value *EventFact) { value.CreatedAt = time.Time{} },
		func(value *EventFact) { id := int64(0); value.EntitlementSourceID = &id },
		func(value *EventFact) { id := int64(-1); value.OrderSourceID = &id },
		func(value *EventFact) { stamp := time.Time{}; value.AfterEndAt = &stamp },
	} {
		invalid := event
		mutate(&invalid)
		history := AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, invalid)})
		if history.Events[0].Reason != "service_event_shape_invalid" {
			t.Fatal("invalid non-null source fact was not rejected")
		}
	}
}

func TestHistoryRejectsNullForRequiredColumnsWithoutRejectingEmptyText(t *testing.T) {
	product, entitlement, event := historyFixtures()
	var products, entitlements, events map[string]json.RawMessage
	_ = json.Unmarshal(raw(t, product), &products)
	_ = json.Unmarshal(raw(t, entitlement), &entitlements)
	_ = json.Unmarshal(raw(t, event), &events)
	products["duration_days"] = json.RawMessage(`null`)
	entitlements["renewal_count"] = json.RawMessage(`null`)
	events["unionid"] = json.RawMessage(`null`)
	history := AdaptHistory([]json.RawMessage{raw(t, products)}, []json.RawMessage{raw(t, entitlements)}, []json.RawMessage{raw(t, events)})
	if history.Products[0].Reason != "service_product_json_invalid" || history.Entitlements[0].Reason != "entitlement_json_invalid" || history.Events[0].Reason != "service_event_json_invalid" {
		t.Fatal("NOT NULL source columns were silently converted to zero values")
	}
	product.MembershipConfigName = ""
	entitlement.UnionID, entitlement.ExternalUserIDSnapshot = "", ""
	event.UnionID = ""
	history = AdaptHistory([]json.RawMessage{raw(t, product)}, []json.RawMessage{raw(t, entitlement)}, []json.RawMessage{raw(t, event)})
	if history.Products[0].Fact == nil || history.Entitlements[0].Fact == nil || history.Events[0].Fact == nil {
		t.Fatal("empty historical text was confused with SQL NULL")
	}
}

func TestHistoryDoesNotCarryOpaqueOrExecutableArchiveFields(t *testing.T) {
	product, entitlement, event := historyFixtures()
	withArchiveOnlyFields := func(value any) json.RawMessage {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw(t, value), &fields); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"tenant_id", "metadata_json", "payload_json", "link_slug", "access_token"} {
			fields[field] = json.RawMessage(`"archive-only-marker"`)
		}
		return raw(t, fields)
	}
	history := AdaptHistory([]json.RawMessage{withArchiveOnlyFields(product)}, []json.RawMessage{withArchiveOnlyFields(entitlement)}, []json.RawMessage{withArchiveOnlyFields(event)})
	encoded := raw(t, history)
	if strings.Contains(string(encoded), "archive-only-marker") || history.Products[0].Fact == nil || history.Entitlements[0].Fact == nil || history.Events[0].Fact == nil {
		t.Fatal("opaque/executable fields escaped the archive boundary")
	}
}

func historyFixtures() (ProductFact, EntitlementFact, EventFact) {
	stamp := time.Date(2026, 7, 1, 0, 0, 0, 123000, time.UTC)
	end := stamp.Add(30 * 24 * time.Hour)
	orderID, entitlementID := int64(9007199254740993), int64(72)
	product := ProductFact{SourceID: 11, TradeProductSourceID: 29, MembershipConfigID: "config", MembershipConfigName: "周期会员", DurationDays: 30, CreatedAt: stamp, UpdatedAt: stamp}
	entitlement := EntitlementFact{SourceID: entitlementID, ServiceProductSourceID: product.SourceID, TradeProductSourceID: product.TradeProductSourceID,
		UnionID: "union", ExternalUserIDSnapshot: "external", MembershipConfigID: "config", Status: "active", StartAt: stamp, EndAt: end,
		LastOrderSourceID: &orderID, LastOutTradeNo: "order", RenewalCount: 3, CreatedAt: stamp, UpdatedAt: end}
	event := EventFact{SourceID: 18, EventID: "event", ServiceProductSourceID: product.SourceID, EntitlementSourceID: &entitlementID,
		TradeProductSourceID: product.TradeProductSourceID, OrderSourceID: &orderID, OutTradeNo: "order", UnionID: "union", EventType: "renewed", DurationDays: 30,
		BeforeStartAt: &stamp, BeforeEndAt: &stamp, AfterStartAt: &stamp, AfterEndAt: &end, CreatedAt: end}
	return product, entitlement, event
}

func raw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

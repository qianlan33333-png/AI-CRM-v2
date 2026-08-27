// Package v1serviceperiod adapts archived V1 service-period rows into static
// historical facts. It has no persistence, entitlement, event or Provider API.
package v1serviceperiod

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

type Disposition string

const (
	DispositionCandidate Disposition = "historical_candidate"
	DispositionPending   Disposition = "pending"
	DispositionInvalid   Disposition = "invalid"
)

// Source IDs and UnionID are unverified source references, never V2 IDs.
// Tenant, opaque metadata/payload and old public links remain archive-only.
type ProductFact struct {
	SourceID             int64     `json:"id"`
	TradeProductSourceID int64     `json:"trade_product_id"`
	MembershipConfigID   string    `json:"membership_config_id"`
	MembershipConfigName string    `json:"membership_config_name"`
	DurationDays         int32     `json:"duration_days"`
	Deleted              bool      `json:"deleted"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type EntitlementFact struct {
	SourceID               int64     `json:"id"`
	ServiceProductSourceID int64     `json:"service_product_id"`
	TradeProductSourceID   int64     `json:"trade_product_id"`
	UnionID                string    `json:"unionid"`
	ExternalUserIDSnapshot string    `json:"external_userid_snapshot"`
	MembershipConfigID     string    `json:"membership_config_id"`
	Status                 string    `json:"status"`
	StartAt                time.Time `json:"start_at"`
	EndAt                  time.Time `json:"end_at"`
	LastOrderSourceID      *int64    `json:"last_order_id"`
	LastOutTradeNo         string    `json:"last_out_trade_no"`
	RenewalCount           int32     `json:"renewal_count"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type EventFact struct {
	SourceID               int64      `json:"id"`
	EventID                string     `json:"event_id"`
	ServiceProductSourceID int64      `json:"service_product_id"`
	EntitlementSourceID    *int64     `json:"entitlement_id"`
	TradeProductSourceID   int64      `json:"trade_product_id"`
	OrderSourceID          *int64     `json:"order_id"`
	OutTradeNo             string     `json:"out_trade_no"`
	UnionID                string     `json:"unionid"`
	EventType              string     `json:"event_type"`
	DurationDays           int32      `json:"duration_days"`
	BeforeStartAt          *time.Time `json:"before_start_at"`
	BeforeEndAt            *time.Time `json:"before_end_at"`
	AfterStartAt           *time.Time `json:"after_start_at"`
	AfterEndAt             *time.Time `json:"after_end_at"`
	CreatedAt              time.Time  `json:"created_at"`
}

type ProductResult struct {
	Disposition Disposition
	Reason      string
	Fact        *ProductFact
}

type EntitlementResult struct {
	Disposition Disposition
	Reason      string
	Fact        *EntitlementFact
}

type EventResult struct {
	Disposition Disposition
	Reason      string
	Fact        *EventFact
}

type History struct {
	Products     []ProductResult
	Entitlements []EntitlementResult
	Events       []EventResult
}

// AdaptHistory conserves input order and row count. Missing optional event
// references remain nil, including failed grants with no unionid/entitlement.
// Customer, trade-product and order target crosswalks belong to the caller.
func AdaptHistory(products, entitlements, events []json.RawMessage) History {
	result := History{Products: make([]ProductResult, len(products)), Entitlements: make([]EntitlementResult, len(entitlements)), Events: make([]EventResult, len(events))}
	productIDs := make(map[int64]int)
	for index, payload := range products {
		var fact ProductFact
		decision := ProductResult{Disposition: DispositionInvalid, Reason: "service_product_json_invalid"}
		if decodeFact(payload, &fact, "id trade_product_id membership_config_id membership_config_name duration_days deleted created_at updated_at") {
			decision.Reason = "service_product_shape_invalid"
			if fact.SourceID > 0 && fact.TradeProductSourceID > 0 && validTimes(fact.CreatedAt, fact.UpdatedAt) {
				decision = ProductResult{Disposition: DispositionCandidate, Fact: &fact}
				productIDs[fact.SourceID]++
			}
		}
		result.Products[index] = decision
	}
	knownProducts := make(map[int64]ProductFact)
	for index, decision := range result.Products {
		if decision.Fact == nil {
			continue
		}
		if productIDs[decision.Fact.SourceID] != 1 {
			result.Products[index] = ProductResult{Disposition: DispositionPending, Reason: "service_product_source_ambiguous"}
		} else {
			knownProducts[decision.Fact.SourceID] = *decision.Fact
		}
	}
	entitlementIDs := make(map[int64]int)
	for index, payload := range entitlements {
		decision := adaptEntitlement(payload, knownProducts)
		result.Entitlements[index] = decision
		if decision.Fact != nil {
			entitlementIDs[decision.Fact.SourceID]++
		}
	}
	knownEntitlements := make(map[int64]EntitlementFact)
	for index, decision := range result.Entitlements {
		if decision.Fact == nil {
			continue
		}
		if entitlementIDs[decision.Fact.SourceID] != 1 {
			result.Entitlements[index] = EntitlementResult{Disposition: DispositionPending, Reason: "entitlement_source_ambiguous"}
		} else {
			knownEntitlements[decision.Fact.SourceID] = *decision.Fact
		}
	}
	for index, payload := range events {
		result.Events[index] = adaptEvent(payload, knownProducts, knownEntitlements)
	}
	return result
}

func adaptEntitlement(payload json.RawMessage, products map[int64]ProductFact) EntitlementResult {
	var fact EntitlementFact
	if !decodeFact(payload, &fact, "id service_product_id trade_product_id unionid external_userid_snapshot membership_config_id status start_at end_at last_out_trade_no renewal_count created_at updated_at") {
		return EntitlementResult{Disposition: DispositionInvalid, Reason: "entitlement_json_invalid"}
	}
	if fact.SourceID < 1 || fact.ServiceProductSourceID < 1 || fact.TradeProductSourceID < 1 || fact.Status == "" ||
		fact.StartAt.IsZero() || fact.EndAt.IsZero() || !validTimes(fact.CreatedAt, fact.UpdatedAt) || !validOptionalID(fact.LastOrderSourceID) {
		return EntitlementResult{Disposition: DispositionInvalid, Reason: "entitlement_shape_invalid"}
	}
	product, found := products[fact.ServiceProductSourceID]
	if !found {
		return EntitlementResult{Disposition: DispositionPending, Reason: "entitlement_service_product_unresolved"}
	}
	if fact.TradeProductSourceID != product.TradeProductSourceID {
		return EntitlementResult{Disposition: DispositionPending, Reason: "entitlement_product_reference_conflict"}
	}
	return EntitlementResult{Disposition: DispositionCandidate, Fact: &fact}
}

func adaptEvent(payload json.RawMessage, products map[int64]ProductFact, entitlements map[int64]EntitlementFact) EventResult {
	var fact EventFact
	if !decodeFact(payload, &fact, "id event_id service_product_id trade_product_id out_trade_no unionid event_type duration_days created_at") {
		return EventResult{Disposition: DispositionInvalid, Reason: "service_event_json_invalid"}
	}
	if fact.SourceID < 1 || fact.ServiceProductSourceID < 1 || fact.TradeProductSourceID < 1 || fact.EventID == "" || fact.EventType == "" || fact.CreatedAt.IsZero() ||
		!validOptionalID(fact.EntitlementSourceID) || !validOptionalID(fact.OrderSourceID) ||
		!validOptionalTime(fact.BeforeStartAt) || !validOptionalTime(fact.BeforeEndAt) || !validOptionalTime(fact.AfterStartAt) || !validOptionalTime(fact.AfterEndAt) {
		return EventResult{Disposition: DispositionInvalid, Reason: "service_event_shape_invalid"}
	}
	product, found := products[fact.ServiceProductSourceID]
	if !found {
		return EventResult{Disposition: DispositionPending, Reason: "service_event_product_unresolved"}
	}
	if fact.TradeProductSourceID != product.TradeProductSourceID {
		return EventResult{Disposition: DispositionPending, Reason: "service_event_product_reference_conflict"}
	}
	if fact.EntitlementSourceID != nil {
		entitlement, found := entitlements[*fact.EntitlementSourceID]
		if !found {
			return EventResult{Disposition: DispositionPending, Reason: "service_event_entitlement_unresolved"}
		}
		if entitlement.ServiceProductSourceID != fact.ServiceProductSourceID || entitlement.TradeProductSourceID != fact.TradeProductSourceID ||
			fact.UnionID != "" && entitlement.UnionID != fact.UnionID {
			return EventResult{Disposition: DispositionPending, Reason: "service_event_entitlement_reference_conflict"}
		}
	}
	return EventResult{Disposition: DispositionCandidate, Fact: &fact}
}

func validTimes(created, updated time.Time) bool {
	return !created.IsZero() && !updated.IsZero() && !updated.Before(created)
}

func validOptionalID(id *int64) bool { return id == nil || *id > 0 }

func validOptionalTime(value *time.Time) bool { return value == nil || !value.IsZero() }

// encoding/json accepts null for primitive Go fields. Reject it only for the
// retained NOT NULL source columns; nullable history references remain nil.
func decodeFact(payload json.RawMessage, target any, required string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range strings.Fields(required) {
		value, found := fields[name]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return json.Unmarshal(payload, target) == nil
}

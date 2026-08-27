package v1domain

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

func TestServicePeriodTargetIDUsesOnlyClosedPairs(t *testing.T) {
	for _, pair := range [][2]string{
		{servicePeriodDefinitionsTable, "product_service_period_history"},
		{servicePeriodEntitlementsTable, "product_service_period_entitlement_history"},
		{servicePeriodEventsTable, "product_service_period_event_history"},
	} {
		domain, target, id := "product", pair[1], "17"
		row := reconciliationRow{TableID: pair[0], TargetDomain: &domain, TargetTable: &target, TargetID: &id, TargetDigest: make([]byte, sha256.Size)}
		if got, table, err := servicePeriodTargetID(row); got != 17 || table != target || err != nil {
			t.Fatalf("valid pair %v: id=%d table=%q err=%v", pair, got, table, err)
		}
		for name, mutate := range map[string]func(*reconciliationRow){
			"wrong domain": func(changed *reconciliationRow) { value := "order"; changed.TargetDomain = &value },
			"wrong source": func(changed *reconciliationRow) { changed.TableID = "public/other" },
			"wrong target": func(changed *reconciliationRow) { value := "products"; changed.TargetTable = &value },
			"zero target":  func(changed *reconciliationRow) { value := "0"; changed.TargetID = &value },
			"short digest": func(changed *reconciliationRow) { changed.TargetDigest = make([]byte, sha256.Size-1) },
		} {
			t.Run(pair[1]+"/"+name, func(t *testing.T) {
				changed := row
				mutate(&changed)
				if _, _, err := servicePeriodTargetID(changed); !errors.Is(err, ErrConflict) {
					t.Fatalf("tampered target accepted: %v", err)
				}
			})
		}
	}
}

func TestServicePeriodReconcileSelectsExactlyThe213RowCoreTables(t *testing.T) {
	if len(servicePeriodReconciledTables) != 3 {
		t.Fatalf("unexpected service-period table count: %d", len(servicePeriodReconciledTables))
	}
	expected := map[string]bool{
		servicePeriodDefinitionsTable:  true,
		servicePeriodEntitlementsTable: true,
		servicePeriodEventsTable:       true,
	}
	for _, table := range servicePeriodReconciledTables {
		if !expected[table] {
			t.Fatalf("unexpected table in core seal: %s", table)
		}
		delete(expected, table)
	}
	if len(expected) != 0 {
		t.Fatalf("core table omitted: %v", expected)
	}
}

func TestServicePeriodReconcilePreservesEmptyConfigurationFields(t *testing.T) {
	definition, entitlement, _ := servicePeriodReconcileRecords()
	definition.MembershipConfigID, definition.MembershipConfigName, entitlement.MembershipConfigID = "", "", ""
	definitionDigest := productapp.ServicePeriodHistoryDefinitionTargetDigest(definition)
	entitlementDigest := productapp.ServicePeriodHistoryEntitlementTargetDigest(entitlement)
	if !servicePeriodDefinitionMatchesTarget(definition, definitionDigest[:]) || !servicePeriodEntitlementMatchesTarget(entitlement, entitlementDigest[:]) {
		t.Fatal("empty source configuration was rejected despite the historical writer contract")
	}
}

func TestServicePeriodReconcileDigestCoversEveryHistoricalFact(t *testing.T) {
	definition, entitlement, event := servicePeriodReconcileRecords()
	definitionDigest := productapp.ServicePeriodHistoryDefinitionTargetDigest(definition)
	entitlementDigest := productapp.ServicePeriodHistoryEntitlementTargetDigest(entitlement)
	eventDigest := productapp.ServicePeriodHistoryEventTargetDigest(event)
	if !servicePeriodDefinitionMatchesTarget(definition, definitionDigest[:]) ||
		!servicePeriodEntitlementMatchesTarget(entitlement, entitlementDigest[:]) ||
		!servicePeriodEventMatchesTarget(event, eventDigest[:]) {
		t.Fatal("exact historical records rejected")
	}
	for name, mutate := range map[string]func(*productport.ServicePeriodHistoryDefinition){
		"source":   func(value *productport.ServicePeriodHistoryDefinition) { value.SourceDefinitionID++ },
		"product":  func(value *productport.ServicePeriodHistoryDefinition) { value.ProductID++ },
		"config":   func(value *productport.ServicePeriodHistoryDefinition) { value.MembershipConfigName += "x" },
		"duration": func(value *productport.ServicePeriodHistoryDefinition) { value.DurationDays++ },
		"deleted":  func(value *productport.ServicePeriodHistoryDefinition) { value.Deleted = !value.Deleted },
		"updated": func(value *productport.ServicePeriodHistoryDefinition) {
			value.UpdatedAt = value.UpdatedAt.Add(time.Microsecond)
		},
	} {
		t.Run("definition/"+name, func(t *testing.T) {
			changed := definition
			mutate(&changed)
			if servicePeriodDefinitionMatchesTarget(changed, definitionDigest[:]) {
				t.Fatal("tampered definition accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*productport.ServicePeriodHistoryEntitlement){
		"source":   func(value *productport.ServicePeriodHistoryEntitlement) { value.SourceEntitlementID++ },
		"parent":   func(value *productport.ServicePeriodHistoryEntitlement) { value.DefinitionID++ },
		"customer": func(value *productport.ServicePeriodHistoryEntitlement) { id := int64(7); value.CustomerID = &id },
		"order":    func(value *productport.ServicePeriodHistoryEntitlement) { id := int64(8); value.LastOrderID = &id },
		"status":   func(value *productport.ServicePeriodHistoryEntitlement) { value.Status += "x" },
		"renewal":  func(value *productport.ServicePeriodHistoryEntitlement) { value.RenewalCount++ },
	} {
		t.Run("entitlement/"+name, func(t *testing.T) {
			changed := entitlement
			mutate(&changed)
			if servicePeriodEntitlementMatchesTarget(changed, entitlementDigest[:]) {
				t.Fatal("tampered entitlement accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*productport.ServicePeriodHistoryEvent){
		"source":      func(value *productport.ServicePeriodHistoryEvent) { value.SourceEventID++ },
		"parent":      func(value *productport.ServicePeriodHistoryEvent) { value.DefinitionID++ },
		"entitlement": func(value *productport.ServicePeriodHistoryEvent) { value.EntitlementID = nil },
		"event":       func(value *productport.ServicePeriodHistoryEvent) { value.EventType += "x" },
		"duration":    func(value *productport.ServicePeriodHistoryEvent) { value.DurationDays++ },
		"times":       func(value *productport.ServicePeriodHistoryEvent) { value.BeforeStartAt = nil },
	} {
		t.Run("event/"+name, func(t *testing.T) {
			changed := event
			mutate(&changed)
			if servicePeriodEventMatchesTarget(changed, eventDigest[:]) {
				t.Fatal("tampered event accepted")
			}
		})
	}
}

func TestServicePeriodReconcileRequiresSameBatchParents(t *testing.T) {
	_, entitlement, event := servicePeriodReconcileRecords()
	targets := map[string]map[string]struct{}{
		"product_service_period_history":             {"101": {}},
		"product_service_period_entitlement_history": {"201": {}},
	}
	if !servicePeriodEntitlementParentMatches(entitlement, targets) || !servicePeriodEventDefinitionMatches(event, targets) ||
		!servicePeriodEventEntitlementMatches(event, entitlement, targets) {
		t.Fatal("exact same-batch parents rejected")
	}
	delete(targets["product_service_period_history"], "101")
	if servicePeriodEntitlementParentMatches(entitlement, targets) || servicePeriodEventDefinitionMatches(event, targets) {
		t.Fatal("missing definition parent accepted")
	}
	targets["product_service_period_history"]["101"] = struct{}{}
	delete(targets["product_service_period_entitlement_history"], "201")
	if servicePeriodEventEntitlementMatches(event, entitlement, targets) {
		t.Fatal("missing entitlement parent accepted")
	}
	entitlement.DefinitionID++
	targets["product_service_period_entitlement_history"]["201"] = struct{}{}
	if servicePeriodEventEntitlementMatches(event, entitlement, targets) {
		t.Fatal("cross-definition entitlement accepted")
	}
}

func TestServicePeriodReconcileRejectsWrongVersionBeforeSeal(t *testing.T) {
	if _, err := ReconcileServicePeriod(nil, nil, "v1-service-period-a2", "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("wrong version accepted: %v", err)
	}
	if _, err := ReconcileServicePeriod(nil, nil, servicePeriodImportVersion, "archive-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("invalid replay input did not fail closed: %v", err)
	}
}

func TestServicePeriodReconcileConvertsNullableFieldsWithoutInventingValues(t *testing.T) {
	stamp := time.Date(2026, 8, 28, 14, 0, 0, 123456789, time.FixedZone("source", 8*3600))
	row := productdb.ProductServicePeriodEventHistory{ID: 301, SourceEventID: 18, DefinitionID: 101, EventID: "event", EventType: "adjusted",
		DurationDays: -30, OutTradeNo: "", CreatedAt: pgtype.Timestamptz{Time: stamp, Valid: true}}
	actual, err := servicePeriodReconcileEvent(row)
	if err != nil || actual.EntitlementID != nil || actual.CustomerID != nil || actual.OrderID != nil ||
		actual.BeforeStartAt != nil || actual.CreatedAt.Location() != time.UTC || actual.CreatedAt.Nanosecond()%1000 != 0 || actual.DurationDays != -30 {
		t.Fatalf("nullable conversion changed source fact: actual=%+v err=%v", actual, err)
	}
	row.CreatedAt = pgtype.Timestamptz{}
	if _, err = servicePeriodReconcileEvent(row); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing required timestamp accepted: %v", err)
	}
}

func servicePeriodReconcileRecords() (productport.ServicePeriodHistoryDefinition, productport.ServicePeriodHistoryEntitlement, productport.ServicePeriodHistoryEvent) {
	stamp := time.Date(2026, 8, 28, 14, 0, 0, 123456000, time.UTC)
	before := stamp.Add(-time.Hour)
	definition := productport.ServicePeriodHistoryDefinition{ID: 101, SourceDefinitionID: 11, ProductID: 501, MembershipConfigID: "config", MembershipConfigName: "周期", DurationDays: -30, CreatedAt: stamp, UpdatedAt: stamp}
	entitlement := productport.ServicePeriodHistoryEntitlement{ID: 201, SourceEntitlementID: 72, DefinitionID: 101, MembershipConfigID: "config", Status: "expired", StartAt: stamp, EndAt: stamp.Add(24 * time.Hour), LastOutTradeNo: "out", RenewalCount: 3, CreatedAt: stamp, UpdatedAt: stamp}
	entitlementID := int64(201)
	event := productport.ServicePeriodHistoryEvent{ID: 301, SourceEventID: 18, DefinitionID: 101, EntitlementID: &entitlementID, EventID: "event-18", EventType: "admin_adjusted", DurationDays: -30, OutTradeNo: "out", BeforeStartAt: &before, CreatedAt: stamp}
	return definition, entitlement, event
}

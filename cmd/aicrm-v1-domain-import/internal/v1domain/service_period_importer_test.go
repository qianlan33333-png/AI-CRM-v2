package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type servicePeriodTxKey struct{}

type servicePeriodUOWFake struct{ commits, rollbacks int }

func (fake *servicePeriodUOWFake) Within(ctx context.Context, callback func(context.Context) error) error {
	if err := callback(context.WithValue(ctx, servicePeriodTxKey{}, fake)); err != nil {
		fake.rollbacks++
		return err
	}
	fake.commits++
	return nil
}

type servicePeriodArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (fake *servicePeriodArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range fake.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type servicePeriodWriterFake struct {
	uow                               *servicePeriodUOWFake
	definitions, entitlements, events int
	failDefinitionReplay              bool
	lastDefinition                    productport.ServicePeriodHistoryDefinition
	lastEntitlement                   productport.ServicePeriodHistoryEntitlement
	lastEvent                         productport.ServicePeriodHistoryEvent
}

func (fake *servicePeriodWriterFake) ImportDefinition(ctx context.Context, source string, payload [sha256.Size]byte, value productport.ServicePeriodHistoryDefinition) (productport.ServicePeriodHistoryReceipt, error) {
	if ctx.Value(servicePeriodTxKey{}) != fake.uow {
		return productport.ServicePeriodHistoryReceipt{}, errors.New("missing transaction")
	}
	fake.definitions++
	if fake.failDefinitionReplay && fake.definitions > 1 {
		return productport.ServicePeriodHistoryReceipt{}, productport.ErrServicePeriodHistoryConflict
	}
	fake.lastDefinition = value
	return servicePeriodReceipt(source, payload, 101, "definition", fake.definitions > 1), nil
}

func (fake *servicePeriodWriterFake) ImportEntitlement(ctx context.Context, source string, payload [sha256.Size]byte, value productport.ServicePeriodHistoryEntitlement) (productport.ServicePeriodHistoryReceipt, error) {
	if ctx.Value(servicePeriodTxKey{}) != fake.uow {
		return productport.ServicePeriodHistoryReceipt{}, errors.New("missing transaction")
	}
	fake.entitlements++
	fake.lastEntitlement = value
	return servicePeriodReceipt(source, payload, 201, "entitlement", fake.entitlements > 1), nil
}

func (fake *servicePeriodWriterFake) ImportEvent(ctx context.Context, source string, payload [sha256.Size]byte, value productport.ServicePeriodHistoryEvent) (productport.ServicePeriodHistoryReceipt, error) {
	if ctx.Value(servicePeriodTxKey{}) != fake.uow {
		return productport.ServicePeriodHistoryReceipt{}, errors.New("missing transaction")
	}
	fake.events++
	fake.lastEvent = value
	return servicePeriodReceipt(source, payload, 301, "event", fake.events > 1), nil
}

func servicePeriodReceipt(source string, payload [sha256.Size]byte, targetID int64, kind string, replayed bool) productport.ServicePeriodHistoryReceipt {
	return productport.ServicePeriodHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID,
		TargetDigest: sha256.Sum256([]byte(kind + "\x00" + source)), Replayed: replayed}
}

type servicePeriodResolverFake struct {
	productID                               int64
	customerID, orderID                     *int64
	productCalls, customerCalls, orderCalls int
	err                                     error
}

func (fake *servicePeriodResolverFake) ResolveServicePeriodProduct(_ context.Context, _ int64) (int64, error) {
	fake.productCalls++
	return fake.productID, fake.err
}

func (fake *servicePeriodResolverFake) ResolveServicePeriodCustomer(_ context.Context, _ string) (*int64, error) {
	fake.customerCalls++
	return fake.customerID, fake.err
}

func (fake *servicePeriodResolverFake) ResolveServicePeriodOrder(_ context.Context, _ int64, _ string) (*int64, error) {
	fake.orderCalls++
	return fake.orderID, fake.err
}

func servicePeriodImporterFixture(t *testing.T, rows map[string][]v1archive.ArchivedRow) (*ServicePeriodImporter, *servicePeriodUOWFake, map[string]*journalTestTx, *servicePeriodWriterFake, *servicePeriodResolverFake) {
	t.Helper()
	uow := &servicePeriodUOWFake{}
	txs, journals := map[string]*journalTestTx{}, map[string]*Journal{}
	for _, table := range servicePeriodTables {
		tx := &journalTestTx{}
		txs[table] = tx
		journals[table] = &Journal{scope: Scope{ImportVersion: "v1-service-period-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
			TableID: table, TargetDomain: "product", TargetTable: servicePeriodTarget(table)}, tx: func(ctx context.Context) (pgx.Tx, error) {
			if ctx.Value(servicePeriodTxKey{}) != uow {
				return nil, errors.New("missing transaction")
			}
			return tx, nil
		}}
	}
	writer := &servicePeriodWriterFake{uow: uow}
	resolver := &servicePeriodResolverFake{productID: 501}
	importer, err := NewServicePeriodImporter(&servicePeriodArchiveFake{rows: rows}, uow, writer, resolver, journals)
	if err != nil {
		t.Fatal(err)
	}
	return importer, uow, txs, writer, resolver
}

func servicePeriodRow(t *testing.T, table string, ordinal int64, payload map[string]any) v1archive.ArchivedRow {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, ordinal))), PayloadHMAC: sha256.Sum256(encoded),
		FieldHMAC: sha256.Sum256([]byte("fields/" + table)), Payload: encoded}
}

func servicePeriodRows(t *testing.T) map[string][]v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 8, 0, 0, 123000, time.UTC)
	definition := servicePeriodRow(t, servicePeriodDefinitionsTable, 1, map[string]any{"id": int64(11), "trade_product_id": int64(29), "membership_config_id": "config", "membership_config_name": "周期会员", "duration_days": -30, "deleted": false, "created_at": stamp, "updated_at": stamp})
	entitlement := servicePeriodRow(t, servicePeriodEntitlementsTable, 1, map[string]any{"id": int64(72), "service_product_id": int64(11), "trade_product_id": int64(29), "unionid": "union", "external_userid_snapshot": "source-only", "membership_config_id": "config", "status": "expired", "start_at": stamp, "end_at": stamp.Add(30 * 24 * time.Hour), "last_order_id": int64(900), "last_out_trade_no": "out", "renewal_count": 3, "created_at": stamp, "updated_at": stamp})
	event := servicePeriodRow(t, servicePeriodEventsTable, 1, map[string]any{"id": int64(18), "event_id": "event-18", "service_product_id": int64(11), "entitlement_id": int64(72), "trade_product_id": int64(29), "order_id": int64(900), "out_trade_no": "out", "unionid": "union", "event_type": "admin_adjusted", "duration_days": -30, "before_start_at": nil, "before_end_at": nil, "after_start_at": nil, "after_end_at": nil, "created_at": stamp})
	return map[string][]v1archive.ArchivedRow{servicePeriodDefinitionsTable: {definition}, servicePeriodEntitlementsTable: {entitlement}, servicePeriodEventsTable: {event}}
}

func missingServicePeriodTerminal() journalTestRow {
	return func(...any) error { return pgx.ErrNoRows }
}

func foundServicePeriodTerminal(row v1archive.ArchivedRow, disposition, reason, targetID string, targetDigest [sha256.Size]byte) journalTestRow {
	return func(values ...any) error {
		*values[0].(*[]byte), *values[1].(*string), *values[2].(*string) = row.PayloadHMAC[:], disposition, reason
		if len(values) == 9 {
			if disposition == "import" {
				domain, table, id := "product", servicePeriodTarget(row.TableID), targetID
				*values[3].(**string), *values[4].(**string), *values[5].(**string) = &domain, &table, &id
				*values[6].(*[]byte) = targetDigest[:]
			}
			*values[7].(*[]byte), *values[8].(*bool) = []byte(`{}`), true
			return nil
		}
		if disposition == "import" {
			id := targetID
			*values[3].(**string), *values[4].(*[]byte) = &id, targetDigest[:]
		}
		*values[5].(*bool) = true
		return nil
	}
}

func prepareServicePeriodQuarantine(tx *journalTestTx, row v1archive.ArchivedRow, reason string) {
	tx.rows = append(tx.rows, missingServicePeriodTerminal(), missingServicePeriodTerminal(), foundServicePeriodTerminal(row, "quarantine", reason, "", [sha256.Size]byte{}))
}

func TestServicePeriodImporterPreservesHistoricalFactsAndReplaysThroughWriter(t *testing.T) {
	rows := servicePeriodRows(t)
	importer, uow, txs, writer, resolver := servicePeriodImporterFixture(t, rows)
	for _, table := range servicePeriodTables {
		row := rows[table][0]
		kind, target := "definition", int64(101)
		if table == servicePeriodEntitlementsTable {
			kind, target = "entitlement", 201
		} else if table == servicePeriodEventsTable {
			kind, target = "event", 301
		}
		receipt := servicePeriodReceipt(SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, target, kind, false)
		txs[table].rows = append(txs[table].rows, foundServicePeriodTerminal(row, "import", "", fmt.Sprintf("%d", target), receipt.TargetDigest))
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ServicePeriodImportResult{ImportedDefinitions: 1, ImportedEntitlements: 1, ImportedEvents: 1}) || uow.commits != 3 ||
		writer.definitions != 1 || writer.entitlements != 1 || writer.events != 1 || resolver.productCalls != 1 || resolver.customerCalls != 2 || resolver.orderCalls != 2 {
		t.Fatalf("result=%+v err=%v writer=%+v resolver=%+v", result, err, writer, resolver)
	}
	if writer.lastDefinition.ProductID != 501 || writer.lastDefinition.DurationDays != -30 || writer.lastEntitlement.CustomerID != nil || writer.lastEntitlement.LastOrderID != nil ||
		writer.lastEvent.CustomerID != nil || writer.lastEvent.OrderID != nil || writer.lastEvent.EntitlementID == nil || *writer.lastEvent.EntitlementID != 201 || writer.lastEvent.DurationDays != -30 {
		t.Fatalf("source facts were rewritten: definition=%+v entitlement=%+v event=%+v", writer.lastDefinition, writer.lastEntitlement, writer.lastEvent)
	}
	for _, table := range servicePeriodTables {
		row := rows[table][0]
		kind, target := "definition", int64(101)
		if table == servicePeriodEntitlementsTable {
			kind, target = "entitlement", 201
		} else if table == servicePeriodEventsTable {
			kind, target = "event", 301
		}
		receipt := servicePeriodReceipt(SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, target, kind, true)
		txs[table].rows = append(txs[table].rows, foundServicePeriodTerminal(row, "import", "", fmt.Sprintf("%d", target), receipt.TargetDigest))
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ServicePeriodImportResult{ImportedDefinitions: 1, ImportedEntitlements: 1, ImportedEvents: 1, Replayed: 3}) || writer.definitions != 2 || writer.entitlements != 2 || writer.events != 2 {
		t.Fatalf("replay result=%+v err=%v writer=%+v", result, err, writer)
	}
}

func TestServicePeriodImporterQuarantinesMissingProductAndDependentFacts(t *testing.T) {
	rows := servicePeriodRows(t)
	importer, _, txs, writer, resolver := servicePeriodImporterFixture(t, rows)
	resolver.productID = 0
	prepareServicePeriodQuarantine(txs[servicePeriodDefinitionsTable], rows[servicePeriodDefinitionsTable][0], "service_period_product_unresolved")
	prepareServicePeriodQuarantine(txs[servicePeriodEntitlementsTable], rows[servicePeriodEntitlementsTable][0], "service_period_definition_unresolved")
	prepareServicePeriodQuarantine(txs[servicePeriodEventsTable], rows[servicePeriodEventsTable][0], "service_period_definition_unresolved")
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ServicePeriodImportResult{Quarantined: 3}) || writer.definitions != 0 || writer.entitlements != 0 || writer.events != 0 {
		t.Fatalf("missing product result=%+v err=%v writer=%+v", result, err, writer)
	}
}

func TestServicePeriodImporterRejectsRedactionAndPropagatesReplayDrift(t *testing.T) {
	rows := servicePeriodRows(t)
	definition := rows[servicePeriodDefinitionsTable][0]
	definition.RedactedFields = []string{"membership_config_id"}
	importer, _, txs, writer, _ := servicePeriodImporterFixture(t, map[string][]v1archive.ArchivedRow{servicePeriodDefinitionsTable: {definition}})
	prepareServicePeriodQuarantine(txs[servicePeriodDefinitionsTable], definition, "redacted_service_period_definition")
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ServicePeriodImportResult{Quarantined: 1}) || writer.definitions != 0 {
		t.Fatalf("redaction result=%+v err=%v", result, err)
	}

	rows = servicePeriodRows(t)
	definition = rows[servicePeriodDefinitionsTable][0]
	importer, _, txs, writer, _ = servicePeriodImporterFixture(t, map[string][]v1archive.ArchivedRow{servicePeriodDefinitionsTable: {definition}})
	receipt := servicePeriodReceipt(SourceIdentifier(definition.SourceKeyHMAC), definition.PayloadHMAC, 101, "definition", false)
	txs[servicePeriodDefinitionsTable].rows = append(txs[servicePeriodDefinitionsTable].rows, foundServicePeriodTerminal(definition, "import", "", "101", receipt.TargetDigest))
	if _, err = importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	writer.failDefinitionReplay = true
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, productport.ErrServicePeriodHistoryConflict) || writer.definitions != 2 {
		t.Fatalf("drift err=%v calls=%d", err, writer.definitions)
	}
}

func TestServicePeriodImporterRejectsReceiptWithDifferentArchivePayload(t *testing.T) {
	rows := servicePeriodRows(t)
	definition := rows[servicePeriodDefinitionsTable][0]
	importer, _, txs, writer, _ := servicePeriodImporterFixture(t, map[string][]v1archive.ArchivedRow{servicePeriodDefinitionsTable: {definition}})
	wrong := definition
	wrong.PayloadHMAC = sha256.Sum256([]byte("different archived payload"))
	receipt := servicePeriodReceipt(SourceIdentifier(definition.SourceKeyHMAC), definition.PayloadHMAC, 101, "definition", false)
	txs[servicePeriodDefinitionsTable].rows = append(txs[servicePeriodDefinitionsTable].rows, foundServicePeriodTerminal(wrong, "import", "", "101", receipt.TargetDigest))
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || writer.definitions != 1 {
		t.Fatalf("payload mismatch err=%v calls=%d", err, writer.definitions)
	}
}

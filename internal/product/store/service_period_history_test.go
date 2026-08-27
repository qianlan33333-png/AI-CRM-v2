package store

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	product "github.com/qianlan33333-png/AI-CRM-v2/internal/product"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

func TestServicePeriodHistoryStoreMapsAllRecordsAndUsesCallerTransaction(t *testing.T) {
	definition, entitlement, event := servicePeriodHistoryFixtures()
	definitionRow := servicePeriodHistoryDefinitionRow(definition)
	definitionRow.ID = 11
	entitlement.DefinitionID = definitionRow.ID
	entitlementRow := servicePeriodHistoryEntitlementRow(entitlement)
	entitlementRow.ID = 12
	event.DefinitionID, event.EntitlementID = definitionRow.ID, &entitlementRow.ID
	eventRow := servicePeriodHistoryEventRow(event)
	eventRow.ID = 13
	tx := &servicePeriodHistoryTestTx{responses: map[string]servicePeriodHistoryTestRow{
		"CreateServicePeriodHistoryDefinition": {values: servicePeriodHistoryDefinitionValues(definitionRow)}, "GetServicePeriodHistoryDefinition": {values: servicePeriodHistoryDefinitionValues(definitionRow)},
		"CreateServicePeriodHistoryEntitlement": {values: servicePeriodHistoryEntitlementValues(entitlementRow)}, "GetServicePeriodHistoryEntitlement": {values: servicePeriodHistoryEntitlementValues(entitlementRow)},
		"CreateServicePeriodHistoryEvent": {values: servicePeriodHistoryEventValues(eventRow)}, "GetServicePeriodHistoryEvent": {values: servicePeriodHistoryEventValues(eventRow)},
	}}
	store := &ServicePeriodHistoryStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
	wantDefinition, _ := servicePeriodHistoryDefinition(definitionRow)
	gotDefinition, err := store.CreateServicePeriodHistoryDefinition(context.Background(), definition)
	if err != nil || !reflect.DeepEqual(gotDefinition, wantDefinition) || !strings.Contains(tx.sql, "CreateServicePeriodHistoryDefinition") {
		t.Fatalf("definition create = %#v, %v", gotDefinition, err)
	}
	if got, err := store.GetServicePeriodHistoryDefinition(context.Background(), gotDefinition.ID); err != nil || !reflect.DeepEqual(got, wantDefinition) {
		t.Fatalf("definition get = %#v, %v", got, err)
	}
	wantEntitlement, _ := servicePeriodHistoryEntitlement(entitlementRow)
	gotEntitlement, err := store.CreateServicePeriodHistoryEntitlement(context.Background(), entitlement)
	if err != nil || !reflect.DeepEqual(gotEntitlement, wantEntitlement) || !strings.Contains(tx.sql, "CreateServicePeriodHistoryEntitlement") {
		t.Fatalf("entitlement create = %#v, %v", gotEntitlement, err)
	}
	if got, err := store.GetServicePeriodHistoryEntitlement(context.Background(), gotEntitlement.ID); err != nil || !reflect.DeepEqual(got, wantEntitlement) {
		t.Fatalf("entitlement get = %#v, %v", got, err)
	}
	wantEvent, _ := servicePeriodHistoryEvent(eventRow)
	gotEvent, err := store.CreateServicePeriodHistoryEvent(context.Background(), event)
	if err != nil || !reflect.DeepEqual(gotEvent, wantEvent) || !strings.Contains(tx.sql, "CreateServicePeriodHistoryEvent") {
		t.Fatalf("event create = %#v, %v", gotEvent, err)
	}
	if got, err := store.GetServicePeriodHistoryEvent(context.Background(), gotEvent.ID); err != nil || !reflect.DeepEqual(got, wantEvent) || !strings.Contains(tx.sql, "FOR UPDATE") {
		t.Fatalf("event get = %#v, %v", got, err)
	}
	if gotDefinition.DurationDays != -7 || gotEntitlement.RenewalCount != -2 || gotEvent.DurationDays != -9 {
		t.Fatal("negative source facts were rewritten")
	}
}

func TestServicePeriodHistoryStoreRejectsInvalidAndClassifiesWrites(t *testing.T) {
	definition, entitlement, event := servicePeriodHistoryFixtures()
	invalidStore := &ServicePeriodHistoryStore{tx: func(context.Context) (pgx.Tx, error) { t.Fatal("invalid input reached transaction"); return nil, nil }}
	definition.SourceDefinitionID = 0
	if _, err := invalidStore.CreateServicePeriodHistoryDefinition(context.Background(), definition); err != productport.ErrServicePeriodHistoryInvalid {
		t.Fatal("invalid definition accepted")
	}
	_, entitlement, event = servicePeriodHistoryFixtures()
	entitlement.Status = ""
	if _, err := invalidStore.CreateServicePeriodHistoryEntitlement(context.Background(), entitlement); err != productport.ErrServicePeriodHistoryInvalid {
		t.Fatal("invalid entitlement accepted")
	}
	event.EventType = ""
	if _, err := invalidStore.CreateServicePeriodHistoryEvent(context.Background(), event); err != productport.ErrServicePeriodHistoryInvalid {
		t.Fatal("invalid event accepted")
	}
	for _, store := range []*ServicePeriodHistoryStore{nil, {}, NewServicePeriodHistoryStore(), {tx: func(context.Context) (pgx.Tx, error) { return nil, nil }}} {
		if _, err := store.GetServicePeriodHistoryDefinition(context.Background(), 1); err != productport.ErrServicePeriodHistoryUnavailable {
			t.Fatal("missing caller transaction accepted")
		}
	}
	for _, cause := range []error{pgx.ErrNoRows, &pgconn.PgError{Code: "23505"}, &pgconn.PgError{Code: "23503"}} {
		tx := &servicePeriodHistoryTestTx{responses: map[string]servicePeriodHistoryTestRow{"CreateServicePeriodHistoryDefinition": {err: cause}}}
		store := &ServicePeriodHistoryStore{tx: func(context.Context) (pgx.Tx, error) { return tx, nil }}
		definition, _, _ := servicePeriodHistoryFixtures()
		if _, err := store.CreateServicePeriodHistoryDefinition(context.Background(), definition); err != productport.ErrServicePeriodHistoryConflict {
			t.Fatalf("write error %v was not conflict", cause)
		}
	}
}

func TestServicePeriodHistoryReaderPagesAndFailsClosed(t *testing.T) {
	definition, entitlement, event := servicePeriodHistoryFixtures()
	definitionRow := servicePeriodHistoryDefinitionRow(definition)
	definitionRow.ID = 11
	entitlement.DefinitionID = definitionRow.ID
	entitlementRow := servicePeriodHistoryEntitlementRow(entitlement)
	entitlementRow.ID = 12
	event.DefinitionID, event.EntitlementID = definitionRow.ID, &entitlementRow.ID
	eventRow := servicePeriodHistoryEventRow(event)
	eventRow.ID = 13
	tx := &servicePeriodHistoryTestTx{responses: map[string]servicePeriodHistoryTestRow{
		"CountServicePeriodHistoryDefinitions": {values: []any{int64(1)}}, "CountServicePeriodHistoryEntitlements": {values: []any{int64(1)}}, "CountServicePeriodHistoryEvents": {values: []any{int64(1)}},
	}, rows: map[string]*servicePeriodHistoryTestRows{
		"ListServicePeriodHistoryDefinitions":  {values: [][]any{servicePeriodHistoryDefinitionProductValues(definitionRow)}},
		"ListServicePeriodHistoryEntitlements": {values: [][]any{servicePeriodHistoryEntitlementValues(entitlementRow)}},
		"ListServicePeriodHistoryEvents":       {values: [][]any{servicePeriodHistoryEventValues(eventRow)}},
	}}
	reader := &ServicePeriodHistoryReader{db: tx}
	definitions, total, err := reader.ListServicePeriodHistoryDefinitions(context.Background(), 1, 0)
	if err != nil || total != 1 || len(definitions) != 1 || definitions[0].DurationDays != -7 || definitions[0].Currency != "CNY" {
		t.Fatalf("definition page = %#v, %d, %v", definitions, total, err)
	}
	entitlements, total, err := reader.ListServicePeriodHistoryEntitlements(context.Background(), definitionRow.ID, 1, 0)
	if err != nil || total != 1 || len(entitlements) != 1 || entitlements[0].CustomerID != nil || entitlements[0].LastOrderID != nil {
		t.Fatalf("entitlement page = %#v, %d, %v", entitlements, total, err)
	}
	events, total, err := reader.ListServicePeriodHistoryEvents(context.Background(), definitionRow.ID, 1, 0)
	if err != nil || total != 1 || len(events) != 1 || events[0].EntitlementID == nil || events[0].CustomerID != nil || events[0].OrderID != nil {
		t.Fatalf("event page = %#v, %d, %v", events, total, err)
	}
	for _, page := range [][2]int32{{0, 0}, {101, 0}, {1, -1}} {
		if _, _, err := reader.ListServicePeriodHistoryDefinitions(context.Background(), page[0], page[1]); err != productport.ErrServicePeriodHistoryInvalid {
			t.Fatal("invalid definition page accepted")
		}
	}
	if _, _, err := reader.ListServicePeriodHistoryEvents(context.Background(), 0, 1, 0); err != productport.ErrServicePeriodHistoryInvalid {
		t.Fatal("invalid definition ID accepted")
	}
	for _, reader := range []*ServicePeriodHistoryReader{nil, {}, NewServicePeriodHistoryReader(nil)} {
		if _, _, err := reader.ListServicePeriodHistoryDefinitions(context.Background(), 1, 0); err != productport.ErrServicePeriodHistoryUnavailable {
			t.Fatal("nil reader accepted")
		}
	}
	for _, name := range []string{"Definitions", "Entitlements", "Events"} {
		tx.responses["CountServicePeriodHistory"+name] = servicePeriodHistoryTestRow{values: []any{int64(0)}}
		tx.rows["ListServicePeriodHistory"+name].values = nil
	}
	definitions, total, err = reader.ListServicePeriodHistoryDefinitions(context.Background(), 1, 0)
	if err != nil || total != 0 || definitions == nil || len(definitions) != 0 {
		t.Fatalf("empty definitions = %#v, %d, %v", definitions, total, err)
	}
	entitlements, total, err = reader.ListServicePeriodHistoryEntitlements(context.Background(), definitionRow.ID, 1, 0)
	if err != nil || total != 0 || entitlements == nil || len(entitlements) != 0 {
		t.Fatalf("empty entitlements = %#v, %d, %v", entitlements, total, err)
	}
	events, total, err = reader.ListServicePeriodHistoryEvents(context.Background(), definitionRow.ID, 1, 0)
	if err != nil || total != 0 || events == nil || len(events) != 0 {
		t.Fatalf("empty events = %#v, %d, %v", events, total, err)
	}
	tx.responses["CountServicePeriodHistoryDefinitions"] = servicePeriodHistoryTestRow{err: errors.New("private")}
	if _, _, err := reader.ListServicePeriodHistoryDefinitions(context.Background(), 1, 0); err != productport.ErrServicePeriodHistoryUnavailable {
		t.Fatal("reader error leaked")
	}
}

var servicePeriodHistoryDatabase = flag.String("service-period-history-test-database-url", "", "optional PostgreSQL test database migrated through 00111")

func TestServicePeriodHistoryPostgresRoundTripAndRollback(t *testing.T) {
	if *servicePeriodHistoryDatabase == "" {
		t.Skip("supply -service-period-history-test-database-url for PostgreSQL integration")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *servicePeriodHistoryDatabase)
	if err != nil {
		t.Fatal("test database open failed")
	}
	defer pool.Close()
	before, err := productdb.New(pool).CountServicePeriodHistoryDefinitions(ctx)
	if err != nil {
		t.Fatal("history migration 00111 is unavailable")
	}
	rollback := errors.New("rollback service-period history fixture")
	err = platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		productRow, err := servicePeriodHistoryTemporaryProduct(txCtx)
		if err != nil {
			return err
		}
		definition, entitlement, event := servicePeriodHistoryFixtures()
		definition.SourceDefinitionID = time.Now().UnixNano()
		definition.ProductID = int64(productRow.ID)
		store := NewServicePeriodHistoryStore()
		storedDefinition, err := store.CreateServicePeriodHistoryDefinition(txCtx, definition)
		if err != nil || storedDefinition.DurationDays != -7 {
			return fmt.Errorf("definition roundtrip: %w", err)
		}
		readDefinition, err := store.GetServicePeriodHistoryDefinition(txCtx, storedDefinition.ID)
		if err != nil || !reflect.DeepEqual(readDefinition, storedDefinition) {
			return errors.New("definition fields changed")
		}
		entitlement.SourceEntitlementID = definition.SourceDefinitionID + 1
		entitlement.DefinitionID = storedDefinition.ID
		storedEntitlement, err := store.CreateServicePeriodHistoryEntitlement(txCtx, entitlement)
		if err != nil || storedEntitlement.CustomerID != nil || storedEntitlement.LastOrderID != nil {
			return fmt.Errorf("entitlement roundtrip: %w", err)
		}
		event.SourceEventID, event.DefinitionID = entitlement.SourceEntitlementID+1, storedDefinition.ID
		storedEvent, err := store.CreateServicePeriodHistoryEvent(txCtx, event)
		if err != nil || storedEvent.EntitlementID != nil || storedEvent.CustomerID != nil || storedEvent.OrderID != nil || storedEvent.DurationDays != -9 {
			return fmt.Errorf("event roundtrip: %w", err)
		}
		transaction, err := platformstore.TxFromContext(txCtx)
		if err != nil {
			return err
		}
		reader := &ServicePeriodHistoryReader{db: transaction}
		definitions, total, err := reader.ListServicePeriodHistoryDefinitions(txCtx, 100, 0)
		if err != nil || total < before+1 || len(definitions) == 0 {
			return errors.New("definition reader failed")
		}
		entitlements, total, err := reader.ListServicePeriodHistoryEntitlements(txCtx, storedDefinition.ID, 100, 0)
		if err != nil || total != 1 || len(entitlements) != 1 {
			return errors.New("entitlement reader failed")
		}
		events, total, err := reader.ListServicePeriodHistoryEvents(txCtx, storedDefinition.ID, 100, 0)
		if err != nil || total != 1 || len(events) != 1 {
			return errors.New("event reader failed")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("PostgreSQL roundtrip = %v", err)
	}
	after, err := productdb.New(pool).CountServicePeriodHistoryDefinitions(ctx)
	if err != nil || after != before {
		t.Fatalf("rollback did not restore history definitions: %d -> %d, %v", before, after, err)
	}
}

func servicePeriodHistoryTemporaryProduct(ctx context.Context) (productport.Product, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	projection, err := json.Marshal(map[string]any{"status": "disabled", "enabled": false})
	if err != nil {
		return productport.Product{}, err
	}
	key := fmt.Sprintf("service-period-history-%d", now.UnixNano())
	return NewHistoricalStaticProductStore().InsertHistoricalStaticProduct(ctx, product.HistoricalStaticProductDefinition{
		SourceIdentifier: key, SourceID: now.UnixNano(), PayloadDigest: [32]byte{1}, OriginalStatus: "v1_history",
		Product: productport.Product{ProductCode: key, Name: "历史周期商品", PriceMinor: 19900, Currency: "CNY", StockQuantity: 0, CreatedBy: 1, CreatedAt: now, UpdatedAt: now, Version: 1, LocalLifecycle: productport.LocalProductDisabled, LegacyAdminProjection: projection},
	})
}

func servicePeriodHistoryFixtures() (productport.ServicePeriodHistoryDefinition, productport.ServicePeriodHistoryEntitlement, productport.ServicePeriodHistoryEvent) {
	stamp := time.Date(2026, 8, 28, 18, 22, 33, 123456789, time.FixedZone("source", 8*3600))
	before, after := stamp.Add(-24*time.Hour), stamp.Add(24*time.Hour)
	return productport.ServicePeriodHistoryDefinition{SourceDefinitionID: 101, ProductID: 8, MembershipConfigID: "hxc", MembershipConfigName: "HXC", DurationDays: -7, Deleted: true, CreatedAt: stamp, UpdatedAt: stamp},
		productport.ServicePeriodHistoryEntitlement{SourceEntitlementID: 102, DefinitionID: 11, MembershipConfigID: "hxc", Status: "expired", StartAt: before, EndAt: after, LastOutTradeNo: "", RenewalCount: -2, CreatedAt: stamp, UpdatedAt: stamp},
		productport.ServicePeriodHistoryEvent{SourceEventID: 103, DefinitionID: 11, EventID: "event-103", EventType: "expired", DurationDays: -9, OutTradeNo: "", BeforeStartAt: &before, BeforeEndAt: &stamp, AfterStartAt: &stamp, AfterEndAt: &after, CreatedAt: stamp}
}

func servicePeriodHistoryDefinitionRow(value productport.ServicePeriodHistoryDefinition) productdb.ProductServicePeriodHistory {
	return productdb.ProductServicePeriodHistory{ID: value.ID, SourceDefinitionID: value.SourceDefinitionID, ProductID: value.ProductID, MembershipConfigID: value.MembershipConfigID, MembershipConfigName: value.MembershipConfigName, DurationDays: value.DurationDays, Deleted: value.Deleted, CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt), UpdatedAt: servicePeriodHistoryTimestamp(value.UpdatedAt)}
}
func servicePeriodHistoryEntitlementRow(value productport.ServicePeriodHistoryEntitlement) productdb.ProductServicePeriodEntitlementHistory {
	return productdb.ProductServicePeriodEntitlementHistory{ID: value.ID, SourceEntitlementID: value.SourceEntitlementID, DefinitionID: value.DefinitionID, CustomerID: servicePeriodHistoryNullableInt64(value.CustomerID), MembershipConfigID: value.MembershipConfigID, Status: value.Status, StartAt: servicePeriodHistoryTimestamp(value.StartAt), EndAt: servicePeriodHistoryTimestamp(value.EndAt), LastOrderID: servicePeriodHistoryNullableInt64(value.LastOrderID), LastOutTradeNo: value.LastOutTradeNo, RenewalCount: value.RenewalCount, CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt), UpdatedAt: servicePeriodHistoryTimestamp(value.UpdatedAt)}
}
func servicePeriodHistoryEventRow(value productport.ServicePeriodHistoryEvent) productdb.ProductServicePeriodEventHistory {
	return productdb.ProductServicePeriodEventHistory{ID: value.ID, SourceEventID: value.SourceEventID, DefinitionID: value.DefinitionID, EntitlementID: servicePeriodHistoryNullableInt64(value.EntitlementID), CustomerID: servicePeriodHistoryNullableInt64(value.CustomerID), OrderID: servicePeriodHistoryNullableInt64(value.OrderID), EventID: value.EventID, EventType: value.EventType, DurationDays: value.DurationDays, OutTradeNo: value.OutTradeNo, BeforeStartAt: servicePeriodHistoryNullableTimestamp(value.BeforeStartAt), BeforeEndAt: servicePeriodHistoryNullableTimestamp(value.BeforeEndAt), AfterStartAt: servicePeriodHistoryNullableTimestamp(value.AfterStartAt), AfterEndAt: servicePeriodHistoryNullableTimestamp(value.AfterEndAt), CreatedAt: servicePeriodHistoryTimestamp(value.CreatedAt)}
}

func servicePeriodHistoryDefinitionValues(value productdb.ProductServicePeriodHistory) []any {
	return []any{value.ID, value.SourceDefinitionID, value.ProductID, value.MembershipConfigID, value.MembershipConfigName, value.DurationDays, value.Deleted, value.CreatedAt, value.UpdatedAt}
}
func servicePeriodHistoryDefinitionProductValues(value productdb.ProductServicePeriodHistory) []any {
	return append(servicePeriodHistoryDefinitionValues(value), "hxc-history", "历史商品", int64(19900), "CNY")
}
func servicePeriodHistoryEntitlementValues(value productdb.ProductServicePeriodEntitlementHistory) []any {
	return []any{value.ID, value.SourceEntitlementID, value.DefinitionID, value.CustomerID, value.MembershipConfigID, value.Status, value.StartAt, value.EndAt, value.LastOrderID, value.LastOutTradeNo, value.RenewalCount, value.CreatedAt, value.UpdatedAt}
}
func servicePeriodHistoryEventValues(value productdb.ProductServicePeriodEventHistory) []any {
	return []any{value.ID, value.SourceEventID, value.DefinitionID, value.EntitlementID, value.CustomerID, value.OrderID, value.EventID, value.EventType, value.DurationDays, value.OutTradeNo, value.BeforeStartAt, value.BeforeEndAt, value.AfterStartAt, value.AfterEndAt, value.CreatedAt}
}

type servicePeriodHistoryTestTx struct {
	pgx.Tx
	responses map[string]servicePeriodHistoryTestRow
	rows      map[string]*servicePeriodHistoryTestRows
	sql       string
	args      []any
}

func (tx *servicePeriodHistoryTestTx) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	tx.sql, tx.args = query, args
	return tx.responses[servicePeriodHistoryQueryName(query)]
}
func (tx *servicePeriodHistoryTestTx) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.sql, tx.args = query, args
	if row := tx.rows[servicePeriodHistoryQueryName(query)]; row != nil {
		row.index = 0
		return row, nil
	}
	return &servicePeriodHistoryTestRows{}, nil
}

type servicePeriodHistoryTestRow struct {
	values []any
	err    error
}

func (row servicePeriodHistoryTestRow) Scan(destination ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(destination) != len(row.values) {
		return errors.New("wrong scan length")
	}
	for index, value := range row.values {
		reflect.ValueOf(destination[index]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type servicePeriodHistoryTestRows struct {
	pgx.Rows
	values [][]any
	index  int
	err    error
}

func (rows *servicePeriodHistoryTestRows) Close()     {}
func (rows *servicePeriodHistoryTestRows) Err() error { return rows.err }
func (rows *servicePeriodHistoryTestRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}
func (rows *servicePeriodHistoryTestRows) Scan(destination ...any) error {
	return (servicePeriodHistoryTestRow{values: rows.values[rows.index-1]}).Scan(destination...)
}
func servicePeriodHistoryQueryName(query string) string {
	const prefix = "-- name: "
	start := strings.Index(query, prefix)
	if start < 0 {
		return ""
	}
	rest := query[start+len(prefix):]
	end := strings.Index(rest, " ")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

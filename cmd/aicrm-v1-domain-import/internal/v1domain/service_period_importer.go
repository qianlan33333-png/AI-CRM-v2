package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1serviceperiod"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

const (
	servicePeriodDefinitionsTable  = "public/service_period_products"
	servicePeriodEntitlementsTable = "public/service_period_entitlements"
	servicePeriodEventsTable       = "public/service_period_events"
)

var servicePeriodTables = [...]string{servicePeriodDefinitionsTable, servicePeriodEntitlementsTable, servicePeriodEventsTable}

type ServicePeriodHistoryWriter interface {
	ImportDefinition(context.Context, string, [sha256.Size]byte, productport.ServicePeriodHistoryDefinition) (productport.ServicePeriodHistoryReceipt, error)
	ImportEntitlement(context.Context, string, [sha256.Size]byte, productport.ServicePeriodHistoryEntitlement) (productport.ServicePeriodHistoryReceipt, error)
	ImportEvent(context.Context, string, [sha256.Size]byte, productport.ServicePeriodHistoryEvent) (productport.ServicePeriodHistoryReceipt, error)
}

type ServicePeriodHistoryResolver interface {
	ResolveServicePeriodProduct(context.Context, int64) (int64, error)
	ResolveServicePeriodCustomer(context.Context, string) (*int64, error)
	ResolveServicePeriodOrder(context.Context, int64, string, int64) (*int64, error)
}

type ServicePeriodImportResult struct {
	ImportedDefinitions  int
	ImportedEntitlements int
	ImportedEvents       int
	Quarantined          int
	Replayed             int
}

type ServicePeriodImporter struct {
	archive  ArchiveSource
	uow      UnitOfWork
	writer   ServicePeriodHistoryWriter
	resolver ServicePeriodHistoryResolver
	journals map[string]*Journal
}

func NewServicePeriodImporter(archive ArchiveSource, uow UnitOfWork, writer ServicePeriodHistoryWriter, resolver ServicePeriodHistoryResolver, journals map[string]*Journal) (*ServicePeriodImporter, error) {
	if archive == nil || uow == nil || writer == nil || resolver == nil || !validServicePeriodJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &ServicePeriodImporter{archive: archive, uow: uow, writer: writer, resolver: resolver, journals: journals}, nil
}

func (importer *ServicePeriodImporter) Import(ctx context.Context, archiveRunID string) (ServicePeriodImportResult, error) {
	if importer == nil || ctx == nil || importer.archive == nil || importer.uow == nil || importer.writer == nil || importer.resolver == nil ||
		!validServicePeriodJournals(importer.journals) || archiveRunID == "" || archiveRunID != importer.journals[servicePeriodDefinitionsTable].scope.ArchiveRunID {
		return ServicePeriodImportResult{}, ErrInvalidScope
	}
	definitions, err := importer.readServicePeriodRows(ctx, archiveRunID, servicePeriodDefinitionsTable)
	if err != nil {
		return ServicePeriodImportResult{}, err
	}
	entitlements, err := importer.readServicePeriodRows(ctx, archiveRunID, servicePeriodEntitlementsTable)
	if err != nil {
		return ServicePeriodImportResult{}, err
	}
	events, err := importer.readServicePeriodRows(ctx, archiveRunID, servicePeriodEventsTable)
	if err != nil {
		return ServicePeriodImportResult{}, err
	}
	history := v1serviceperiod.AdaptHistory(servicePeriodPayloads(definitions), servicePeriodPayloads(entitlements), servicePeriodPayloads(events))
	result := ServicePeriodImportResult{}
	definitionTargets := map[int64]servicePeriodDefinitionTarget{}
	for index, row := range definitions {
		outcome, target, err := importer.importServicePeriodDefinition(ctx, importer.journals[servicePeriodDefinitionsTable], row, history.Products[index])
		if err != nil {
			return ServicePeriodImportResult{}, err
		}
		result.add(outcome)
		if target.definitionID > 0 {
			if old, found := definitionTargets[target.sourceID]; found && old != target {
				return ServicePeriodImportResult{}, ErrConflict
			}
			definitionTargets[target.sourceID] = target
		}
	}
	entitlementTargets := map[int64]int64{}
	for index, row := range entitlements {
		outcome, targetID, err := importer.importServicePeriodEntitlement(ctx, importer.journals[servicePeriodEntitlementsTable], row, history.Entitlements[index], definitionTargets)
		if err != nil {
			return ServicePeriodImportResult{}, err
		}
		result.add(outcome)
		if targetID.sourceID > 0 {
			if old, found := entitlementTargets[targetID.sourceID]; found && old != targetID.targetID {
				return ServicePeriodImportResult{}, ErrConflict
			}
			entitlementTargets[targetID.sourceID] = targetID.targetID
		}
	}
	for index, row := range events {
		outcome, err := importer.importServicePeriodEvent(ctx, importer.journals[servicePeriodEventsTable], row, history.Events[index], definitionTargets, entitlementTargets)
		if err != nil {
			return ServicePeriodImportResult{}, err
		}
		result.add(outcome)
	}
	return result, nil
}

type servicePeriodOutcome struct{ definitions, entitlements, events, quarantined, replayed int }

func (result *ServicePeriodImportResult) add(outcome servicePeriodOutcome) {
	result.ImportedDefinitions += outcome.definitions
	result.ImportedEntitlements += outcome.entitlements
	result.ImportedEvents += outcome.events
	result.Quarantined += outcome.quarantined
	result.Replayed += outcome.replayed
}

type servicePeriodDefinitionTarget struct{ sourceID, definitionID, productID int64 }
type servicePeriodEntitlementTarget struct{ sourceID, targetID int64 }

func (importer *ServicePeriodImporter) importServicePeriodDefinition(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, decision v1serviceperiod.ProductResult) (servicePeriodOutcome, servicePeriodDefinitionTarget, error) {
	if servicePeriodRedacted(row.RedactedFields, "id", "trade_product_id", "membership_config_id", "membership_config_name", "duration_days", "deleted", "created_at", "updated_at") {
		outcome, err := importer.quarantineServicePeriod(ctx, journal, row, "redacted_service_period_definition")
		return outcome, servicePeriodDefinitionTarget{}, err
	}
	if decision.Disposition != v1serviceperiod.DispositionCandidate || decision.Fact == nil {
		outcome, err := importer.quarantineServicePeriod(ctx, journal, row, "invalid_service_period_definition")
		return outcome, servicePeriodDefinitionTarget{}, err
	}
	fact := *decision.Fact
	var target servicePeriodDefinitionTarget
	replayed, quarantined := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		target, replayed, quarantined = servicePeriodDefinitionTarget{}, false, false
		productID, err := importer.resolver.ResolveServicePeriodProduct(tx, fact.TradeProductSourceID)
		if err != nil {
			return err
		}
		if productID == 0 {
			quarantined = true
			var err error
			replayed, err = recordServicePeriodTerminal(tx, journal, row, "service_period_product_unresolved")
			return err
		}
		if productID < 0 {
			return ErrConflict
		}
		definition := productport.ServicePeriodHistoryDefinition{SourceDefinitionID: fact.SourceID, ProductID: productID,
			MembershipConfigID: fact.MembershipConfigID, MembershipConfigName: fact.MembershipConfigName, DurationDays: fact.DurationDays,
			Deleted: fact.Deleted, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
		receipt, err := importer.writer.ImportDefinition(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, definition)
		if err != nil {
			return err
		}
		if err = verifyServicePeriodReceipt(tx, journal, row, receipt); err != nil {
			return err
		}
		target = servicePeriodDefinitionTarget{sourceID: fact.SourceID, definitionID: receipt.TargetID, productID: productID}
		replayed = receipt.Replayed
		return nil
	})
	if err != nil {
		return servicePeriodOutcome{}, servicePeriodDefinitionTarget{}, err
	}
	if quarantined {
		return servicePeriodOutcome{quarantined: 1, replayed: boolCount(replayed)}, servicePeriodDefinitionTarget{}, nil
	}
	return servicePeriodOutcome{definitions: 1, replayed: boolCount(replayed)}, target, nil
}

func (importer *ServicePeriodImporter) importServicePeriodEntitlement(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, decision v1serviceperiod.EntitlementResult, definitions map[int64]servicePeriodDefinitionTarget) (servicePeriodOutcome, servicePeriodEntitlementTarget, error) {
	if servicePeriodRedacted(row.RedactedFields, "id", "service_product_id", "trade_product_id", "unionid", "membership_config_id", "status", "start_at", "end_at", "last_order_id", "last_out_trade_no", "renewal_count", "created_at", "updated_at") {
		outcome, err := importer.quarantineServicePeriod(ctx, journal, row, "redacted_service_period_entitlement")
		return outcome, servicePeriodEntitlementTarget{}, err
	}
	if decision.Disposition != v1serviceperiod.DispositionCandidate || decision.Fact == nil {
		outcome, err := importer.quarantineServicePeriod(ctx, journal, row, "invalid_service_period_entitlement")
		return outcome, servicePeriodEntitlementTarget{}, err
	}
	fact := *decision.Fact
	var target servicePeriodEntitlementTarget
	replayed, quarantined := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		target, replayed, quarantined = servicePeriodEntitlementTarget{}, false, false
		definition, found := definitions[fact.ServiceProductSourceID]
		if !found || definition.productID < 1 {
			quarantined = true
			var err error
			replayed, err = recordServicePeriodTerminal(tx, journal, row, "service_period_definition_unresolved")
			return err
		}
		customerID, err := importer.resolveServicePeriodCustomer(tx, fact.UnionID)
		if err != nil {
			return err
		}
		orderID, err := importer.resolveServicePeriodOrder(tx, fact.LastOrderSourceID, fact.LastOutTradeNo, definition.productID)
		if err != nil {
			return err
		}
		value := productport.ServicePeriodHistoryEntitlement{SourceEntitlementID: fact.SourceID, DefinitionID: definition.definitionID, CustomerID: customerID,
			MembershipConfigID: fact.MembershipConfigID, Status: fact.Status, StartAt: fact.StartAt, EndAt: fact.EndAt, LastOrderID: orderID,
			LastOutTradeNo: fact.LastOutTradeNo, RenewalCount: fact.RenewalCount, CreatedAt: fact.CreatedAt, UpdatedAt: fact.UpdatedAt}
		receipt, err := importer.writer.ImportEntitlement(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if err != nil {
			return err
		}
		if err = verifyServicePeriodReceipt(tx, journal, row, receipt); err != nil {
			return err
		}
		target = servicePeriodEntitlementTarget{sourceID: fact.SourceID, targetID: receipt.TargetID}
		replayed = receipt.Replayed
		return nil
	})
	if err != nil {
		return servicePeriodOutcome{}, servicePeriodEntitlementTarget{}, err
	}
	if quarantined {
		return servicePeriodOutcome{quarantined: 1, replayed: boolCount(replayed)}, servicePeriodEntitlementTarget{}, nil
	}
	return servicePeriodOutcome{entitlements: 1, replayed: boolCount(replayed)}, target, nil
}

func (importer *ServicePeriodImporter) importServicePeriodEvent(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, decision v1serviceperiod.EventResult, definitions map[int64]servicePeriodDefinitionTarget, entitlements map[int64]int64) (servicePeriodOutcome, error) {
	if servicePeriodRedacted(row.RedactedFields, "id", "event_id", "service_product_id", "entitlement_id", "trade_product_id", "order_id", "out_trade_no", "unionid", "event_type", "duration_days", "before_start_at", "before_end_at", "after_start_at", "after_end_at", "created_at") {
		return importer.quarantineServicePeriod(ctx, journal, row, "redacted_service_period_event")
	}
	if decision.Disposition != v1serviceperiod.DispositionCandidate || decision.Fact == nil {
		return importer.quarantineServicePeriod(ctx, journal, row, "invalid_service_period_event")
	}
	fact := *decision.Fact
	replayed, quarantined := false, false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		replayed, quarantined = false, false
		definition, found := definitions[fact.ServiceProductSourceID]
		if !found || definition.productID < 1 {
			quarantined = true
			var err error
			replayed, err = recordServicePeriodTerminal(tx, journal, row, "service_period_definition_unresolved")
			return err
		}
		var entitlementID *int64
		if fact.EntitlementSourceID != nil {
			id, found := entitlements[*fact.EntitlementSourceID]
			if !found {
				quarantined = true
				var err error
				replayed, err = recordServicePeriodTerminal(tx, journal, row, "service_period_entitlement_unresolved")
				return err
			}
			entitlementID = &id
		}
		customerID, err := importer.resolveServicePeriodCustomer(tx, fact.UnionID)
		if err != nil {
			return err
		}
		orderID, err := importer.resolveServicePeriodOrder(tx, fact.OrderSourceID, fact.OutTradeNo, definition.productID)
		if err != nil {
			return err
		}
		value := productport.ServicePeriodHistoryEvent{SourceEventID: fact.SourceID, DefinitionID: definition.definitionID, EntitlementID: entitlementID,
			CustomerID: customerID, OrderID: orderID, EventID: fact.EventID, EventType: fact.EventType, DurationDays: fact.DurationDays,
			OutTradeNo: fact.OutTradeNo, BeforeStartAt: fact.BeforeStartAt, BeforeEndAt: fact.BeforeEndAt, AfterStartAt: fact.AfterStartAt,
			AfterEndAt: fact.AfterEndAt, CreatedAt: fact.CreatedAt}
		receipt, err := importer.writer.ImportEvent(tx, SourceIdentifier(row.SourceKeyHMAC), row.PayloadHMAC, value)
		if err != nil {
			return err
		}
		if err = verifyServicePeriodReceipt(tx, journal, row, receipt); err != nil {
			return err
		}
		replayed = receipt.Replayed
		return nil
	})
	if err != nil {
		return servicePeriodOutcome{}, err
	}
	if quarantined {
		return servicePeriodOutcome{quarantined: 1, replayed: boolCount(replayed)}, nil
	}
	return servicePeriodOutcome{events: 1, replayed: boolCount(replayed)}, nil
}

func (importer *ServicePeriodImporter) resolveServicePeriodCustomer(ctx context.Context, unionID string) (*int64, error) {
	if unionID == "" {
		return nil, nil
	}
	customerID, err := importer.resolver.ResolveServicePeriodCustomer(ctx, unionID)
	if err != nil || customerID == nil {
		return customerID, err
	}
	if *customerID < 1 {
		return nil, ErrConflict
	}
	return customerID, nil
}

func (importer *ServicePeriodImporter) resolveServicePeriodOrder(ctx context.Context, sourceID *int64, outTradeNo string, productID int64) (*int64, error) {
	if sourceID == nil {
		return nil, nil
	}
	orderID, err := importer.resolver.ResolveServicePeriodOrder(ctx, *sourceID, outTradeNo, productID)
	if err != nil || orderID == nil {
		return orderID, err
	}
	if *orderID < 1 {
		return nil, ErrConflict
	}
	return orderID, nil
}

func (importer *ServicePeriodImporter) quarantineServicePeriod(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, reason string) (servicePeriodOutcome, error) {
	replayed := false
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		var err error
		replayed, err = recordServicePeriodTerminal(tx, journal, row, reason)
		return err
	})
	return servicePeriodOutcome{quarantined: 1, replayed: boolCount(replayed)}, err
}

func (importer *ServicePeriodImporter) readServicePeriodRows(ctx context.Context, archiveRunID, tableID string) ([]v1archive.ArchivedRow, error) {
	rows := []v1archive.ArchivedRow{}
	err := importer.archive.EachTableRow(ctx, archiveRunID, tableID, func(row v1archive.ArchivedRow) error {
		if row.TableID != tableID || row.AdapterID != v1archive.DefaultAdapterID || row.SourceOrdinal < 1 ||
			row.SourceKeyHMAC == [sha256.Size]byte{} || row.PayloadHMAC == [sha256.Size]byte{} {
			return ErrConflict
		}
		rows = append(rows, row)
		return nil
	})
	return rows, err
}

func servicePeriodPayloads(rows []v1archive.ArchivedRow) []json.RawMessage {
	payloads := make([]json.RawMessage, len(rows))
	for index, row := range rows {
		payloads[index] = row.Payload
	}
	return payloads
}

func recordServicePeriodTerminal(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, reason string) (bool, error) {
	existing, found, err := journal.LoadTerminal(ctx, SourceIdentifier(row.SourceKeyHMAC))
	if err != nil {
		return false, err
	}
	if found {
		if existing.SourceKeyDigest != row.SourceKeyHMAC || existing.PayloadDigest != row.PayloadHMAC || existing.Disposition != "quarantine" || existing.Reason != reason ||
			existing.TargetID != "" || existing.TargetDigest != [sha256.Size]byte{} || len(existing.Metadata) != 0 {
			return false, ErrConflict
		}
		return true, nil
	}
	return false, journal.Record(ctx, TerminalReceipt{SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "quarantine", Reason: reason})
}

func verifyServicePeriodReceipt(ctx context.Context, journal *Journal, row v1archive.ArchivedRow, receipt productport.ServicePeriodHistoryReceipt) error {
	if receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC || receipt.TargetID < 1 || receipt.TargetDigest == [sha256.Size]byte{} {
		return ErrConflict
	}
	terminal, found, err := journal.LoadTerminal(ctx, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" ||
		terminal.Reason != "" || terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func validServicePeriodJournals(journals map[string]*Journal) bool {
	if len(journals) != len(servicePeriodTables) {
		return false
	}
	var version, run string
	for index, table := range servicePeriodTables {
		journal := journals[table]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.TableID != table || journal.scope.AdapterID != v1archive.DefaultAdapterID ||
			journal.scope.TargetDomain != "product" || journal.scope.TargetTable != servicePeriodTarget(table) {
			return false
		}
		if index == 0 {
			version, run = journal.scope.ImportVersion, journal.scope.ArchiveRunID
		} else if journal.scope.ImportVersion != version || journal.scope.ArchiveRunID != run {
			return false
		}
	}
	return true
}

func servicePeriodTarget(table string) string {
	switch table {
	case servicePeriodDefinitionsTable:
		return "product_service_period_history"
	case servicePeriodEntitlementsTable:
		return "product_service_period_entitlement_history"
	case servicePeriodEventsTable:
		return "product_service_period_event_history"
	default:
		return ""
	}
}

func servicePeriodRedacted(fields []string, required ...string) bool {
	for _, field := range fields {
		for _, name := range required {
			if field == name {
				return true
			}
		}
	}
	return false
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

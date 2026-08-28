package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

const servicePeriodImportVersion = "v1-service-period-a1"

var servicePeriodReconciledTables = []string{
	servicePeriodDefinitionsTable,
	servicePeriodEntitlementsTable,
	servicePeriodEventsTable,
}

// ReconcileServicePeriod seals only the three V1 service-period history tables.
// It deliberately excludes usage snapshots and any current membership state.
func ReconcileServicePeriod(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	if importVersion != servicePeriodImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, importVersion, archiveRunID, servicePeriodReconciledTables)
}

// verifyServicePeriodTarget is selected by reconcile.go after the owner adds
// the three closed source-to-target mappings. Reads use the generated product
// queries so reconciliation cannot write or invent a target fact.
func verifyServicePeriodTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	id, targetTable, err := servicePeriodTargetID(row)
	if err != nil || ctx == nil || tx == nil {
		return "", ErrConflict
	}
	queries := productdb.New(tx)
	verified := false
	switch targetTable {
	case "product_service_period_history":
		stored, readErr := queries.GetServicePeriodHistoryDefinition(ctx, id)
		actual, convertErr := servicePeriodReconcileDefinition(stored)
		err = firstServicePeriodError(readErr, convertErr)
		verified = err == nil && servicePeriodDefinitionMatchesTarget(actual, row.TargetDigest)
	case "product_service_period_entitlement_history":
		stored, readErr := queries.GetServicePeriodHistoryEntitlement(ctx, id)
		actual, convertErr := servicePeriodReconcileEntitlement(stored)
		err = firstServicePeriodError(readErr, convertErr)
		verified = err == nil && servicePeriodEntitlementParentMatches(actual, targets) &&
			servicePeriodEntitlementMatchesTarget(actual, row.TargetDigest)
	case "product_service_period_event_history":
		stored, readErr := queries.GetServicePeriodHistoryEvent(ctx, id)
		actual, convertErr := servicePeriodReconcileEvent(stored)
		err = firstServicePeriodError(readErr, convertErr)
		if err == nil {
			verified = servicePeriodEventDefinitionMatches(actual, targets) &&
				servicePeriodEventMatchesTarget(actual, row.TargetDigest)
			if verified && actual.EntitlementID != nil {
				entitlement, parentErr := queries.GetServicePeriodHistoryEntitlement(ctx, *actual.EntitlementID)
				parent, convertParentErr := servicePeriodReconcileEntitlement(entitlement)
				err = firstServicePeriodError(parentErr, convertParentErr)
				verified = err == nil && servicePeriodEventEntitlementMatches(actual, parent, targets)
			}
		}
	}
	if err != nil || !verified {
		return "", targetVerificationError(targetTable, *row.TargetID, err)
	}
	return targetTable + ":" + *row.TargetID + ":v1_history:" + hex.EncodeToString(row.TargetDigest), nil
}

func servicePeriodTargetID(row reconciliationRow) (int64, string, error) {
	if row.TargetDomain == nil || *row.TargetDomain != "product" || row.TargetTable == nil || row.TargetID == nil || len(row.TargetDigest) != sha256.Size {
		return 0, "", ErrConflict
	}
	targetTable := *row.TargetTable
	validPair := (row.TableID == servicePeriodDefinitionsTable && targetTable == "product_service_period_history") ||
		(row.TableID == servicePeriodEntitlementsTable && targetTable == "product_service_period_entitlement_history") ||
		(row.TableID == servicePeriodEventsTable && targetTable == "product_service_period_event_history")
	if !validPair {
		return 0, "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	return id, targetTable, err
}

func servicePeriodDefinitionMatchesTarget(value productport.ServicePeriodHistoryDefinition, expected []byte) bool {
	digest := productapp.ServicePeriodHistoryDefinitionTargetDigest(value)
	return validServicePeriodReconcileDefinition(value) && len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func servicePeriodEntitlementMatchesTarget(value productport.ServicePeriodHistoryEntitlement, expected []byte) bool {
	digest := productapp.ServicePeriodHistoryEntitlementTargetDigest(value)
	return validServicePeriodReconcileEntitlement(value) && len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func servicePeriodEventMatchesTarget(value productport.ServicePeriodHistoryEvent, expected []byte) bool {
	digest := productapp.ServicePeriodHistoryEventTargetDigest(value)
	return validServicePeriodReconcileEvent(value) && len(expected) == sha256.Size && equalBytes(digest[:], expected)
}

func servicePeriodEntitlementParentMatches(value productport.ServicePeriodHistoryEntitlement, targets map[string]map[string]struct{}) bool {
	return containsTarget(targets, "product_service_period_history", strconv.FormatInt(value.DefinitionID, 10))
}

func servicePeriodEventDefinitionMatches(value productport.ServicePeriodHistoryEvent, targets map[string]map[string]struct{}) bool {
	return containsTarget(targets, "product_service_period_history", strconv.FormatInt(value.DefinitionID, 10))
}

func servicePeriodEventEntitlementMatches(event productport.ServicePeriodHistoryEvent, entitlement productport.ServicePeriodHistoryEntitlement, targets map[string]map[string]struct{}) bool {
	return event.EntitlementID != nil && entitlement.ID == *event.EntitlementID && entitlement.DefinitionID == event.DefinitionID &&
		containsTarget(targets, "product_service_period_entitlement_history", strconv.FormatInt(*event.EntitlementID, 10))
}

func servicePeriodReconcileDefinition(value productdb.ProductServicePeriodHistory) (productport.ServicePeriodHistoryDefinition, error) {
	created, err := servicePeriodReconcileRequiredTime(value.CreatedAt)
	updated, updatedErr := servicePeriodReconcileRequiredTime(value.UpdatedAt)
	if err = firstServicePeriodError(err, updatedErr); err != nil {
		return productport.ServicePeriodHistoryDefinition{}, err
	}
	return productport.ServicePeriodHistoryDefinition{ID: value.ID, SourceDefinitionID: value.SourceDefinitionID, ProductID: value.ProductID,
		MembershipConfigID: value.MembershipConfigID, MembershipConfigName: value.MembershipConfigName, DurationDays: value.DurationDays,
		Deleted: value.Deleted, CreatedAt: created, UpdatedAt: updated}, nil
}

func servicePeriodReconcileEntitlement(value productdb.ProductServicePeriodEntitlementHistory) (productport.ServicePeriodHistoryEntitlement, error) {
	customer, err := servicePeriodReconcileOptionalID(value.CustomerID)
	order, orderErr := servicePeriodReconcileOptionalID(value.LastOrderID)
	start, startErr := servicePeriodReconcileRequiredTime(value.StartAt)
	end, endErr := servicePeriodReconcileRequiredTime(value.EndAt)
	created, createdErr := servicePeriodReconcileRequiredTime(value.CreatedAt)
	updated, updatedErr := servicePeriodReconcileRequiredTime(value.UpdatedAt)
	if err = firstServicePeriodError(err, orderErr, startErr, endErr, createdErr, updatedErr); err != nil {
		return productport.ServicePeriodHistoryEntitlement{}, err
	}
	return productport.ServicePeriodHistoryEntitlement{ID: value.ID, SourceEntitlementID: value.SourceEntitlementID, DefinitionID: value.DefinitionID,
		CustomerID: customer, MembershipConfigID: value.MembershipConfigID, Status: value.Status, StartAt: start, EndAt: end,
		LastOrderID: order, LastOutTradeNo: value.LastOutTradeNo, RenewalCount: value.RenewalCount, CreatedAt: created, UpdatedAt: updated}, nil
}

func servicePeriodReconcileEvent(value productdb.ProductServicePeriodEventHistory) (productport.ServicePeriodHistoryEvent, error) {
	entitlement, err := servicePeriodReconcileOptionalID(value.EntitlementID)
	customer, customerErr := servicePeriodReconcileOptionalID(value.CustomerID)
	order, orderErr := servicePeriodReconcileOptionalID(value.OrderID)
	beforeStart, beforeStartErr := servicePeriodReconcileOptionalTime(value.BeforeStartAt)
	beforeEnd, beforeEndErr := servicePeriodReconcileOptionalTime(value.BeforeEndAt)
	afterStart, afterStartErr := servicePeriodReconcileOptionalTime(value.AfterStartAt)
	afterEnd, afterEndErr := servicePeriodReconcileOptionalTime(value.AfterEndAt)
	created, createdErr := servicePeriodReconcileRequiredTime(value.CreatedAt)
	if err = firstServicePeriodError(err, customerErr, orderErr, beforeStartErr, beforeEndErr, afterStartErr, afterEndErr, createdErr); err != nil {
		return productport.ServicePeriodHistoryEvent{}, err
	}
	return productport.ServicePeriodHistoryEvent{ID: value.ID, SourceEventID: value.SourceEventID, DefinitionID: value.DefinitionID,
		EntitlementID: entitlement, CustomerID: customer, OrderID: order, EventID: value.EventID, EventType: value.EventType,
		DurationDays: value.DurationDays, OutTradeNo: value.OutTradeNo, BeforeStartAt: beforeStart, BeforeEndAt: beforeEnd,
		AfterStartAt: afterStart, AfterEndAt: afterEnd, CreatedAt: created}, nil
}

func servicePeriodReconcileRequiredTime(value pgtype.Timestamptz) (time.Time, error) {
	if !value.Valid || value.Time.IsZero() {
		return time.Time{}, ErrConflict
	}
	return value.Time.UTC().Truncate(time.Microsecond), nil
}

func servicePeriodReconcileOptionalTime(value pgtype.Timestamptz) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.Time.IsZero() {
		return nil, ErrConflict
	}
	normalized := value.Time.UTC().Truncate(time.Microsecond)
	return &normalized, nil
}

func servicePeriodReconcileOptionalID(value pgtype.Int8) (*int64, error) {
	if !value.Valid {
		return nil, nil
	}
	if value.Int64 < 1 {
		return nil, ErrConflict
	}
	id := value.Int64
	return &id, nil
}

func validServicePeriodReconcileDefinition(value productport.ServicePeriodHistoryDefinition) bool {
	return value.ID > 0 && value.SourceDefinitionID > 0 && value.ProductID > 0 &&
		!value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validServicePeriodReconcileEntitlement(value productport.ServicePeriodHistoryEntitlement) bool {
	return value.ID > 0 && value.SourceEntitlementID > 0 && value.DefinitionID > 0 && value.Status != "" &&
		!value.StartAt.IsZero() && !value.EndAt.IsZero() && !value.CreatedAt.IsZero() && !value.UpdatedAt.IsZero() && !value.UpdatedAt.Before(value.CreatedAt)
}

func validServicePeriodReconcileEvent(value productport.ServicePeriodHistoryEvent) bool {
	return value.ID > 0 && value.SourceEventID > 0 && value.DefinitionID > 0 && value.EventID != "" && value.EventType != "" && !value.CreatedAt.IsZero()
}

func firstServicePeriodError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

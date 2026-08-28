package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

// ServicePeriodHistoryWriter writes static facts and receipts in the caller's
// transaction. It has no current-member, Product mutation, event or Provider API.
type ServicePeriodHistoryWriter struct {
	store   productport.ServicePeriodHistoryStore
	journal productport.ServicePeriodHistoryJournal
}

func NewServicePeriodHistoryWriter(store productport.ServicePeriodHistoryStore, journal productport.ServicePeriodHistoryJournal) (*ServicePeriodHistoryWriter, error) {
	if nilServicePeriodDependency(store) || nilServicePeriodDependency(journal) {
		return nil, productport.ErrServicePeriodHistoryUnavailable
	}
	return &ServicePeriodHistoryWriter{store: store, journal: journal}, nil
}

func (writer *ServicePeriodHistoryWriter) ImportDefinition(ctx context.Context, source string, payload [32]byte, record productport.ServicePeriodHistoryDefinition) (productport.ServicePeriodHistoryReceipt, error) {
	record = normalizeHistoryDefinition(record)
	if record.ID != 0 || record.SourceDefinitionID < 1 || record.ProductID < 1 ||
		!historyText(record.MembershipConfigID, record.MembershipConfigName) || !historyTimes(record.CreatedAt, record.UpdatedAt) {
		return productport.ServicePeriodHistoryReceipt{}, productport.ErrServicePeriodHistoryInvalid
	}
	return writer.importHistory(ctx, "definitions", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = id
		return ServicePeriodHistoryDefinitionTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual productport.ServicePeriodHistoryDefinition
		var err error
		if id == 0 {
			actual, err = writer.store.CreateServicePeriodHistoryDefinition(ctx, record)
		} else {
			actual, err = writer.store.GetServicePeriodHistoryDefinition(ctx, id)
		}
		return actual.ID, ServicePeriodHistoryDefinitionTargetDigest(actual), err
	})
}

func (writer *ServicePeriodHistoryWriter) ImportEntitlement(ctx context.Context, source string, payload [32]byte, record productport.ServicePeriodHistoryEntitlement) (productport.ServicePeriodHistoryReceipt, error) {
	record = normalizeHistoryEntitlement(record)
	if record.ID != 0 || record.SourceEntitlementID < 1 || record.DefinitionID < 1 ||
		!historyOptionalID(record.CustomerID) || !historyOptionalID(record.LastOrderID) || record.Status == "" ||
		!historyText(record.MembershipConfigID, record.Status, record.LastOutTradeNo) ||
		!historyTimes(record.CreatedAt, record.UpdatedAt) || !historyTime(record.StartAt) || !historyTime(record.EndAt) {
		return productport.ServicePeriodHistoryReceipt{}, productport.ErrServicePeriodHistoryInvalid
	}
	return writer.importHistory(ctx, "entitlements", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = id
		return ServicePeriodHistoryEntitlementTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		var actual productport.ServicePeriodHistoryEntitlement
		var err error
		if id == 0 {
			actual, err = writer.store.CreateServicePeriodHistoryEntitlement(ctx, record)
		} else {
			actual, err = writer.store.GetServicePeriodHistoryEntitlement(ctx, id)
		}
		return actual.ID, ServicePeriodHistoryEntitlementTargetDigest(actual), err
	})
}

func (writer *ServicePeriodHistoryWriter) ImportEvent(ctx context.Context, source string, payload [32]byte, record productport.ServicePeriodHistoryEvent) (productport.ServicePeriodHistoryReceipt, error) {
	record = normalizeHistoryEvent(record)
	if record.ID != 0 || record.SourceEventID < 1 || record.DefinitionID < 1 ||
		!historyOptionalID(record.EntitlementID) || !historyOptionalID(record.CustomerID) || !historyOptionalID(record.OrderID) ||
		record.EventID == "" || record.EventType == "" || !historyText(record.EventID, record.EventType, record.OutTradeNo) ||
		!historyTime(record.CreatedAt) || !historyOptionalTimes(record.BeforeStartAt, record.BeforeEndAt, record.AfterStartAt, record.AfterEndAt) {
		return productport.ServicePeriodHistoryReceipt{}, productport.ErrServicePeriodHistoryInvalid
	}
	return writer.importHistory(ctx, "events", source, payload, func(id int64) [32]byte {
		expected := record
		expected.ID = id
		return ServicePeriodHistoryEventTargetDigest(expected)
	}, func(id int64) (int64, [32]byte, error) {
		if record.EntitlementID != nil {
			parent, err := writer.store.GetServicePeriodHistoryEntitlement(ctx, *record.EntitlementID)
			if err != nil {
				return 0, [32]byte{}, err
			}
			if parent.ID != *record.EntitlementID || parent.DefinitionID != record.DefinitionID {
				return 0, [32]byte{}, productport.ErrServicePeriodHistoryConflict
			}
		}
		var actual productport.ServicePeriodHistoryEvent
		var err error
		if id == 0 {
			actual, err = writer.store.CreateServicePeriodHistoryEvent(ctx, record)
		} else {
			actual, err = writer.store.GetServicePeriodHistoryEvent(ctx, id)
		}
		return actual.ID, ServicePeriodHistoryEventTargetDigest(actual), err
	})
}

// access creates when id is zero, otherwise it must read the actual target.
func (writer *ServicePeriodHistoryWriter) importHistory(ctx context.Context, kind, source string, payload [32]byte, expected func(int64) [32]byte, access func(int64) (int64, [32]byte, error)) (productport.ServicePeriodHistoryReceipt, error) {
	var empty productport.ServicePeriodHistoryReceipt
	if writer == nil || nilServicePeriodDependency(writer.store) || nilServicePeriodDependency(writer.journal) || ctx == nil {
		return empty, productport.ErrServicePeriodHistoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return empty, productport.ErrServicePeriodHistoryUnavailable
	}
	if source == "" || strings.TrimSpace(source) != source || !historyText(source) || payload == [32]byte{} {
		return empty, productport.ErrServicePeriodHistoryInvalid
	}
	receipt, found, err := writer.journal.LoadServicePeriodHistory(ctx, kind, source)
	if err != nil {
		return empty, historyWriteError(err)
	}
	if found {
		if receipt.SourceIdentifier != source || receipt.PayloadDigest != payload || receipt.TargetID < 1 || receipt.TargetDigest != expected(receipt.TargetID) {
			return empty, productport.ErrServicePeriodHistoryConflict
		}
		id, digest, readErr := access(receipt.TargetID)
		if readErr != nil {
			return empty, historyWriteError(readErr)
		}
		if id != receipt.TargetID || digest != receipt.TargetDigest {
			return empty, productport.ErrServicePeriodHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	id, digest, err := access(0)
	if err != nil {
		return empty, historyWriteError(err)
	}
	if id < 1 || digest != expected(id) {
		return empty, productport.ErrServicePeriodHistoryConflict
	}
	receipt = productport.ServicePeriodHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: id, TargetDigest: digest}
	if err = writer.journal.RecordServicePeriodHistory(ctx, kind, receipt); err != nil {
		return empty, historyWriteError(err)
	}
	return receipt, nil
}

// Target digests include every static port field, including the V2 target ID.
// UTC microseconds match PostgreSQL persistence, without changing source text.
func ServicePeriodHistoryDefinitionTargetDigest(record productport.ServicePeriodHistoryDefinition) [32]byte {
	return servicePeriodHistoryDigest("definitions", normalizeHistoryDefinition(record))
}

func ServicePeriodHistoryEntitlementTargetDigest(record productport.ServicePeriodHistoryEntitlement) [32]byte {
	return servicePeriodHistoryDigest("entitlements", normalizeHistoryEntitlement(record))
}

func ServicePeriodHistoryEventTargetDigest(record productport.ServicePeriodHistoryEvent) [32]byte {
	return servicePeriodHistoryDigest("events", normalizeHistoryEvent(record))
}

func servicePeriodHistoryDigest(kind string, record any) [32]byte {
	encoded, err := json.Marshal(record)
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(append([]byte("service_period_history\x00"+kind+"\x00"), encoded...))
}

func normalizeHistoryDefinition(record productport.ServicePeriodHistoryDefinition) productport.ServicePeriodHistoryDefinition {
	record.CreatedAt, record.UpdatedAt = historyMicro(record.CreatedAt), historyMicro(record.UpdatedAt)
	return record
}

func normalizeHistoryEntitlement(record productport.ServicePeriodHistoryEntitlement) productport.ServicePeriodHistoryEntitlement {
	record.CreatedAt, record.UpdatedAt = historyMicro(record.CreatedAt), historyMicro(record.UpdatedAt)
	record.StartAt, record.EndAt = historyMicro(record.StartAt), historyMicro(record.EndAt)
	return record
}

func normalizeHistoryEvent(record productport.ServicePeriodHistoryEvent) productport.ServicePeriodHistoryEvent {
	record.CreatedAt = historyMicro(record.CreatedAt)
	record.BeforeStartAt, record.BeforeEndAt = historyMicroPointer(record.BeforeStartAt), historyMicroPointer(record.BeforeEndAt)
	record.AfterStartAt, record.AfterEndAt = historyMicroPointer(record.AfterStartAt), historyMicroPointer(record.AfterEndAt)
	return record
}

func historyMicro(value time.Time) time.Time { return value.UTC().Truncate(time.Microsecond) }

func historyMicroPointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := historyMicro(*value)
	return &normalized
}

func historyTime(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999
}

func historyTimes(created, updated time.Time) bool {
	return historyTime(created) && historyTime(updated) && !updated.Before(created)
}

func historyOptionalTimes(values ...*time.Time) bool {
	for _, value := range values {
		if value != nil && !historyTime(*value) {
			return false
		}
	}
	return true
}

func historyOptionalID(value *int64) bool { return value == nil || *value > 0 }

func historyText(values ...string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func historyWriteError(err error) error {
	switch {
	case errors.Is(err, productport.ErrServicePeriodHistoryInvalid):
		return productport.ErrServicePeriodHistoryInvalid
	case errors.Is(err, productport.ErrServicePeriodHistoryConflict):
		return productport.ErrServicePeriodHistoryConflict
	default:
		return productport.ErrServicePeriodHistoryUnavailable
	}
}

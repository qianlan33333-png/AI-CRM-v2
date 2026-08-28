package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
)

var customerStateHistoryReconciledTables = []string{
	customerStateHistorySnapshotTable,
	customerStateHistoryChangeTable,
	customerStateHistoryTermTable,
}

func ReconcileCustomerStateHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != customerStateHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, customerStateHistoryReconciledTables)
}

func isCustomerStateHistorySource(table string) bool {
	for _, candidate := range customerStateHistoryReconciledTables {
		if table == candidate {
			return true
		}
	}
	return false
}

// verifyCustomerStateHistoryTarget only reads through the reconciliation
// transaction. It checks field digest separately because it is a private
// source binding, then uses the owner digest for every stored field.
func verifyCustomerStateHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, _ map[string]map[string]struct{}) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyCustomerStateHistoryRow(ctx, contactstore.NewCustomerStateHistoryReader(tx), row)
}

func verifyCustomerStateHistoryRow(ctx context.Context, reader contactport.CustomerStateHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isCustomerStateHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != customerStateHistoryDomain ||
		row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size ||
		len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var sourceKey, payload, field [sha256.Size]byte
	copy(sourceKey[:], row.SourceKeyDigest)
	copy(payload[:], row.PayloadDigest)
	copy(field[:], row.FieldDigest)

	var actualSourceKey, actualPayload, actualField, digest [sha256.Size]byte
	switch row.TableID {
	case customerStateHistorySnapshotTable:
		if *row.TargetTable != customerStateHistorySnapshotTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalCustomerStatusSnapshot(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = contactapp.HistoricalCustomerStatusSnapshotDigest(value)
	case customerStateHistoryChangeTable:
		if *row.TargetTable != customerStateHistoryChangeTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalCustomerStatusChange(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = contactapp.HistoricalCustomerStatusChangeDigest(value)
	case customerStateHistoryTermTable:
		if *row.TargetTable != customerStateHistoryTermTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalClassTermTagMapping(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = contactapp.HistoricalClassTermTagMappingDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualSourceKey != sourceKey || actualPayload != payload || actualField != field || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

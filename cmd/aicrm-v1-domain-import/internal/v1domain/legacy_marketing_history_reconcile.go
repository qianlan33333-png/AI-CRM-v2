package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

var legacyMarketingHistoryReconciledTables = []string{
	legacyMarketingStateTable,
	legacyMarketingValueTable,
}

func ReconcileLegacyMarketingHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != legacyMarketingHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, legacyMarketingHistoryReconciledTables)
}

func isLegacyMarketingHistorySource(table string) bool {
	for _, candidate := range legacyMarketingHistoryReconciledTables {
		if table == candidate {
			return true
		}
	}
	return false
}

// verifyLegacyMarketingHistoryTarget reads only through the reconciliation
// transaction. The owner digest covers every private source field.
func verifyLegacyMarketingHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, _ map[string]map[string]struct{}) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyLegacyMarketingHistoryRow(ctx, segmentstore.NewLegacyMarketingHistoryReader(tx), row)
}

func verifyLegacyMarketingHistoryRow(ctx context.Context, reader segmentport.LegacyMarketingHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isLegacyMarketingHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != legacyMarketingHistoryDomain ||
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
	case legacyMarketingStateTable:
		if *row.TargetTable != legacyMarketingStateTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalLegacyMarketingState(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalLegacyMarketingStateDigest(value)
	case legacyMarketingValueTable:
		if *row.TargetTable != legacyMarketingValueTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalLegacyMarketingValue(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalLegacyMarketingValueDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualSourceKey != sourceKey || actualPayload != payload || actualField != field || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

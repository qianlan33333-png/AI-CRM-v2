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

var marketingStateHistoryReconciledTables = []string{
	marketingStateSnapshotTable,
	marketingStateChangeTable,
	valueSegmentSnapshotTable,
	valueSegmentChangeTable,
}

func ReconcileMarketingStateHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != marketingStateHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, marketingStateHistoryReconciledTables)
}

func isMarketingStateHistorySource(table string) bool {
	for _, candidate := range marketingStateHistoryReconciledTables {
		if table == candidate {
			return true
		}
	}
	return false
}

// verifyMarketingStateHistoryTarget reads only through the reconciliation
// transaction. The owner digest covers every private source field.
func verifyMarketingStateHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, _ map[string]map[string]struct{}) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyMarketingStateHistoryRow(ctx, segmentstore.NewMarketingStateHistoryReader(tx), row)
}

func verifyMarketingStateHistoryRow(ctx context.Context, reader segmentport.MarketingStateHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isMarketingStateHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != marketingStateHistoryDomain ||
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
	case marketingStateSnapshotTable:
		if *row.TargetTable != marketingStateSnapshotTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalMarketingStateSnapshot(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalMarketingStateSnapshotDigest(value)
	case marketingStateChangeTable:
		if *row.TargetTable != marketingStateChangeTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalMarketingStateChange(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalMarketingStateChangeDigest(value)
	case valueSegmentSnapshotTable:
		if *row.TargetTable != valueSegmentSnapshotTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalValueSegmentSnapshot(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalValueSegmentSnapshotDigest(value)
	case valueSegmentChangeTable:
		if *row.TargetTable != valueSegmentChangeTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalValueSegmentChange(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = segmentapp.HistoricalValueSegmentChangeDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualSourceKey != sourceKey || actualPayload != payload || actualField != field || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

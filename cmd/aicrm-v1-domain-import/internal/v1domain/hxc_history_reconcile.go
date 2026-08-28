package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcapp "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/app"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
)

var hxcHistoryReconciledTables = []string{
	v1hxchistory.DashboardMetaTableID,
	v1hxchistory.DashboardSnapshotTableID,
	v1hxchistory.ActivationStatusTableID,
	v1hxchistory.HuangxiaocanActivationID,
	v1hxchistory.ExperienceLeadsTableID,
	v1hxchistory.ImportBatchesTableID,
	v1hxchistory.SendRecordsTableID,
	v1hxchistory.SendConfigTableID,
}

// ReconcileHXCHistory seals the six immutable observations and two
// archive-only legacy sources. Generic target dispatch is deliberately kept in
// the main reconciliation owner.
func ReconcileHXCHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != hxcHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, hxcHistoryReconciledTables)
}

func isHXCHistorySource(tableID string) bool {
	for _, table := range hxcHistoryReconciledTables {
		if tableID == table {
			return true
		}
	}
	return false
}

// verifyHXCHistoryRow checks only the immutable HXC history tables. The two
// runtime archive sources intentionally have no target and are reconciled as
// archive terminals by the generic owner.
func verifyHXCHistoryRow(ctx context.Context, reader hxcport.HXCHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || row.TargetDomain == nil || row.TargetTable == nil || row.TargetID == nil ||
		*row.TargetDomain != hxcHistoryDomain || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var sourceKey, payload [sha256.Size]byte
	copy(sourceKey[:], row.SourceKeyDigest)
	copy(payload[:], row.PayloadDigest)

	var actualKey, actualPayload [sha256.Size]byte
	var digest [sha256.Size]byte
	switch row.TableID {
	case v1hxchistory.DashboardMetaTableID:
		if *row.TargetTable != hxcHistoryMetaTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalHXCMeta(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = hxcapp.HistoricalHXCMetaDigest(value)
	case v1hxchistory.DashboardSnapshotTableID:
		if *row.TargetTable != hxcHistorySnapshotTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalHXCSnapshot(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = hxcapp.HistoricalHXCSnapshotDigest(value)
	case v1hxchistory.ActivationStatusTableID, v1hxchistory.HuangxiaocanActivationID:
		if *row.TargetTable != hxcHistoryActivationTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalHXCActivation(ctx, id)
		if readErr != nil || value.ID != id || value.SourceTable != row.TableID {
			return "", ErrConflict
		}
		actualKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = hxcapp.HistoricalHXCActivationDigest(value)
	case v1hxchistory.ExperienceLeadsTableID:
		if *row.TargetTable != hxcHistoryLeadTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalHXCLead(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = hxcapp.HistoricalHXCLeadDigest(value)
	case v1hxchistory.ImportBatchesTableID:
		if *row.TargetTable != hxcHistoryBatchTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalHXCBatch(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = hxcapp.HistoricalHXCBatchDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualKey != sourceKey || actualPayload != payload || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

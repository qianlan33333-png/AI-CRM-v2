package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	outboundstore "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store"
)

func ReconcileBroadcastJobHistory(ctx context.Context, pool *pgxpool.Pool, version, run string) (ReconciliationResult, error) {
	if version != broadcastJobHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, run, []string{broadcastJobHistoryTableID})
}

func verifyBroadcastJobHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyBroadcastJobHistoryRow(ctx, outboundstore.NewBroadcastJobHistoryReader(tx), row)
}

func verifyBroadcastJobHistoryRow(ctx context.Context, reader outboundport.BroadcastJobHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || row.TableID != broadcastJobHistoryTableID || row.TargetDomain == nil || *row.TargetDomain != "outbound" ||
		row.TargetTable == nil || *row.TargetTable != broadcastJobHistoryTargetTable || row.TargetID == nil ||
		len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	fact, err := reader.GetHistoricalBroadcastJob(ctx, id)
	if err != nil || fact.ID != id || !equalBytes(fact.SourceKeyDigest[:], row.SourceKeyDigest) ||
		!equalBytes(fact.SourcePayloadDigest[:], row.PayloadDigest) || !equalBytes(fact.SourceFieldDigest[:], row.FieldDigest) {
		return "", ErrConflict
	}
	digest, err := outboundapp.HistoricalBroadcastJobDigest(fact)
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

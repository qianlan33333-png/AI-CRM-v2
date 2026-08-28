package v1domain

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	wecomapp "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/app"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
	wecomstore "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/store"
)

func ReconcileMessageHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != messageHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, []string{messageHistoryTableID})
}

func verifyMessageHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	return verifyMessageHistoryRow(ctx, wecomstore.NewMessageHistoryReader(tx), row)
}

func verifyMessageHistoryRow(ctx context.Context, reader wecomport.MessageHistoryReader, row reconciliationRow) (string, error) {
	if reader == nil || row.TableID != messageHistoryTableID || row.TargetDomain == nil || *row.TargetDomain != "wecom" || row.TargetTable == nil || *row.TargetTable != messageHistoryTargetTable || row.TargetID == nil || len(row.PayloadDigest) != 32 || len(row.TargetDigest) != 32 {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	actual, err := reader.GetHistoricalMessage(ctx, id)
	if err != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
		return "", ErrConflict
	}
	digest, err := wecomapp.HistoricalMessageDigest(actual)
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

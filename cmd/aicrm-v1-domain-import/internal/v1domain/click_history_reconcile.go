package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/app"
	port "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
	store "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/store"
	"strconv"
)

var radarClickHistoryReconciledTables = []string{"public/radar_click_events"}

func ReconcileRadarClickHistory(ctx context.Context, pool *pgxpool.Pool, version, runID string) (ReconciliationResult, error) {
	if version != "v1-radar-click-history-a1" {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, runID, radarClickHistoryReconciledTables)
}
func isRadarClickHistorySource(table string) bool {
	for _, v := range radarClickHistoryReconciledTables {
		if table == v {
			return true
		}
	}
	return false
}
func verifyRadarClickHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyRadarClickHistoryRow(ctx, store.NewRadarClickHistoryReader(tx), row)
}
func verifyRadarClickHistoryRow(ctx context.Context, reader port.RadarClickHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isRadarClickHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != "radar" || row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var key, payload, field [32]byte
	copy(key[:], row.SourceKeyDigest)
	copy(payload[:], row.PayloadDigest)
	copy(field[:], row.FieldDigest)
	var actualKey, actualPayload, actualField, digest [32]byte
	switch row.TableID {
	case "public/radar_click_events":
		if *row.TargetTable != "radar_v1_click_history" {
			return "", ErrConflict
		}
		v, e := reader.GetHistoricalRadarClick(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload, actualField = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = app.HistoricalRadarClickDigest(v)
	default:
		return "", ErrConflict
	}
	if err != nil || key != actualKey || payload != actualPayload || field != actualField || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"strconv"
)

var wecomContactHistoryReconciledTables = []string{"public/wecom_external_contact_event_logs", "public/wecom_external_contact_follow_users"}

func ReconcileWeComContactHistory(ctx context.Context, pool *pgxpool.Pool, version, runID string) (ReconciliationResult, error) {
	if version != "v1-wecom-contact-history-a1" {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, runID, wecomContactHistoryReconciledTables)
}
func isWeComContactHistorySource(table string) bool {
	for _, source := range wecomContactHistoryReconciledTables {
		if source == table {
			return true
		}
	}
	return false
}
func verifyWeComContactHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyWeComContactHistoryRow(ctx, contactstore.NewWeComContactHistoryReader(tx), row)
}
func verifyWeComContactHistoryRow(ctx context.Context, reader contactport.WeComContactHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isWeComContactHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != "contact" || row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
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
	case "public/wecom_external_contact_event_logs":
		if *row.TargetTable != "contact_v1_wecom_event_log_history" {
			return "", ErrConflict
		}
		v, e := reader.GetHistoricalWeComExternalContactEventLog(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload, actualField = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = contactapp.HistoricalWeComExternalContactEventLogDigest(v)
	case "public/wecom_external_contact_follow_users":
		if *row.TargetTable != "contact_v1_wecom_follow_user_history" {
			return "", ErrConflict
		}
		v, e := reader.GetHistoricalWeComExternalContactFollowUser(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload, actualField = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = contactapp.HistoricalWeComExternalContactFollowUserDigest(v)
	default:
		return "", ErrConflict
	}
	if err != nil || key != actualKey || payload != actualPayload || field != actualField || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	port "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	store "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/store"
	"strconv"
)

var marketingConfigHistoryReconciledTables = []string{"public/marketing_automation_configs", "public/marketing_automation_question_rules"}

func ReconcileMarketingConfigHistory(ctx context.Context, pool *pgxpool.Pool, version, runID string) (ReconciliationResult, error) {
	if version != "v1-marketing-config-history-a1" {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, runID, marketingConfigHistoryReconciledTables)
}
func isMarketingConfigHistorySource(table string) bool {
	for _, v := range marketingConfigHistoryReconciledTables {
		if table == v {
			return true
		}
	}
	return false
}
func verifyMarketingConfigHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	if tx == nil {
		return "", ErrConflict
	}
	return verifyMarketingConfigHistoryRow(ctx, store.NewMarketingConfigHistoryReader(tx), row)
}
func verifyMarketingConfigHistoryRow(ctx context.Context, reader port.MarketingConfigHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || !isMarketingConfigHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != "automation" || row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
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
	case "public/marketing_automation_configs":
		if *row.TargetTable != "automation_v1_marketing_config_history" {
			return "", ErrConflict
		}
		v, e := reader.GetHistoricalMarketingAutomationConfig(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload, actualField = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = app.HistoricalMarketingAutomationConfigDigest(v)
	case "public/marketing_automation_question_rules":
		if *row.TargetTable != "automation_v1_marketing_rule_history" {
			return "", ErrConflict
		}
		v, e := reader.GetHistoricalMarketingAutomationRule(ctx, id)
		if e != nil || v.ID != id {
			return "", ErrConflict
		}
		actualKey, actualPayload, actualField = v.SourceKeyDigest, v.SourcePayloadDigest, v.SourceFieldDigest
		digest, err = app.HistoricalMarketingAutomationRuleDigest(v)
	default:
		return "", ErrConflict
	}
	if err != nil || key != actualKey || payload != actualPayload || field != actualField || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

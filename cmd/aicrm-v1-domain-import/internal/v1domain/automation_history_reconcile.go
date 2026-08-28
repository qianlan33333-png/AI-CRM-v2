package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	automationapp "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/app"
	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
)

var automationHistoryReconciledTables = []string{
	automationHistorySOPTable,
	automationHistoryConfigTable,
	automationHistoryPromptTable,
	automationHistoryAgentTable,
}

// ReconcileAutomationHistory seals exactly the four read-only Automation
// history projections. Current automation, rule, queue, and Provider tables
// are deliberately outside this reconciliation set.
func ReconcileAutomationHistory(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	if importVersion != automationHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, importVersion, archiveRunID, automationHistoryReconciledTables)
}

// verifyAutomationHistoryRow is intentionally reader-based. Main-line
// composition supplies the owner store reader bound to the reconciliation
// transaction; this package never opens a second transaction.
func verifyAutomationHistoryRow(ctx context.Context, reader automationport.AutomationHistoryReader, row reconciliationRow) (string, error) {
	if ctx == nil || reader == nil || row.TargetDomain == nil || row.TargetTable == nil || row.TargetID == nil ||
		*row.TargetDomain != automationHistoryDomain || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var sourceKey, payload [sha256.Size]byte
	copy(sourceKey[:], row.SourceKeyDigest)
	copy(payload[:], row.PayloadDigest)

	var actualSourceKey, actualPayload [sha256.Size]byte
	var digest [sha256.Size]byte
	switch row.TableID {
	case automationHistorySOPTable:
		if *row.TargetTable != automationHistorySOPTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalAutomationSOP(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = automationapp.HistoricalAutomationSOPDigest(value)
	case automationHistoryConfigTable:
		if *row.TargetTable != automationHistoryConfigTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalAutomationConfig(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = automationapp.HistoricalAutomationConfigDigest(value)
	case automationHistoryPromptTable:
		if *row.TargetTable != automationHistoryPromptTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalAutomationPrompt(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = automationapp.HistoricalAutomationPromptDigest(value)
	case automationHistoryAgentTable:
		if *row.TargetTable != automationHistoryAgentTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalAutomationAgent(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload = value.SourceKeyDigest, value.SourcePayloadDigest
		digest, err = automationapp.HistoricalAutomationAgentDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualSourceKey != sourceKey || actualPayload != payload || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

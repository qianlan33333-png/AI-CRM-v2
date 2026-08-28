package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var campaignDefinitionHistoryReconciledTables = []string{
	campaignDefinitionHistoryDefinitionTable,
	campaignDefinitionHistoryStepTable,
}

func ReconcileCampaignDefinitionHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string, sourceHMACKey []byte) (ReconciliationResult, error) {
	if version != campaignDefinitionHistoryImportVersion || len(sourceHMACKey) < sha256.Size {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTablesWithCampaignDefinitionKey(ctx, pool, version, archiveRunID, campaignDefinitionHistoryReconciledTables, sourceHMACKey)
}

func isCampaignDefinitionHistorySource(table string) bool {
	for _, candidate := range campaignDefinitionHistoryReconciledTables {
		if table == candidate {
			return true
		}
	}
	return false
}

// verifyCampaignDefinitionHistoryTarget is called only by the root-owned
// selected-row reconciliation path. It reads through the serializable caller
// transaction and never considers a historical definition executable.
func verifyCampaignDefinitionHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, archiveRunID string, sourceHMACKey []byte) (string, error) {
	if tx == nil || archiveRunID == "" || len(sourceHMACKey) < sha256.Size {
		return "", ErrConflict
	}
	reader := campaignstore.NewCampaignDefinitionHistoryReader(tx)
	resolver := NewCampaignDefinitionCurrentResolver(archiveRunID, reader).WithTx(tx)
	return verifyCampaignDefinitionHistoryRow(ctx, reader, resolver, row, sourceHMACKey)
}

type campaignDefinitionHistoryTargetReader interface {
	GetHistoricalCampaignDefinition(context.Context, int64) (campaignport.HistoricalCampaignDefinition, error)
	GetHistoricalCampaignDefinitionStep(context.Context, int64) (campaignport.HistoricalCampaignDefinitionStep, error)
}

func verifyCampaignDefinitionHistoryRow(ctx context.Context, reader campaignDefinitionHistoryTargetReader, parent CampaignDefinitionCurrentParentResolver, row reconciliationRow, sourceHMACKey []byte) (string, error) {
	if ctx == nil || reader == nil || !isCampaignDefinitionHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != campaignDefinitionHistoryDomain ||
		row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size ||
		len(row.FieldDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size || len(sourceHMACKey) < sha256.Size {
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
	case campaignDefinitionHistoryDefinitionTable:
		if *row.TargetTable != campaignDefinitionHistoryDefinitionTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalCampaignDefinition(ctx, id)
		if readErr != nil || value.ID != id {
			return "", ErrConflict
		}
		if value.OriginalDisposition != row.PriorDisposition || value.OriginalReason != row.PriorReason {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = campaignapp.HistoricalCampaignDefinitionDigest(value)
	case campaignDefinitionHistoryStepTable:
		if *row.TargetTable != campaignDefinitionHistoryStepTarget {
			return "", ErrConflict
		}
		value, readErr := reader.GetHistoricalCampaignDefinitionStep(ctx, id)
		if readErr != nil || value.ID != id || !validCampaignDefinitionHistoryParent(ctx, reader, parent, value, sourceHMACKey) {
			return "", ErrConflict
		}
		if value.OriginalDisposition != row.PriorDisposition || value.OriginalReason != row.PriorReason {
			return "", ErrConflict
		}
		actualSourceKey, actualPayload, actualField = value.SourceKeyDigest, value.SourcePayloadDigest, value.SourceFieldDigest
		digest, err = campaignapp.HistoricalCampaignDefinitionStepDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || actualSourceKey != sourceKey || actualPayload != payload || actualField != field || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

func validCampaignDefinitionHistoryParent(ctx context.Context, reader campaignDefinitionHistoryTargetReader, parent CampaignDefinitionCurrentParentResolver, value campaignport.HistoricalCampaignDefinitionStep, sourceHMACKey []byte) bool {
	switch value.SourceParentState {
	case "history_definition":
		if value.HistoryDefinitionID == nil || *value.HistoryDefinitionID < 1 || value.CurrentCampaignCode != nil {
			return false
		}
		parent, err := reader.GetHistoricalCampaignDefinition(ctx, *value.HistoryDefinitionID)
		return err == nil && parent.ID == *value.HistoryDefinitionID && parent.SourceID == value.CampaignSourceID
	case "current_definition":
		if value.HistoryDefinitionID != nil || value.CurrentCampaignCode == nil || *value.CurrentCampaignCode == "" || parent == nil {
			return false
		}
		sourceKey, err := v1archive.SourceKeyHMAC(sourceHMACKey, strings.TrimPrefix(campaignDefinitionHistoryDefinitionTable, "public/"), []byte("["+strconv.FormatInt(value.CampaignSourceID, 10)+"]"))
		if err != nil {
			return false
		}
		code, found, err := parent.ResolveVerifiedCurrentCampaignDefinition(ctx, value.CampaignSourceID, sourceKey)
		return err == nil && found && code == *value.CurrentCampaignCode
	case "unresolved_definition":
		return value.HistoryDefinitionID == nil && value.CurrentCampaignCode == nil
	default:
		return false
	}
}

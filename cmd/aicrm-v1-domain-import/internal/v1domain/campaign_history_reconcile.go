package v1domain

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	campaignapp "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/app"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	campaignstore "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/store"
)

var campaignHistoryReconciledTables = []string{
	campaignHistorySegmentsTable,
	campaignHistoryMembersTable,
	campaignHistoryPlansTable,
	campaignHistoryRecipientsTable,
	campaignHistoryMessagesTable,
}

func ReconcileCampaignHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != campaignHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, campaignHistoryReconciledTables)
}

func isCampaignHistorySource(tableID string) bool {
	for _, source := range campaignHistoryReconciledTables {
		if tableID == source {
			return true
		}
	}
	return false
}

func verifyCampaignHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	return verifyCampaignHistoryRow(ctx, campaignstore.NewCampaignHistoryReader(tx), row)
}

func verifyCampaignHistoryRow(ctx context.Context, reader campaignport.CampaignHistoryReader, row reconciliationRow) (string, error) {
	if reader == nil || len(row.PayloadDigest) != 32 || len(row.TargetDigest) != 32 || row.TargetDomain == nil || *row.TargetDomain != campaignHistoryTargetDomain || row.TargetTable == nil || row.TargetID == nil {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var digest [32]byte
	switch row.TableID {
	case campaignHistorySegmentsTable:
		if *row.TargetTable != "campaign_v1_history_segments" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalCampaignSegment(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = campaignapp.HistoricalCampaignSegmentDigest(actual)
	case campaignHistoryMembersTable:
		if *row.TargetTable != "campaign_v1_history_members" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalCampaignMember(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = campaignapp.HistoricalCampaignMemberDigest(actual)
	case campaignHistoryPlansTable:
		if *row.TargetTable != "campaign_v1_history_broadcast_plans" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalBroadcastPlan(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = campaignapp.HistoricalBroadcastPlanDigest(actual)
	case campaignHistoryRecipientsTable:
		if *row.TargetTable != "campaign_v1_history_broadcast_recipients" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalBroadcastRecipient(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = campaignapp.HistoricalBroadcastRecipientDigest(actual)
	case campaignHistoryMessagesTable:
		if *row.TargetTable != "campaign_v1_history_broadcast_messages" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalBroadcastMessage(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = campaignapp.HistoricalBroadcastMessageDigest(actual)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	productapp "github.com/qianlan33333-png/AI-CRM-v2/internal/product/app"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
	productstore "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store"
)

var memberGridHistoryReconciledTables = []string{
	v1membergridhistory.MemberViewsTableID,
	v1membergridhistory.UsageSnapshotsTableID,
	v1membergridhistory.UsageSyncRunsTableID,
	v1membergridhistory.MemberCollaboratorsTableID,
	v1membergridhistory.MemberSharesTableID,
}

// ReconcileMemberGridHistory seals only Product-owned historical facts and
// archive-only context. It does not attest to a current view, entitlement,
// staff role, or share.
func ReconcileMemberGridHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != memberGridHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, memberGridHistoryReconciledTables)
}

func isMemberGridHistorySource(tableID string) bool {
	for _, candidate := range memberGridHistoryReconciledTables {
		if tableID == candidate {
			return true
		}
	}
	return false
}

// verifyMemberGridHistoryTarget checks the actual typed, historical target.
// The three context tables have no target and therefore cannot reach here.
func verifyMemberGridHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, _ map[string]map[string]struct{}) (string, error) {
	return verifyMemberGridHistoryRow(ctx, productstore.NewMemberGridHistoryReader(tx), row)
}

func verifyMemberGridHistoryRow(ctx context.Context, reader productport.MemberGridHistoryReader, row reconciliationRow) (string, error) {
	if reader == nil || row.TargetDomain == nil || *row.TargetDomain != "product" || row.TargetTable == nil || row.TargetID == nil ||
		len(row.SourceKeyDigest) != sha256.Size || len(row.PayloadDigest) != sha256.Size || len(row.TargetDigest) != sha256.Size {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var digest [sha256.Size]byte
	switch row.TableID {
	case v1membergridhistory.MemberViewsTableID:
		if *row.TargetTable != memberGridHistoryViewTargetTable {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalMemberView(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = productapp.HistoricalMemberViewDigest(actual)
	case v1membergridhistory.UsageSnapshotsTableID:
		if *row.TargetTable != memberGridHistoryUsageTargetTable {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalMemberUsage(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = productapp.HistoricalMemberUsageDigest(actual)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

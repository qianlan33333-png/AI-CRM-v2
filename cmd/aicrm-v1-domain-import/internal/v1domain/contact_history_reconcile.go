package v1domain

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
)

var contactHistoryReconciledTables = []string{
	"public/sidebar_customer_profile_fields", "public/owner_migration_results",
	"public/owner_migration_import_sessions", "public/owner_migration_previews",
}

func ReconcileContactHistory(ctx context.Context, pool *pgxpool.Pool, version, archiveRunID string) (ReconciliationResult, error) {
	if version != contactHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, archiveRunID, contactHistoryReconciledTables)
}

func isContactHistorySource(table string) bool {
	for _, source := range contactHistoryReconciledTables {
		if table == source {
			return true
		}
	}
	return false
}

func verifyContactHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow) (string, error) {
	return verifyContactHistoryRow(ctx, contactstore.NewContactHistoryReader(tx), row)
}

func verifyContactHistoryRow(ctx context.Context, reader contactport.ContactHistoryReader, row reconciliationRow) (string, error) {
	if reader == nil || row.TargetDomain == nil || *row.TargetDomain != "contact" || row.TargetTable == nil || row.TargetID == nil || len(row.SourceKeyDigest) != 32 || len(row.PayloadDigest) != 32 || len(row.TargetDigest) != 32 {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var digest [32]byte
	switch row.TableID {
	case "public/sidebar_customer_profile_fields":
		if *row.TargetTable != "contact_v1_sidebar_profile_history" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalSidebarProfile(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = contactapp.HistoricalSidebarProfileDigest(actual)
	case "public/owner_migration_results":
		if *row.TargetTable != "contact_v1_owner_migration_result_history" {
			return "", ErrConflict
		}
		actual, readErr := reader.GetHistoricalOwnerMigrationResult(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = contactapp.HistoricalOwnerMigrationResultDigest(actual)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

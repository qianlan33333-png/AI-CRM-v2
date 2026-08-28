package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
)

const AudienceHistoryImportVersion = "v1-audience-history-a1"

func ReconcileAudienceHistory(ctx context.Context, pool *pgxpool.Pool, version, run string) (ReconciliationResult, error) {
	if version != AudienceHistoryImportVersion {
		return ReconciliationResult{}, ErrInvalidScope
	}
	tables := make([]string, 0, len(audienceHistoryScopes))
	for _, scope := range audienceHistoryScopes {
		tables = append(tables, scope.source)
	}
	return reconcileTables(ctx, pool, version, run, tables)
}

func isAudienceHistorySource(table string) bool {
	for _, scope := range audienceHistoryScopes {
		if scope.source == table {
			return true
		}
	}
	return false
}

func verifyAudienceHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	if ctx == nil || tx == nil || row.TargetDomain == nil || *row.TargetDomain != "segment" ||
		row.TargetTable == nil || row.TargetID == nil || len(row.TargetDigest) != sha256.Size ||
		row.Reason != "" || len(row.Metadata) != 0 && string(row.Metadata) != "{}" {
		return "", ErrConflict
	}
	expected, ok := targetBySourceTable[row.TableID]
	if !ok || !isAudienceHistorySource(row.TableID) || expected.table != *row.TargetTable {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	reader := segmentstore.NewAudienceHistoryReader(tx)
	var digest [32]byte
	switch *row.TargetTable {
	case "segment_v1_audience_groups":
		value, readErr := reader.GetHistoricalAudienceGroup(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		digest, err = segmentapp.HistoricalAudienceGroupDigest(value)
	case "segment_v1_audience_packages":
		value, readErr := reader.GetHistoricalAudiencePackage(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		if value.GroupHistoryID != nil && !containsTarget(targets, "segment_v1_audience_groups", strconv.FormatInt(*value.GroupHistoryID, 10)) {
			return "", ErrConflict
		}
		if err = verifyAudienceCurrentVersion(ctx, reader, value, targets); err != nil {
			return "", err
		}
		digest, err = segmentapp.HistoricalAudiencePackageDigest(value)
	case "segment_v1_audience_versions":
		value, readErr := reader.GetHistoricalAudienceVersion(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		if !containsTarget(targets, "segment_v1_audience_packages", strconv.FormatInt(value.PackageHistoryID, 10)) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalAudienceVersionDigest(value)
	case "segment_v1_audience_senders":
		value, readErr := reader.GetHistoricalAudienceSender(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		if !containsTarget(targets, "segment_v1_audience_packages", strconv.FormatInt(value.PackageHistoryID, 10)) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalAudienceSenderDigest(value)
	case "segment_v1_audience_rules":
		value, readErr := reader.GetHistoricalAudienceRule(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		digest, err = segmentapp.HistoricalAudienceRuleDigest(value)
	case "segment_v1_audience_rule_versions":
		value, readErr := reader.GetHistoricalAudienceRuleVersion(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		if !containsTarget(targets, "segment_v1_audience_rules", strconv.FormatInt(value.RuleHistoryID, 10)) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalAudienceRuleVersionDigest(value)
	case "segment_v1_definitions":
		value, readErr := reader.GetHistoricalAudienceDefinition(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		digest, err = segmentapp.HistoricalAudienceDefinitionDigest(value)
	case "segment_v1_audience_members":
		value, readErr := reader.GetHistoricalAudienceMember(ctx, id)
		if readErr != nil {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, readErr)
		}
		if !containsTarget(targets, "segment_v1_audience_packages", strconv.FormatInt(value.PackageHistoryID, 10)) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalAudienceMemberDigest(value)
	default:
		return "", ErrConflict
	}
	if err != nil || !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return *row.TargetTable + ":" + *row.TargetID + ":history_only:" + hex.EncodeToString(digest[:]), nil
}

func verifyAudienceCurrentVersion(ctx context.Context, reader segmentport.AudienceHistoryReader, value segmentport.HistoricalAudiencePackage, targets map[string]map[string]struct{}) error {
	if value.CurrentVersionSourceID == nil {
		return nil
	}
	for offset := int32(0); ; offset += 100 {
		versions, total, err := reader.ListHistoricalAudienceVersions(ctx, value.ID, 100, offset)
		if err != nil {
			return err
		}
		for _, version := range versions {
			if version.SourceID == *value.CurrentVersionSourceID {
				if version.PackageHistoryID != value.ID || !containsTarget(targets, "segment_v1_audience_versions", strconv.FormatInt(version.ID, 10)) {
					return ErrConflict
				}
				return nil
			}
		}
		if len(versions) == 0 || int64(offset)+int64(len(versions)) >= total || offset > 2147483547 {
			return ErrConflict
		}
	}
}

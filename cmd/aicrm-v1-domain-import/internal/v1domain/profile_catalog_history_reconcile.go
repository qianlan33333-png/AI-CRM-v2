package v1domain

import (
	"context"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1profilecatalog"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	segmentapp "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/app"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
	segmentstore "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/store"
	"strconv"
)

func isProfileCatalogHistorySource(table string) bool {
	switch table {
	case v1profilecatalog.ProfileTemplatesTableID, v1profilecatalog.ProfileCategoriesTableID, v1profilecatalog.ProfileOptionMappingsTableID, v1profilecatalog.SignupTagRulesTableID:
		return true
	}
	return false
}
func ReconcileProfileCatalogHistory(ctx context.Context, pool *pgxpool.Pool, version, run string) (ReconciliationResult, error) {
	if version != "v1-profile-catalog-history-a1" {
		return ReconciliationResult{}, ErrInvalidScope
	}
	return reconcileTables(ctx, pool, version, run, []string{v1profilecatalog.ProfileTemplatesTableID, v1profilecatalog.ProfileCategoriesTableID, v1profilecatalog.ProfileOptionMappingsTableID, v1profilecatalog.SignupTagRulesTableID})
}
func verifyProfileCatalogHistoryTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	return verifyProfileCatalogHistoryRow(ctx, segmentstore.NewProfileCatalogHistoryReader(tx), contactstore.NewSignupTagHistoryReader(tx), row, targets)
}
func verifyProfileCatalogHistoryRow(ctx context.Context, profiles segmentport.ProfileCatalogHistoryReader, tags contactport.SignupTagHistoryReader, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	expected, ok := targetBySourceTable[row.TableID]
	if !ok || !isProfileCatalogHistorySource(row.TableID) || row.TargetDomain == nil || *row.TargetDomain != expected.domain || row.TargetTable == nil || *row.TargetTable != expected.table || row.TargetID == nil || len(row.PayloadDigest) != 32 || len(row.SourceKeyDigest) != 32 || len(row.TargetDigest) != 32 {
		return "", ErrConflict
	}
	id, err := positiveID(*row.TargetID)
	if err != nil || strconv.FormatInt(id, 10) != *row.TargetID {
		return "", ErrConflict
	}
	var digest [32]byte
	switch row.TableID {
	case v1profilecatalog.ProfileTemplatesTableID:
		if profiles == nil {
			return "", ErrConflict
		}
		actual, readErr := profiles.GetHistoricalProfileTemplate(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalProfileTemplateDigest(actual)
		if err != nil {
			return "", ErrConflict
		}
	case v1profilecatalog.ProfileCategoriesTableID:
		if profiles == nil {
			return "", ErrConflict
		}
		actual, readErr := profiles.GetHistoricalProfileCategory(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalProfileCategoryDigest(actual)
		if err != nil {
			return "", ErrConflict
		}
		if _, found := targets[v1profilecatalog.ProfileTemplatesTargetTable][strconv.FormatInt(actual.TemplateHistoryID, 10)]; !found {
			return "", ErrConflict
		}
		template, readErr := profiles.GetHistoricalProfileTemplate(ctx, actual.TemplateHistoryID)
		if readErr != nil || template.ID != actual.TemplateHistoryID || template.SourceID != actual.TemplateSourceID {
			return "", ErrConflict
		}
	case v1profilecatalog.ProfileOptionMappingsTableID:
		if profiles == nil {
			return "", ErrConflict
		}
		actual, readErr := profiles.GetHistoricalProfileOptionMapping(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = segmentapp.HistoricalProfileOptionMappingDigest(actual)
		if err != nil {
			return "", ErrConflict
		}
		if _, found := targets[v1profilecatalog.ProfileTemplatesTargetTable][strconv.FormatInt(actual.TemplateHistoryID, 10)]; !found {
			return "", ErrConflict
		}
		template, readErr := profiles.GetHistoricalProfileTemplate(ctx, actual.TemplateHistoryID)
		if readErr != nil || template.ID != actual.TemplateHistoryID || template.SourceID != actual.TemplateSourceID {
			return "", ErrConflict
		}
		if _, found := targets[v1profilecatalog.ProfileCategoriesTargetTable][strconv.FormatInt(actual.CategoryHistoryID, 10)]; !found {
			return "", ErrConflict
		}
		category, readErr := profiles.GetHistoricalProfileCategory(ctx, actual.CategoryHistoryID)
		if readErr != nil || category.ID != actual.CategoryHistoryID || category.SourceID != actual.CategorySourceID || category.TemplateHistoryID != actual.TemplateHistoryID || category.TemplateSourceID != actual.TemplateSourceID {
			return "", ErrConflict
		}
	case v1profilecatalog.SignupTagRulesTableID:
		if tags == nil {
			return "", ErrConflict
		}
		actual, readErr := tags.GetHistoricalSignupTagRule(ctx, id)
		if readErr != nil || actual.ID != id || !equalBytes(actual.SourceKeyDigest[:], row.SourceKeyDigest) || !equalBytes(actual.SourcePayloadDigest[:], row.PayloadDigest) {
			return "", ErrConflict
		}
		digest, err = contactapp.HistoricalSignupTagRuleDigest(actual)
		if err != nil {
			return "", ErrConflict
		}
	default:
		return "", ErrConflict
	}
	if !equalBytes(digest[:], row.TargetDigest) {
		return "", ErrConflict
	}
	return "history_only:" + hex.EncodeToString(digest[:]), nil
}

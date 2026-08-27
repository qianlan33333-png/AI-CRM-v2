package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var reconciledTables = []string{
	"public/campaigns",
	"public/campaign_steps",
	"public/questionnaires",
	"public/questionnaire_questions",
	"public/questionnaire_options",
	"public/questionnaire_submissions",
	"public/questionnaire_submission_answers",
	"public/miniprogram_library",
	"public/radar_links",
	"public/wechat_shop_orders",
}

var targetBySourceTable = map[string]struct {
	domain string
	table  string
}{
	"public/campaigns":                        {"campaign", "cloud_campaigns"},
	"public/campaign_steps":                   {"campaign", "cloud_campaign_steps"},
	"public/questionnaires":                   {"survey", "questionnaires"},
	"public/questionnaire_questions":          {"survey", "questionnaire_questions"},
	"public/questionnaire_options":            {"survey", "questionnaire_options"},
	"public/questionnaire_submissions":        {"survey", "questionnaire_submissions"},
	"public/questionnaire_submission_answers": {"survey", "questionnaire_submission_answers"},
	"public/miniprogram_library":              {"media", "media_miniprograms"},
	"public/radar_links":                      {"radar", "radar_links"},
	"public/wechat_shop_orders":               {"order", "order_wechat_shop_materials"},
}

type ReconciliationResult struct {
	SelectedSourceCount int64  `json:"selected_source_count"`
	ReceiptCount        int64  `json:"receipt_count"`
	ImportedCount       int64  `json:"imported_count"`
	ArchivedCount       int64  `json:"archived_count"`
	QuarantinedCount    int64  `json:"quarantined_count"`
	VerifiedCount       int64  `json:"verified_count"`
	ComparisonDigest    string `json:"comparison_digest"`
	Replayed            bool   `json:"replayed"`
}

type reconciliationRow struct {
	TableID         string
	SourceKeyDigest []byte
	PayloadDigest   []byte
	Disposition     string
	Reason          string
	TargetDomain    *string
	TargetTable     *string
	TargetID        *string
	TargetDigest    []byte
	Metadata        []byte
	Verified        bool
}

// ReconcileAll seals one complete import only after every selected archive row
// has a verified terminal receipt and each imported target still exists in its
// deliberately non-executing V2 state.
func ReconcileAll(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	if pool == nil || !validVersion(importVersion) || !validToken(archiveRunID, 128) {
		return ReconciliationResult{}, ErrInvalidScope
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return ReconciliationResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `LOCK TABLE public.v1_domain_import_receipts IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return ReconciliationResult{}, err
	}
	var phase, adapterID string
	if err = tx.QueryRow(ctx, `SELECT migration.phase,migration.adapter_id
FROM public.data_migration_runs migration
JOIN public.v1_archive_runs archive USING (run_id)
WHERE migration.run_id=$1`, archiveRunID).Scan(&phase, &adapterID); err != nil || phase != "reconciled" || adapterID != v1archive.DefaultAdapterID {
		if err == nil {
			err = fmt.Errorf("archive run is not a reconciled %s run", v1archive.DefaultAdapterID)
		}
		return ReconciliationResult{}, err
	}

	var result ReconciliationResult
	for _, tableID := range reconciledTables {
		var selectedCount, sourceCount, receiptCount int64
		if err = tx.QueryRow(ctx, `SELECT row_count FROM public.v1_archive_tables WHERE run_id=$1 AND table_id=$2`, archiveRunID, tableID).Scan(&selectedCount); err != nil {
			return ReconciliationResult{}, fmt.Errorf("required archive table %s is missing: %w", tableID, err)
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.v1_archive_records WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3`, archiveRunID, v1archive.DefaultAdapterID, tableID).Scan(&sourceCount); err != nil {
			return ReconciliationResult{}, err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.v1_domain_import_receipts WHERE import_version=$1 AND archive_run_id=$2 AND adapter_id=$3 AND table_id=$4`, importVersion, archiveRunID, v1archive.DefaultAdapterID, tableID).Scan(&receiptCount); err != nil {
			return ReconciliationResult{}, err
		}
		if selectedCount != sourceCount || sourceCount != receiptCount {
			return ReconciliationResult{}, fmt.Errorf("receipt count mismatch for %s: selected=%d source=%d receipt=%d", tableID, selectedCount, sourceCount, receiptCount)
		}
		result.SelectedSourceCount += selectedCount
	}

	rows, err := tx.Query(ctx, `SELECT table_id,source_key_digest,payload_digest,disposition,reason,
target_domain,target_table,target_id,target_digest,metadata,verified
FROM public.v1_domain_import_receipts
WHERE import_version=$1 AND archive_run_id=$2
ORDER BY table_id,source_key_digest`, importVersion, archiveRunID)
	if err != nil {
		return ReconciliationResult{}, err
	}
	receipts := make([]reconciliationRow, 0, result.SelectedSourceCount)
	for rows.Next() {
		var row reconciliationRow
		if err = rows.Scan(&row.TableID, &row.SourceKeyDigest, &row.PayloadDigest, &row.Disposition, &row.Reason,
			&row.TargetDomain, &row.TargetTable, &row.TargetID, &row.TargetDigest, &row.Metadata, &row.Verified); err != nil {
			return ReconciliationResult{}, err
		}
		receipts = append(receipts, row)
	}
	if err = rows.Err(); err != nil {
		return ReconciliationResult{}, err
	}
	rows.Close()

	importedTargets := make(map[string]map[string]struct{})
	for _, row := range receipts {
		if row.Disposition != "import" || row.TargetTable == nil || row.TargetID == nil {
			continue
		}
		if importedTargets[*row.TargetTable] == nil {
			importedTargets[*row.TargetTable] = make(map[string]struct{})
		}
		importedTargets[*row.TargetTable][*row.TargetID] = struct{}{}
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for _, row := range receipts {
		result.ReceiptCount++
		if !row.Verified {
			return ReconciliationResult{}, fmt.Errorf("unverified receipt for %s", row.TableID)
		}
		result.VerifiedCount++
		proof := "terminal:" + row.Disposition
		switch row.Disposition {
		case "import":
			result.ImportedCount++
			proof, err = verifyImportedTarget(ctx, tx, row, importedTargets)
			if err != nil {
				return ReconciliationResult{}, err
			}
		case "archive":
			result.ArchivedCount++
		case "quarantine":
			result.QuarantinedCount++
		default:
			return ReconciliationResult{}, fmt.Errorf("unknown disposition for %s", row.TableID)
		}
		if err = encoder.Encode([]any{row.TableID, hex.EncodeToString(row.SourceKeyDigest), hex.EncodeToString(row.PayloadDigest),
			row.Disposition, row.Reason, stringValue(row.TargetDomain), stringValue(row.TargetTable), stringValue(row.TargetID),
			hex.EncodeToString(row.TargetDigest), proof}); err != nil {
			return ReconciliationResult{}, err
		}
	}
	if result.SelectedSourceCount != result.ReceiptCount {
		return ReconciliationResult{}, fmt.Errorf("selected source count does not match all receipts")
	}
	digest := hash.Sum(nil)
	result.ComparisonDigest = hex.EncodeToString(digest)

	command, err := tx.Exec(ctx, `INSERT INTO public.v1_domain_import_reconciliation_receipts
(import_version,archive_run_id,selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (import_version,archive_run_id) DO NOTHING`, importVersion, archiveRunID, result.SelectedSourceCount,
		result.ReceiptCount, result.ImportedCount, result.ArchivedCount, result.QuarantinedCount, result.VerifiedCount, digest)
	if err != nil {
		return ReconciliationResult{}, err
	}
	result.Replayed = command.RowsAffected() == 0
	if result.Replayed {
		var selected, receipts, imported, archived, quarantined, verified int64
		var foundDigest []byte
		err = tx.QueryRow(ctx, `SELECT selected_source_count,receipt_count,imported_count,archived_count,quarantined_count,verified_count,comparison_digest
FROM public.v1_domain_import_reconciliation_receipts WHERE import_version=$1 AND archive_run_id=$2`, importVersion, archiveRunID).
			Scan(&selected, &receipts, &imported, &archived, &quarantined, &verified, &foundDigest)
		if err != nil || selected != result.SelectedSourceCount || receipts != result.ReceiptCount || imported != result.ImportedCount ||
			archived != result.ArchivedCount || quarantined != result.QuarantinedCount || verified != result.VerifiedCount || !equalBytes(foundDigest, digest) {
			return ReconciliationResult{}, ErrConflict
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return ReconciliationResult{}, err
	}
	return result, nil
}

func verifyImportedTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, importedTargets map[string]map[string]struct{}) (string, error) {
	expected, ok := targetBySourceTable[row.TableID]
	if !ok || row.TargetDomain == nil || row.TargetTable == nil || row.TargetID == nil ||
		*row.TargetDomain != expected.domain || *row.TargetTable != expected.table || len(row.TargetDigest) != sha256.Size {
		return "", fmt.Errorf("invalid imported target for %s", row.TableID)
	}
	var proof string
	switch expected.table {
	case "cloud_campaigns":
		var approval, runtime string
		var version, plans, commands, touchPlans, handoffs, mediaBindings int64
		err := tx.QueryRow(ctx, `SELECT campaign.approval_status,campaign.runtime_status,campaign.version,
  (SELECT count(*) FROM public.cloud_campaign_local_plans WHERE campaign_code=campaign.campaign_code),
  (SELECT count(*) FROM public.cloud_campaign_local_commands WHERE campaign_code=campaign.campaign_code),
  (SELECT count(*) FROM public.cloud_campaign_touch_plans WHERE campaign_code=campaign.campaign_code),
  (SELECT count(*) FROM public.outbound_campaign_handoffs WHERE campaign_code=campaign.campaign_code),
  (SELECT count(*) FROM public.media_campaign_delivery_bindings WHERE campaign_code=campaign.campaign_code)
FROM public.cloud_campaigns campaign WHERE campaign.campaign_code=$1 FOR SHARE OF campaign`, *row.TargetID).
			Scan(&approval, &runtime, &version, &plans, &commands, &touchPlans, &handoffs, &mediaBindings)
		if err != nil || approval != "rejected" || runtime != "paused" || version != 1 || plans != 0 || commands != 0 || touchPlans != 0 || handoffs != 0 || mediaBindings != 0 {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = approval + ":" + runtime + ":" + strconv.FormatInt(version, 10) + ":no_runtime"
	case "cloud_campaign_steps":
		campaignCode, index, err := parseCampaignStepTarget(*row.TargetID)
		if err != nil {
			return "", err
		}
		var approval, runtime string
		err = tx.QueryRow(ctx, `SELECT campaign.approval_status,campaign.runtime_status
FROM public.cloud_campaign_steps step JOIN public.cloud_campaigns campaign USING (campaign_code)
WHERE step.campaign_code=$1 AND step.step_index=$2 FOR SHARE OF step,campaign`, campaignCode, index).Scan(&approval, &runtime)
		if err != nil || approval != "rejected" || runtime != "paused" {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = approval + ":" + runtime
	case "questionnaires":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var version, submissionCount int64
		var disabled, assessmentFree bool
		err = tx.QueryRow(ctx, `SELECT version,submission_count,is_disabled,
NOT assessment_enabled AND assessment_config='{}'::jsonb
FROM public.questionnaires WHERE id=$1 FOR SHARE`, id).Scan(&version, &submissionCount, &disabled, &assessmentFree)
		if err != nil || version != 1 || submissionCount < 0 || !assessmentFree {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = fmt.Sprintf("%d:%d:%t:no_assessment", version, submissionCount, disabled)
	case "questionnaire_questions", "questionnaire_options", "questionnaire_submissions", "questionnaire_submission_answers":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var parentTable, parentID string
		switch expected.table {
		case "questionnaire_questions":
			parentTable = "questionnaires"
			err = tx.QueryRow(ctx, `SELECT questionnaire_id::text FROM public.questionnaire_questions WHERE id=$1 FOR SHARE`, id).Scan(&parentID)
		case "questionnaire_options":
			parentTable = "questionnaire_questions"
			err = tx.QueryRow(ctx, `SELECT question_id::text FROM public.questionnaire_options WHERE id=$1 FOR SHARE`, id).Scan(&parentID)
		case "questionnaire_submissions":
			parentTable = "questionnaires"
			err = tx.QueryRow(ctx, `SELECT questionnaire_id::text FROM public.questionnaire_submissions WHERE id=$1 FOR SHARE`, id).Scan(&parentID)
		case "questionnaire_submission_answers":
			var questionID, submissionQuestionnaireID, questionQuestionnaireID string
			err = tx.QueryRow(ctx, `SELECT answer.submission_id::text,answer.question_id::text,
submission.questionnaire_id::text,question.questionnaire_id::text
FROM public.questionnaire_submission_answers answer
JOIN public.questionnaire_submissions submission ON submission.id=answer.submission_id
JOIN public.questionnaire_questions question ON question.id=answer.question_id
WHERE answer.id=$1 FOR SHARE OF answer,submission,question`, id).
				Scan(&parentID, &questionID, &submissionQuestionnaireID, &questionQuestionnaireID)
			if err == nil && (!containsTarget(importedTargets, "questionnaire_submissions", parentID) ||
				!containsTarget(importedTargets, "questionnaire_questions", questionID) || submissionQuestionnaireID != questionQuestionnaireID) {
				err = ErrConflict
			}
			proof = parentID + ":" + questionID + ":" + submissionQuestionnaireID
		}
		if err != nil {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		if expected.table != "questionnaire_submission_answers" {
			if !containsTarget(importedTargets, parentTable, parentID) {
				return "", fmt.Errorf("target %s/%s points outside imported %s set", expected.table, *row.TargetID, parentTable)
			}
			proof = parentID
		}
	case "media_miniprograms":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var receiptMetadata struct {
			SourceID int64 `json:"source_id"`
		}
		if json.Unmarshal(row.Metadata, &receiptMetadata) != nil || receiptMetadata.SourceID < 1 {
			return "", fmt.Errorf("invalid media receipt metadata")
		}
		var sourceID, version int64
		var enabled bool
		var imageURL, mediaID string
		var imageID *int64
		var expires any
		err = tx.QueryRow(ctx, `SELECT legacy_source_id,version,enabled,thumbnail_image_url,thumbnail_image_id,thumbnail_media_id,thumbnail_media_expires_at
FROM public.media_miniprograms WHERE id=$1 FOR SHARE`, id).Scan(&sourceID, &version, &enabled, &imageURL, &imageID, &mediaID, &expires)
		if err != nil || sourceID != receiptMetadata.SourceID || version != 1 || enabled || imageURL != "" || imageID != nil || mediaID != "" || expires != nil {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = strconv.FormatInt(sourceID, 10) + ":1:disabled:no_provider_media"
	case "radar_links":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var status string
		var version, events int64
		var localOnly bool
		err = tx.QueryRow(ctx, `SELECT link.status,link.version,link.cover_image_id IS NULL AND link.attachment_id IS NULL,
(SELECT count(*) FROM public.radar_link_events WHERE link_id=link.id)
FROM public.radar_links link WHERE link.id=$1 FOR SHARE OF link`, id).Scan(&status, &version, &localOnly, &events)
		if err != nil || status != "draft" || version != 1 || !localOnly || events != 0 {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = status + ":" + strconv.FormatInt(version, 10) + ":no_events"
	case "order_wechat_shop_materials":
		var source, readiness string
		var providerVerified bool
		var syncRequests, refunds int64
		var evidenceDigest []byte
		err := tx.QueryRow(ctx, `SELECT material.source,material.readiness,material.provider_verified,material.evidence_digest,
(SELECT count(*) FROM public.order_wechat_shop_material_sync_requests WHERE provider_order_id=material.provider_order_id),
(SELECT count(*) FROM public.order_wechat_shop_refunds WHERE provider_order_id=material.provider_order_id)
FROM public.order_wechat_shop_materials material WHERE material.provider_order_id=$1 FOR SHARE OF material`, *row.TargetID).
			Scan(&source, &readiness, &providerVerified, &evidenceDigest, &syncRequests, &refunds)
		if err != nil || source != "legacy_raw" || readiness != "provider_sync_required" || providerVerified || !equalBytes(evidenceDigest, row.TargetDigest) || syncRequests != 0 || refunds != 0 {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = source + ":" + readiness + ":unverified:no_external_work"
	default:
		return "", fmt.Errorf("unsupported target table %s", expected.table)
	}
	return expected.table + ":" + *row.TargetID + ":" + proof, nil
}

func parseCampaignStepTarget(value string) (string, int64, error) {
	index := strings.LastIndexByte(value, ':')
	if index < 1 || index == len(value)-1 {
		return "", 0, ErrConflict
	}
	step, err := positiveID(value[index+1:])
	if err != nil || step > campaign.MaximumSteps || !campaign.ValidCampaignCode(value[:index]) {
		return "", 0, ErrConflict
	}
	return value[:index], step, nil
}

func containsTarget(targets map[string]map[string]struct{}, table, id string) bool {
	_, found := targets[table][id]
	return found
}

func positiveID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, ErrConflict
	}
	return id, nil
}

func targetVerificationError(table, id string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("missing target %s/%s", table, id)
	}
	if err != nil {
		return fmt.Errorf("verify target %s/%s: %w", table, id, err)
	}
	return fmt.Errorf("unsafe target %s/%s", table, id)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	orderport "github.com/qianlan33333-png/AI-CRM-v2/internal/order/port"
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

var staticReconciledTables = []string{
	"public/wecom_corp_tag_groups", "public/wecom_corp_tags", "public/contact_tags",
	"public/image_library", "public/attachment_library", "public/wechat_pay_products",
}
var financeReconciledTables = []string{"public/wechat_pay_orders", "public/wechat_pay_refunds"}

var channelReconciledTables = []string{
	"public/automation_channel", "public/automation_channel_assignee", "public/automation_channel_contact",
	"public/automation_channel_entry_effect_log", "public/automation_channel_entry_runtime", "public/automation_channel_qrcode_asset",
	"public/automation_channel_scene_alias", "public/channel_welcome_effect_dependency", "public/channel_welcome_effect_graph",
}

var targetBySourceTable = map[string]struct {
	domain string
	table  string
}{
	"public/campaigns":                                 {"campaign", "cloud_campaigns"},
	"public/campaign_steps":                            {"campaign", "cloud_campaign_steps"},
	"public/questionnaires":                            {"survey", "questionnaires"},
	"public/questionnaire_questions":                   {"survey", "questionnaire_questions"},
	"public/questionnaire_options":                     {"survey", "questionnaire_options"},
	"public/questionnaire_submissions":                 {"survey", "questionnaire_submissions"},
	"public/questionnaire_submission_answers":          {"survey", "questionnaire_submission_answers"},
	"public/miniprogram_library":                       {"media", "media_miniprograms"},
	"public/radar_links":                               {"radar", "radar_links"},
	"public/wechat_shop_orders":                        {"order", "order_wechat_shop_materials"},
	"public/wecom_corp_tag_groups":                     {"contact", "tag_groups"},
	"public/wecom_corp_tags":                           {"contact", "tags"},
	"public/contact_tags":                              {"contact", "customer_tags"},
	"public/image_library":                             {"media", "media_images"},
	"public/attachment_library":                        {"media", "media_attachments"},
	"public/wechat_pay_products":                       {"product", "products"},
	"public/wechat_pay_orders":                         {"order", "order_list_projections"},
	"public/wechat_pay_refunds":                        {"order", "order_historical_refunds"},
	"public/automation_channel":                        {"contact", "channels"},
	"public/automation_channel_contact":                {"contact", "channel_historical_contacts"},
	"public/automation_channel_assignee":               {"contact", "channel_historical_assignees"},
	"public/service_period_products":                   {"product", "product_service_period_history"},
	"public/service_period_entitlements":               {"product", "product_service_period_entitlement_history"},
	"public/service_period_events":                     {"product", "product_service_period_event_history"},
	"public/commerce_coupons":                          {"coupon", "coupons"},
	"public/commerce_coupon_product_bindings":          {"coupon", "coupon_targets"},
	"public/commerce_coupon_claims":                    {"coupon", "coupon_v1_history_claims"},
	"public/commerce_coupon_redemptions":               {"coupon", "coupon_v1_history_redemptions"},
	"public/automation_group_ops_plans":                {"groupops", "group_ops_plans"},
	"public/group_chats":                               {"groupops", "group_ops_v1_history_directory"},
	"public/wecom_group_chat_snapshots":                {"groupops", "group_ops_v1_history_directory"},
	"public/automation_group_ops_plan_groups":          {"groupops", "group_ops_v1_history_groups"},
	"public/automation_group_ops_plan_nodes":           {"groupops", "group_ops_v1_history_nodes"},
	"public/ai_audience_package_group":                 {"segment", "segment_v1_audience_groups"},
	"public/ai_audience_package":                       {"segment", "segment_v1_audience_packages"},
	"public/ai_audience_package_version":               {"segment", "segment_v1_audience_versions"},
	"public/ai_audience_package_sender":                {"segment", "segment_v1_audience_senders"},
	"public/audience_rule":                             {"segment", "segment_v1_audience_rules"},
	"public/audience_rule_version":                     {"segment", "segment_v1_audience_rule_versions"},
	"public/segments":                                  {"segment", "segment_v1_definitions"},
	"public/ai_audience_member_current":                {"segment", "segment_v1_audience_members"},
	"public/service_period_member_views":               {"product", "product_v1_member_view_history"},
	"public/service_period_huangyoucan_usage_snapshot": {"product", "product_v1_member_usage_history"},
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
	return reconcileTables(ctx, pool, importVersion, archiveRunID, reconciledTables)
}

// ReconcileStatic seals only the following static package. The earlier ten
// tables retain their own immutable import version and reconciliation.
func ReconcileStatic(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	return reconcileTables(ctx, pool, importVersion, archiveRunID, staticReconciledTables)
}

func ReconcileFinance(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	return reconcileTables(ctx, pool, importVersion, archiveRunID, financeReconciledTables)
}

func ReconcileChannel(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	return reconcileTables(ctx, pool, importVersion, archiveRunID, channelReconciledTables)
}

func ReconcileGroupOps(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string) (ReconciliationResult, error) {
	return reconcileTables(ctx, pool, importVersion, archiveRunID, groupOpsReconciledTables)
}

func reconcileTables(ctx context.Context, pool *pgxpool.Pool, importVersion, archiveRunID string, tables []string) (ReconciliationResult, error) {
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
	for _, tableID := range tables {
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
		if slices.Contains(groupOpsReconciledTables, row.TableID) {
			if err = validateGroupOpsDisposition(row.TableID, row.Disposition); err != nil {
				return ReconciliationResult{}, err
			}
		}
		if row.TableID == "public/wechat_pay_orders" || row.TableID == "public/wechat_pay_refunds" || servicePeriodTarget(row.TableID) != "" ||
			row.TableID == "public/commerce_coupons" || row.TableID == "public/commerce_coupon_product_bindings" ||
			row.TableID == "public/commerce_coupon_claims" || row.TableID == "public/commerce_coupon_redemptions" || slices.Contains(groupOpsReconciledTables, row.TableID) || isAudienceHistorySource(row.TableID) || isMemberGridHistorySource(row.TableID) {
			var sourceMatches bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.v1_archive_records
WHERE run_id=$1 AND adapter_id=$2 AND table_id=$3 AND source_key_digest=$4 AND payload_digest=$5)`,
				archiveRunID, v1archive.DefaultAdapterID, row.TableID, row.SourceKeyDigest, row.PayloadDigest).Scan(&sourceMatches); err != nil {
				return ReconciliationResult{}, err
			}
			if !sourceMatches {
				return ReconciliationResult{}, ErrConflict
			}
		}
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
	if isAudienceHistorySource(row.TableID) {
		return verifyAudienceHistoryTarget(ctx, tx, row, importedTargets)
	}
	switch expected.table {
	case "product_service_period_history", "product_service_period_entitlement_history", "product_service_period_event_history":
		return verifyServicePeriodTarget(ctx, tx, row, importedTargets)
	case "product_v1_member_view_history", "product_v1_member_usage_history":
		return verifyMemberGridHistoryTarget(ctx, tx, row, importedTargets)
	case "coupons", "coupon_targets", "coupon_v1_history_claims", "coupon_v1_history_redemptions":
		return verifyCouponTarget(ctx, tx, row, importedTargets)
	case "group_ops_plans", "group_ops_v1_history_directory", "group_ops_v1_history_groups", "group_ops_v1_history_nodes":
		return verifyGroupOpsTarget(ctx, tx, row, importedTargets)
	case "order_list_projections", "order_historical_refunds":
		return verifyFinanceTarget(ctx, tx, row, importedTargets)
	case "channel_historical_contacts":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var target contactport.HistoricalChannelContact
		err = tx.QueryRow(ctx, `SELECT id,channel_id,source_contact_id,customer_id,owner_reference,first_entered_at,last_entered_at,enter_count,created_at,updated_at
FROM public.channel_historical_contacts WHERE id=$1 FOR SHARE`, id).Scan(&target.ID, &target.ChannelID, &target.SourceContactID, &target.CustomerID, &target.OwnerReference,
			&target.FirstEnteredAt, &target.LastEnteredAt, &target.EnterCount, &target.CreatedAt, &target.UpdatedAt)
		target.FirstEnteredAt, target.LastEnteredAt = target.FirstEnteredAt.UTC(), target.LastEnteredAt.UTC()
		target.CreatedAt, target.UpdatedAt = target.CreatedAt.UTC(), target.UpdatedAt.UTC()
		digest, digestErr := contactapp.HistoricalChannelContactTargetDigest(target)
		_, sameBatch := importedTargets["channels"][strconv.FormatInt(target.ChannelID, 10)]
		if err != nil || digestErr != nil || !sameBatch || !equalBytes(digest[:], row.TargetDigest) {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = "history_only:" + hex.EncodeToString(digest[:])
	case "channel_historical_assignees":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var target contactport.HistoricalChannelAssignee
		err = tx.QueryRow(ctx, `SELECT id,channel_id,source_assignee_id,staff_reference,display_name_snapshot,priority,ratio_percent,max_scans_24h,status,
to_char(source_created_at,'YYYY-MM-DD"T"HH24:MI:SS.US'),to_char(source_updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US')
FROM public.channel_historical_assignees WHERE id=$1 FOR SHARE`, id).Scan(&target.ID, &target.ChannelID, &target.SourceAssigneeID, &target.StaffReference, &target.DisplayNameSnapshot,
			&target.Priority, &target.RatioPercent, &target.MaxScans24h, &target.Status, &target.SourceCreatedAt, &target.SourceUpdatedAt)
		digest, digestErr := contactapp.HistoricalChannelAssigneeTargetDigest(target)
		_, sameBatch := importedTargets["channels"][strconv.FormatInt(target.ChannelID, 10)]
		if err != nil || digestErr != nil || !sameBatch || !equalBytes(digest[:], row.TargetDigest) {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = "history_only:" + hex.EncodeToString(digest[:])
	case "channels":
		id, err := positiveID(*row.TargetID)
		if err != nil {
			return "", err
		}
		var target contactport.HistoricalChannelRecord
		var noAssets bool
		err = tx.QueryRow(ctx, `SELECT c.id,c.code,c.name,c.status,c.config,c.created_by,c.updated_by,c.created_at,c.updated_at,a.config_digest,
NOT EXISTS(SELECT 1 FROM public.channel_acquisition_asset_bindings b WHERE b.channel_id=c.id)
FROM public.channels c JOIN public.channel_acquisition_legacy_archives a ON a.channel_id=c.id
WHERE c.id=$1 AND a.status='legacy_unverified' FOR SHARE OF c,a`, id).
			Scan(&target.ID, &target.Code, &target.Name, &target.Status, &target.Projection, &target.CreatedBy, &target.UpdatedBy,
				&target.CreatedAt, &target.UpdatedAt, &target.LegacyConfigDigest, &noAssets)
		digest, digestErr := contactapp.HistoricalChannelTargetDigest(target)
		if err != nil || digestErr != nil || !noAssets || !equalBytes(digest[:], row.TargetDigest) {
			return "", targetVerificationError(expected.table, *row.TargetID, err)
		}
		proof = "inactive:" + hex.EncodeToString(digest[:])
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
		staticProof, staticErr := verifyStaticTarget(ctx, tx, row, importedTargets)
		if staticErr != nil {
			return "", staticErr
		}
		proof = staticProof
	}
	return expected.table + ":" + *row.TargetID + ":" + proof, nil
}

func verifyStaticTarget(ctx context.Context, tx pgx.Tx, row reconciliationRow, targets map[string]map[string]struct{}) (string, error) {
	id, err := positiveID(*row.TargetID)
	if err != nil {
		return "", err
	}
	var metadata map[string]string
	switch *row.TargetTable {
	case "tag_groups":
		var name string
		err = tx.QueryRow(ctx, `SELECT name FROM public.tag_groups WHERE id=$1 FOR SHARE`, id).Scan(&name)
		if err != nil || strings.TrimSpace(name) == "" {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "local_tag_group", nil
	case "tags":
		var groupID, providerID string
		err = tx.QueryRow(ctx, `SELECT group_id::text,wecom_tag_id FROM public.tags WHERE id=$1 FOR SHARE`, id).Scan(&groupID, &providerID)
		if err != nil || providerID == "" || !containsTarget(targets, "tag_groups", groupID) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "local_tag:" + groupID, nil
	case "customer_tags":
		if json.Unmarshal(row.Metadata, &metadata) != nil {
			return "", ErrConflict
		}
		customerID, err := positiveID(metadata["customer_id"])
		if err != nil || !containsTarget(targets, "tags", *row.TargetID) {
			return "", ErrConflict
		}
		expectedDigest := sha256.Sum256([]byte("v1.contact_tags\x00" + metadata["customer_id"] + "\x00" + *row.TargetID))
		var taggedBy string
		err = tx.QueryRow(ctx, `SELECT tagged_by FROM public.customer_tags WHERE customer_id=$1 AND tag_id=$2 FOR SHARE`, customerID, id).Scan(&taggedBy)
		if err != nil || taggedBy != "migration:v1-contact-tags" || !equalBytes(expectedDigest[:], row.TargetDigest) {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "local_customer_tag:" + metadata["customer_id"], nil
	case "media_images", "media_attachments":
		var checksumMetadata struct {
			Checksum string `json:"checksum"`
		}
		if json.Unmarshal(row.Metadata, &checksumMetadata) != nil {
			return "", ErrConflict
		}
		var checksum []byte
		var safe bool
		if *row.TargetTable == "media_images" {
			err = tx.QueryRow(ctx, `SELECT image.checksum,NOT image.enabled AND image.file_size=octet_length(blob.content)
AND image.checksum=blob.checksum AND blob.checksum=sha256(blob.content)
FROM public.media_images image JOIN public.media_image_blobs blob ON blob.image_id=image.id
WHERE image.id=$1 FOR SHARE OF image,blob`, id).Scan(&checksum, &safe)
		} else {
			err = tx.QueryRow(ctx, `SELECT attachment.checksum,NOT attachment.enabled AND attachment.version=1
AND attachment.mime_type='application/pdf' AND attachment.file_size=octet_length(blob.content)
AND attachment.checksum=blob.checksum AND blob.checksum=sha256(blob.content)
FROM public.media_attachments attachment JOIN public.media_attachment_blobs blob ON blob.attachment_id=attachment.id
WHERE attachment.id=$1 FOR SHARE OF attachment,blob`, id).Scan(&checksum, &safe)
		}
		if err != nil || !safe || len(checksum) != sha256.Size || hex.EncodeToString(checksum) != checksumMetadata.Checksum {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "disabled:blob:" + checksumMetadata.Checksum, nil
	case "products":
		var expected struct {
			Code      string `json:"target_product_code"`
			Name      string `json:"target_product_name"`
			Price     string `json:"price_minor"`
			Currency  string `json:"currency"`
			CreatedBy string `json:"created_by"`
		}
		if json.Unmarshal(row.Metadata, &expected) != nil {
			return "", ErrConflict
		}
		var code, name, price, currency, actor string
		var safe bool
		err = tx.QueryRow(ctx, `SELECT product_code,name,price_minor::text,currency,created_by::text,
local_lifecycle='disabled' AND version=1 AND stock_quantity=0 AND description=''
AND legacy_admin_projection->>'enabled'='false'
AND NOT EXISTS(SELECT 1 FROM public.product_images WHERE product_id=products.id)
FROM public.products WHERE id=$1 FOR SHARE`, id).Scan(&code, &name, &price, &currency, &actor, &safe)
		if err != nil || !safe || code != expected.Code || name != expected.Name || price != expected.Price || currency != "CNY" || currency != expected.Currency || actor != expected.CreatedBy {
			return "", targetVerificationError(*row.TargetTable, *row.TargetID, err)
		}
		return "disabled:" + code + ":" + price + ":" + currency, nil
	default:
		return "", fmt.Errorf("unsupported target table %s", *row.TargetTable)
	}
}

func readFinanceOrder(ctx context.Context, tx pgx.Tx, id int64) (orderport.Record, error) {
	var order orderport.Record
	err := tx.QueryRow(ctx, `SELECT id,record_origin,provider,provider_label,merchant_order_no,platform_transaction_no,
customer_id,payer_name_snapshot,mobile_snapshot,identity_kind,identity_value,
product_id,product_code,product_name_snapshot,amount_minor,currency,status,status_label,detail_url,created_at,updated_at
FROM public.order_list_projections WHERE id=$1 AND pe01_contract_version IS NULL FOR SHARE`, id).
		Scan(&order.ID, &order.RecordOrigin, &order.Provider, &order.ProviderLabel, &order.MerchantOrderNo, &order.PlatformTransactionNo,
			&order.CustomerID, &order.PayerNameSnapshot, &order.MobileSnapshot, &order.IdentityKind, &order.IdentityValue,
			&order.ProductID, &order.ProductCode, &order.ProductNameSnapshot, &order.AmountMinor, &order.Currency,
			&order.Status, &order.StatusLabel, &order.DetailURL, &order.CreatedAt, &order.UpdatedAt)
	return order, err
}

func readFinanceRefund(ctx context.Context, tx pgx.Tx, id int64) (orderport.HistoricalRefund, error) {
	var refund orderport.HistoricalRefund
	err := tx.QueryRow(ctx, `SELECT id,order_id,source_refund_id,refund_number,provider_refund_id,transaction_id,status,
amount_minor,order_amount_minor,currency,reason,created_at,updated_at
FROM public.order_historical_refunds WHERE id=$1 FOR SHARE`, id).
		Scan(&refund.ID, &refund.OrderID, &refund.SourceRefundID, &refund.RefundNumber, &refund.ProviderRefundID,
			&refund.TransactionID, &refund.Status, &refund.AmountMinor, &refund.OrderAmountMinor, &refund.Currency,
			&refund.Reason, &refund.CreatedAt, &refund.UpdatedAt)
	return refund, err
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

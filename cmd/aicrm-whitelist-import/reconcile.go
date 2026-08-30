package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type domainReconciliation struct {
	SourceCount   int64 `json:"source_count"`
	TargetCount   int64 `json:"target_count"`
	RejectedCount int64 `json:"explicit_rejected_count"`
}

type reconcileResult struct {
	RunID                    string                          `json:"run_id"`
	Domains                  map[string]domainReconciliation `json:"domains"`
	RequiredIdentityCoverage string                          `json:"required_identity_mapping_coverage"`
	OrphanRows               int64                           `json:"orphan_rows"`
	IdentityConflicts        int64                           `json:"identity_conflicts"`
	OrderAmountMinor         int64                           `json:"order_amount_minor"`
	ExternalEffects          int64                           `json:"external_effects"`
	ProviderReceipts         int64                           `json:"real_provider_receipts"`
	LegacyMaterialReferences int64                           `json:"legacy_material_references"`
	OldCampaignRows          int64                           `json:"old_campaign_rows"`
	OldMessageRows           int64                           `json:"old_message_rows"`
	TargetDigest             string                          `json:"target_digest"`
}

var reconciledDomains = []string{"identity", "product", "order", "questionnaire", "membership", "radar", "channel", "audience", "automation"}

func reconcileWhitelist(ctx context.Context, pool *pgxpool.Pool, runID string) (reconcileResult, error) {
	if pool == nil {
		return reconcileResult{}, errors.New("target database unavailable")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return reconcileResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	return reconcileWhitelistTx(ctx, tx, runID)
}

func reconcileWhitelistTx(ctx context.Context, tx pgx.Tx, runID string) (reconcileResult, error) {
	result := reconcileResult{RunID: runID, Domains: map[string]domainReconciliation{}, RequiredIdentityCoverage: "100%"}
	var runState string
	if err := tx.QueryRow(ctx, `SELECT state FROM public.whitelist_import_runs WHERE id=$1`, runID).Scan(&runState); err != nil {
		return reconcileResult{}, errors.New("whitelist import run not found")
	}
	if runState != "running" && runState != "completed" {
		return reconcileResult{}, errors.New("whitelist import run is not reconcilable")
	}
	for _, domain := range reconciledDomains {
		var counts domainReconciliation
		if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE disposition='MIGRATE'),count(*) FILTER (WHERE disposition<>'MIGRATE') FROM public.whitelist_import_domain_receipts WHERE run_id=$1 AND domain=$2`, runID, domain).Scan(&counts.SourceCount, &counts.TargetCount, &counts.RejectedCount); err != nil {
			return reconcileResult{}, err
		}
		if counts.SourceCount != counts.TargetCount+counts.RejectedCount {
			return reconcileResult{}, fmt.Errorf("%s reconciliation silently dropped rows", domain)
		}
		result.Domains[domain] = counts
	}
	if err := tx.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.order_list_projections WHERE customer_id IS NULL OR NOT EXISTS (SELECT 1 FROM public.customers WHERE id=order_list_projections.customer_id))+
  (SELECT count(*) FROM public.questionnaire_submissions WHERE customer_id IS NULL OR NOT EXISTS (SELECT 1 FROM public.customers WHERE id=questionnaire_submissions.customer_id))+
  (SELECT count(*) FROM public.product_local_entitlements WHERE NOT EXISTS (SELECT 1 FROM public.customers WHERE id=product_local_entitlements.customer_id))+
  (SELECT count(*) FROM public.service_period_members WHERE NOT EXISTS (SELECT 1 FROM public.customers WHERE id=service_period_members.customer_id))+
  (SELECT count(*) FROM public.segment_members WHERE NOT EXISTS (SELECT 1 FROM public.customers WHERE id=segment_members.customer_id)),
  (SELECT count(*) FROM (SELECT source_system,source_entity,reference_digest FROM public.source_subject_refs GROUP BY 1,2,3 HAVING count(DISTINCT customer_id)>1) AS conflict),
  (SELECT COALESCE(sum(amount_minor),0)::bigint FROM public.order_list_projections)`).Scan(&result.OrphanRows, &result.IdentityConflicts, &result.OrderAmountMinor); err != nil {
		return reconcileResult{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.external_effects)+(SELECT count(*) FROM public.external_effect_attempts)+(SELECT count(*) FROM public.external_effect_receipts)+(SELECT count(*) FROM public.external_effect_reconciliations),
	  (SELECT count(*) FROM public.order_provider_callback_receipts),
	  (SELECT count(*) FROM public.product_images)+
	  (SELECT count(*) FROM public.products WHERE
	    legacy_admin_projection ?| ARRAY['image_ids','material_ids'] OR
	    COALESCE(legacy_admin_projection->'lead_program_id','null'::jsonb)<>'null'::jsonb OR
	    COALESCE(legacy_admin_projection->'lead_channel_id','null'::jsonb)<>'null'::jsonb OR
	    COALESCE(legacy_admin_projection->'completion_target','null'::jsonb)<>'null'::jsonb OR
	    COALESCE(legacy_admin_projection->'wecom_tagging','null'::jsonb) NOT IN ('null'::jsonb,'{}'::jsonb,'[]'::jsonb))+
	  (SELECT count(*) FROM public.channels WHERE
	    COALESCE(config->'staff_id','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'staff_ids','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'employee_id','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'employee_ids','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'tag_id','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'tag_ids','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'welcome_material_id','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'welcome_material_ids','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'welcome_message','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'material_id','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb) OR
	    COALESCE(config->'material_ids','null'::jsonb) NOT IN ('null'::jsonb,'""'::jsonb,'[]'::jsonb,'{}'::jsonb))+
  (SELECT count(*) FROM public.radar_links WHERE cover_image_id IS NOT NULL OR attachment_id IS NOT NULL)+
  (SELECT count(*) FROM public.automation_agent_configurations WHERE
    jsonb_array_length(COALESCE(fixed_content_package_json->'image_library_ids','[]'::jsonb))>0 OR
    jsonb_array_length(COALESCE(fixed_content_package_json->'miniprogram_library_ids','[]'::jsonb))>0 OR
    jsonb_array_length(COALESCE(fixed_content_package_json->'attachment_library_ids','[]'::jsonb))>0 OR
    jsonb_array_length(COALESCE(fixed_content_package_json->'group_invite_library_ids','[]'::jsonb))>0 OR fixed_content_package_json ? 'dynamic_miniprogram_card'),
	  (SELECT count(*) FROM public.cloud_campaigns),
  (CASE WHEN to_regclass('public.wecom_message_archive') IS NULL AND to_regclass('public.messages') IS NULL THEN 0 ELSE 1 END)`).Scan(
		&result.ExternalEffects, &result.ProviderReceipts, &result.LegacyMaterialReferences, &result.OldCampaignRows, &result.OldMessageRows); err != nil {
		return reconcileResult{}, err
	}
	var audienceNotPaused, automationOpen, radarEvents, channelEvents int64
	if err := tx.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.ai_audience_package_metadata WHERE lifecycle<>'paused'),
  (SELECT count(*) FROM public.automation_agent_configurations WHERE status<>'paused' OR execution_enabled),
	  (SELECT count(*) FROM public.radar_link_events),
	  (SELECT count(*) FROM public.channel_acquisition_entrant_receipts)`).Scan(&audienceNotPaused, &automationOpen, &radarEvents, &channelEvents); err != nil {
		return reconcileResult{}, err
	}
	if result.OrphanRows != 0 || result.IdentityConflicts != 0 || result.ExternalEffects != 0 || result.ProviderReceipts != 0 || result.LegacyMaterialReferences != 0 || result.OldCampaignRows != 0 || result.OldMessageRows != 0 || audienceNotPaused != 0 || automationOpen != 0 || radarEvents != 0 || channelEvents != 0 {
		return reconcileResult{}, fmt.Errorf("whitelist hard gate failed: orphan=%d conflicts=%d effects=%d provider=%d materials=%d campaign=%d message=%d audience=%d automation=%d radar_events=%d channel_events=%d", result.OrphanRows, result.IdentityConflicts, result.ExternalEffects, result.ProviderReceipts, result.LegacyMaterialReferences, result.OldCampaignRows, result.OldMessageRows, audienceNotPaused, automationOpen, radarEvents, channelEvents)
	}
	digestInput, err := json.Marshal(struct {
		Domains          map[string]domainReconciliation `json:"domains"`
		OrderAmountMinor int64                           `json:"order_amount_minor"`
	}{result.Domains, result.OrderAmountMinor})
	if err != nil {
		return reconcileResult{}, err
	}
	digest := sha256.Sum256(digestInput)
	result.TargetDigest = hex.EncodeToString(digest[:])
	return result, nil
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type digestResult struct {
	SourceDigest string           `json:"source_digest"`
	Counts       map[string]int64 `json:"counts"`
}

type importResult struct {
	RunID        string           `json:"run_id"`
	SourceDigest string           `json:"source_digest"`
	Counts       map[string]int64 `json:"counts"`
	Reconcile    reconcileResult  `json:"reconciliation"`
}

type copySpec struct {
	domain       string
	sourceEntity string
	targetTable  string
	query        string
	transform    func(string, []byte, map[int64]int64) ([]byte, error)
}

type questionnaireReference struct {
	entity string
	value  string
}

type questionnaireResolutionRow struct {
	recordType       string
	submissionID     int64
	matchCount       int64
	existingCustomer int64
	unionID          string
	externalUserID   string
	openID           string
	mobile           string
	createdAt        time.Time
}

type syntheticCustomer struct {
	id        int64
	createdAt time.Time
	updatedAt time.Time
}

type questionnaireResolution struct {
	bySubmission map[int64]int64
	byOrder      map[int64]int64
	references   map[int64][]questionnaireReference
	synthetic    []syntheticCustomer
}

var whitelistCopySpecs = []copySpec{
	{sourceEntity: "admin_users", targetTable: "admin_users", query: sameRows("admin_users", "id")},
	{sourceEntity: "admin_sessions", targetTable: "admin_sessions", query: sameRows("admin_sessions", "id")},
	{sourceEntity: "settings", targetTable: "settings", query: `
SELECT key,to_jsonb(item),to_jsonb(item)
FROM public.settings AS item
WHERE key IN ('wecom.corp_id','wecom.agent_id','outbound.rate_per_second','outbound.max_attempts')
ORDER BY key`},
	{domain: "channel", sourceEntity: "channels", targetTable: "channels", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || jsonb_build_object(
  'config', item.config - ARRAY['staff_id','staff_ids','employee_id','employee_ids','tag_id','tag_ids','welcome_material_id','welcome_material_ids','welcome_message','material_id','material_ids'])
FROM public.channels AS item ORDER BY id`},
	{domain: "product", sourceEntity: "products", targetTable: "products", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || jsonb_build_object(
  'legacy_admin_projection',
    item.legacy_admin_projection - ARRAY['image_ids','material_ids'] || jsonb_build_object(
      'lead_program_id',null,
      'lead_channel_id',null,
      'completion_target',null,
      'wecom_tagging','{}'::jsonb))
FROM public.products AS item ORDER BY id`},
	{domain: "order", sourceEntity: "order_list_projections", targetTable: "order_list_projections", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || '{"payer_name_snapshot":"","mobile_snapshot":"","identity_kind":"","identity_value":""}'::jsonb
FROM public.order_list_projections AS item WHERE product_id IS NOT NULL ORDER BY id`, transform: addOrderCustomer},
	{domain: "order", sourceEntity: "order_historical_refunds", targetTable: "order_refund_facts", query: `
SELECT refund.id::text, to_jsonb(refund), jsonb_build_object(
	  'id',refund.id,'order_id',refund.order_id,'provider',orders.provider,
  'provider_refund_reference',COALESCE(NULLIF(refund.provider_refund_id,''),refund.refund_number),
  'amount_minor',refund.amount_minor,'currency',refund.currency,'status',refund.status,
  'reason',refund.reason,'created_at',refund.created_at,'completed_at',
  CASE WHEN lower(refund.status) IN ('completed','success','succeeded','refunded') THEN refund.updated_at ELSE NULL END)
FROM public.order_historical_refunds AS refund
JOIN public.order_list_projections AS orders ON orders.id=refund.order_id
WHERE lower(refund.status) IN ('completed','success','succeeded','refunded','failed','rejected','cancelled')
  AND orders.product_id IS NOT NULL
ORDER BY refund.id`},
	{domain: "membership", sourceEntity: "product_local_entitlements", targetTable: "product_local_entitlements", query: sameRows("product_local_entitlements", "id")},
	{domain: "questionnaire", sourceEntity: "questionnaires", targetTable: "questionnaires", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || '{"submission_count":0}'::jsonb
FROM public.questionnaires AS item ORDER BY id`},
	{domain: "questionnaire", sourceEntity: "questionnaire_questions", targetTable: "questionnaire_questions", query: sameRows("questionnaire_questions", "id")},
	{domain: "questionnaire", sourceEntity: "questionnaire_options", targetTable: "questionnaire_options", query: sameRows("questionnaire_options", "id")},
	{domain: "questionnaire", sourceEntity: "questionnaire_submissions", targetTable: "questionnaire_submissions", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || '{"respondent_key":"","openid":"","unionid":"","external_userid":"","customer_name":"","follow_user_userid":"","mobile":"","campaign_id":"","staff_id":""}'::jsonb
FROM public.questionnaire_submissions AS item ORDER BY id`, transform: addSubmissionCustomer},
	{domain: "questionnaire", sourceEntity: "questionnaire_submission_answers", targetTable: "questionnaire_submission_answers", query: sameRows("questionnaire_submission_answers", "id")},
	{domain: "membership", sourceEntity: "service_period_member_views", targetTable: "service_period_member_views", query: sameRows("service_period_member_views", "id")},
	{domain: "membership", sourceEntity: "service_period_members", targetTable: "service_period_members", query: sameRows("service_period_members", "id")},
	{domain: "radar", sourceEntity: "radar_links", targetTable: "radar_links", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || '{"cover_image_id":null,"attachment_id":null}'::jsonb
FROM public.radar_links AS item ORDER BY id`},
	{domain: "audience", sourceEntity: "ai_audience_package_groups", targetTable: "ai_audience_package_groups", query: `
SELECT item.id::text,to_jsonb(item),to_jsonb(item)
FROM public.ai_audience_package_groups AS item
WHERE EXISTS (
  SELECT 1 FROM public.ai_audience_package_metadata AS metadata
  WHERE metadata.group_id=item.id AND NOT EXISTS (
    SELECT 1 FROM public.segment_members AS member
    LEFT JOIN public.customers AS customer ON customer.id=member.customer_id
    WHERE member.segment_id=metadata.segment_id AND customer.id IS NULL))
ORDER BY item.id`},
	{domain: "audience", sourceEntity: "segments", targetTable: "segments", query: `
SELECT item.id::text, to_jsonb(item), to_jsonb(item)
FROM public.segments AS item
JOIN public.ai_audience_package_metadata AS metadata ON metadata.segment_id=item.id
WHERE NOT EXISTS (
  SELECT 1 FROM public.segment_members AS member
  LEFT JOIN public.customers AS customer ON customer.id=member.customer_id
  WHERE member.segment_id=item.id AND customer.id IS NULL)
ORDER BY item.id`},
	{domain: "audience", sourceEntity: "ai_audience_package_metadata", targetTable: "ai_audience_package_metadata", query: `
SELECT segment_id::text, to_jsonb(item), to_jsonb(item) || '{"lifecycle":"paused"}'::jsonb
FROM public.ai_audience_package_metadata AS item
WHERE NOT EXISTS (
  SELECT 1 FROM public.segment_members AS member
  LEFT JOIN public.customers AS customer ON customer.id=member.customer_id
  WHERE member.segment_id=item.segment_id AND customer.id IS NULL)
ORDER BY segment_id`},
	{domain: "audience", sourceEntity: "ai_audience_package_configuration_versions", targetTable: "ai_audience_package_configuration_versions", query: `
SELECT package_id::text||':'||version::text, to_jsonb(item), to_jsonb(item)
FROM public.ai_audience_package_configuration_versions AS item
WHERE NOT EXISTS (
  SELECT 1 FROM public.segment_members AS member
  LEFT JOIN public.customers AS customer ON customer.id=member.customer_id
  WHERE member.segment_id=item.package_id AND customer.id IS NULL)
ORDER BY package_id,version`},
	{domain: "audience", sourceEntity: "segment_members", targetTable: "segment_members", query: `
SELECT item.segment_id::text||':'||item.customer_id::text, to_jsonb(item), to_jsonb(item)
FROM public.segment_members AS item
JOIN public.ai_audience_package_metadata AS metadata ON metadata.segment_id=item.segment_id
WHERE NOT EXISTS (
  SELECT 1 FROM public.segment_members AS candidate
  LEFT JOIN public.customers AS customer ON customer.id=candidate.customer_id
  WHERE candidate.segment_id=item.segment_id AND customer.id IS NULL)
ORDER BY item.segment_id,item.customer_id`},
	{domain: "automation", sourceEntity: "automation_agent_configurations", targetTable: "automation_agent_configurations", query: `
SELECT id::text, to_jsonb(item), to_jsonb(item) || jsonb_build_object(
  'status','paused','execution_enabled',false,'fixed_content_package_json',jsonb_build_object(
    'content_text',COALESCE(item.fixed_content_package_json->>'content_text',''),
    'image_library_ids','[]'::jsonb,'miniprogram_library_ids','[]'::jsonb,
    'attachment_library_ids','[]'::jsonb,'group_invite_library_ids','[]'::jsonb))
FROM public.automation_agent_configurations AS item
WHERE status<>'archived' ORDER BY id`},
	{domain: "hxc", sourceEntity: "hxc_user_current", targetTable: "hxc_user_current", query: sameRows("hxc_user_current", "hxc_user_id")},
	{domain: "hxc", sourceEntity: "hxc_current_sync_runs", targetTable: "hxc_current_sync_runs", query: sameRows("hxc_current_sync_runs", "id")},
}

func sameRows(table, key string) string {
	return fmt.Sprintf("SELECT %s::text,to_jsonb(item),to_jsonb(item) FROM public.%s AS item ORDER BY %s", key, table, key)
}

func sourceDigest(ctx context.Context, sourceURL, archiveRunID string, allowTestSource bool) (digestResult, error) {
	pool, err := pgxpool.New(ctx, sourceURL)
	if err != nil {
		return digestResult{}, errors.New("source database unavailable")
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return digestResult{}, errors.New("source database unavailable")
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	sourceBinding, err := validateSource(ctx, tx, archiveRunID, allowTestSource)
	if err != nil {
		return digestResult{}, err
	}
	questionnaireCustomers, err := resolveQuestionnaireCustomers(ctx, tx)
	if err != nil {
		return digestResult{}, err
	}
	digest, counts, err := walkSource(ctx, tx, nil, questionnaireCustomers, sourceBinding, "", nil)
	if err != nil {
		return digestResult{}, err
	}
	return digestResult{SourceDigest: digest, Counts: counts}, nil
}

func importWhitelist(ctx context.Context, config cliConfig) (importResult, error) {
	if config.sourceURL == config.targetURL {
		return importResult{}, errors.New("source and target databases must differ")
	}
	sourcePool, err := pgxpool.New(ctx, config.sourceURL)
	if err != nil {
		return importResult{}, errors.New("source database unavailable")
	}
	defer sourcePool.Close()
	targetPool, err := pgxpool.New(ctx, config.targetURL)
	if err != nil {
		return importResult{}, errors.New("target database unavailable")
	}
	defer targetPool.Close()

	source, err := sourcePool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return importResult{}, errors.New("source database unavailable")
	}
	defer source.Rollback(ctx) //nolint:errcheck
	target, err := targetPool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return importResult{}, errors.New("target database unavailable")
	}
	defer target.Rollback(ctx) //nolint:errcheck

	sourceBinding, err := validateSource(ctx, source, config.archiveRunID, config.allowTestSource)
	if err != nil {
		return importResult{}, err
	}
	if err = validateTarget(ctx, target, config.allowTestName); err != nil {
		return importResult{}, err
	}
	questionnaireCustomers, err := resolveQuestionnaireCustomers(ctx, source)
	if err != nil {
		return importResult{}, err
	}
	if _, err = target.Exec(ctx, `INSERT INTO public.whitelist_import_runs(id,source_digest,state,started_at) VALUES($1,decode($2,'hex'),'running',$3)`, config.runID, config.sourceDigest, time.Now().UTC()); err != nil {
		return importResult{}, errors.New("target is not empty or run id already exists")
	}
	digest, counts, err := walkSource(ctx, source, target, questionnaireCustomers, sourceBinding, config.runID, insertReceipt)
	if err != nil {
		return importResult{}, err
	}
	if digest != config.sourceDigest {
		return importResult{}, fmt.Errorf("source digest mismatch: computed %s", digest)
	}
	if err = resetIdentitySequences(ctx, target); err != nil {
		return importResult{}, err
	}
	if err = refreshCatalogCounters(ctx, target); err != nil {
		return importResult{}, err
	}
	result, err := reconcileWhitelistTx(ctx, target, config.runID)
	if err != nil {
		return importResult{}, err
	}
	report, err := json.Marshal(result)
	if err != nil {
		return importResult{}, err
	}
	if _, err = target.Exec(ctx, `UPDATE public.whitelist_import_runs SET state='completed',completed_at=$2,report=$3 WHERE id=$1 AND state='running'`, config.runID, time.Now().UTC(), report); err != nil {
		return importResult{}, err
	}
	if err = target.Commit(ctx); err != nil {
		return importResult{}, err
	}
	return importResult{RunID: config.runID, SourceDigest: digest, Counts: counts, Reconcile: result}, nil
}

type receiptWriter func(context.Context, pgx.Tx, string, copySpec, string, []byte, string) error

func walkSource(ctx context.Context, source, target pgx.Tx, questionnaireCustomers questionnaireResolution, sourceBinding, runID string, writeReceipt receiptWriter) (string, map[string]int64, error) {
	hasher := sha256.New()
	writeDigest(hasher, "v1_archive_binding", sourceBinding, []byte(sourceBinding))
	counts := map[string]int64{}
	customerIDs, err := importCustomers(ctx, source, target, hasher, counts, questionnaireCustomers, runID, writeReceipt)
	if err != nil {
		return "", nil, err
	}
	if err = verifyRequiredIdentities(ctx, source, customerIDs, questionnaireCustomers.byOrder); err != nil {
		return "", nil, err
	}
	for _, spec := range whitelistCopySpecs {
		rows, queryErr := source.Query(ctx, spec.query)
		if queryErr != nil {
			return "", nil, fmt.Errorf("read %s: %w", spec.sourceEntity, queryErr)
		}
		for rows.Next() {
			var key string
			var sourcePayload, targetPayload []byte
			if queryErr = rows.Scan(&key, &sourcePayload, &targetPayload); queryErr != nil {
				rows.Close()
				return "", nil, queryErr
			}
			writeDigest(hasher, spec.sourceEntity, key, sourcePayload)
			if spec.transform != nil {
				mappings := questionnaireCustomers.bySubmission
				if spec.sourceEntity == "order_list_projections" {
					mappings = questionnaireCustomers.byOrder
				}
				targetPayload, queryErr = spec.transform(key, targetPayload, mappings)
				if queryErr != nil {
					rows.Close()
					return "", nil, queryErr
				}
			}
			if target != nil {
				statement := fmt.Sprintf(`INSERT INTO public.%s OVERRIDING SYSTEM VALUE SELECT (jsonb_populate_record(NULL::public.%s,$1::jsonb)).*`, spec.targetTable, spec.targetTable)
				if _, queryErr = target.Exec(ctx, statement, targetPayload); queryErr != nil {
					rows.Close()
					return "", nil, fmt.Errorf("write %s: %w", spec.targetTable, queryErr)
				}
				if spec.domain != "" && writeReceipt != nil {
					if queryErr = writeReceipt(ctx, target, runID, spec, key, sourcePayload, key); queryErr != nil {
						rows.Close()
						return "", nil, queryErr
					}
				}
			}
			if spec.domain != "" {
				counts[spec.domain]++
			}
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return "", nil, queryErr
		}
		rows.Close()
	}
	if err = recordSkippedRefunds(ctx, source, target, hasher, counts, runID, writeReceipt); err != nil {
		return "", nil, err
	}
	if err = recordRejectedOrders(ctx, source, target, hasher, counts, runID); err != nil {
		return "", nil, err
	}
	if err = recordSkippedAudiences(ctx, source, target, hasher, counts, runID, writeReceipt); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), counts, nil
}

func recordRejectedOrders(ctx context.Context, source, target pgx.Tx, hasher hash.Hash, counts map[string]int64, runID string) error {
	for _, rejected := range []struct {
		sourceEntity string
		targetTable  string
		query        string
		reason       string
	}{
		{"order_list_projections", "order_list_projections", `SELECT id::text,to_jsonb(item) FROM public.order_list_projections AS item WHERE product_id IS NULL ORDER BY id`, "order has no exact product relation"},
		{"order_historical_refunds", "order_refund_facts", `
SELECT refund.id::text,to_jsonb(refund)
FROM public.order_historical_refunds AS refund
JOIN public.order_list_projections AS orders ON orders.id=refund.order_id
WHERE orders.product_id IS NULL
  AND lower(refund.status) IN ('completed','success','succeeded','refunded','failed','rejected','cancelled')
ORDER BY refund.id`, "parent order has no exact product relation"},
	} {
		rows, err := source.Query(ctx, rejected.query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var key string
			var payload []byte
			if err = rows.Scan(&key, &payload); err != nil {
				rows.Close()
				return err
			}
			writeDigest(hasher, rejected.sourceEntity+".rejected", key, payload)
			counts["order_rejected"]++
			if target != nil {
				spec := copySpec{domain: "order", sourceEntity: rejected.sourceEntity, targetTable: rejected.targetTable}
				if err = insertRejectedReceipt(ctx, target, runID, spec, key, payload, rejected.reason); err != nil {
					rows.Close()
					return err
				}
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

func recordSkippedRefunds(ctx context.Context, source, target pgx.Tx, hasher hash.Hash, counts map[string]int64, runID string, writeReceipt receiptWriter) error {
	rows, err := source.Query(ctx, `
SELECT refund.id::text,to_jsonb(refund)
FROM public.order_historical_refunds AS refund
WHERE lower(refund.status) NOT IN ('completed','success','succeeded','refunded','failed','rejected','cancelled')
ORDER BY refund.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var payload []byte
		if err = rows.Scan(&key, &payload); err != nil {
			return err
		}
		writeDigest(hasher, "order_historical_refunds.skipped", key, payload)
		counts["order_skipped"]++
		if target != nil && writeReceipt != nil {
			spec := copySpec{domain: "order", sourceEntity: "order_historical_refunds", targetTable: "order_refund_facts"}
			if err = insertSkippedReceipt(ctx, target, runID, spec, key, payload, "refund is not a final provider fact"); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func importCustomers(ctx context.Context, source, target pgx.Tx, hasher hash.Hash, counts map[string]int64, questionnaireCustomers questionnaireResolution, runID string, writeReceipt receiptWriter) (map[int64]int64, error) {
	questionnaireCustomerIDs := make([]int64, 0, len(questionnaireCustomers.bySubmission)+len(questionnaireCustomers.byOrder))
	seen := map[int64]struct{}{}
	for _, mappings := range []map[int64]int64{questionnaireCustomers.bySubmission, questionnaireCustomers.byOrder} {
		for _, customerID := range mappings {
			if _, ok := seen[customerID]; !ok {
				seen[customerID] = struct{}{}
				questionnaireCustomerIDs = append(questionnaireCustomerIDs, customerID)
			}
		}
	}
	sort.Slice(questionnaireCustomerIDs, func(i, j int) bool { return questionnaireCustomerIDs[i] < questionnaireCustomerIDs[j] })
	rows, err := source.Query(ctx, `
SELECT customer.id,to_jsonb(customer),customer.created_at,customer.updated_at
FROM public.customers AS customer
WHERE customer.id IN (
  SELECT customer_id FROM public.order_list_projections WHERE customer_id IS NOT NULL
  UNION SELECT customer_id FROM public.product_local_entitlements
	  UNION SELECT customer_id FROM public.service_period_members
	  UNION SELECT member.customer_id FROM public.segment_members AS member JOIN public.ai_audience_package_metadata AS metadata ON metadata.segment_id=member.segment_id
	  UNION SELECT customer_id FROM public.hxc_user_current WHERE customer_id IS NOT NULL
	  UNION SELECT unnest($1::bigint[]))
ORDER BY customer.id`, questionnaireCustomerIDs)
	if err != nil {
		return nil, err
	}
	ids := map[int64]int64{}
	for rows.Next() {
		var id int64
		var payload []byte
		var createdAt, updatedAt time.Time
		if err = rows.Scan(&id, &payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		key := fmt.Sprint(id)
		writeDigest(hasher, "customers", key, payload)
		ids[id] = id
		if target == nil {
			counts["identity"]++
			continue
		}
		if _, err = target.Exec(ctx, `INSERT INTO public.customers(id,state,created_at,updated_at) OVERRIDING SYSTEM VALUE VALUES($1,'active',$2,$3)`, id, createdAt, updatedAt); err != nil {
			return nil, err
		}
		reference := digestBytes("aicrm_v2_frozen\x00customer\x00" + key)
		if _, err = target.Exec(ctx, `INSERT INTO public.source_subject_refs(customer_id,source_system,source_entity,reference_digest,assurance,created_at) VALUES($1,'aicrm_v2_frozen','customer',$2,'legacy_stable',$3)`, id, reference, time.Now().UTC()); err != nil {
			return nil, err
		}
		if writeReceipt != nil {
			spec := copySpec{domain: "identity", sourceEntity: "customers", targetTable: "customers"}
			if err = writeReceipt(ctx, target, runID, spec, key, payload, key); err != nil {
				return nil, err
			}
		}
		counts["identity"]++
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, customer := range questionnaireCustomers.synthetic {
		payload, marshalErr := json.Marshal(struct {
			ID        int64     `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		}{customer.id, customer.createdAt, customer.updatedAt})
		if marshalErr != nil {
			return nil, marshalErr
		}
		key := fmt.Sprint(customer.id)
		writeDigest(hasher, "questionnaire_subjects", key, payload)
		ids[customer.id] = customer.id
		if target != nil {
			if _, err = target.Exec(ctx, `INSERT INTO public.customers(id,state,created_at,updated_at) OVERRIDING SYSTEM VALUE VALUES($1,'active',$2,$3)`, customer.id, customer.createdAt, customer.updatedAt); err != nil {
				return nil, err
			}
			if writeReceipt != nil {
				spec := copySpec{domain: "identity", sourceEntity: "questionnaire_subjects", targetTable: "customers"}
				if err = writeReceipt(ctx, target, runID, spec, key, payload, key); err != nil {
					return nil, err
				}
			}
		}
		counts["identity"]++
	}
	customerIDs := make([]int64, 0, len(questionnaireCustomers.references))
	for customerID := range questionnaireCustomers.references {
		customerIDs = append(customerIDs, customerID)
	}
	sort.Slice(customerIDs, func(i, j int) bool { return customerIDs[i] < customerIDs[j] })
	for _, customerID := range customerIDs {
		for _, reference := range questionnaireCustomers.references[customerID] {
			digest := digestBytes("aicrm_v2_frozen\x00" + reference.entity + "\x00" + reference.value)
			key := fmt.Sprintf("%d:%s:%s", customerID, reference.entity, hex.EncodeToString(digest))
			writeDigest(hasher, "source_subject_refs", key, digest)
			if target != nil {
				if _, err = target.Exec(ctx, `INSERT INTO public.source_subject_refs(customer_id,source_system,source_entity,reference_digest,assurance,created_at) VALUES($1,'aicrm_v2_frozen',$2,$3,'legacy_stable',$4)`, customerID, reference.entity, digest, time.Now().UTC()); err != nil {
					return nil, err
				}
			}
		}
	}
	return ids, nil
}

func resolveQuestionnaireCustomers(ctx context.Context, source pgx.Tx) (questionnaireResolution, error) {
	var maxCustomerID int64
	if err := source.QueryRow(ctx, `SELECT COALESCE(max(id),0) FROM public.customers`).Scan(&maxCustomerID); err != nil {
		return questionnaireResolution{}, err
	}
	rows, err := source.Query(ctx, `
SELECT submission.id,count(DISTINCT identity.customer_id),min(identity.customer_id),
  submission.unionid,submission.external_userid,submission.openid,submission.mobile,submission.created_at
FROM public.questionnaire_submissions AS submission
LEFT JOIN public.identities AS identity ON identity.customer_id IS NOT NULL AND (
  (submission.unionid<>'' AND identity.kind='unionid' AND identity.normalized_value=submission.unionid) OR
  (submission.external_userid<>'' AND identity.kind='wecom_external_userid' AND identity.normalized_value=submission.external_userid) OR
  (submission.openid<>'' AND identity.kind IN ('mp_openid','oa_openid') AND identity.normalized_value=submission.openid) OR
  (submission.mobile<>'' AND identity.kind='phone' AND identity.normalized_value=submission.mobile))
GROUP BY submission.id,submission.unionid,submission.external_userid,submission.openid,submission.mobile,submission.created_at
ORDER BY submission.id`)
	if err != nil {
		return questionnaireResolution{}, err
	}
	defer rows.Close()
	resolvedRows := []questionnaireResolutionRow{}
	for rows.Next() {
		row := questionnaireResolutionRow{recordType: "questionnaire"}
		var customerID pgtype.Int8
		if err = rows.Scan(&row.submissionID, &row.matchCount, &customerID, &row.unionID, &row.externalUserID, &row.openID, &row.mobile, &row.createdAt); err != nil {
			return questionnaireResolution{}, err
		}
		if customerID.Valid {
			row.existingCustomer = customerID.Int64
		}
		resolvedRows = append(resolvedRows, row)
	}
	if err = rows.Err(); err != nil {
		return questionnaireResolution{}, err
	}
	rows.Close()
	orderRows, err := source.Query(ctx, `
SELECT orders.id,count(DISTINCT identity.customer_id),min(identity.customer_id),
  orders.identity_kind,orders.identity_value,orders.created_at
FROM public.order_list_projections AS orders
LEFT JOIN public.identities AS identity ON orders.customer_id IS NULL AND identity.customer_id IS NOT NULL AND (
  (orders.identity_kind='unionid' AND identity.kind='unionid' AND identity.normalized_value=orders.identity_value) OR
  (orders.identity_kind IN ('external_userid','userid') AND identity.kind='wecom_external_userid' AND identity.normalized_value=orders.identity_value))
WHERE orders.product_id IS NOT NULL AND orders.customer_id IS NULL
GROUP BY orders.id,orders.identity_kind,orders.identity_value,orders.created_at
ORDER BY orders.id`)
	if err != nil {
		return questionnaireResolution{}, err
	}
	defer orderRows.Close()
	for orderRows.Next() {
		row := questionnaireResolutionRow{recordType: "order"}
		var identityKind, identityValue string
		var customerID pgtype.Int8
		if err = orderRows.Scan(&row.submissionID, &row.matchCount, &customerID, &identityKind, &identityValue, &row.createdAt); err != nil {
			return questionnaireResolution{}, err
		}
		if customerID.Valid {
			row.existingCustomer = customerID.Int64
		}
		switch identityKind {
		case "unionid":
			row.unionID = identityValue
		case "external_userid", "userid":
			row.externalUserID = identityValue
		default:
			if identityValue != "" {
				return questionnaireResolution{}, fmt.Errorf("order %d has unsupported identity kind %q", row.submissionID, identityKind)
			}
		}
		resolvedRows = append(resolvedRows, row)
	}
	if err = orderRows.Err(); err != nil {
		return questionnaireResolution{}, err
	}
	return resolveQuestionnaireRows(resolvedRows, maxCustomerID)
}

func resolveQuestionnaireRows(rows []questionnaireResolutionRow, maxCustomerID int64) (questionnaireResolution, error) {
	result := questionnaireResolution{bySubmission: map[int64]int64{}, byOrder: map[int64]int64{}, references: map[int64][]questionnaireReference{}}
	referenceOwners := map[string]int64{}
	referenceSeen := map[int64]map[string]struct{}{}
	syntheticIndexes := map[int64]int{}
	nextCustomerID := maxCustomerID + 1
	for _, row := range rows {
		if row.matchCount > 1 || (row.matchCount == 1 && row.existingCustomer < 1) {
			return questionnaireResolution{}, fmt.Errorf("%s %d identity mapping count is %d", row.recordType, row.submissionID, row.matchCount)
		}
		references := questionnaireReferences(row)
		candidates := map[int64]struct{}{}
		if row.matchCount == 1 {
			candidates[row.existingCustomer] = struct{}{}
		}
		for _, reference := range references {
			if owner := referenceOwners[reference.entity+"\x00"+reference.value]; owner > 0 {
				candidates[owner] = struct{}{}
			}
		}
		if len(candidates) > 1 {
			return questionnaireResolution{}, fmt.Errorf("%s %d has conflicting subject references", row.recordType, row.submissionID)
		}
		customerID := int64(0)
		for candidate := range candidates {
			customerID = candidate
		}
		if customerID == 0 {
			customerID = nextCustomerID
			nextCustomerID++
			syntheticIndexes[customerID] = len(result.synthetic)
			result.synthetic = append(result.synthetic, syntheticCustomer{id: customerID, createdAt: row.createdAt, updatedAt: row.createdAt})
		} else if index, ok := syntheticIndexes[customerID]; ok && row.createdAt.After(result.synthetic[index].updatedAt) {
			result.synthetic[index].updatedAt = row.createdAt
		}
		if row.recordType == "order" {
			result.byOrder[row.submissionID] = customerID
		} else {
			result.bySubmission[row.submissionID] = customerID
		}
		if referenceSeen[customerID] == nil {
			referenceSeen[customerID] = map[string]struct{}{}
		}
		for _, reference := range references {
			key := reference.entity + "\x00" + reference.value
			referenceOwners[key] = customerID
			if _, ok := referenceSeen[customerID][key]; ok {
				continue
			}
			referenceSeen[customerID][key] = struct{}{}
			result.references[customerID] = append(result.references[customerID], reference)
		}
	}
	return result, nil
}

func questionnaireReferences(row questionnaireResolutionRow) []questionnaireReference {
	references := make([]questionnaireReference, 0, 4)
	for _, candidate := range []questionnaireReference{
		{entity: "unionid", value: row.unionID},
		{entity: "wecom_external_userid", value: row.externalUserID},
		{entity: "openid", value: row.openID},
		{entity: "phone", value: row.mobile},
	} {
		if candidate.value != "" {
			references = append(references, candidate)
		}
	}
	if len(references) == 0 {
		entity := "questionnaire_submission"
		if row.recordType == "order" {
			entity = "order"
		}
		references = append(references, questionnaireReference{entity: entity, value: fmt.Sprint(row.submissionID)})
	}
	return references
}

func addOrderCustomer(key string, payload []byte, mappings map[int64]int64) ([]byte, error) {
	var orderID int64
	if _, err := fmt.Sscan(key, &orderID); err != nil {
		return nil, errors.New("invalid order id")
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	if value["customer_id"] != nil {
		return payload, nil
	}
	customerID, ok := mappings[orderID]
	if !ok {
		return nil, fmt.Errorf("order %d has no customer mapping", orderID)
	}
	value["customer_id"] = customerID
	return json.Marshal(value)
}

func addSubmissionCustomer(key string, payload []byte, mappings map[int64]int64) ([]byte, error) {
	var submissionID int64
	if _, err := fmt.Sscan(key, &submissionID); err != nil {
		return nil, errors.New("invalid questionnaire submission id")
	}
	customerID, ok := mappings[submissionID]
	if !ok {
		return nil, fmt.Errorf("questionnaire submission %d has no customer mapping", submissionID)
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	value["customer_id"] = customerID
	return json.Marshal(value)
}

func verifyRequiredIdentities(ctx context.Context, source pgx.Tx, customerIDs map[int64]int64, orderCustomers map[int64]int64) error {
	var invalidEntitlements, invalidMembers int64
	if err := source.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.product_local_entitlements WHERE customer_id IS NULL),
	  (SELECT count(*) FROM public.service_period_members WHERE customer_id IS NULL)`).Scan(&invalidEntitlements, &invalidMembers); err != nil {
		return err
	}
	if invalidEntitlements != 0 || invalidMembers != 0 {
		return fmt.Errorf("required identity mapping failed: entitlements=%d memberships=%d", invalidEntitlements, invalidMembers)
	}
	orderRows, err := source.Query(ctx, `SELECT id,customer_id FROM public.order_list_projections WHERE product_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for orderRows.Next() {
		var orderID int64
		var sourceCustomerID pgtype.Int8
		if err = orderRows.Scan(&orderID, &sourceCustomerID); err != nil {
			orderRows.Close()
			return err
		}
		customerID := sourceCustomerID.Int64
		if !sourceCustomerID.Valid {
			customerID = orderCustomers[orderID]
		}
		if customerID < 1 || customerIDs[customerID] < 1 {
			orderRows.Close()
			return fmt.Errorf("order %d is missing a required customer", orderID)
		}
	}
	if err = orderRows.Err(); err != nil {
		orderRows.Close()
		return err
	}
	orderRows.Close()
	rows, err := source.Query(ctx, `
SELECT DISTINCT customer_id FROM (
	  SELECT customer_id FROM public.product_local_entitlements
	  UNION ALL SELECT customer_id FROM public.service_period_members
) AS required ORDER BY customer_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return err
		}
		if _, ok := customerIDs[id]; !ok {
			return fmt.Errorf("required customer %d is missing", id)
		}
	}
	return rows.Err()
}

func recordSkippedAudiences(ctx context.Context, source, target pgx.Tx, hasher hash.Hash, counts map[string]int64, runID string, writeReceipt receiptWriter) error {
	currentRows, err := source.Query(ctx, `
SELECT metadata.segment_id::text,to_jsonb(metadata)
FROM public.ai_audience_package_metadata AS metadata
WHERE EXISTS (
  SELECT 1 FROM public.segment_members AS member
  LEFT JOIN public.customers AS customer ON customer.id=member.customer_id
  WHERE member.segment_id=metadata.segment_id AND customer.id IS NULL)
ORDER BY metadata.segment_id`)
	if err != nil {
		return err
	}
	for currentRows.Next() {
		var key string
		var payload []byte
		if err = currentRows.Scan(&key, &payload); err != nil {
			currentRows.Close()
			return err
		}
		writeDigest(hasher, "ai_audience_package_metadata.skipped", key, payload)
		counts["audience_skipped"]++
		if target != nil && writeReceipt != nil {
			spec := copySpec{domain: "audience", sourceEntity: "ai_audience_package_metadata", targetTable: "segments"}
			if err = insertSkippedReceipt(ctx, target, runID, spec, key, payload, "package member mapping is incomplete; recreate with V2 audience logic"); err != nil {
				currentRows.Close()
				return err
			}
		}
	}
	if err = currentRows.Err(); err != nil {
		currentRows.Close()
		return err
	}
	currentRows.Close()

	rows, err := source.Query(ctx, `
SELECT package.id::text,to_jsonb(package)
FROM public.segment_v1_audience_packages AS package
WHERE package.original_status='active'
  AND NOT EXISTS (SELECT 1 FROM public.ai_audience_v1_editable_package_projections AS projection WHERE projection.package_history_id=package.id)
ORDER BY package.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var payload []byte
		if err = rows.Scan(&key, &payload); err != nil {
			return err
		}
		writeDigest(hasher, "segment_v1_audience_packages.skipped", key, payload)
		counts["audience_skipped"]++
		if target != nil && writeReceipt != nil {
			spec := copySpec{domain: "audience", sourceEntity: "segment_v1_audience_packages", targetTable: "segments"}
			if err = insertSkippedReceipt(ctx, target, runID, spec, key, payload, "identity mapping or legacy rule requires redesign"); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func insertReceipt(ctx context.Context, target pgx.Tx, runID string, spec copySpec, key string, payload []byte, targetID string) error {
	_, err := target.Exec(ctx, `INSERT INTO public.whitelist_import_domain_receipts
(run_id,domain,source_entity,source_key_digest,source_payload_digest,target_entity,target_id,disposition,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,'MIGRATE',$8)`, runID, spec.domain, spec.sourceEntity, digestBytes(spec.sourceEntity+"\x00"+key), digestBytes(string(payload)), spec.targetTable, targetID, time.Now().UTC())
	return err
}

func insertSkippedReceipt(ctx context.Context, target pgx.Tx, runID string, spec copySpec, key string, payload []byte, reason string) error {
	_, err := target.Exec(ctx, `INSERT INTO public.whitelist_import_domain_receipts
(run_id,domain,source_entity,source_key_digest,source_payload_digest,target_entity,target_id,disposition,reason,created_at)
VALUES($1,$2,$3,$4,$5,$6,NULL,'ARCHIVE_NOT_ON_V2',$7,$8)`, runID, spec.domain, spec.sourceEntity, digestBytes(spec.sourceEntity+"\x00"+key), digestBytes(string(payload)), spec.targetTable, reason, time.Now().UTC())
	return err
}

func insertRejectedReceipt(ctx context.Context, target pgx.Tx, runID string, spec copySpec, key string, payload []byte, reason string) error {
	_, err := target.Exec(ctx, `INSERT INTO public.whitelist_import_domain_receipts
(run_id,domain,source_entity,source_key_digest,source_payload_digest,target_entity,target_id,disposition,reason,created_at)
VALUES($1,$2,$3,$4,$5,$6,NULL,'REJECT',$7,$8)`, runID, spec.domain, spec.sourceEntity, digestBytes(spec.sourceEntity+"\x00"+key), digestBytes(string(payload)), spec.targetTable, reason, time.Now().UTC())
	return err
}

func writeDigest(hasher hash.Hash, entity, key string, payload []byte) {
	hasher.Write([]byte(entity))
	hasher.Write([]byte{0})
	hasher.Write([]byte(key))
	hasher.Write([]byte{0})
	hasher.Write(payload)
	hasher.Write([]byte{'\n'})
}

func digestBytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func validateSource(ctx context.Context, source pgx.Tx, archiveRunID string, allowTestSource bool) (string, error) {
	var version int64
	if err := source.QueryRow(ctx, `SELECT COALESCE(max(version_id),0) FROM public.goose_db_version WHERE is_applied`).Scan(&version); err != nil || version < 145 {
		return "", fmt.Errorf("source schema 145 is required")
	}
	if allowTestSource {
		return "test-source-schema-145", nil
	}
	var snapshotDigest []byte
	var tableCount, archivedTableCount int
	var rowCount, archiveRecordCount, terminalDispositionCount int64
	if err := source.QueryRow(ctx, `SELECT run.snapshot_digest,run.table_count,run.row_count,
  receipt.archived_table_count,receipt.archive_record_count,receipt.terminal_disposition_count
FROM public.v1_archive_runs AS run
JOIN public.v1_archive_reconciliation_receipts AS receipt USING(run_id)
WHERE run.run_id=$1`, archiveRunID).Scan(&snapshotDigest, &tableCount, &rowCount, &archivedTableCount, &archiveRecordCount, &terminalDispositionCount); err != nil {
		return "", errors.New("sealed V1 archive run is required")
	}
	if len(snapshotDigest) != sha256.Size || tableCount < 1 || tableCount != archivedTableCount || rowCount != archiveRecordCount || rowCount != terminalDispositionCount {
		return "", errors.New("V1 archive reconciliation is incomplete")
	}
	return archiveRunID + ":" + hex.EncodeToString(snapshotDigest), nil
}

func validateTarget(ctx context.Context, target pgx.Tx, allowTestName bool) error {
	var database string
	var version int
	if err := target.QueryRow(ctx, `SELECT current_database(),version FROM public.whitelist_schema_version WHERE singleton`).Scan(&database, &version); err != nil || version != 2 {
		return errors.New("target whitelist schema version 2 is required")
	}
	if database != "aicrm_v2_core" && !allowTestName {
		return errors.New("target database must be named aicrm_v2_core")
	}
	var existing int64
	if err := target.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM public.whitelist_import_runs)+
  (SELECT count(*) FROM public.customers)+
  (SELECT count(*) FROM public.products)+
  (SELECT count(*) FROM public.order_list_projections)+
  (SELECT count(*) FROM public.questionnaires)+
  (SELECT count(*) FROM public.segments)+
  (SELECT count(*) FROM public.automation_agent_configurations)+
  (SELECT count(*) FROM public.external_effects)`).Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("target whitelist database is not empty")
	}
	return nil
}

var identitySequenceTables = []string{"admin_sessions", "admin_users", "channels", "products", "order_list_projections", "product_local_entitlements", "questionnaires", "questionnaire_questions", "questionnaire_options", "questionnaire_submissions", "questionnaire_submission_answers", "service_period_member_views", "service_period_members", "radar_links", "ai_audience_package_groups", "segments", "automation_agent_configurations", "customers"}

func resetIdentitySequences(ctx context.Context, target pgx.Tx) error {
	tables := append([]string(nil), identitySequenceTables...)
	sort.Strings(tables)
	for _, table := range tables {
		var sequence pgtype.Text
		if err := target.QueryRow(ctx, `SELECT pg_get_serial_sequence($1,'id')`, "public."+table).Scan(&sequence); err != nil {
			return fmt.Errorf("find %s sequence: %w", table, err)
		}
		if !sequence.Valid {
			continue
		}
		statement := fmt.Sprintf(`SELECT setval($1,GREATEST(COALESCE((SELECT max(id) FROM public.%s),0),1),COALESCE((SELECT max(id) FROM public.%s),0)>0)`, table, table)
		if _, err := target.Exec(ctx, statement, sequence.String); err != nil {
			return fmt.Errorf("reset %s sequence: %w", table, err)
		}
	}
	return nil
}

func refreshCatalogCounters(ctx context.Context, target pgx.Tx) error {
	statements := []string{
		`UPDATE public.product_catalog_counters SET total_products=(SELECT count(*) FROM public.products) WHERE singleton`,
		`UPDATE public.order_list_projection_counters SET total_orders=(SELECT count(*) FROM public.order_list_projections) WHERE singleton`,
		`UPDATE public.questionnaire_catalog_counters SET total_questionnaires=(SELECT count(*) FROM public.questionnaires) WHERE singleton`,
	}
	for _, statement := range statements {
		if _, err := target.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

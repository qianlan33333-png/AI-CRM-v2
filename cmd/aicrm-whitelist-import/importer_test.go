package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSubmissionCustomerIsAddedWithoutLegacyIdentity(t *testing.T) {
	payload, err := addSubmissionCustomer("41", []byte(`{"id":41,"openid":"","mobile":""}`), map[int64]int64{41: 73})
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err = json.Unmarshal(payload, &row); err != nil {
		t.Fatal(err)
	}
	if row["customer_id"] != float64(73) {
		t.Fatalf("customer_id=%v", row["customer_id"])
	}
}

func TestQuestionnaireSubjectsUseStableReferencesAndSubmissionScope(t *testing.T) {
	createdAt := time.Unix(10, 0).UTC()
	resolution, err := resolveQuestionnaireRows([]questionnaireResolutionRow{
		{submissionID: 1, unionID: "union-one", createdAt: createdAt},
		{submissionID: 2, unionID: "union-one", createdAt: createdAt.Add(time.Minute)},
		{submissionID: 3, createdAt: createdAt.Add(2 * time.Minute)},
	}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.bySubmission[1] != 101 || resolution.bySubmission[2] != 101 {
		t.Fatalf("stable unionid was not reused: %#v", resolution.bySubmission)
	}
	if resolution.bySubmission[3] != 102 {
		t.Fatalf("anonymous submission customer=%d", resolution.bySubmission[3])
	}
	if len(resolution.synthetic) != 2 || resolution.synthetic[0].updatedAt != createdAt.Add(time.Minute) {
		t.Fatalf("synthetic customers=%#v", resolution.synthetic)
	}
	if got := resolution.references[102]; len(got) != 1 || got[0].entity != "questionnaire_submission" || got[0].value != "3" {
		t.Fatalf("anonymous reference=%#v", got)
	}
}

func TestOrderSubjectSharesUnionIDWithQuestionnaire(t *testing.T) {
	createdAt := time.Unix(20, 0).UTC()
	resolution, err := resolveQuestionnaireRows([]questionnaireResolutionRow{
		{recordType: "questionnaire", submissionID: 1, unionID: "shared", createdAt: createdAt},
		{recordType: "order", submissionID: 9, unionID: "shared", createdAt: createdAt.Add(time.Minute)},
	}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.bySubmission[1] != 51 || resolution.byOrder[9] != 51 || len(resolution.synthetic) != 1 {
		t.Fatalf("resolution=%#v", resolution)
	}
}

func TestOrderCustomerIsAddedAndInvalidProductsAreExcluded(t *testing.T) {
	payload, err := addOrderCustomer("9", []byte(`{"id":9,"customer_id":null}`), map[int64]int64{9: 51})
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]any
	if err = json.Unmarshal(payload, &row); err != nil {
		t.Fatal(err)
	}
	if row["customer_id"] != float64(51) {
		t.Fatalf("customer_id=%v", row["customer_id"])
	}
	for _, spec := range whitelistCopySpecs {
		if spec.sourceEntity == "order_list_projections" && !strings.Contains(spec.query, "product_id IS NOT NULL") {
			t.Fatal("orders without an exact product relation are not excluded")
		}
		if spec.sourceEntity == "order_historical_refunds" && !strings.Contains(spec.query, "orders.product_id IS NOT NULL") {
			t.Fatal("refunds for rejected orders are not excluded")
		}
	}
}

func TestProductCopyClearsLegacyReferencesWithoutBreakingCanonicalProjection(t *testing.T) {
	for _, spec := range whitelistCopySpecs {
		if spec.sourceEntity != "products" {
			continue
		}
		for _, cleared := range []string{"'lead_program_id',null", "'lead_channel_id',null", "'completion_target',null", "'wecom_tagging','{}'::jsonb"} {
			if !strings.Contains(spec.query, cleared) {
				t.Fatalf("product projection does not explicitly clear %s", cleared)
			}
		}
		for _, removed := range []string{"image_ids", "material_ids"} {
			if !strings.Contains(spec.query, removed) {
				t.Fatalf("product projection does not remove %s", removed)
			}
		}
		return
	}
	t.Fatal("product copy spec is missing")
}

func TestReconciliationAcceptsExplicitlyClearedProductReferences(t *testing.T) {
	payload, err := os.ReadFile("reconcile.go")
	if err != nil {
		t.Fatal(err)
	}
	query := string(payload)
	if strings.Contains(query, "legacy_admin_projection ?| ARRAY['lead_program_id','lead_channel_id','completion_target','wecom_tagging','image_ids','material_ids']") {
		t.Fatal("reconciliation still treats canonical null keys as legacy references")
	}
	for _, cleared := range []string{
		"COALESCE(legacy_admin_projection->'lead_program_id','null'::jsonb)<>'null'::jsonb",
		"COALESCE(legacy_admin_projection->'lead_channel_id','null'::jsonb)<>'null'::jsonb",
		"COALESCE(legacy_admin_projection->'completion_target','null'::jsonb)<>'null'::jsonb",
		"COALESCE(legacy_admin_projection->'wecom_tagging','null'::jsonb) NOT IN ('null'::jsonb,'{}'::jsonb,'[]'::jsonb)",
	} {
		if !strings.Contains(query, cleared) {
			t.Fatalf("reconciliation is missing cleared-reference predicate %s", cleared)
		}
	}
}

func TestReconciliationAcceptsEmptyChannelReferenceDefaults(t *testing.T) {
	payload, err := os.ReadFile("reconcile.go")
	if err != nil {
		t.Fatal(err)
	}
	query := string(payload)
	if strings.Contains(query, "channels WHERE config ?|") {
		t.Fatal("reconciliation still treats empty channel reference keys as references")
	}
	for _, key := range []string{"staff_id", "tag_ids", "welcome_material_id", "welcome_message", "material_ids"} {
		predicate := "COALESCE(config->'" + key + "','null'::jsonb) NOT IN ('null'::jsonb,'\"\"'::jsonb,'[]'::jsonb,'{}'::jsonb)"
		if !strings.Contains(query, predicate) {
			t.Fatalf("reconciliation is missing semantic channel predicate %s", key)
		}
	}
}

func TestQuestionnaireSubjectKeepsExistingCustomer(t *testing.T) {
	resolution, err := resolveQuestionnaireRows([]questionnaireResolutionRow{{
		submissionID: 7, matchCount: 1, existingCustomer: 42, unionID: "known", createdAt: time.Unix(1, 0).UTC(),
	}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.bySubmission[7] != 42 || len(resolution.synthetic) != 0 {
		t.Fatalf("resolution=%#v", resolution)
	}
}

func TestQuestionnaireSubjectRejectsConflictingReferences(t *testing.T) {
	_, err := resolveQuestionnaireRows([]questionnaireResolutionRow{
		{submissionID: 1, unionID: "one", createdAt: time.Unix(1, 0).UTC()},
		{submissionID: 2, mobile: "two", createdAt: time.Unix(2, 0).UTC()},
		{submissionID: 3, unionID: "one", mobile: "two", createdAt: time.Unix(3, 0).UTC()},
	}, 10)
	if err == nil || !strings.Contains(err.Error(), "conflicting subject references") {
		t.Fatalf("error=%v", err)
	}
}

func TestAudienceCopySpecsExcludeIncompletePackages(t *testing.T) {
	for _, spec := range whitelistCopySpecs {
		if spec.domain != "audience" {
			continue
		}
		if spec.sourceEntity == "ai_audience_package_groups" || spec.sourceEntity == "segments" ||
			spec.sourceEntity == "ai_audience_package_metadata" || spec.sourceEntity == "ai_audience_package_configuration_versions" ||
			spec.sourceEntity == "segment_members" {
			if !strings.Contains(spec.query, "customer.id IS NULL") {
				t.Fatalf("%s does not exclude packages with incomplete member mapping", spec.sourceEntity)
			}
		}
	}
}

func TestWhitelistTargetsContainNoLegacyHistory(t *testing.T) {
	for _, spec := range whitelistCopySpecs {
		target := strings.ToLower(spec.targetTable)
		if strings.Contains(target, "campaign") || strings.Contains(target, "message") || strings.Contains(target, "provider") || strings.Contains(target, "history") {
			t.Fatalf("legacy history target is registered: %s", spec.targetTable)
		}
	}
}

func TestSettingsCopyIsStrongTypedAndSecretFree(t *testing.T) {
	for _, spec := range whitelistCopySpecs {
		if spec.sourceEntity != "settings" {
			continue
		}
		for _, secret := range []string{"wecom.secret", "ai.api_key", "database.url", "jwt_secret"} {
			if strings.Contains(spec.query, secret) {
				t.Fatalf("settings copy includes secret key %s", secret)
			}
		}
		if !strings.Contains(spec.query, "outbound.max_attempts") {
			t.Fatal("settings copy does not use the strong-typed allowlist")
		}
		return
	}
	t.Fatal("settings copy spec is missing")
}

func TestIdentitySequenceResetIncludesImportedAdminSessions(t *testing.T) {
	for _, table := range identitySequenceTables {
		if table == "admin_sessions" {
			return
		}
	}
	t.Fatal("admin_sessions sequence is not reset after whitelist import")
}

func TestWhitelistBaselinePreservesCurrentCapabilitiesWithoutHistoricalRows(t *testing.T) {
	payload, err := os.ReadFile("../../schema/whitelist_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(payload)
	for _, required := range []string{
		"CREATE TABLE public.whitelist_schema_version",
		"CREATE TABLE public.customers",
		"CREATE TABLE public.source_subject_refs",
		"CREATE TABLE public.products",
		"CREATE TABLE public.order_refund_facts",
		"CREATE TABLE public.hxc_user_current",
		"CREATE TABLE public.river_job",
		"CREATE TABLE public.tag_groups",
		"CREATE TABLE public.tags",
		"CREATE TABLE public.coupons",
		"CREATE TABLE public.media_images",
		"CREATE TABLE public.media_attachments",
		"CREATE TABLE public.media_miniprograms",
		"CREATE TABLE public.group_ops_plans",
		"CREATE TABLE public.admin_ops_config_categories",
		"COPY public.product_catalog_counters",
		"COPY public.order_list_projection_counters",
		"COPY public.questionnaire_catalog_counters",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("baseline is missing %s", required)
		}
	}
	for _, historicalTable := range []string{
		"campaigns",
		"wecom_message_archive",
		"order_historical_refunds",
		"radar_link_events",
		"staff",
	} {
		copyStart := strings.Index(schema, "COPY public."+historicalTable+" ")
		if copyStart < 0 {
			continue
		}
		copyBody := schema[copyStart:]
		dataStart := strings.Index(copyBody, "\n")
		dataEnd := strings.Index(copyBody, "\n\\.\n")
		if dataStart < 0 || dataEnd < 0 || strings.TrimSpace(copyBody[dataStart:dataEnd]) != "" {
			t.Errorf("baseline contains historical rows for %s", historicalTable)
		}
	}
}

func TestWhitelistAuthenticationDoesNotDependOnLegacyStaffDirectory(t *testing.T) {
	payload, err := os.ReadFile("../../internal/auth/store/queries/auth.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(payload)
	for _, query := range []string{"FindAdminUserForVerifiedLogin", "GetActiveSession"} {
		start := strings.Index(schema, "-- name: "+query+" ")
		if start < 0 {
			t.Fatalf("missing auth query %s", query)
		}
		end := strings.Index(schema[start+1:], "-- name:")
		section := schema[start:]
		if end >= 0 {
			section = schema[start : start+1+end]
		}
		if strings.Contains(section, "JOIN staff") {
			t.Fatalf("whitelist auth query %s depends on legacy staff directory", query)
		}
	}
}

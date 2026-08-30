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

func TestWhitelistBaselineContainsOnlyCurrentBusinessBoundary(t *testing.T) {
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
		"INSERT INTO public.product_catalog_counters",
		"INSERT INTO public.order_list_projection_counters",
		"INSERT INTO public.questionnaire_catalog_counters",
	} {
		if !strings.Contains(schema, required) {
			t.Errorf("baseline is missing %s", required)
		}
	}
	for _, forbidden := range []string{
		"CREATE TABLE public.campaigns",
		"CREATE TABLE public.wecom_message_archive",
		"CREATE TABLE public.order_historical_refunds",
		"CREATE TABLE public.radar_link_events",
		"CREATE TABLE public.staff",
	} {
		if strings.Contains(schema, forbidden) {
			t.Errorf("baseline contains forbidden history table %s", forbidden)
		}
	}
}

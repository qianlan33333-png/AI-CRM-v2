package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
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

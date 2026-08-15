package main

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const specPath = "../../api/openapi.yaml"
const mappingPath = "../../docs/api-mapping.jsonl"

func TestFrozenOpenAPI(t *testing.T) {
	doc, ids, err := load(specPath, mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(doc, ids); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnsafeContractMutations(t *testing.T) {
	tests := map[string]func(*testing.T){
		"signoff regression": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-p1-signoff-status"] = "PENDING_HUMAN_SIGNOFF"
			reject(t, doc, ids)
		},
		"forged decision evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-p1-decision-evidence"] = "G1-D01-FORGED"
			reject(t, doc, ids)
		},
		"unknown legacy link": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-9999"}
			reject(t, doc, ids)
		},
		"identity in customer": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["Customer"].Value.Properties["external_userid"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"unscoped identity": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["IdentityRef"].Value.Required = []string{"type", "value", "assurance", "source"}
			reject(t, doc, ids)
		},
		"missing operation": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Delete("/api/v1/admin/config/overview")
			reject(t, doc, ids)
		},
		"browser JWT substitution": func(t *testing.T) {
			doc, ids := fresh(t)
			scheme := doc.Components.SecuritySchemes["AdminSession"].Value
			scheme.Type, scheme.In, scheme.Name, scheme.Scheme, scheme.BearerFormat = "http", "", "", "bearer", "JWT"
			reject(t, doc, ids)
		},
		"logout without CSRF": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/auth/logout").Post.Parameters = nil
			reject(t, doc, ids)
		},
		"logout without CSRF failure response": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/auth/logout").Post.Responses.Delete("403")
			reject(t, doc, ids)
		},
		"missing operation capability": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/customers").Get.Extensions, "x-aicrm-capability")
			reject(t, doc, ids)
		},
		"sales customer scope widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"config granted to ops": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/admin/config/overview").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global",
			}
			reject(t, doc, ids)
		},
		"settings manage granted to ops": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/config/app-settings").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{"admin": "global", "ops": "global"}
			reject(t, doc, ids)
		},
		"JSON settings resource write missing": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/config/app-settings").Put = nil
			reject(t, doc, ids)
		},
		"JSON settings resource action token missing": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/admin/config/app-settings").Put.Extensions, "x-aicrm-route-bound-action-token")
			reject(t, doc, ids)
		},
		"secret value exposed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyMaskedAppSetting"].Value.Properties["value"] = doc.Components.Schemas["AdminConfigEntry"]
			reject(t, doc, ids)
		},
		"secret form accepts replacement": func(t *testing.T) {
			doc, ids := fresh(t)
			maximum := uint64(100)
			doc.Components.Schemas["LegacyAppSettingsSaveForm"].Value.Properties["setting__database.url"].Value.MaxLength = &maximum
			reject(t, doc, ids)
		},
		"settings version derives audit state": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyEditableAppSetting"].Value.Properties["version"].Value.Enum = []any{"1"}
			reject(t, doc, ids)
		},
		"role denial without forbidden response": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Delete("403")
			reject(t, doc, ids)
		},
		"missing stage evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/stages").Get.Extensions, "x-p2-decision-evidence")
			reject(t, doc, ids)
		},
		"sales stage write widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/stages").Post.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"stage write without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/stages/{stage_id}").Patch.Parameters = nil
			reject(t, doc, ids)
		},
		"missing contact evidence": func(t *testing.T) {
			doc, ids := fresh(t)
			delete(doc.Paths.Value("/api/v1/customers").Get.Extensions, "x-p3-decision-evidence")
			reject(t, doc, ids)
		},
		"unknown contact legacy link": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-9999"}
			reject(t, doc, ids)
		},
		"sales tag scope widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
				"admin": "global", "ops": "global", "sales": "global",
			}
			reject(t, doc, ids)
		},
		"tag dependency response removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/tags").Get.Responses.Delete("503")
			reject(t, doc, ids)
		},
		"automation capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get.Extensions["x-aicrm-capability"] = "automation.write"
			reject(t, doc, ids)
		},
		"automation evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get.Extensions["x-p4-decision-evidence"] = "P4-W0-D01-FORGED"
			reject(t, doc, ids)
		},
		"automation identity filter removed": func(t *testing.T) {
			doc, ids := fresh(t)
			operation := doc.Paths.Value("/api/admin/automation-conversion/agent-runs").Get
			operation.Parameters = operation.Parameters[:7]
			reject(t, doc, ids)
		},
		"product evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products").Get.Extensions["x-p4-decision-evidence"] = "P4-I01A-FORGED"
			reject(t, doc, ids)
		},
		"product legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products/{product_id}").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0526"}
			reject(t, doc, ids)
		},
		"product capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/products").Get.Extensions["x-aicrm-capability"] = "products.write"
			reject(t, doc, ids)
		},
		"media evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-p4-decision-evidence"] = "P4-H01A1-FORGED"
			reject(t, doc, ids)
		},
		"media legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0362"}
			reject(t, doc, ids)
		},
		"media capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/image-library/upload").Post.Extensions["x-aicrm-capability"] = "media.images.read"
			reject(t, doc, ids)
		},
		"group invite evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library").Get.Extensions["x-p4-decision-evidence"] = "P4-H03-FORGED"
			reject(t, doc, ids)
		},
		"group invite legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library/{item_id}").Delete.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0338"}
			reject(t, doc, ids)
		},
		"group invite capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/group-invite-library").Get.Extensions["x-aicrm-capability"] = "media.library.write"
			reject(t, doc, ids)
		},
		"order evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-p4-decision-evidence"] = "P4-I03-FORGED"
			reject(t, doc, ids)
		},
		"order legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0406"}
			reject(t, doc, ids)
		},
		"order capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/orders").Get.Extensions["x-aicrm-capability"] = "order.write"
			reject(t, doc, ids)
		},
		"survey evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires").Get.Extensions["x-p4-decision-evidence"] = "P4-F01A-FORGED"
			reject(t, doc, ids)
		},
		"survey legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}").Get.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0424"}
			reject(t, doc, ids)
		},
		"survey capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires").Get.Extensions["x-aicrm-capability"] = "questionnaires.write"
			reject(t, doc, ids)
		},
		"survey assessment enabled": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyQuestionnaireCreateRequest"].Value.Properties["assessment_enabled"].Value.Enum = nil
			reject(t, doc, ids)
		},
		"survey F01B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}").Patch.Extensions["x-p4-decision-evidence"] = "P4-F01AB-FORGED"
			reject(t, doc, ids)
		},
		"survey F01B legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/duplicate").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0430"}
			reject(t, doc, ids)
		},
		"survey F01B CSRF removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/questionnaires/{questionnaire_id}/enable").Post.Parameters = nil
			reject(t, doc, ids)
		},
		"channel evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels").Get.Extensions["x-p4-decision-evidence"] = "P4-C01-FORGED"
			reject(t, doc, ids)
		},
		"channel legacy mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels/{channel_id}").Patch.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0195"}
			reject(t, doc, ids)
		},
		"channel capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/channels").Get.Extensions["x-aicrm-capability"] = "channels.write"
			reject(t, doc, ids)
		},
		"channel request opened": func(t *testing.T) {
			doc, ids := fresh(t)
			opened := true
			doc.Components.Schemas["LegacyChannelWriteRequest"].Value.AdditionalProperties.Has = &opened
			reject(t, doc, ids)
		},
		"coupon A+B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/h5/coupons/available").Get.Extensions["x-p4-decision-evidence"] = "P4-COUPON-AB-FORGED"
			reject(t, doc, ids)
		},
		"coupon H5 claim lacks idempotency": func(t *testing.T) {
			doc, ids := fresh(t)
			removeRequiredHeader(t, doc.Paths.Value("/api/h5/coupons/{public_slug}/claim").Post, "Idempotency-Key")
			reject(t, doc, ids)
		},
		"coupon H5 claim accepts public auth": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/h5/coupons/{public_slug}/claim").Post.Extensions["x-aicrm-auth-scheme"] = "public"
			reject(t, doc, ids)
		},
		"coupon claim leaks customer ID": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["LegacyCouponClaim"].Value.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"offset pagination reintroduced": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers").Get.Parameters[0].Value.Name = "offset"
			reject(t, doc, ids)
		},
		"estimated total removed": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerListResponse"].Value.Required = []string{"items", "next_cursor", "total", "watermark"}
			reject(t, doc, ids)
		},
		"stage smuggled into profile update": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerUpdateRequest"].Value.Properties["stage_id"] = doc.Components.Schemas["HealthResponse"]
			reject(t, doc, ids)
		},
		"event actor became optional": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Components.Schemas["CustomerEvent"].Value.Required = []string{"id", "customer_id", "event_type", "payload", "occurred_at"}
			reject(t, doc, ids)
		},
		"customer tag write without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/v1/customers/{customer_id}/tags/{tag_id}").Put.Parameters = nil
			reject(t, doc, ids)
		},
		"tag A+B evidence forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/sync").Post.Extensions["x-p4-decision-evidence"] = "P4-B02AB-FORGED"
			reject(t, doc, ids)
		},
		"tag A+B mapping forged": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/live/mark").Post.Extensions["x-legacy-mapping-ids"] = []string{"LEGACY-API-0559"}
			reject(t, doc, ids)
		},
		"tag A+B queue capability widened": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/live/unmark").Post.Extensions["x-aicrm-capability"] = "customers.read"
			reject(t, doc, ids)
		},
		"tag A+B queue without csrf": func(t *testing.T) {
			doc, ids := fresh(t)
			doc.Paths.Value("/api/admin/wecom/tags/sync-due").Post.Parameters = nil
			reject(t, doc, ids)
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			test(t)
		})
	}
}

func TestRejectsP3IdentityContractMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *openapi3.T)
	}{
		{
			name: "resolve discriminator mapping missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				delete(doc.Components.Schemas["ResolveIdentityResponse"].Value.Discriminator.Mapping, "not_found")
			},
		},
		{
			name: "bind discriminator mapping points to wrong variant",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["BindIdentityResponse"].Value.Discriminator.Mapping["merged"] =
					"#/components/schemas/BindIdentityBound"
			},
		},
		{
			name: "admin identity ref admits assurance",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["assurance"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity ref admits source",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["source"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity ref admits confidence alias",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["confidence"] =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "admin identity kind admits namespaced legacy kind",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityRef"].Value.Properties["type"].Value.Enum =
					append(doc.Components.Schemas["IdentityRef"].Value.Properties["type"].Value.Enum, "ext:legacy")
			},
		},
		{
			name: "review fingerprint permits unkeyed digest",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["identity_fingerprint"].Value.Pattern =
					`^[a-f0-9]{64}$`
			},
		},
		{
			name: "review fingerprint permits non canonical base64url tail",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["identity_fingerprint"].Value.Pattern =
					`^hmac-sha256-v[1-9][0-9]*:[A-Za-z0-9_-]{22}$`
			},
		},
		{
			name: "review exposes raw identity alias",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["raw_identity"] =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page admits unknown fields",
			mutate: func(t *testing.T, doc *openapi3.T) {
				allowed := true
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.AdditionalProperties.Has = &allowed
			},
		},
		{
			name: "resolve response ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				response := doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Value("200")
				response.Value.Content["application/json"].Schema = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "bind request ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/bind").Post.RequestBody.Value.Content["application/json"].Schema =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review list response ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				response := doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Responses.Value("200")
				response.Value.Content["application/json"].Schema = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page item ref drifts",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.Properties["items"].Value.Items =
					doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review page cursor stops being nullable",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReviewPage"].Value.Properties["next_cursor"].Value.Nullable = false
			},
		},
		{
			name: "review admits more than two roots",
			mutate: func(t *testing.T, doc *openapi3.T) {
				max := uint64(3)
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["customer_ids"].Value.MaxItems = &max
			},
		},
		{
			name: "bind lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/bind").Post, "X-CSRF-Token")
			},
		},
		{
			name: "bind lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/bind").Post, "Idempotency-Key")
			},
		},
		{
			name: "ingest lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/ingest").Post, "X-CSRF-Token")
			},
		},
		{
			name: "ingest lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/ingest").Post, "Idempotency-Key")
			},
		},
		{
			name: "approve review lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post, "X-CSRF-Token")
			},
		},
		{
			name: "approve review lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post, "Idempotency-Key")
			},
		},
		{
			name: "reject review lacks csrf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post, "X-CSRF-Token")
			},
		},
		{
			name: "reject review lacks idempotency key",
			mutate: func(t *testing.T, doc *openapi3.T) {
				removeRequiredHeader(t, doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post, "Idempotency-Key")
			},
		},
		{
			name: "review list operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews")
			},
		},
		{
			name: "review approval operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews/{review_id}/approve")
			},
		},
		{
			name: "review rejection operation missing",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Delete("/api/v1/identity/merge-reviews/{review_id}/reject")
			},
		},
		{
			name: "review list lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Responses.Delete("503")
			},
		},
		{
			name: "identity resolve lacks semantic validation response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/resolve").Post.Responses.Delete("422")
			},
		},
		{
			name: "review approval lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post.Responses.Delete("503")
			},
		},
		{
			name: "review rejection lacks unavailable response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post.Responses.Delete("503")
			},
		},
		{
			name: "review approval lacks conflict response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/approve").Post.Responses.Delete("409")
			},
		},
		{
			name: "review rejection lacks conflict response",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews/{review_id}/reject").Post.Responses.Delete("409")
			},
		},
		{
			name: "review leaks raw identity value",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["value"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review leaks normalized identity value",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Components.Schemas["IdentityMergeReview"].Value.Properties["normalized_value"] = doc.Components.Schemas["HealthResponse"]
			},
		},
		{
			name: "review omits explicit resolved timestamp",
			mutate: func(t *testing.T, doc *openapi3.T) {
				review := doc.Components.Schemas["IdentityMergeReview"].Value
				required := make([]string, 0, len(review.Required))
				for _, field := range review.Required {
					if field != "resolved_at" {
						required = append(required, field)
					}
				}
				review.Required = required
			},
		},
		{
			name: "forged P3 identity evidence",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-p3-decision-evidence"] = "P3-I00-FORGED"
			},
		},
		{
			name: "review capability drift",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-aicrm-capability"] = "identity.review.write"
			},
		},
		{
			name: "review RBAC widened to sales",
			mutate: func(t *testing.T, doc *openapi3.T) {
				doc.Paths.Value("/api/v1/identity/merge-reviews").Get.Extensions["x-aicrm-rbac-scopes"] = map[string]any{
					"admin": "global", "ops": "global", "sales": "global",
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc, ids := fresh(t)
			test.mutate(t, doc)
			reject(t, doc, ids)
		})
	}
}

func TestP3IdentityStatusFieldMatricesAreFrozen(t *testing.T) {
	doc, _ := fresh(t)
	assertStatusFieldMatrix(t, "resolveIdentity", successResponseSchema(t, doc, "/api/v1/identity/resolve"), map[string][]string{
		"found":     {"customer_id"},
		"not_found": {},
		"conflict":  {},
	})
	assertStatusFieldMatrix(t, "bindIdentity", successResponseSchema(t, doc, "/api/v1/identity/bind"), map[string][]string{
		"bound":         {"customer_id"},
		"already_bound": {"customer_id"},
		"merged":        {"customer_id", "primary_customer_id", "merge_audit_id"},
		"manual_review": {"review_id"},
		"rejected":      {},
	})
	assertStatusFieldMatrix(t, "ingestIdentityEvent", successResponseSchema(t, doc, "/api/v1/identity/ingest"), map[string][]string{
		"attributed": {"customer_id", "event_id"},
		"pending":    {"pending_event_id"},
		"conflict":   {"pending_event_id"},
	})
}

func TestRejectsP3IdentityStatusFieldMatrixMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *openapi3.T)
	}{
		{
			name: "resolve response no longer uses oneOf",
			mutate: func(t *testing.T, doc *openapi3.T) {
				successResponseSchema(t, doc, "/api/v1/identity/resolve").OneOf = nil
			},
		},
		{
			name: "resolve response loses status discriminator",
			mutate: func(t *testing.T, doc *openapi3.T) {
				successResponseSchema(t, doc, "/api/v1/identity/resolve").Discriminator = nil
			},
		},
		{
			name: "bind manual review admits customer ID",
			mutate: func(t *testing.T, doc *openapi3.T) {
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/bind"), "manual_review")
				branch.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
				branch.Required = append(branch.Required, "customer_id")
			},
		},
		{
			name: "ingest pending admits customer ID",
			mutate: func(t *testing.T, doc *openapi3.T) {
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/ingest"), "pending")
				branch.Properties["customer_id"] = doc.Components.Schemas["HealthResponse"]
				branch.Required = append(branch.Required, "customer_id")
			},
		},
		{
			name: "resolve branch admits unknown fields",
			mutate: func(t *testing.T, doc *openapi3.T) {
				allowed := true
				branch := statusBranch(t, successResponseSchema(t, doc, "/api/v1/identity/resolve"), "found")
				branch.AdditionalProperties.Has = &allowed
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc, ids := fresh(t)
			test.mutate(t, doc)
			reject(t, doc, ids)
		})
	}
}

func fresh(t *testing.T) (*openapi3.T, map[string]bool) {
	t.Helper()
	doc, ids, err := load(specPath, mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	return doc, ids
}
func reject(t *testing.T, doc *openapi3.T, ids map[string]bool) {
	t.Helper()
	if err := validate(doc, ids); err == nil {
		t.Fatal("mutation was accepted")
	}
}

func removeRequiredHeader(t *testing.T, operation *openapi3.Operation, name string) {
	t.Helper()
	if operation == nil {
		t.Fatal("operation is missing")
	}
	parameters := make(openapi3.Parameters, 0, len(operation.Parameters))
	removed := false
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == "header" && ref.Value.Name == name {
			removed = true
			continue
		}
		parameters = append(parameters, ref)
	}
	if !removed {
		t.Fatalf("required header %q is missing before mutation", name)
	}
	operation.Parameters = parameters
}

func successResponseSchema(t *testing.T, doc *openapi3.T, path string) *openapi3.Schema {
	t.Helper()
	item := doc.Paths.Value(path)
	if item == nil || item.Post == nil {
		t.Fatalf("%s POST operation is missing", path)
	}
	response := item.Post.Responses.Value("200")
	if response == nil || response.Value == nil {
		t.Fatalf("%s success response is missing", path)
	}
	media := response.Value.Content["application/json"]
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("%s success schema is missing", path)
	}
	return media.Schema.Value
}

func assertStatusFieldMatrix(t *testing.T, operation string, schema *openapi3.Schema, want map[string][]string) {
	t.Helper()
	if len(schema.OneOf) != len(want) {
		t.Fatalf("%s oneOf branches = %d; want %d", operation, len(schema.OneOf), len(want))
	}
	if schema.Discriminator == nil || schema.Discriminator.PropertyName != "status" {
		t.Fatalf("%s response must discriminate oneOf branches by status", operation)
	}
	if len(schema.Properties) != 0 || len(schema.Required) != 0 {
		t.Fatalf("%s response must expose status fields only through oneOf branches", operation)
	}
	seen := make(map[string]bool, len(schema.OneOf))
	for _, ref := range schema.OneOf {
		if ref == nil || ref.Value == nil {
			t.Fatalf("%s has an unresolved oneOf branch", operation)
		}
		if !strings.HasPrefix(ref.Ref, "#/components/schemas/") {
			t.Fatalf("%s has an inline or external oneOf branch", operation)
		}
		branch := ref.Value
		if branch.Type == nil || !branch.Type.Is("object") || branch.AdditionalProperties.Has == nil || *branch.AdditionalProperties.Has {
			t.Fatalf("%s response branch must be a closed object", operation)
		}
		status, ok := branchStatus(branch)
		if !ok {
			t.Fatalf("%s response branch lacks a singleton status enum", operation)
		}
		expectedFields, knownStatus := want[status]
		if !knownStatus || seen[status] {
			t.Fatalf("%s has unexpected or duplicate status branch %q", operation, status)
		}
		seen[status] = true
		wantFields := append([]string{"status"}, expectedFields...)
		if !sameStringSet(schemaFieldNames(branch.Properties), wantFields) || !sameStringSet(branch.Required, wantFields) {
			t.Fatalf("%s status %q fields=%v required=%v; want %v", operation, status, schemaFieldNames(branch.Properties), branch.Required, wantFields)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("%s status branches=%v; want %v", operation, seen, want)
	}
}

func statusBranch(t *testing.T, schema *openapi3.Schema, wanted string) *openapi3.Schema {
	t.Helper()
	for _, ref := range schema.OneOf {
		if ref != nil && ref.Value != nil {
			if status, ok := branchStatus(ref.Value); ok && status == wanted {
				return ref.Value
			}
		}
	}
	t.Fatalf("status branch %q is missing", wanted)
	return nil
}

func branchStatus(schema *openapi3.Schema) (string, bool) {
	status := schema.Properties["status"]
	if status == nil || status.Value == nil || len(status.Value.Enum) != 1 {
		return "", false
	}
	value, ok := status.Value.Enum[0].(string)
	return value, ok && value != ""
}

func schemaFieldNames(properties openapi3.Schemas) []string {
	fields := make([]string, 0, len(properties))
	for field := range properties {
		fields = append(fields, field)
	}
	return fields
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	values := make(map[string]int, len(got))
	for _, value := range got {
		values[value]++
	}
	for _, value := range want {
		values[value]--
	}
	for _, count := range values {
		if count != 0 {
			return false
		}
	}
	return true
}

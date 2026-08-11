package main

import (
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
	}
	for name, test := range tests {
		t.Run(name, test)
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

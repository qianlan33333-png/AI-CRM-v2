package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"reflect"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

var p1CandidateOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "resolveIdentity": true, "bindIdentity": true,
	"ingestIdentityEvent": true, "getAuthSession": true, "logoutAdmin": true,
	"getAdminConfigOverview": true,
}

var p2StageOperations = map[string]bool{
	"listStages": true, "createStage": true, "renameStage": true,
}

var p3ContactOperations = map[string]bool{
	"listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

var contactOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "listTags": true, "setCustomerStage": true,
	"addCustomerTag": true, "removeCustomerTag": true,
}

type authorizationContract struct {
	capability string
	scopes     map[string]string
}

var authorizationContracts = map[string]authorizationContract{
	"listCustomers":          {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"getCustomer":            {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"updateCustomer":         {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"listCustomerEvents":     {"customer.events.read", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"resolveIdentity":        {"identity.resolve", map[string]string{"admin": "global", "ops": "global"}},
	"bindIdentity":           {"identity.bind", map[string]string{"admin": "global", "ops": "global"}},
	"ingestIdentityEvent":    {"identity.ingest", map[string]string{"admin": "global", "ops": "global"}},
	"getAuthSession":         {"auth.session.read", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"logoutAdmin":            {"auth.session.logout", map[string]string{"admin": "self", "ops": "self", "sales": "self"}},
	"getAdminConfigOverview": {"config.overview.read", map[string]string{"admin": "global"}},
	"listStages":             {"stages.read", map[string]string{"admin": "global", "ops": "global", "sales": "global"}},
	"createStage":            {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"renameStage":            {"stages.write", map[string]string{"admin": "global", "ops": "global"}},
	"listTags":               {"customers.read", map[string]string{"admin": "global", "ops": "global", "sales": "global"}},
	"setCustomerStage":       {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"addCustomerTag":         {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
	"removeCustomerTag":      {"customers.write", map[string]string{"admin": "global", "ops": "global", "sales": "owner_staff"}},
}

const g1DecisionEvidence = "G1-D01-2026-08-10"
const p2StageDecisionEvidence = "P2-16-2026-08-11"
const p3ContactDecisionEvidence = "P3-C00-2026-08-12"

func main() {
	spec := flag.String("spec", "../api/openapi.yaml", "OpenAPI document")
	mapping := flag.String("mapping", "../docs/api-mapping.jsonl", "legacy API mapping")
	flag.Parse()
	doc, ids, err := load(*spec, *mapping)
	if err == nil {
		err = validate(doc, ids)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi-contract:", err)
		os.Exit(1)
	}
	fmt.Println("openapi-contract: PASS (p1_operations=10 approved=10 legacy_links=16 p2_stage_operations=3 p3_contact_operations=4)")
}

func load(spec, mapping string) (*openapi3.T, map[string]bool, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(spec)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(mapping)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	ids := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var row struct {
			MappingID string `json:"mapping_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, nil, err
		}
		if row.MappingID == "" || ids[row.MappingID] {
			return nil, nil, errors.New("invalid legacy mapping IDs")
		}
		ids[row.MappingID] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) != 781 {
		return nil, nil, fmt.Errorf("legacy mapping inventory=%d", len(ids))
	}
	return doc, ids, nil
}

func validate(doc *openapi3.T, known map[string]bool) error {
	if err := doc.Validate(context.Background()); err != nil {
		return err
	}
	if len(doc.Security) == 0 {
		return errors.New("business API lacks default security")
	}
	seenP1, seenP2, seenP3, links := map[string]bool{}, map[string]bool{}, map[string]bool{}, 0
	for path, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if path == "/healthz" {
				continue
			}
			if seenP1[op.OperationID] || seenP2[op.OperationID] || seenP3[op.OperationID] ||
				(!p1CandidateOperations[op.OperationID] && !p2StageOperations[op.OperationID] && !p3ContactOperations[op.OperationID]) {
				return fmt.Errorf("unexpected or duplicate candidate operation: %s", op.OperationID)
			}
			if p1CandidateOperations[op.OperationID] {
				seenP1[op.OperationID] = true
				status, ok := op.Extensions["x-p1-signoff-status"].(string)
				if !ok || status != "APPROVED" {
					return fmt.Errorf("%s lacks approved G1 signoff", op.OperationID)
				}
				evidence, ok := op.Extensions["x-p1-decision-evidence"].(string)
				if !ok || evidence != g1DecisionEvidence {
					return fmt.Errorf("%s has missing or forged G1 evidence", op.OperationID)
				}
				ids, err := stringList(op.Extensions["x-legacy-mapping-ids"])
				if err != nil || len(ids) == 0 {
					return fmt.Errorf("%s lacks legacy links", op.OperationID)
				}
				for _, id := range ids {
					if !known[id] {
						return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
					}
					links++
				}
			} else if p2StageOperations[op.OperationID] {
				seenP2[op.OperationID] = true
				evidence, ok := op.Extensions["x-p2-decision-evidence"].(string)
				if !ok || evidence != p2StageDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P2 stage evidence", op.OperationID)
				}
			} else {
				seenP3[op.OperationID] = true
				if ids, ok := op.Extensions["x-legacy-mapping-ids"]; ok {
					legacyIDs, err := stringList(ids)
					if err != nil || len(legacyIDs) == 0 {
						return fmt.Errorf("%s has invalid legacy links", op.OperationID)
					}
					for _, id := range legacyIDs {
						if !known[id] {
							return fmt.Errorf("%s links unknown mapping %s", op.OperationID, id)
						}
						links++
					}
				}
			}
			if contactOperations[op.OperationID] {
				evidence, ok := op.Extensions["x-p3-decision-evidence"].(string)
				if !ok || evidence != p3ContactDecisionEvidence {
					return fmt.Errorf("%s has missing or forged P3 contact evidence", op.OperationID)
				}
			}
			contract := authorizationContracts[op.OperationID]
			capability, ok := op.Extensions["x-aicrm-capability"].(string)
			if !ok || capability != contract.capability {
				return fmt.Errorf("%s capability=%q", op.OperationID, capability)
			}
			scopes, err := stringMap(op.Extensions["x-aicrm-rbac-scopes"])
			if err != nil || !reflect.DeepEqual(scopes, contract.scopes) {
				return fmt.Errorf("%s RBAC scopes=%v", op.OperationID, scopes)
			}
			if len(contract.scopes) < 3 && op.Responses.Value("403") == nil {
				return fmt.Errorf("%s denies a role but lacks 403", op.OperationID)
			}
		}
	}
	if len(seenP1) != 10 || len(seenP2) != 3 || len(seenP3) != 4 || links != 16 {
		return fmt.Errorf("candidate inventory mismatch: p1=%d p2_stages=%d p3_contact=%d links=%d", len(seenP1), len(seenP2), len(seenP3), links)
	}
	for id := range p1CandidateOperations {
		if !seenP1[id] {
			return fmt.Errorf("missing candidate operation: %s", id)
		}
	}
	for id := range p2StageOperations {
		if !seenP2[id] {
			return fmt.Errorf("missing P2 stage operation: %s", id)
		}
	}
	for id := range p3ContactOperations {
		if !seenP3[id] {
			return fmt.Errorf("missing P3 contact operation: %s", id)
		}
	}
	customer := doc.Components.Schemas["Customer"]
	if customer == nil || customer.Value == nil {
		return errors.New("Customer schema missing")
	}
	for _, name := range []string{"external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := customer.Value.Properties[name]; ok {
			return fmt.Errorf("Customer contains external identity: %s", name)
		}
	}
	identity := doc.Components.Schemas["IdentityRef"]
	if identity == nil || identity.Value == nil {
		return errors.New("IdentityRef schema missing")
	}
	required := append([]string(nil), identity.Value.Required...)
	sort.Strings(required)
	want := []string{"assurance", "scope", "source", "type", "value"}
	if fmt.Sprint(required) != fmt.Sprint(want) {
		return fmt.Errorf("IdentityRef required fields=%v", required)
	}
	if doc.Components.Schemas["ErrorResponse"] == nil {
		return errors.New("ErrorResponse schema missing")
	}
	if err := validateBrowserSessionContract(doc); err != nil {
		return err
	}
	if err := validateStageContract(doc); err != nil {
		return err
	}
	if err := validateContactContract(doc); err != nil {
		return err
	}
	return nil
}

func validateBrowserSessionContract(doc *openapi3.T) error {
	scheme := doc.Components.SecuritySchemes["AdminSession"]
	if scheme == nil || scheme.Value == nil || scheme.Value.Type != "apiKey" ||
		scheme.Value.In != "cookie" || scheme.Value.Name != "aicrm_session" {
		return errors.New("AdminSession must remain an opaque aicrm_session cookie")
	}
	logout := doc.Paths.Value("/api/v1/auth/logout")
	if logout == nil || logout.Post == nil {
		return errors.New("logout operation missing")
	}
	var csrf *openapi3.Parameter
	for _, ref := range logout.Post.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("logout lacks required X-CSRF-Token header")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("logout CSRF token shape is not frozen")
	}
	for _, status := range []string{"204", "401", "403"} {
		if logout.Post.Responses.Value(status) == nil {
			return fmt.Errorf("logout response missing: %s", status)
		}
	}
	return nil
}

func validateStageContract(doc *openapi3.T) error {
	stages := doc.Paths.Value("/api/v1/stages")
	stage := doc.Paths.Value("/api/v1/stages/{stage_id}")
	if stages == nil || stages.Get == nil || stages.Post == nil || stage == nil || stage.Patch == nil {
		return errors.New("P2 stage operations are incomplete")
	}
	for name, operation := range map[string]*openapi3.Operation{
		"createStage": stages.Post,
		"renameStage": stage.Patch,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403", "422", "503"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	if stages.Post.Responses.Value("201") == nil || stage.Patch.Responses.Value("200") == nil ||
		stage.Patch.Responses.Value("404") == nil {
		return errors.New("P2 stage success or not-found responses drifted")
	}
	return nil
}

func validateContactContract(doc *openapi3.T) error {
	customers := doc.Paths.Value("/api/v1/customers")
	detail := doc.Paths.Value("/api/v1/customers/{customer_id}")
	events := doc.Paths.Value("/api/v1/customers/{customer_id}/events")
	stage := doc.Paths.Value("/api/v1/customers/{customer_id}/stage")
	tags := doc.Paths.Value("/api/v1/customers/{customer_id}/tags/{tag_id}")
	catalog := doc.Paths.Value("/api/v1/tags")
	if customers == nil || customers.Get == nil || detail == nil || detail.Get == nil || detail.Patch == nil ||
		events == nil || events.Get == nil || stage == nil || stage.Put == nil ||
		tags == nil || tags.Put == nil || tags.Delete == nil || catalog == nil || catalog.Get == nil {
		return errors.New("P3 contact operations are incomplete")
	}

	wantFilters := []string{
		"added_after", "added_before", "channel_id", "cursor", "is_deleted", "keyword", "last_interact_after",
		"last_interact_before", "limit", "owner_staff_id", "stage_id", "tag_id",
	}
	gotFilters := make([]string, 0, len(customers.Get.Parameters))
	for _, ref := range customers.Get.Parameters {
		if ref == nil || ref.Value == nil || ref.Value.In != "query" {
			return errors.New("listCustomers has invalid query parameter")
		}
		if ref.Value.Name == "offset" {
			return errors.New("listCustomers must not expose offset pagination")
		}
		gotFilters = append(gotFilters, ref.Value.Name)
	}
	sort.Strings(gotFilters)
	if fmt.Sprint(gotFilters) != fmt.Sprint(wantFilters) {
		return fmt.Errorf("listCustomers filters=%v", gotFilters)
	}

	listResponse := doc.Components.Schemas["CustomerListResponse"]
	if listResponse == nil || listResponse.Value == nil {
		return errors.New("CustomerListResponse schema missing")
	}
	required := append([]string(nil), listResponse.Value.Required...)
	sort.Strings(required)
	wantRequired := []string{"items", "next_cursor", "total", "total_is_estimate", "watermark"}
	if fmt.Sprint(required) != fmt.Sprint(wantRequired) {
		return fmt.Errorf("CustomerListResponse required=%v", required)
	}

	update := doc.Components.Schemas["CustomerUpdateRequest"]
	if update == nil || update.Value == nil {
		return errors.New("CustomerUpdateRequest schema missing")
	}
	for _, name := range []string{"stage_id", "external_userid", "unionid", "openid", "phone", "mobile"} {
		if _, ok := update.Value.Properties[name]; ok {
			return fmt.Errorf("CustomerUpdateRequest contains forbidden field: %s", name)
		}
	}

	event := doc.Components.Schemas["CustomerEvent"]
	if event == nil || event.Value == nil || event.Value.Properties["actor"] == nil {
		return errors.New("CustomerEvent actor is not frozen")
	}
	actorRequired := false
	for _, name := range event.Value.Required {
		actorRequired = actorRequired || name == "actor"
	}
	if !actorRequired {
		return errors.New("CustomerEvent actor became optional")
	}

	for name, operation := range map[string]*openapi3.Operation{
		"updateCustomer": detail.Patch, "setCustomerStage": stage.Put,
		"addCustomerTag": tags.Put, "removeCustomerTag": tags.Delete,
	} {
		if err := validateRequiredCSRF(operation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		for _, status := range []string{"401", "403"} {
			if operation.Responses.Value(status) == nil {
				return fmt.Errorf("%s response missing: %s", name, status)
			}
		}
	}
	return nil
}

func validateRequiredCSRF(operation *openapi3.Operation) error {
	var csrf *openapi3.Parameter
	for _, ref := range operation.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.Name == "X-CSRF-Token" {
			csrf = ref.Value
			break
		}
	}
	if csrf == nil || csrf.In != "header" || !csrf.Required || csrf.Schema == nil || csrf.Schema.Value == nil {
		return errors.New("required X-CSRF-Token header is missing")
	}
	schema := csrf.Schema.Value
	if schema.MinLength != 43 || schema.MaxLength == nil || *schema.MaxLength != 43 || schema.Pattern != "^[A-Za-z0-9_-]{43}$" {
		return errors.New("CSRF token shape is not frozen")
	}
	return nil
}

func stringList(value any) ([]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result []string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for _, item := range result {
		if item == "" {
			return nil, errors.New("blank list item")
		}
	}
	return result, nil
}

func stringMap(value any) (map[string]string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, errors.New("empty string map")
	}
	for key, item := range result {
		if key == "" || item == "" {
			return nil, errors.New("blank string map entry")
		}
	}
	return result, nil
}

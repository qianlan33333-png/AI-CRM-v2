package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/getkin/kin-openapi/openapi3"
)

var candidateOperations = map[string]bool{
	"listCustomers": true, "getCustomer": true, "updateCustomer": true,
	"listCustomerEvents": true, "resolveIdentity": true, "bindIdentity": true,
	"ingestIdentityEvent": true, "getAuthSession": true, "logoutAdmin": true,
	"getAdminConfigOverview": true,
}

const g1DecisionEvidence = "G1-D01-2026-08-10"

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
	fmt.Println("openapi-contract: PASS (candidate_operations=10 approved=10 pending=0 legacy_links=14)")
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
	seen, links := map[string]bool{}, 0
	for path, item := range doc.Paths.Map() {
		for _, op := range item.Operations() {
			if path == "/healthz" {
				continue
			}
			if !candidateOperations[op.OperationID] || seen[op.OperationID] {
				return fmt.Errorf("unexpected or duplicate candidate operation: %s", op.OperationID)
			}
			seen[op.OperationID] = true
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
		}
	}
	if len(seen) != 10 || links != 14 {
		return fmt.Errorf("candidate inventory mismatch: operations=%d links=%d", len(seen), links)
	}
	for id := range candidateOperations {
		if !seen[id] {
			return fmt.Errorf("missing candidate operation: %s", id)
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

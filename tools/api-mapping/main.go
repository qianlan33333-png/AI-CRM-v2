package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
)

const legacySHA = "6cb989c071255437d75953dabb943318a74eb8f4"

type legacyRoute struct {
	AccessScope     string   `json:"access_scope"`
	Audience        string   `json:"audience"`
	AuthScheme      string   `json:"auth_scheme"`
	Capability      string   `json:"capability"`
	CapabilityOwner string   `json:"capability_owner"`
	CSRF            bool     `json:"csrf"`
	DataSource      string   `json:"data_source"`
	ExternalEffects string   `json:"external_effects"`
	Layer           string   `json:"layer"`
	Methods         []string `json:"methods,omitempty"`
	Path            string   `json:"path,omitempty"`
	PIILevel        string   `json:"pii_level"`
	PrincipalTypes  []string `json:"principal_types"`
	RateLimit       string   `json:"rate_limit"`
	RequiresAuth    bool     `json:"requires_auth"`
	Rollback        string   `json:"rollback"`
	RouteName       string   `json:"route_name,omitempty"`
	RuntimeOwner    string   `json:"runtime_owner"`
}
type routeDocument struct {
	RouteCount   int           `json:"route_count"`
	SourceCommit string        `json:"source_commit"`
	Routes       []legacyRoute `json:"routes"`
}
type mappingRow struct {
	MappingID              string          `json:"mapping_id"`
	LegacySourceSHA        string          `json:"legacy_source_sha"`
	Partition              string          `json:"partition"`
	LegacyPath             string          `json:"legacy_path"`
	LegacyRouteName        string          `json:"legacy_route_name"`
	ManifestMethods        []string        `json:"manifest_methods"`
	ManifestMethodBundle   []string        `json:"manifest_method_bundle"`
	SourceMethods          []string        `json:"source_methods"`
	SourceOnlyMethods      []string        `json:"source_only_methods"`
	SourceFile             string          `json:"source_file"`
	SourceLine             int             `json:"source_line"`
	Handler                string          `json:"handler"`
	InputSignature         json.RawMessage `json:"input_signature"`
	InputStatus            string          `json:"input_status"`
	OutputSignature        json.RawMessage `json:"output_signature"`
	OutputStatus           string          `json:"output_status"`
	StaticCallTargets      []string        `json:"static_call_targets"`
	ManifestContract       legacyRoute     `json:"manifest_contract"`
	DiscrepancyFlags       []string        `json:"discrepancy_flags"`
	ReviewFlags            []string        `json:"review_flags"`
	SourceEvidence         []string        `json:"source_evidence"`
	CandidateV2OperationID string          `json:"candidate_v2_operation_id"`
	CandidateV2Method      string          `json:"candidate_v2_method"`
	CandidateV2Path        string          `json:"candidate_v2_path"`
	Disposition            string          `json:"disposition"`
	DispositionReason      string          `json:"disposition_reason"`
	TargetMappingID        string          `json:"target_mapping_id"`
	Signoff                string          `json:"signoff"`
	DecisionEvidence       []string        `json:"decision_evidence"`
}
type counts struct{ s02, s03, s04, partialInput, partialOutput int }

var (
	idPattern       = regexp.MustCompile(`^LEGACY-API-[0-9]{4}$`)
	evidencePattern = regexp.MustCompile(`^aicrm_next/.+[.]py:[1-9][0-9]*-[1-9][0-9]*$`)
)

func same[T comparable](left, right []T) bool   { return slices.Equal(left, right) }
func oneOf(value string, values ...string) bool { return slices.Contains(values, value) }
func sortedUnique(values []string) bool {
	return slices.IsSorted(values) && len(values) == len(slices.Compact(slices.Clone(values)))
}
func text(raw json.RawMessage, allowBlank bool) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && (allowBlank || strings.TrimSpace(value) != "")
}
func nullableText(raw json.RawMessage, allowBlank bool) bool {
	return bytes.Equal(raw, []byte("null")) || text(raw, allowBlank)
}
func fact(raw json.RawMessage, marker string, s02 bool) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	return s02 && len(value) == 3 && text(value["detail"], false) && nullableText(value["source"], false) && text(value["status"], false) || !s02 && len(value) == 2 && text(value[marker], false) && nullableText(value["value"], false)
}
func partition(route legacyRoute) string {
	s02 := []string{"customer_read_model", "customer_tags", "identity_contact", "sidebar_write", "admin_auth", "admin_config", "admin_jobs"}
	s03 := []string{"ai_audience_ops", "auth_wecom", "channel_entry", "ops_enrollment", "send_content"}
	if slices.Contains(s02, route.CapabilityOwner) || route.RouteName == "oauth_token" {
		return "S02"
	}
	if slices.Contains(s03, route.CapabilityOwner) {
		return "S03"
	}
	if route.CapabilityOwner == "automation_engine" && hasPrefix(route.Path, "/api/admin/automation-conversion/group-ops", "/api/automation/group-ops", "/api/admin/channels", "/api/admin/channel-welcome-materials", "/api/admin/wecom-customer-acquisition-links", "/admin/channels", "/admin/automation-conversion/group-ops") {
		return "S03"
	}
	if route.CapabilityOwner == "platform_foundation" && (hasPrefix(route.Path, "/api/admin/external-effects", "/api/external-effects", "/api/admin/push-center") || route.Path == "/api/admin/wecom/execution-diagnostics") {
		return "S03"
	}
	return "S04"
}
func hasPrefix(value string, prefixes ...string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(value, prefix) })
}
func difference(left, right []string) []string {
	var result []string
	for _, value := range left {
		if !slices.Contains(right, value) {
			result = append(result, value)
		}
	}
	return result
}
func validate(mapping io.Reader, routes routeDocument, expectedRows int) ([]mappingRow, counts, error) {
	if routes.SourceCommit != legacySHA || routes.RouteCount != expectedRows || len(routes.Routes) != expectedRows {
		return nil, counts{}, fmt.Errorf("legacy route authority mismatch")
	}
	bundles := map[string][]string{}
	for _, route := range routes.Routes {
		key := route.Path + "\x00" + route.RouteName
		bundles[key] = append(bundles[key], route.Methods...)
	}
	for key, methods := range bundles {
		slices.Sort(methods)
		bundles[key] = slices.Compact(methods)
	}
	scanner := bufio.NewScanner(mapping)
	scanner.Buffer(make([]byte, 64*1024), 16*1024)
	rows, seen, files, result := []mappingRow{}, map[string]bool{}, map[string]bool{}, counts{}
	extra := map[string]int{}
	for scanner.Scan() {
		var row mappingRow
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&row); err != nil {
			return nil, result, fmt.Errorf("line %d: %w", len(rows)+1, err)
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return nil, result, fmt.Errorf("line %d: trailing JSON value", len(rows)+1)
		}
		canonical, _ := json.Marshal(row)
		if !bytes.Equal(canonical, scanner.Bytes()) {
			return nil, result, fmt.Errorf("line %d: non-canonical or duplicate JSON", len(rows)+1)
		}
		if len(rows) >= len(routes.Routes) {
			return nil, result, fmt.Errorf("too many rows")
		}
		legacy := routes.Routes[len(rows)]
		if row.MappingID != fmt.Sprintf("LEGACY-API-%04d", len(rows)+1) || !idPattern.MatchString(row.MappingID) || seen[row.MappingID] {
			return nil, result, fmt.Errorf("line %d: unstable mapping identity", len(rows)+1)
		}
		seen[row.MappingID] = true
		if row.LegacySourceSHA != legacySHA || row.LegacyPath != legacy.Path || row.LegacyRouteName != legacy.RouteName || !same(row.ManifestMethods, legacy.Methods) || row.Partition != partition(legacy) {
			return nil, result, fmt.Errorf("%s: route authority drift", row.MappingID)
		}
		bundle := bundles[legacy.Path+"\x00"+legacy.RouteName]
		if !same(row.ManifestMethodBundle, bundle) || !sortedUnique(row.SourceMethods) || !same(row.SourceOnlyMethods, difference(row.SourceMethods, bundle)) {
			return nil, result, fmt.Errorf("%s: method bundle drift", row.MappingID)
		}
		for _, method := range bundle {
			if !slices.Contains(row.SourceMethods, method) {
				return nil, result, fmt.Errorf("%s: source lost canonical method", row.MappingID)
			}
		}
		for _, method := range row.SourceOnlyMethods {
			if !oneOf(method, "HEAD", "OPTIONS") {
				return nil, result, fmt.Errorf("%s: unexpected source-only method", row.MappingID)
			}
			extra[row.Partition]++
		}
		if !manifestEqual(row.ManifestContract, legacy) {
			return nil, result, fmt.Errorf("%s: manifest contract drift", row.MappingID)
		}
		if row.SourceFile == "" || row.SourceLine < 1 || row.Handler == "" || len(row.InputSignature) < 2 || row.InputSignature[0] != '[' || len(row.OutputSignature) <= 2 || row.OutputSignature[0] != '{' || len(row.SourceEvidence) != 1 || !evidencePattern.MatchString(row.SourceEvidence[0]) || !strings.HasPrefix(row.SourceEvidence[0], row.SourceFile+":") {
			return nil, result, fmt.Errorf("%s: incomplete source evidence", row.MappingID)
		}
		var inputs []map[string]json.RawMessage
		var output map[string]json.RawMessage
		var inputValue, outputValue any
		if json.Unmarshal(row.InputSignature, &inputs) != nil || json.Unmarshal(row.OutputSignature, &output) != nil || json.Unmarshal(row.InputSignature, &inputValue) != nil || json.Unmarshal(row.OutputSignature, &outputValue) != nil {
			return nil, result, fmt.Errorf("%s: malformed static signature", row.MappingID)
		}
		canonicalInput, _ := json.Marshal(inputValue)
		canonicalOutput, _ := json.Marshal(outputValue)
		if !bytes.Equal(canonicalInput, row.InputSignature) || !bytes.Equal(canonicalOutput, row.OutputSignature) {
			return nil, result, fmt.Errorf("%s: non-canonical static signature", row.MappingID)
		}
		inputExtra := map[string][2]string{"S02": {"status", "status_detail"}, "S03": {"annotation_status", "default_status"}, "S04": {"annotation_proof", "default_proof"}}[row.Partition]
		for _, input := range inputs {
			if len(input) != 6 || !text(input["name"], false) || !text(input["kind"], false) || !text(input["annotation"], false) || !nullableText(input["default"], false) || !text(input[inputExtra[0]], false) || !text(input[inputExtra[1]], false) {
				return nil, result, fmt.Errorf("%s: empty input declaration", row.MappingID)
			}
		}
		outputFields := map[string][]string{"S02": {"response_class", "response_model", "responses", "return_annotation", "status_code"}, "S03": {"body_schema_fields_status", "decorator_response_model", "decorator_responses", "decorator_status_code", "function_return_annotation"}, "S04": {"body_schema_scope", "decorator_response_class", "decorator_response_model", "decorator_responses", "decorator_status_code", "function_return_annotation"}}[row.Partition]
		anchor := map[string]string{"S02": "response_class", "S03": "body_schema_fields_status", "S04": "body_schema_scope"}[row.Partition]
		validOutput := len(output) == len(outputFields)
		for _, field := range outputFields {
			validOutput = validOutput && (row.Partition == "S02" && fact(output[field], "status", true) || field == anchor || row.Partition == "S03" && fact(output[field], "status", false) || row.Partition == "S04" && fact(output[field], "proof", false))
		}
		if !validOutput || row.Partition != "S02" && !text(output[anchor], false) {
			return nil, result, fmt.Errorf("%s: empty output declaration", row.MappingID)
		}
		if !oneOf(row.InputStatus, "EXACT_STATIC_DECLARATION", "PARTIAL_IMPORTED_SCHEMA") || !oneOf(row.OutputStatus, "EXACT_STATIC_DECLARATION", "PARTIAL_IMPORTED_SCHEMA") {
			return nil, result, fmt.Errorf("%s: unresolved input/output schema", row.MappingID)
		}
		if row.ManifestMethods == nil || row.ManifestMethodBundle == nil || row.SourceMethods == nil || row.SourceOnlyMethods == nil || row.StaticCallTargets == nil || row.DiscrepancyFlags == nil || row.ReviewFlags == nil || row.SourceEvidence == nil || row.DecisionEvidence == nil || !sortedUnique(row.SourceMethods) || !sortedUnique(row.StaticCallTargets) || !sortedUnique(row.DiscrepancyFlags) || !sortedUnique(row.ReviewFlags) {
			return nil, result, fmt.Errorf("%s: unstable static facts", row.MappingID)
		}
		if (len(row.SourceOnlyMethods) > 0) != slices.Contains(row.DiscrepancyFlags, "SOURCE_METHOD_BUNDLE_DIFFERS") {
			return nil, result, fmt.Errorf("%s: auxiliary method flag drift", row.MappingID)
		}
		effectFlag := "MANIFEST_EXTERNAL_EFFECTS_" + strings.ToUpper(legacy.ExternalEffects)
		if (legacy.ExternalEffects != "none") != slices.Contains(row.DiscrepancyFlags, effectFlag) {
			return nil, result, fmt.Errorf("%s: external effect flag drift", row.MappingID)
		}
		if err := validateState(row); err != nil {
			return nil, result, err
		}
		files[row.Partition+"\x00"+row.SourceFile] = true
		switch row.Partition {
		case "S02":
			result.s02++
		case "S03":
			result.s03++
		case "S04":
			result.s04++
		}
		if row.InputStatus != "EXACT_STATIC_DECLARATION" {
			result.partialInput++
		}
		if row.OutputStatus != "EXACT_STATIC_DECLARATION" {
			result.partialOutput++
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, result, err
	}
	fileCount := map[string]int{}
	for key := range files {
		fileCount[strings.SplitN(key, "\x00", 2)[0]]++
	}
	if len(rows) != expectedRows || result.s02 != 156 || result.s03 != 184 || result.s04 != 441 || result.partialInput != 121 || result.partialOutput != 121 || fileCount["S02"] != 13 || fileCount["S03"] != 15 || fileCount["S04"] != 36 || extra["S02"] != 20 || extra["S03"] != 9 || extra["S04"] != 22 {
		return nil, result, fmt.Errorf("inventory mismatch: rows=%d partitions=%d/%d/%d files=%d/%d/%d auxiliary=%d/%d/%d", len(rows), result.s02, result.s03, result.s04, fileCount["S02"], fileCount["S03"], fileCount["S04"], extra["S02"], extra["S03"], extra["S04"])
	}
	return rows, result, nil
}
func manifestEqual(left, right legacyRoute) bool {
	return left.AccessScope == right.AccessScope && left.Audience == right.Audience && left.AuthScheme == right.AuthScheme && left.Capability == right.Capability && left.CapabilityOwner == right.CapabilityOwner && left.CSRF == right.CSRF && left.DataSource == right.DataSource && left.ExternalEffects == right.ExternalEffects && left.Layer == right.Layer && same(left.Methods, right.Methods) && left.Path == right.Path && left.PIILevel == right.PIILevel && same(left.PrincipalTypes, right.PrincipalTypes) && left.RateLimit == right.RateLimit && left.RequiresAuth == right.RequiresAuth && left.Rollback == right.Rollback && left.RouteName == right.RouteName && left.RuntimeOwner == right.RuntimeOwner
}
func validateState(row mappingRow) error {
	if row.Disposition != "UNREVIEWED" {
		return fmt.Errorf("%s: invalid disposition", row.MappingID)
	}
	if row.Signoff != "PENDING_HUMAN_SIGNOFF" || row.DispositionReason != "" || row.TargetMappingID != "" || len(row.DecisionEvidence) != 0 || row.CandidateV2OperationID != "PENDING_HUMAN_DESIGN" || row.CandidateV2Method != "PENDING_HUMAN_DESIGN" || row.CandidateV2Path != "PENDING_HUMAN_DESIGN" {
		return fmt.Errorf("%s: unreviewed row claims a decision", row.MappingID)
	}
	return nil
}
func main() {
	mappingPath := flag.String("mapping", "../docs/api-mapping.jsonl", "mapping JSONL")
	routesPath := flag.String("routes", "../docs/evidence/p1/legacy-routes-6cb989c.json", "legacy route authority")
	completion := flag.Bool("completion", false, "require P1 product signoff")
	flag.Parse()
	routeFile, err := os.Open(*routesPath)
	if err != nil {
		panic(err)
	}
	defer routeFile.Close()
	var routes routeDocument
	if err := json.NewDecoder(routeFile).Decode(&routes); err != nil {
		panic(err)
	}
	mappingFile, err := os.Open(*mappingPath)
	if err != nil {
		panic(err)
	}
	defer mappingFile.Close()
	rows, count, err := validate(mappingFile, routes, 781)
	if err != nil {
		fmt.Fprintln(os.Stderr, "api-mapping:", err)
		os.Exit(1)
	}
	pending := 0
	for _, row := range rows {
		if row.Disposition == "UNREVIEWED" {
			pending++
		}
	}
	if *completion && pending > 0 {
		fmt.Fprintf(os.Stderr, "api-mapping P1 completion: PENDING_HUMAN_SIGNOFF (%d routes)\n", pending)
		os.Exit(2)
	}
	fmt.Printf("api-mapping: PASS (rows=781 partitions=%d/%d/%d pending=%d partial=%d/%d)\n", count.s02, count.s03, count.s04, pending, count.partialInput, count.partialOutput)
}

// replacement-baseline creates the stage-0 replacement ledgers.  It is a
// deliberately conservative inventory: absence of a frozen, cross-layer
// target is recorded as UNCLASSIFIED rather than inferred from an old route.
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	legacySnapshotSHA = "6cb989c071255437d75953dabb943318a74eb8f4"
	legacyMainSHA     = "aa71de28140ca78851c2db3dfd824ad0a2cce224"
	currentMainSHA    = "f50f5a37a4949d2ac85f612fbc99a3e7326d4dcd"
)

type matrixRecord struct {
	ID, Page, Section, Action, TriggeredAPI, ExpectedResult, Disposition, Implementation, Verification, Evidence, TargetFeatureID, ImplementationEvidence, VerificationEvidence string
}

type apiRecord struct {
	MappingID       string   `json:"mapping_id"`
	LegacyPath      string   `json:"legacy_path"`
	LegacyRouteName string   `json:"legacy_route_name"`
	Partition       string   `json:"partition"`
	SourceEvidence  []string `json:"source_evidence"`
	ManifestMethods []string `json:"manifest_methods"`
	MethodBundle    []string `json:"manifest_method_bundle"`
	CandidateOp     string   `json:"candidate_v2_operation_id"`
	CandidateMethod string   `json:"candidate_v2_method"`
	CandidatePath   string   `json:"candidate_v2_path"`
	TargetMappingID string   `json:"target_mapping_id"`
	Disposition     string   `json:"disposition"`
	Signoff         string   `json:"signoff"`
	Discrepancies   []string `json:"discrepancy_flags"`
	Manifest        struct {
		Audience        string `json:"audience"`
		ExternalEffects string `json:"external_effects"`
		Layer           string `json:"layer"`
		Path            string `json:"path"`
		AuthScheme      string `json:"auth_scheme"`
		RequiresAuth    bool   `json:"requires_auth"`
		CSRF            bool   `json:"csrf"`
		AccessScope     string `json:"access_scope"`
		CapabilityOwner string `json:"capability_owner"`
	} `json:"manifest_contract"`
}

type migrationRecord struct {
	MappingID, LegacyTable, LegacyDomain, SourcePresence, TargetSchemaStatus, WatermarkStrategy, FKStrategy, SafetyRule, Decision, Implementation, Verification string
	CandidateTargets                                                                                                                                            []string
	FieldCount                                                                                                                                                  int
}

type output struct {
	capabilities [][]string
	routes       [][]string
	protocols    [][]string
	effects      [][]string
	migrations   [][]string
	deltas       [][]string
	assets       [][]string
}

func main() {
	write := flag.Bool("write", false, "write deterministic replacement ledgers")
	check := flag.Bool("check", false, "validate committed replacement ledgers")
	matrix := flag.String("matrix", "../docs/feature-matrix.csv", "feature Matrix CSV")
	api := flag.String("api", "../docs/api-mapping.jsonl", "API mapping JSONL")
	migration := flag.String("migration", "../docs/migration-mapping.jsonl", "migration mapping JSONL")
	outDir := flag.String("out", "../docs/replacement", "replacement ledger directory")
	flag.Parse()
	if *write == *check {
		fatal(errors.New("choose exactly one of -write or -check"))
	}
	generated, err := build(*matrix, *api, *migration)
	if err != nil {
		fatal(err)
	}
	if *write {
		if err := writeAll(*outDir, generated); err != nil {
			fatal(err)
		}
		return
	}
	if err := validate(*outDir, generated); err != nil {
		fatal(err)
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "replacement-baseline:", err); os.Exit(1) }

func build(matrixPath, apiPath, migrationPath string) (output, error) {
	matrix, err := loadMatrix(matrixPath)
	if err != nil {
		return output{}, err
	}
	apis, err := loadAPIs(apiPath)
	if err != nil {
		return output{}, err
	}
	migrations, err := loadMigrations(migrationPath)
	if err != nil {
		return output{}, err
	}
	assets, err := frozenAssets()
	if err != nil {
		return output{}, err
	}
	result := output{}
	result.assets = assets
	for _, row := range matrix {
		disposition, status := classifyMatrix(row)
		result.capabilities = append(result.capabilities, []string{
			row.ID, "FEATURE_MATRIX", unknown(row.Page), unknown(row.Section), unknown(row.Action), unknown(row.TriggeredAPI), unknown(row.ExpectedResult), unknown(row.TargetFeatureID),
			disposition, status, unknown(row.Implementation), unknown(row.Verification), unknown(row.ImplementationEvidence), unknown(row.VerificationEvidence),
			"UNMAPPED", "UNMAPPED", "UNMAPPED", "UNMAPPED", "UNMAPPED", "UNMAPPED", "UNMAPPED", "NOT_EXECUTED", "UNMAPPED",
			"docs/feature-matrix.csv:" + row.ID, "Matrix is source evidence, not V2 completion evidence.",
		})
	}
	for _, row := range apis {
		disposition, certainty := classifyRoute(row)
		evidence := strings.Join(row.SourceEvidence, ";")
		if evidence == "" {
			evidence = "docs/api-mapping.jsonl:" + row.MappingID
		}
		result.routes = append(result.routes, []string{
			row.MappingID, unknown(row.Partition), unknown(row.LegacyPath), unknown(row.LegacyRouteName), unknown(strings.Join(row.ManifestMethods, "|")), unknown(strings.Join(row.MethodBundle, "|")), unknown(row.Manifest.Path),
			unknown(row.Manifest.Audience), unknown(row.Manifest.Layer), unknown(row.Manifest.AuthScheme), fmt.Sprint(row.Manifest.RequiresAuth), fmt.Sprint(row.Manifest.CSRF), unknown(row.Manifest.AccessScope), unknown(row.Manifest.CapabilityOwner),
			unknown(row.Manifest.ExternalEffects), unknown(row.CandidateMethod), unknown(row.CandidatePath), unknown(row.CandidateOp), unknown(row.TargetMappingID), unknown(row.Disposition), unknown(row.Signoff), noneOr(strings.Join(row.Discrepancies, "|")), disposition, certainty, evidence,
		})
		if disposition == "EXTERNAL_PROTOCOL" {
			result.protocols = append(result.protocols, []string{
				row.MappingID, unknown(row.LegacyPath), unknown(strings.Join(row.ManifestMethods, "|")), unknown(row.Manifest.Path), unknown(row.Manifest.Audience), unknown(row.Manifest.AuthScheme), fmt.Sprint(row.Manifest.RequiresAuth), fmt.Sprint(row.Manifest.CSRF), unknown(row.Manifest.AccessScope), unknown(row.Manifest.CapabilityOwner),
				unknown(row.CandidateMethod), unknown(row.CandidatePath), unknown(row.CandidateOp), unknown(row.TargetMappingID), unknown(row.Disposition), unknown(row.Signoff), noneOr(strings.Join(row.Discrepancies, "|")), unknown(row.Manifest.ExternalEffects),
				"INVENTORIED", evidence, "NOT_EXECUTED", "Protocol endpoint is inventoried; method, auth, acknowledgement, and adapter contract remain unmapped unless independently evidenced.",
			})
		}
		if row.Manifest.ExternalEffects != "" && row.Manifest.ExternalEffects != "none" {
			result.effects = append(result.effects, []string{
				row.MappingID, unknown(row.LegacyPath), unknown(row.Manifest.CapabilityOwner), unknown(row.Manifest.AuthScheme), unknown(row.Manifest.AccessScope), unknown(row.Manifest.ExternalEffects), "EXTERNAL_AUTHORIZATION_REQUIRED", "NOT_EXECUTED", evidence,
				"No provider, deployment, dispatch, or external effect was run by this baseline.",
			})
		}
	}
	for _, row := range migrations {
		layer := classifyMigration(row.Decision)
		result.migrations = append(result.migrations, []string{
			row.MappingID, unknown(row.LegacyTable), unknown(row.LegacyDomain), unknown(row.SourcePresence), unknown(row.Decision), layer, unknown(row.TargetSchemaStatus), unknown(strings.Join(row.CandidateTargets, "|")), unknown(row.WatermarkStrategy), unknown(row.FKStrategy), unknown(row.SafetyRule),
			fmt.Sprint(row.FieldCount), unknown(row.Implementation), unknown(row.Verification), "docs/migration-mapping.jsonl:" + row.MappingID,
			"NOT_EXECUTED", "The mapping is a migration decision, not proof that data moved.",
		})
	}
	// The supplied 6cb -> aa71 source comparison is initial replacement catch-up,
	// not a closed post-freeze delta or a claim that this worktree modified repo1.
	result.deltas = [][]string{{
		"REPO1-INITIAL-CATCHUP", legacyMainSHA, legacySnapshotSHA + ".." + legacyMainSHA, "INITIAL_REPLACEMENT_CATCHUP",
		"UNCLASSIFIED_SOURCE_DRIFT", "UNMAPPED", "UNMAPPED", "NOT_EXECUTED", "OPEN",
		"Formal freeze is advisory only and not externally enforced. No exact V2 absorption SHA or verification evidence exists.",
	}}
	sortRows(result.capabilities)
	sortRows(result.routes)
	sortRows(result.protocols)
	sortRows(result.effects)
	sortRows(result.migrations)
	return result, nil
}

func frozenAssets() ([][]string, error) {
	ledger, err := os.ReadFile(repoFile("docs/evidence/p4/backend-capability-ledger.md"))
	if err != nil {
		return nil, err
	}
	migrations := map[string]string{
		"00054": "00054_service_period_member_grid_management.sql",
		"00061": "00061_survey_operations_local_config.sql",
		"00063": "00063_group_ops_local_plans.sql",
		"00064": "00064_service_period_members.sql",
		"00065": "00065_customer_contact_policies.sql",
		"00066": "00066_campaign_initiation_snapshots.sql",
		"00067": "00067_campaign_touch_plan_review_handoff.sql",
		"00068": "00068_outbound_campaign_handoff_acceptance.sql",
		"00069": "00069_sidebar_customer_profile_receipts.sql",
		"00070": "00070_contact_owner_reassignment.sql",
	}
	rows, packageID, inside := [][]string{}, "", false
	for _, line := range strings.Split(string(ledger), "\n") {
		if strings.Contains(line, "p4-backend-freeze-operation-ids:start") {
			inside = true
			continue
		}
		if strings.Contains(line, "p4-backend-freeze-operation-ids:end") {
			break
		}
		if !inside {
			continue
		}
		if strings.HasPrefix(line, "### ") {
			parts := strings.Fields(strings.TrimPrefix(line, "### "))
			if len(parts) > 0 {
				packageID = parts[0]
			}
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if migrations[packageID] == "" {
				return nil, fmt.Errorf("frozen P4 asset has unknown package %q", packageID)
			}
			rows = append(rows, []string{packageID, migrations[packageID], strings.TrimSpace(strings.TrimPrefix(line, "- ")), "docs/evidence/p4/backend-capability-ledger.md", "V2_LOCAL_BACKEND", "NOT_EXECUTED"})
		}
	}
	rows = append(rows,
		[]string{"00071", "00071_customer_safe_exports.sql", "createCustomerSafeExport", "docs/evidence/p4/customer-safe-export-local-core-v2-backend.md", "V2_LOCAL_BACKEND", "NOT_EXECUTED"},
		[]string{"00071", "00071_customer_safe_exports.sql", "getCustomerSafeExport", "docs/evidence/p4/customer-safe-export-local-core-v2-backend.md", "V2_LOCAL_BACKEND", "NOT_EXECUTED"},
		[]string{"00071", "00071_customer_safe_exports.sql", "downloadCustomerSafeExport", "docs/evidence/p4/customer-safe-export-local-core-v2-backend.md", "V2_LOCAL_BACKEND", "NOT_EXECUTED"},
	)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][0] == rows[j][0] {
			return rows[i][2] < rows[j][2]
		}
		return rows[i][0] < rows[j][0]
	})
	return rows, nil
}

func loadMatrix(path string) ([]matrixRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, errors.New("feature Matrix is empty")
	}
	index := columnIndex(records[0])
	want := []string{"feature_id", "page", "section", "action", "triggered_api", "expected_result", "disposition", "implementation", "verification", "source_evidence", "target_feature_id", "implementation_evidence", "verification_evidence"}
	for _, key := range want {
		if _, ok := index[key]; !ok {
			return nil, fmt.Errorf("feature Matrix missing %s", key)
		}
	}
	rows := make([]matrixRecord, 0, len(records)-1)
	for _, row := range records[1:] {
		if len(row) != len(records[0]) {
			return nil, errors.New("feature Matrix has malformed CSV row")
		}
		rows = append(rows, matrixRecord{
			ID: row[index["feature_id"]], Page: row[index["page"]], Section: row[index["section"]], Action: row[index["action"]], TriggeredAPI: row[index["triggered_api"]], ExpectedResult: row[index["expected_result"]],
			Disposition: row[index["disposition"]], Implementation: row[index["implementation"]], Verification: row[index["verification"]], Evidence: row[index["source_evidence"]], TargetFeatureID: row[index["target_feature_id"]],
			ImplementationEvidence: row[index["implementation_evidence"]], VerificationEvidence: row[index["verification_evidence"]],
		})
	}
	return rows, nil
}

func loadAPIs(path string) ([]apiRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	rows := []apiRecord{}
	for {
		var row apiRecord
		err := decoder.Decode(&row)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if row.MappingID == "" {
			return nil, errors.New("API mapping missing mapping_id")
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func loadMigrations(path string) ([]migrationRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	rows := []migrationRecord{}
	for {
		var raw struct {
			MappingID          string     `json:"mapping_id"`
			LegacyTable        string     `json:"legacy_table"`
			LegacyDomain       string     `json:"legacy_domain"`
			SourcePresence     string     `json:"source_presence"`
			TargetSchemaStatus string     `json:"target_schema_status"`
			CandidateTargets   []string   `json:"candidate_targets"`
			WatermarkStrategy  string     `json:"watermark_strategy"`
			FKStrategy         string     `json:"fk_strategy"`
			SafetyRule         string     `json:"safety_rule"`
			Decision           string     `json:"decision"`
			Implementation     string     `json:"implementation"`
			Verification       string     `json:"verification"`
			FieldMappings      []struct{} `json:"field_mappings"`
		}
		err := decoder.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if raw.MappingID == "" {
			return nil, errors.New("migration mapping missing mapping_id")
		}
		rows = append(rows, migrationRecord{MappingID: raw.MappingID, LegacyTable: raw.LegacyTable, LegacyDomain: raw.LegacyDomain, SourcePresence: raw.SourcePresence, TargetSchemaStatus: raw.TargetSchemaStatus, CandidateTargets: raw.CandidateTargets, WatermarkStrategy: raw.WatermarkStrategy, FKStrategy: raw.FKStrategy, SafetyRule: raw.SafetyRule, Decision: raw.Decision, Implementation: raw.Implementation, Verification: raw.Verification, FieldCount: len(raw.FieldMappings)})
	}
	return rows, nil
}

func classifyMatrix(row matrixRecord) (string, string) {
	if row.Disposition == "DEPRECATED" {
		return "RETIREMENT_APPROVED", "RETIRED"
	}
	if row.TriggeredAPI == "none (client-only)" {
		return "UI_ONLY", "REPLACED_BY_NEW_FRONTEND"
	}
	return "BACKEND_REQUIRED", "UNMAPPED"
}

func classifyRoute(row apiRecord) (string, string) {
	if row.MappingID == "LEGACY-API-0053" {
		return "UNCLASSIFIED_SOURCE_DRIFT", "UNCLASSIFIED"
	}
	// These audiences are delivered outside an authenticated admin page.  They
	// must remain protocol inventory even if a legacy client happened to render
	// them as H5, callback, or integration UI.
	if row.Manifest.Audience == "public_h5" || row.Manifest.Audience == "callback" || row.Manifest.Audience == "external_integration" {
		return "EXTERNAL_PROTOCOL", "INVENTORIED"
	}
	if publicStableProtocol[row.MappingID] {
		return "EXTERNAL_PROTOCOL", "INVENTORIED"
	}
	return "UNCLASSIFIED", "UNCLASSIFIED"
}

// These six public browser/authentication URLs have a stable legacy protocol
// despite their admin audience metadata. They are explicit source facts, not
// an inference from an external-effect flag.
var publicStableProtocol = map[string]bool{
	"LEGACY-API-0753": true, "LEGACY-API-0754": true, "LEGACY-API-0755": true,
	"LEGACY-API-0758": true, "LEGACY-API-0759": true, "LEGACY-API-0760": true,
}

func classifyMigration(decision string) string {
	switch decision {
	case "MIGRATE", "REBUILD", "RESET_RUNTIME", "MANUAL_REENTRY":
		return "BACKEND_REQUIRED"
	case "ARCHIVE_ONLY", "DROP", "NOT_APPLICABLE":
		return "RETIREMENT_APPROVED"
	case "DEFER":
		return "DEFERRED_UNMAPPED"
	default:
		return "UNCLASSIFIED"
	}
}

func columnIndex(header []string) map[string]int {
	out := make(map[string]int, len(header))
	for i, value := range header {
		out[value] = i
	}
	return out
}

func unknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "UNMAPPED"
	}
	return value
}

func noneOr(value string) string {
	if strings.TrimSpace(value) == "" {
		return "NONE"
	}
	return value
}

func sortRows(rows [][]string) {
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
}

func writeAll(dir string, data output) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for name, rows := range ledgerFiles(data) {
		if err := writeCSV(filepath.Join(dir, name), rows); err != nil {
			return err
		}
	}
	return writeReadiness(filepath.Join(dir, "cutover-readiness.md"), data)
}

func ledgerFiles(data output) map[string][][]string {
	return map[string][][]string{
		"backend-capability-ledger.csv":       append([][]string{{"capability_id", "source_kind", "page", "section", "action", "triggered_api", "expected_result", "target_feature_id", "disposition", "capability_status", "legacy_implementation", "legacy_verification", "implementation_evidence", "verification_evidence", "domain_owner", "canonical_method", "canonical_path", "canonical_operation_id", "actor_scope", "security_refs", "protocol_refs", "external_effect_status", "migration_refs", "source_evidence", "notes"}}, data.capabilities...),
		"legacy-route-disposition-ledger.csv": append([][]string{{"mapping_id", "partition", "legacy_path", "legacy_route_name", "manifest_methods", "manifest_method_bundle", "manifest_path", "audience", "layer", "auth_scheme", "requires_auth", "csrf", "access_scope", "capability_owner", "legacy_external_effects", "candidate_v2_method", "candidate_v2_path", "candidate_v2_operation_id", "target_mapping_id", "legacy_disposition", "signoff", "discrepancy_flags", "route_disposition", "classification_status", "source_evidence"}}, data.routes...),
		"external-protocol-ledger.csv":        append([][]string{{"mapping_id", "legacy_path", "manifest_methods", "manifest_path", "audience", "auth_scheme", "requires_auth", "csrf", "access_scope", "capability_owner", "candidate_v2_method", "candidate_v2_path", "candidate_v2_operation_id", "target_mapping_id", "legacy_disposition", "signoff", "discrepancy_flags", "legacy_external_effects", "protocol_status", "source_evidence", "external_effect_status", "notes"}}, data.protocols...),
		"external-effects-ledger.csv":         append([][]string{{"mapping_id", "legacy_path", "capability_owner", "auth_scheme", "access_scope", "legacy_external_effects", "authorization_gate", "effect_status", "source_evidence", "notes"}}, data.effects...),
		"data-migration-ledger.csv":           append([][]string{{"mapping_id", "legacy_table", "legacy_domain", "source_presence", "legacy_decision", "replacement_disposition", "target_schema_status", "candidate_targets", "watermark_strategy", "fk_strategy", "safety_rule", "field_mapping_count", "legacy_implementation", "legacy_verification", "source_evidence", "migration_execution_status", "notes"}}, data.migrations...),
		"post-freeze-delta-ledger.csv":        append([][]string{{"delta_id", "formal_freeze_sha", "legacy_commit_or_range", "change_class", "impact_items", "absorption_pr", "absorption_sha", "verification", "status", "notes"}}, data.deltas...),
		"frozen-local-assets.csv":             append([][]string{{"package_id", "migration", "operation_id", "source_evidence", "capability_layer", "external_effect_status"}}, data.assets...),
	}
}

func writeCSV(path string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	err = w.WriteAll(rows)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeReadiness(path string, data output) error {
	text := fmt.Sprintf(`# P4 Backend Replacement Cutover Readiness

## Frozen sources

- V2 formal baseline: origin/main@%s (PR #482 merge).
- Legacy snapshot authority: AI-CRM-main@%s; declared authoritative repo1
  origin/main process freeze: %s. Its enforcement is ADVISORY_ONLY and
  NOT_EXTERNALLY_ENFORCED; this worktree did not write repo1 or absorb it.
- Input denominators: Matrix %d, API mapping/routes %d, migration mapping %d.

## Layered inventory, not a release claim

- Matrix disposition: 283 BACKEND_REQUIRED, 3 UI_ONLY, 8
  RETIREMENT_APPROVED. BACKEND_REQUIRED is UNMAPPED unless independent V2
  evidence is listed in frozen-local-assets.csv; Matrix evidence never upgrades
  a capability to domain/API/PG verification.
- Migration disposition: 86 BACKEND_REQUIRED, 72 RETIREMENT_APPROVED, 158
  DEFERRED_UNMAPPED. No data migration is marked executed.
- Route actual breakdown: %d EXTERNAL_PROTOCOL, %d UNCLASSIFIED, and %d
  UNCLASSIFIED_SOURCE_DRIFT. Public H5, callback, external-integration, and
  declared external-effect routes remain protocol inventory. In particular,
  LEGACY-API-0778 preserves the public URL protocol but does not recreate old
  HTML; its backing read capability remains unmapped. LEGACY-API-0053 remains
  UNCLASSIFIED_SOURCE_DRIFT because api-mapping and route-triage disagree.
- Frozen V2 local assets in frozen-local-assets.csv are 11 packages / 76 unique operationIds: the prior
  10-package/73-operation P4 receipt inventory plus PR #482 Customer Safe
  Export (00071, 3 operations). It is a V2 backend asset and does not revive
  deprecated USER OPS or alter legacy Matrix rows.

## Gate status

NOT_READY. UNCLASSIFIED routes, UNCLASSIFIED_SOURCE_DRIFT, DEFERRED_UNMAPPED
migrations, UNMAPPED capabilities, and every NOT_EXECUTED external effect block
cutover. This baseline verifies ledger structure and evidence references only.
It makes no claim about main/Nightly success, deployment, data migration,
Provider execution, payment/refund, callbacks, shadow traffic, or any external
effect. Those are independent exit gates recorded in their own ledgers.
`, currentMainSHA, legacySnapshotSHA, legacyMainSHA, len(data.capabilities), len(data.routes), len(data.migrations), countAt(data.routes, 22, "EXTERNAL_PROTOCOL"), countAt(data.routes, 22, "UNCLASSIFIED"), countAt(data.routes, 22, "UNCLASSIFIED_SOURCE_DRIFT"))
	return os.WriteFile(path, []byte(text), 0644)
}

func countAt(rows [][]string, index int, value string) int {
	count := 0
	for _, row := range rows {
		if row[index] == value {
			count++
		}
	}
	return count
}

func validate(dir string, expected output) error {
	for name, want := range ledgerFiles(expected) {
		got, err := readCSV(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if !equalCSV(got, want) {
			return fmt.Errorf("%s is not the deterministic projection of its authoritative source", name)
		}
		seen := map[string]bool{}
		for _, row := range got[1:] {
			key := row[0]
			if name == "frozen-local-assets.csv" {
				key = row[0] + ":" + row[2]
			}
			if key == "" || seen[key] {
				return fmt.Errorf("%s has non-unique primary key", name)
			}
			seen[key] = true
		}
	}
	if err := validateBreakdowns(expected); err != nil {
		return err
	}
	if err := validateReadiness(filepath.Join(dir, "cutover-readiness.md")); err != nil {
		return err
	}
	return validateFrozenAssets()
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

func equalCSV(left, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if len(left[i]) != len(right[i]) {
			return false
		}
		for j := range left[i] {
			if left[i][j] != right[i][j] {
				return false
			}
		}
	}
	return true
}

func validateBreakdowns(data output) error {
	matrix := map[string]int{}
	for _, row := range data.capabilities {
		matrix[row[8]]++
		if !oneOf(row[8], "BACKEND_REQUIRED", "UI_ONLY", "RETIREMENT_APPROVED") || !oneOf(row[9], "UNMAPPED", "REPLACED_BY_NEW_FRONTEND", "RETIRED") {
			return fmt.Errorf("invalid capability state %q/%q for %s", row[8], row[9], row[0])
		}
		if row[8] == "BACKEND_REQUIRED" && row[9] != "UNMAPPED" {
			return fmt.Errorf("Matrix backend item %s was promoted without V2 evidence", row[0])
		}
	}
	if matrix["BACKEND_REQUIRED"] != 283 || matrix["UI_ONLY"] != 3 || matrix["RETIREMENT_APPROVED"] != 8 {
		return fmt.Errorf("unexpected Matrix breakdown: %#v", matrix)
	}
	migrations := map[string]int{}
	for _, row := range data.migrations {
		migrations[row[5]]++
		if !oneOf(row[5], "BACKEND_REQUIRED", "RETIREMENT_APPROVED", "DEFERRED_UNMAPPED", "UNCLASSIFIED") || row[15] != "NOT_EXECUTED" {
			return fmt.Errorf("invalid migration ledger state for %s", row[0])
		}
	}
	if migrations["BACKEND_REQUIRED"] != 86 || migrations["RETIREMENT_APPROVED"] != 72 || migrations["DEFERRED_UNMAPPED"] != 158 {
		return fmt.Errorf("unexpected migration breakdown: %#v", migrations)
	}
	if len(data.routes) != 781 {
		return fmt.Errorf("unexpected route count: %d", len(data.routes))
	}
	routeDisposition := map[string]int{}
	for _, row := range data.routes {
		routeDisposition[row[22]]++
		if !oneOf(row[22], "EXTERNAL_PROTOCOL", "UNCLASSIFIED", "UNCLASSIFIED_SOURCE_DRIFT") || !oneOf(row[23], "INVENTORIED", "UNCLASSIFIED") {
			return fmt.Errorf("invalid route state for %s", row[0])
		}
		if row[7] == "public_h5" && row[22] == "FRONTEND_PAGE_ONLY" {
			return fmt.Errorf("public H5 route %s was incorrectly treated as UI-only", row[0])
		}
		if row[0] == "LEGACY-API-0053" && (row[22] != "UNCLASSIFIED_SOURCE_DRIFT" || row[23] != "UNCLASSIFIED") {
			return errors.New("LEGACY-API-0053 source drift was lost")
		}
	}
	if routeDisposition["EXTERNAL_PROTOCOL"] != 178 || routeDisposition["UNCLASSIFIED"] != 602 || routeDisposition["UNCLASSIFIED_SOURCE_DRIFT"] != 1 {
		return fmt.Errorf("unexpected route breakdown: %#v", routeDisposition)
	}
	for _, row := range data.protocols {
		if row[18] != "INVENTORIED" || row[20] != "NOT_EXECUTED" {
			return fmt.Errorf("protocol %s was promoted beyond its evidence", row[0])
		}
	}
	for _, row := range data.effects {
		if row[7] != "NOT_EXECUTED" || row[6] != "EXTERNAL_AUTHORIZATION_REQUIRED" {
			return fmt.Errorf("external effect %s was promoted", row[0])
		}
	}
	if len(data.assets) != 76 || countAt(data.assets, 0, "00071") != 3 {
		return fmt.Errorf("frozen local asset inventory is not 11 packages / 76 operations")
	}
	packages := map[string]bool{}
	for _, row := range data.assets {
		packages[row[0]] = true
		if row[4] != "V2_LOCAL_BACKEND" || row[5] != "NOT_EXECUTED" {
			return fmt.Errorf("invalid frozen local asset state for %s", row[2])
		}
	}
	if len(packages) != 11 {
		return fmt.Errorf("frozen local asset package count = %d, want 11", len(packages))
	}
	for _, row := range data.deltas {
		if row[8] == "CLOSED" && (row[6] == "" || row[7] == "" || row[7] == "NOT_EXECUTED") {
			return fmt.Errorf("delta %s closed without exact absorption SHA and evidence", row[0])
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateReadiness(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, required := range []string{"NOT_READY", "11 packages / 76 unique operationIds", "does not revive", "no claim"} {
		if !strings.Contains(string(b), required) {
			return fmt.Errorf("cutover readiness missing %q", required)
		}
	}
	return nil
}

func validateFrozenAssets() error {
	ledger, err := os.ReadFile("../docs/evidence/p4/backend-capability-ledger.md")
	if err != nil {
		return err
	}
	inside, ids := false, []string{}
	for _, line := range strings.Split(string(ledger), "\n") {
		if strings.Contains(line, "p4-backend-freeze-operation-ids:start") {
			inside = true
			continue
		}
		if strings.Contains(line, "p4-backend-freeze-operation-ids:end") {
			inside = false
			continue
		}
		if inside && strings.HasPrefix(line, "- ") {
			ids = append(ids, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	if len(ids) != 73 {
		return fmt.Errorf("frozen P4 receipt operation count = %d, want 73", len(ids))
	}
	ids = append(ids, "createCustomerSafeExport", "getCustomerSafeExport", "downloadCustomerSafeExport")
	openAPI, err := os.ReadFile(repoFile("api/openapi.yaml"))
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !strings.Contains(string(openAPI), "operationId: "+id) {
			return fmt.Errorf("frozen asset operation %s is absent from OpenAPI", id)
		}
	}
	if len(ids) != 76 {
		return fmt.Errorf("frozen P4 replacement operation count = %d, want 76", len(ids))
	}
	return nil
}

func repoFile(path string) string {
	for _, candidate := range []string{filepath.Join("..", path), filepath.Join("..", "..", path)} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("..", path)
}

package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	legacySHA   = "6cb989c071255437d75953dabb943318a74eb8f4"
	manifestSHA = "710a01ee3813051b4ec13de8ef8b8ad64b39bc380b3a5a81c669580df24b488e"
)

type routeDoc struct {
	SchemaVersion        int               `json:"schema_version"`
	SourceKind           string            `json:"source_kind"`
	SourceCommit         string            `json:"source_commit"`
	SourceManifest       string            `json:"source_manifest"`
	SourceManifestSHA256 string            `json:"source_manifest_sha256"`
	RouteCount           int               `json:"route_count"`
	Routes               []json.RawMessage `json:"routes"`
}

type routeFact struct {
	Path, Name, Owner string
	Methods           []string
	Canonical         []byte
}

type lifecycleIndex struct {
	SchemaVersion           int              `json:"schema_version"`
	LegacySourceSHA         string           `json:"legacy_source_sha"`
	LifecycleManifestSHA256 string           `json:"lifecycle_manifest_sha256"`
	TableCount              int              `json:"table_count"`
	Tables                  []lifecycleTable `json:"tables"`
}

type lifecycleTable struct {
	LegacyTable     string `json:"legacy_table"`
	LegacyDomain    string `json:"legacy_domain"`
	LegacyLifecycle string `json:"legacy_lifecycle"`
	MigrationSource string `json:"migration_source"`
	SourceLine      int    `json:"source_line"`
}

type field struct{ Source, Target, Reason string }
type migrationFact struct {
	Table, Presence, Domain, Lifecycle, Source, Recommendation, Safety, Decision, Signoff, Implementation, Verification string
	Columns, Targets                                                                                                    []string
	Fields                                                                                                              []field
	Evidence                                                                                                            []string
}

type paths struct{ routes, api, triage, lifecycle, migration string }

var identity = regexp.MustCompile(`(?i)(external_?userid|unionid|openid|mobile|phone)`)

var approvedNotMigratedRoutes = map[string]bool{
	"LEGACY-API-0012": true, "LEGACY-API-0383": true, "LEGACY-API-0598": true,
	"LEGACY-API-0607": true, "LEGACY-API-0640": true, "LEGACY-API-0678": true,
	"LEGACY-API-0683": true, "LEGACY-API-0684": true, "LEGACY-API-0686": true,
	"LEGACY-API-0702": true, "LEGACY-API-0704": true, "LEGACY-API-0748": true,
}

type apiDecisionEvidence struct {
	DecisionID string `json:"decision_id"`
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
	Decision   string `json:"decision"`
}

func main() {
	routes := flag.String("routes", "../docs/evidence/p1/legacy-routes-6cb989c.json", "P1-S01 route manifest")
	api := flag.String("api", "../docs/api-mapping.jsonl", "API candidate mapping")
	triage := flag.String("triage", "../docs/evidence/p1/route-triage.csv", "G1 route triage decisions")
	lifecycle := flag.String("lifecycle", "../docs/evidence/p1/migration-lifecycle-index-6cb989c.json", "legacy lifecycle index")
	migration := flag.String("migration", "../docs/migration-mapping.jsonl", "migration mapping")
	flag.Parse()
	result, err := reconcile(paths{*routes, *api, *triage, *lifecycle, *migration})
	if err != nil {
		fmt.Fprintln(os.Stderr, "p1-reconciliation:", err)
		os.Exit(1)
	}
	fmt.Println(result)
}

func reconcile(p paths) (string, error) {
	routes, err := loadRoutes(p.routes)
	if err != nil {
		return "", err
	}
	tiers, err := loadTriage(p.triage)
	if err != nil {
		return "", err
	}
	counts, decisions, err := reconcileAPI(p.api, routes, tiers)
	if err != nil {
		return "", err
	}
	tables, err := loadLifecycle(p.lifecycle)
	if err != nil {
		return "", err
	}
	fields, pending, err := reconcileMigration(p.migration, tables)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("p1-reconciliation: PASS (routes=781 s02=%d s03=%d s04=%d migrate_routes=%d deferred_post_launch_routes=%d not_migrated_routes=%d tables=316 fields=%d pending_routes=0 pending_tables=%d)", counts[0], counts[1], counts[2], decisions[0], decisions[1], decisions[2], fields, pending), nil
}

func loadTriage(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("route triage: %w", err)
	}
	if len(records) != 782 {
		return nil, fmt.Errorf("route triage rows=%d, want 781", len(records)-1)
	}
	columns := map[string]int{}
	for index, name := range records[0] {
		columns[name] = index
	}
	for _, required := range []string{"mapping_id", "recommended_tier", "human_signoff"} {
		if _, ok := columns[required]; !ok {
			return nil, fmt.Errorf("route triage lacks %s", required)
		}
	}
	tiers, counts := make(map[string]string, 781), map[string]int{}
	for index, record := range records[1:] {
		id := record[columns["mapping_id"]]
		if id != fmt.Sprintf("LEGACY-API-%04d", index+1) || tiers[id] != "" {
			return nil, fmt.Errorf("route triage line %d has unstable identity", index+2)
		}
		tier := record[columns["recommended_tier"]]
		if !oneOf(tier, "A", "B", "C") || record[columns["human_signoff"]] != "APPROVED" {
			return nil, fmt.Errorf("%s has invalid tier or pending signoff", id)
		}
		tiers[id] = tier
		counts[tier]++
	}
	if counts["A"] != 501 || counts["B"] != 268 || counts["C"] != 12 {
		return nil, fmt.Errorf("route triage tier mismatch: A=%d B=%d C=%d", counts["A"], counts["B"], counts["C"])
	}
	return tiers, nil
}

func loadRoutes(path string) (map[string]routeFact, error) {
	var doc routeDoc
	if err := strictJSONFile(path, &doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != 1 || doc.SourceCommit != legacySHA || doc.SourceManifestSHA256 != "3bb11a48c8bbc520fb9da5128726594232bca0b4e0f0c7ed1f63bb4b3c2263bd" || doc.RouteCount != 781 || len(doc.Routes) != 781 {
		return nil, errors.New("route manifest header or inventory mismatch")
	}
	result := make(map[string]routeFact, 781)
	for _, raw := range doc.Routes {
		fact, err := parseRoute(raw)
		if err != nil {
			return nil, err
		}
		key := routeKey(fact.Path, fact.Name, fact.Methods)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate route: %s", key)
		}
		result[key] = fact
	}
	return result, nil
}

func parseRoute(raw json.RawMessage) (routeFact, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return routeFact{}, err
	}
	path, err := text(fields, "path")
	if err != nil {
		return routeFact{}, err
	}
	name, err := text(fields, "route_name")
	if err != nil {
		return routeFact{}, err
	}
	owner, err := text(fields, "capability_owner")
	if err != nil {
		return routeFact{}, err
	}
	methods, err := texts(fields, "methods")
	if err != nil || len(methods) == 0 {
		return routeFact{}, errors.New("route has invalid methods")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return routeFact{}, err
	}
	canonical, _ := json.Marshal(value)
	return routeFact{path, name, owner, methods, canonical}, nil
}

func reconcileAPI(path string, routes map[string]routeFact, tiers map[string]string) ([3]int, [3]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return [3]int{}, [3]int{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	seen, counts, decisions := map[string]bool{}, [3]int{}, [3]int{}
	line := 0
	for scanner.Scan() {
		line++
		var row map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return counts, decisions, fmt.Errorf("API line %d: %w", line, err)
		}
		id, _ := text(row, "mapping_id")
		if id != fmt.Sprintf("LEGACY-API-%04d", line) {
			return counts, decisions, fmt.Errorf("API line %d has unstable mapping_id", line)
		}
		pathValue, e1 := text(row, "legacy_path")
		name, e2 := text(row, "legacy_route_name")
		methods, e3 := texts(row, "manifest_methods")
		if e1 != nil || e2 != nil || e3 != nil {
			return counts, decisions, fmt.Errorf("%s has incomplete route key", id)
		}
		key := routeKey(pathValue, name, methods)
		authority, ok := routes[key]
		if !ok || seen[key] {
			return counts, decisions, fmt.Errorf("%s has missing or duplicate authority route", id)
		}
		seen[key] = true
		var embedded any
		if err := json.Unmarshal(row["manifest_contract"], &embedded); err != nil {
			return counts, decisions, fmt.Errorf("%s has invalid embedded manifest", id)
		}
		canonical, _ := json.Marshal(embedded)
		if !bytes.Equal(canonical, authority.Canonical) {
			return counts, decisions, fmt.Errorf("%s embedded manifest drifted", id)
		}
		partition, _ := text(row, "partition")
		expected := expectedPartition(authority)
		if partition != expected {
			return counts, decisions, fmt.Errorf("%s is in %s, want %s", id, partition, expected)
		}
		counts[map[string]int{"S02": 0, "S03": 1, "S04": 2}[partition]]++
		if value, _ := text(row, "legacy_source_sha"); value != legacySHA {
			return counts, decisions, fmt.Errorf("%s has wrong legacy SHA", id)
		}
		disposition, _ := text(row, "disposition")
		signoff, _ := text(row, "signoff")
		operation, _ := text(row, "candidate_v2_operation_id")
		method, _ := text(row, "candidate_v2_method")
		candidatePath, _ := text(row, "candidate_v2_path")
		var evidence []apiDecisionEvidence
		if err := json.Unmarshal(row["decision_evidence"], &evidence); err != nil {
			return counts, decisions, fmt.Errorf("%s has invalid decision evidence", id)
		}
		reason, _ := text(row, "disposition_reason")
		targetMapping, _ := text(row, "target_mapping_id")
		tier := tiers[id]
		switch tier {
		case "A":
			expectedEvidence := apiDecisionEvidence{"G1-D02", "repository_owner", "2026-08-10", "MIGRATE"}
			if disposition != "MIGRATE" || signoff != "APPROVED" || operation != "PENDING_HUMAN_DESIGN" || method != "PENDING_HUMAN_DESIGN" || candidatePath != "PENDING_HUMAN_DESIGN" || targetMapping != "" || reason != "G1-D02 approved tier A route for 1:1 legacy semantic migration; target v2 operation remains domain-contract work." || len(evidence) != 1 || evidence[0] != expectedEvidence {
				return counts, decisions, fmt.Errorf("%s has unapproved or forged tier A disposition", id)
			}
			decisions[0]++
		case "B":
			expectedEvidence := apiDecisionEvidence{"G1-D02", "repository_owner", "2026-08-10", "DEFERRED_POST_LAUNCH"}
			if disposition != "DEFERRED_POST_LAUNCH" || signoff != "APPROVED" || operation != "PENDING_HUMAN_DESIGN" || method != "PENDING_HUMAN_DESIGN" || candidatePath != "PENDING_HUMAN_DESIGN" || targetMapping != "" || reason != "G1-D02 deferred tier B route until post-launch reassessment; this is not deprecation or NOT_MIGRATED." || len(evidence) != 1 || evidence[0] != expectedEvidence {
				return counts, decisions, fmt.Errorf("%s has unapproved or forged tier B disposition", id)
			}
			decisions[1]++
		case "C":
			if !approvedNotMigratedRoutes[id] || disposition != "NOT_MIGRATED" || signoff != "APPROVED" || operation != "NOT_APPLICABLE" || method != "NOT_APPLICABLE" || candidatePath != "NOT_APPLICABLE" || targetMapping != "" || reason != "G1-D01 approved tier C route as not migrated." || len(evidence) != 1 || evidence[0] != (apiDecisionEvidence{"G1-D01", "repository_owner", "2026-08-10", "NOT_MIGRATED"}) {
				return counts, decisions, fmt.Errorf("%s has unapproved or forged tier C disposition", id)
			}
			decisions[2]++
		default:
			return counts, decisions, fmt.Errorf("%s lacks signed tier", id)
		}
	}
	if err := scanner.Err(); err != nil {
		return counts, decisions, err
	}
	if line != 781 || len(seen) != len(routes) || counts != [3]int{156, 184, 441} || decisions != [3]int{501, 268, 12} {
		return counts, decisions, fmt.Errorf("route partition or decision mismatch: rows=%d seen=%d partitions=%v decisions=%v", line, len(seen), counts, decisions)
	}
	return counts, decisions, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func expectedPartition(route routeFact) string {
	s02 := map[string]bool{"customer_read_model": true, "customer_tags": true, "identity_contact": true, "sidebar_write": true, "admin_auth": true, "admin_config": true, "admin_jobs": true}
	if s02[route.Owner] || route.Name == "oauth_token" {
		return "S02"
	}
	s03 := map[string]bool{"ai_audience_ops": true, "auth_wecom": true, "channel_entry": true, "ops_enrollment": true, "send_content": true}
	if s03[route.Owner] {
		return "S03"
	}
	automation := []string{"/api/admin/automation-conversion/group-ops", "/api/automation/group-ops", "/api/admin/channels", "/api/admin/channel-welcome-materials", "/api/admin/wecom-customer-acquisition-links", "/admin/channels", "/admin/automation-conversion/group-ops"}
	platform := []string{"/api/admin/external-effects", "/api/external-effects", "/api/admin/push-center"}
	if route.Owner == "automation_engine" && under(route.Path, automation) || route.Owner == "platform_foundation" && (under(route.Path, platform) || route.Path == "/api/admin/wecom/execution-diagnostics") {
		return "S03"
	}
	return "S04"
}

func under(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}
func routeKey(path, name string, methods []string) string {
	copyMethods := append([]string(nil), methods...)
	sort.Strings(copyMethods)
	return path + "\x00" + name + "\x00" + strings.Join(copyMethods, ",")
}

func loadLifecycle(path string) (map[string]lifecycleTable, error) {
	var index lifecycleIndex
	if err := strictJSONFile(path, &index); err != nil {
		return nil, err
	}
	if index.SchemaVersion != 1 || index.LegacySourceSHA != legacySHA || index.LifecycleManifestSHA256 != manifestSHA || index.TableCount != 316 || len(index.Tables) != 316 {
		return nil, errors.New("lifecycle index header or inventory mismatch")
	}
	result, previous := map[string]lifecycleTable{}, ""
	for _, table := range index.Tables {
		if table.LegacyTable <= previous || table.LegacyDomain == "" || table.LegacyLifecycle == "" || table.MigrationSource == "" || table.SourceLine < 1 {
			return nil, errors.New("invalid lifecycle table")
		}
		result[table.LegacyTable] = table
		previous = table.LegacyTable
	}
	return result, nil
}

func reconcileMigration(path string, lifecycle map[string]lifecycleTable) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	seen, fields, pending := map[string]bool{}, 0, 0
	for scanner.Scan() {
		row, err := parseMigration(scanner.Bytes())
		if err != nil {
			return 0, 0, err
		}
		indexed, ok := lifecycle[row.Table]
		if !ok || seen[row.Table] || indexed.LegacyDomain != row.Domain || indexed.LegacyLifecycle != row.Lifecycle || indexed.MigrationSource != row.Source {
			return 0, 0, fmt.Errorf("migration table missing, duplicate, or drifted: %s", row.Table)
		}
		seen[row.Table] = true
		mapped := map[string]bool{}
		for _, field := range row.Fields {
			if mapped[field.Source] || strings.TrimSpace(field.Reason) == "" || !strings.Contains(field.Reason, row.Table+"."+field.Source) || !strings.Contains(field.Reason, field.Target) {
				return 0, 0, fmt.Errorf("%s.%s has missing or unbound reason", row.Table, field.Source)
			}
			if identity.MatchString(field.Source) && (strings.HasPrefix(field.Target, "planned:customers.") || strings.HasPrefix(field.Target, "physical:customers.")) {
				return 0, 0, fmt.Errorf("identity field targets customers: %s.%s", row.Table, field.Source)
			}
			mapped[field.Source] = true
			fields++
		}
		if len(row.Columns) != len(mapped) {
			return 0, 0, fmt.Errorf("%s field coverage mismatch", row.Table)
		}
		for _, column := range row.Columns {
			if !mapped[column] {
				return 0, 0, fmt.Errorf("%s.%s is unmapped", row.Table, column)
			}
		}
		joined := strings.Join(row.Targets, " ")
		if strings.Contains(joined, "outbound_") && !strings.Contains(row.Safety, "never reactivate legacy execution or sending") || strings.Contains(joined, "automations") && !strings.Contains(row.Safety, "no migration action, external call, job enqueue, provider retry, or runtime activation") {
			return 0, 0, fmt.Errorf("%s can reactivate execution", row.Table)
		}
		if row.Recommendation == "FRAMEWORK_ONLY" {
			if row.Decision != "NOT_APPLICABLE" || row.Signoff != "NOT_REQUIRED" || len(row.Evidence) == 0 {
				return 0, 0, fmt.Errorf("%s has invalid framework decision", row.Table)
			}
		} else {
			decision := expectedMigrationDecision(row)
			evidence := []string{"G1-D02-2026-08-10", "approved_by=repository_owner", "approved_at=2026-08-10", "decision=" + decision}
			if decision == "" || row.Decision != decision || row.Signoff != "APPROVED" || !equalText(row.Evidence, evidence) {
				return 0, 0, fmt.Errorf("%s has fake migration signoff", row.Table)
			}
		}
		if row.Implementation != "NOT_STARTED" || row.Verification != "NOT_RUN" {
			return 0, 0, fmt.Errorf("%s claims migration execution", row.Table)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if len(seen) != 316 || len(seen) != len(lifecycle) || fields != 3313 || pending != 0 {
		return 0, 0, fmt.Errorf("migration inventory mismatch: tables=%d fields=%d pending=%d", len(seen), fields, pending)
	}
	return fields, pending, nil
}

func expectedMigrationDecision(row migrationFact) string {
	if row.Presence == "ABSENT_AT_HEAD" {
		return "DEFER"
	}
	return map[string]string{
		"MIGRATE_CANDIDATE":        "MIGRATE",
		"ARCHIVE_ONLY_CANDIDATE":   "ARCHIVE_ONLY",
		"DROP_CANDIDATE":           "DROP",
		"MANUAL_REENTRY_CANDIDATE": "MANUAL_REENTRY",
		"REBUILD_CANDIDATE":        "REBUILD",
		"RESET_RUNTIME_CANDIDATE":  "RESET_RUNTIME",
		"PENDING_TARGET_SCHEMA":    "DEFER",
	}[row.Recommendation]
}

func equalText(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func parseMigration(raw []byte) (migrationFact, error) {
	var row map[string]json.RawMessage
	if err := json.Unmarshal(raw, &row); err != nil {
		return migrationFact{}, err
	}
	get := func(key string) string { v, _ := text(row, key); return v }
	fact := migrationFact{Table: get("legacy_table"), Presence: get("source_presence"), Domain: get("legacy_domain"), Lifecycle: get("legacy_lifecycle"), Source: get("migration_source"), Recommendation: get("recommendation"), Safety: get("safety_rule"), Decision: get("decision"), Signoff: get("signoff"), Implementation: get("implementation"), Verification: get("verification")}
	var columns []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(row["legacy_columns"], &columns); err != nil {
		return fact, err
	}
	for _, c := range columns {
		fact.Columns = append(fact.Columns, c.Name)
	}
	var fields []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(row["field_mappings"], &fields); err != nil {
		return fact, err
	}
	for _, f := range fields {
		fact.Fields = append(fact.Fields, field{f.Source, f.Target, f.Reason})
	}
	if err := json.Unmarshal(row["candidate_targets"], &fact.Targets); err != nil {
		return fact, err
	}
	if err := json.Unmarshal(row["decision_evidence"], &fact.Evidence); err != nil {
		return fact, err
	}
	return fact, nil
}

func strictJSONFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}
func text(fields map[string]json.RawMessage, key string) (string, error) {
	var value string
	if err := json.Unmarshal(fields[key], &value); err != nil || value == "" {
		return "", fmt.Errorf("missing text field: %s", key)
	}
	return value, nil
}
func texts(fields map[string]json.RawMessage, key string) ([]string, error) {
	var value []string
	if err := json.Unmarshal(fields[key], &value); err != nil {
		return nil, fmt.Errorf("invalid string list: %s", key)
	}
	return value, nil
}

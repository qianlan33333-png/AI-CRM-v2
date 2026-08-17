package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	legacySHA            = "6cb989c071255437d75953dabb943318a74eb8f4"
	lifecycleManifestSHA = "710a01ee3813051b4ec13de8ef8b8ad64b39bc380b3a5a81c669580df24b488e"
	g1D02Evidence        = "G1-D02-2026-08-10"
)

type column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Ordinal  int    `json:"ordinal"`
}

type fieldMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

type lifecycleTable struct {
	LegacyTable     string `json:"legacy_table"`
	LegacyDomain    string `json:"legacy_domain"`
	LegacyLifecycle string `json:"legacy_lifecycle"`
	MigrationSource string `json:"migration_source"`
	SourceLine      int    `json:"source_line"`
}

type lifecycleIndex struct {
	SchemaVersion           int              `json:"schema_version"`
	LegacySourceSHA         string           `json:"legacy_source_sha"`
	LifecycleManifestSHA256 string           `json:"lifecycle_manifest_sha256"`
	TableCount              int              `json:"table_count"`
	Tables                  []lifecycleTable `json:"tables"`
}

type mappingRow struct {
	MappingID          string         `json:"mapping_id"`
	LegacyTable        string         `json:"legacy_table"`
	SourcePresence     string         `json:"source_presence"`
	LegacyLifecycle    string         `json:"legacy_lifecycle"`
	LegacyDomain       string         `json:"legacy_domain"`
	MigrationSource    string         `json:"migration_source"`
	LegacyColumns      []column       `json:"legacy_columns"`
	Recommendation     string         `json:"recommendation"`
	CandidateTargets   []string       `json:"candidate_targets"`
	TargetSchemaStatus string         `json:"target_schema_status"`
	FieldMappings      []fieldMapping `json:"field_mappings"`
	ConversionRule     string         `json:"conversion_rule"`
	DefaultStrategy    string         `json:"default_strategy"`
	DropReason         string         `json:"drop_reason"`
	SafetyRule         string         `json:"safety_rule"`
	LegacyKeyStrategy  string         `json:"legacy_key_strategy"`
	WatermarkStrategy  string         `json:"watermark_strategy"`
	FKStrategy         string         `json:"fk_strategy"`
	LegacySourceSHA    string         `json:"legacy_source_sha"`
	SourceEvidence     []string       `json:"source_evidence"`
	Decision           string         `json:"decision"`
	Implementation     string         `json:"implementation"`
	Verification       string         `json:"verification"`
	Signoff            string         `json:"signoff"`
	DecisionEvidence   []string       `json:"decision_evidence"`
	Notes              string         `json:"notes"`
}

type expected struct{ rows, physical, framework, columns int }

// ownershipCatalog is the narrow authority needed to validate target tables.
// The table-ownership document is the canonical declaration; this checker must
// not grow a second, historical business-table registry when a new owner is
// introduced.
type ownershipCatalog struct {
	tables map[string]string
}

type physicalSchema struct {
	tables map[string]map[string]bool
}

var (
	namePattern         = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	targetTable, target = regexp.MustCompile(`^(planned|physical):[a-z][a-z0-9_]*$`), regexp.MustCompile(`^(planned|physical):[a-z][a-z0-9_]*[.][a-z][a-z0-9_]*$`)
	identityCol         = regexp.MustCompile(`(?i)(external_?userid|unionid|openid|mobile|phone)`)
	migrationFile       = regexp.MustCompile(`^([0-9]{5})_[a-z0-9][a-z0-9_]*[.]sql$`)
	createTable         = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:public[.])?([a-z][a-z0-9_]*)\s*\((.*?)\);`)
	special             = set("PENDING_TARGET_SCHEMA", "DROP_WITH_REASON", "ARCHIVE_ONLY", "MANUAL_REENTRY", "REBUILD_FROM_CANONICAL", "RESET_RUNTIME_STATE", "FRAMEWORK_METADATA")
)

func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func expectedDecision(row mappingRow) string {
	if row.SourcePresence == "ABSENT_AT_HEAD" {
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

func approvedEvidence(decision string) []string {
	return []string{g1D02Evidence, "approved_by=repository_owner", "approved_at=2026-08-10", "decision=" + decision}
}

func equalStrings(left, right []string) bool {
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

func validate(input io.Reader, index lifecycleIndex, ownership ownershipCatalog, schema physicalSchema, want expected) ([]mappingRow, error) {
	indexed, err := validateLifecycleIndex(index, want.rows)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	rows, tables, ids := []mappingRow{}, map[string]bool{}, map[string]bool{}
	physical, framework, columns := 0, 0, 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var row mappingRow
		decoder := json.NewDecoder(strings.NewReader(string(line)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&row); err != nil {
			return nil, fmt.Errorf("line %d: %w", len(rows)+1, err)
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return nil, fmt.Errorf("line %d: trailing JSON value", len(rows)+1)
		}
		if row.MappingID != fmt.Sprintf("LEGACY-T14-%03d", len(rows)+1) {
			return nil, fmt.Errorf("line %d: unstable mapping_id", len(rows)+1)
		}
		if !namePattern.MatchString(row.LegacyTable) || tables[row.LegacyTable] || ids[row.MappingID] {
			return nil, fmt.Errorf("%s: duplicate or invalid identity", row.MappingID)
		}
		tables[row.LegacyTable], ids[row.MappingID] = true, true
		if row.LegacySourceSHA != legacySHA || row.MigrationSource == "" || row.LegacyLifecycle == "" || row.LegacyDomain == "" || len(row.SourceEvidence) == 0 {
			return nil, fmt.Errorf("%s: incomplete legacy evidence", row.MappingID)
		}
		indexedTable, ok := indexed[row.LegacyTable]
		if !ok || indexedTable.LegacyDomain != row.LegacyDomain || indexedTable.LegacyLifecycle != row.LegacyLifecycle || indexedTable.MigrationSource != row.MigrationSource {
			return nil, fmt.Errorf("%s: lifecycle index disagrees with mapping", row.MappingID)
		}
		if !oneOf(row.SourcePresence, "HEAD_PHYSICAL", "ABSENT_AT_HEAD", "FRAMEWORK_METADATA") {
			return nil, fmt.Errorf("%s: invalid source_presence", row.MappingID)
		}
		if !oneOf(row.Recommendation, "MIGRATE_CANDIDATE", "ARCHIVE_ONLY_CANDIDATE", "DROP_CANDIDATE", "MANUAL_REENTRY_CANDIDATE", "REBUILD_CANDIDATE", "RESET_RUNTIME_CANDIDATE", "PENDING_TARGET_SCHEMA", "FRAMEWORK_ONLY") {
			return nil, fmt.Errorf("%s: invalid recommendation", row.MappingID)
		}
		if !oneOf(row.TargetSchemaStatus, "FROZEN_PHYSICAL", "PENDING_TARGET_SCHEMA", "NO_TARGET") {
			return nil, fmt.Errorf("%s: invalid target schema status", row.MappingID)
		}
		for _, candidate := range row.CandidateTargets {
			if !validCandidateTarget(candidate, ownership, schema) {
				return nil, fmt.Errorf("%s: invalid candidate target", row.MappingID)
			}
		}
		if len(row.CandidateTargets) > 0 && row.TargetSchemaStatus == "NO_TARGET" {
			return nil, fmt.Errorf("%s: candidate target has no target schema", row.MappingID)
		}
		if row.TargetSchemaStatus == "FROZEN_PHYSICAL" {
			for _, candidate := range row.CandidateTargets {
				if !strings.HasPrefix(candidate, "physical:") {
					return nil, fmt.Errorf("%s: frozen target is not physical", row.MappingID)
				}
			}
			for _, field := range row.FieldMappings {
				if !special[field.Target] && !strings.HasPrefix(field.Target, "physical:") {
					return nil, fmt.Errorf("%s: frozen field target is not physical", row.MappingID)
				}
			}
		}
		if row.Recommendation == "MIGRATE_CANDIDATE" && (len(row.CandidateTargets) == 0 || !strings.Contains(row.LegacyKeyStrategy, "IMPORT_LEDGER_REQUIRED")) {
			return nil, fmt.Errorf("%s: migration candidate lacks target or import ledger", row.MappingID)
		}
		if !oneOf(row.WatermarkStrategy, "UPDATED_AT_PLUS_KEY", "CREATED_AT_PLUS_KEY", "FULL_ONLY", "PENDING_SOURCE_SCHEMA") {
			return nil, fmt.Errorf("%s: invalid watermark strategy", row.MappingID)
		}
		if row.ConversionRule == "" || row.DefaultStrategy == "" || row.SafetyRule == "" || row.LegacyKeyStrategy == "" || row.WatermarkStrategy == "" || row.FKStrategy == "" {
			return nil, fmt.Errorf("%s: incomplete conversion contract", row.MappingID)
		}
		if row.SourcePresence == "HEAD_PHYSICAL" {
			physical++
			columns += len(row.LegacyColumns)
		}
		if row.SourcePresence == "FRAMEWORK_METADATA" {
			framework++
		}
		if row.SourcePresence == "ABSENT_AT_HEAD" && (len(row.LegacyColumns) != 0 || len(row.FieldMappings) != 0) {
			return nil, fmt.Errorf("%s: absent table carries physical columns", row.MappingID)
		}
		if row.SourcePresence != "ABSENT_AT_HEAD" {
			if err := validateFields(row, ownership, schema); err != nil {
				return nil, err
			}
		}
		if row.Decision == "NOT_APPLICABLE" && row.Signoff == "NOT_REQUIRED" && row.Recommendation == "FRAMEWORK_ONLY" && len(row.DecisionEvidence) > 0 {
			// Framework metadata is outside the business migration decision set.
		} else if decision := expectedDecision(row); decision != "" && row.Decision == decision && row.Signoff == "APPROVED" && equalStrings(row.DecisionEvidence, approvedEvidence(decision)) {
			// G1-D02 approved the recommendation, while absent source tables fail closed to DEFER.
		} else {
			return nil, fmt.Errorf("%s: decision and signoff evidence disagree", row.MappingID)
		}
		if row.Implementation != "NOT_STARTED" || row.Verification != "NOT_RUN" {
			return nil, fmt.Errorf("%s: P1 mapping cannot claim implementation or verification", row.MappingID)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(rows) != want.rows || physical != want.physical || framework != want.framework || columns != want.columns {
		return nil, fmt.Errorf("inventory mismatch: rows=%d physical=%d framework=%d columns=%d", len(rows), physical, framework, columns)
	}
	if !sort.SliceIsSorted(rows, func(i, j int) bool { return rows[i].LegacyTable < rows[j].LegacyTable }) {
		return nil, errors.New("legacy tables must be sorted")
	}
	return rows, nil
}

func validateLifecycleIndex(index lifecycleIndex, want int) (map[string]lifecycleTable, error) {
	if index.SchemaVersion != 1 || index.LegacySourceSHA != legacySHA || index.LifecycleManifestSHA256 != lifecycleManifestSHA || index.TableCount != want || len(index.Tables) != want {
		return nil, errors.New("lifecycle index header or inventory mismatch")
	}
	tables := make(map[string]lifecycleTable, len(index.Tables))
	previous := ""
	for _, item := range index.Tables {
		if !namePattern.MatchString(item.LegacyTable) || item.LegacyTable <= previous || item.LegacyDomain == "" || item.LegacyLifecycle == "" || item.MigrationSource == "" || item.SourceLine < 1 {
			return nil, fmt.Errorf("lifecycle index has invalid table: %s", item.LegacyTable)
		}
		tables[item.LegacyTable] = item
		previous = item.LegacyTable
	}
	return tables, nil
}

func validateFields(row mappingRow, ownership ownershipCatalog, schema physicalSchema) error {
	columns, mappings := map[string]bool{}, map[string]string{}
	previousOrdinal := 0
	for _, item := range row.LegacyColumns {
		if !namePattern.MatchString(item.Name) || item.Type == "" || item.Ordinal <= previousOrdinal || columns[item.Name] {
			return fmt.Errorf("%s: invalid legacy column inventory", row.MappingID)
		}
		columns[item.Name] = true
		previousOrdinal = item.Ordinal
	}
	identityTarget, identityScope, identityProvenance := false, false, false
	for _, item := range row.FieldMappings {
		if !columns[item.Source] || mappings[item.Source] != "" || !validTarget(item.Target, ownership, schema) {
			return fmt.Errorf("%s: invalid or duplicate field mapping", row.MappingID)
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" || len(reason) > 1200 || strings.ContainsAny(reason, "\r\n") || !strings.Contains(reason, row.LegacyTable+"."+item.Source) || !strings.Contains(reason, item.Target) {
			return fmt.Errorf("%s: field mapping lacks a bound reason", row.MappingID)
		}
		if identityCol.MatchString(item.Source) && !strings.HasPrefix(item.Target, "planned:identities.") && !strings.HasPrefix(item.Target, "planned:pending_events.") && !special[item.Target] {
			return fmt.Errorf("%s: external identity has an unsafe target", row.MappingID)
		}
		identityTarget = identityTarget || strings.HasPrefix(item.Target, "planned:identities.")
		identityScope = identityScope || item.Target == "planned:identities.scope"
		identityProvenance = identityProvenance || (strings.HasPrefix(item.Target, "planned:identities.") && strings.Contains(strings.ToLower(reason), "provenance"))
		mappings[item.Source] = item.Target
	}
	if len(columns) != len(mappings) {
		return fmt.Errorf("%s: every legacy column must be mapped once", row.MappingID)
	}
	if identityTarget && (!identityScope || !strings.Contains(strings.ToLower(row.ConversionRule), "scope") || !identityProvenance) {
		return fmt.Errorf("%s: identity mapping lacks scoped provenance", row.MappingID)
	}
	if strings.Contains(strings.Join(row.CandidateTargets, " "), "outbound_") && !strings.Contains(row.SafetyRule, "never reactivate legacy execution or sending") {
		return fmt.Errorf("%s: outbound history lacks no-reactivation rule", row.MappingID)
	}
	if row.LegacyTable == "user_ops_do_not_disturb_next" && !strings.Contains(row.SafetyRule, "active suppression must remain effective") {
		return fmt.Errorf("%s: active suppression lacks a launch blocker", row.MappingID)
	}
	return nil
}

func loadLifecycleIndex(path string) (lifecycleIndex, error) {
	file, err := os.Open(path)
	if err != nil {
		return lifecycleIndex{}, err
	}
	defer file.Close()
	var index lifecycleIndex
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return lifecycleIndex{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return lifecycleIndex{}, errors.New("lifecycle index has a trailing JSON value")
	}
	return index, nil
}

func loadOwnership(path string) (ownershipCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return ownershipCatalog{}, err
	}
	defer file.Close()

	result := ownershipCatalog{tables: map[string]string{}}
	packages := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	inOwners, listMode, currentOwner := false, false, ""
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, " ") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			inOwners = line == "owners:"
			listMode, currentOwner = false, ""
			continue
		}
		if !inOwners {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.HasSuffix(line, ":") {
			owner := strings.TrimSuffix(strings.TrimSpace(line), ":")
			if !namePattern.MatchString(owner) || packages[owner] != "" {
				return ownershipCatalog{}, fmt.Errorf("table ownership has an invalid or duplicate owner: %q", owner)
			}
			packages[owner], currentOwner, listMode = "pending", owner, false
			continue
		}
		if currentOwner == "" {
			return ownershipCatalog{}, errors.New("table ownership has an entry outside an owner")
		}
		if strings.HasPrefix(line, "    package: ") {
			pkg := strings.TrimSpace(strings.TrimPrefix(line, "    package: "))
			if pkg != "internal/"+currentOwner {
				return ownershipCatalog{}, fmt.Errorf("table ownership has a non-canonical package for %s", currentOwner)
			}
			packages[currentOwner], listMode = pkg, false
			continue
		}
		if strings.HasPrefix(line, "    tables: [") && strings.HasSuffix(line, "]") {
			inlineTables := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "    tables: ["), "]"))
			if inlineTables == "" {
				listMode = false
				continue
			}
			for _, table := range strings.Split(inlineTables, ",") {
				if err := result.add(currentOwner, strings.TrimSpace(table)); err != nil {
					return ownershipCatalog{}, err
				}
			}
			listMode = false
			continue
		}
		if line == "    tables:" {
			listMode = true
			continue
		}
		if listMode && strings.HasPrefix(line, "      - ") {
			if err := result.add(currentOwner, strings.TrimSpace(strings.TrimPrefix(line, "      - "))); err != nil {
				return ownershipCatalog{}, err
			}
			continue
		}
		listMode = false
	}
	if err := scanner.Err(); err != nil {
		return ownershipCatalog{}, err
	}
	for owner, pkg := range packages {
		if pkg != "internal/"+owner {
			return ownershipCatalog{}, fmt.Errorf("table ownership lacks a canonical package for %s", owner)
		}
	}
	if len(result.tables) == 0 {
		return ownershipCatalog{}, errors.New("table ownership has no owned tables")
	}
	return result, nil
}

func (catalog ownershipCatalog) add(owner, table string) error {
	if !namePattern.MatchString(table) {
		return fmt.Errorf("table ownership has an invalid table for %s: %q", owner, table)
	}
	if existing := catalog.tables[table]; existing != "" {
		return fmt.Errorf("table ownership assigns %s to both %s and %s", table, existing, owner)
	}
	catalog.tables[table] = owner
	return nil
}

func (catalog ownershipCatalog) owns(table string) bool {
	return catalog.tables[table] != ""
}

func loadPhysicalSchema(path string) (physicalSchema, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return physicalSchema{}, err
	}
	schema := physicalSchema{tables: map[string]map[string]bool{}}
	seenNumbers := map[int]bool{}
	maxNumber, count := 0, 0
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 {
			return physicalSchema{}, fmt.Errorf("migration schema has a symlink: %s", entry.Name())
		}
		if !entry.Type().IsRegular() {
			continue
		}
		matches := migrationFile.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		number, _ := strconv.Atoi(matches[1])
		if seenNumbers[number] {
			return physicalSchema{}, fmt.Errorf("migration schema has duplicate number %05d", number)
		}
		seenNumbers[number] = true
		if number > maxNumber {
			maxNumber = number
		}
		count++
		data, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return physicalSchema{}, err
		}
		if down := bytes.Index(data, []byte("-- +goose Down")); down >= 0 {
			data = data[:down]
		}
		for _, match := range createTable.FindAllSubmatch(data, -1) {
			table := string(match[1])
			if schema.tables[table] != nil {
				return physicalSchema{}, fmt.Errorf("migration schema creates %s more than once", table)
			}
			columns := map[string]bool{}
			for _, rawLine := range bytes.Split(match[2], []byte("\n")) {
				fields := strings.Fields(strings.TrimSpace(string(rawLine)))
				if len(fields) == 0 {
					continue
				}
				column := strings.TrimSuffix(fields[0], ",")
				if oneOf(strings.ToUpper(column), "CONSTRAINT", "PRIMARY", "FOREIGN", "UNIQUE", "CHECK", "EXCLUDE") || !namePattern.MatchString(column) {
					continue
				}
				columns[column] = true
			}
			schema.tables[table] = columns
		}
	}
	if count == 0 || count != maxNumber {
		return physicalSchema{}, errors.New("migration schema has a duplicate or gap in numbered DDL")
	}
	return schema, nil
}

func validCandidateTarget(value string, ownership ownershipCatalog, schema physicalSchema) bool {
	if !targetTable.MatchString(value) {
		return false
	}
	parts := strings.SplitN(value, ":", 2)
	if parts[0] == "physical" {
		return ownership.owns(parts[1]) && schema.tables[parts[1]] != nil
	}
	// A planned target is non-executable by definition and remains valid only
	// while its row is PENDING_TARGET_SCHEMA. FROZEN_PHYSICAL targets above
	// must bind to an owned table created by numbered DDL.
	return true
}

func validTarget(value string, ownership ownershipCatalog, schema physicalSchema) bool {
	if special[value] {
		return true
	}
	if !target.MatchString(value) {
		return false
	}
	parts := strings.SplitN(strings.SplitN(value, ":", 2)[1], ".", 2)
	if strings.HasPrefix(value, "physical:") {
		return ownership.owns(parts[0]) && schema.tables[parts[0]][parts[1]]
	}
	return true
}

func main() {
	path := flag.String("mapping", "../docs/migration-mapping.jsonl", "mapping JSONL")
	indexPath := flag.String("index", "../docs/evidence/p1/migration-lifecycle-index-6cb989c.json", "frozen lifecycle index")
	ownershipPath := flag.String("ownership", "../docs/architecture/table-ownership.yml", "canonical table ownership")
	migrationsPath := flag.String("migrations", "../migrations", "numbered target DDL")
	completion := flag.Bool("completion", false, "require human signoff")
	flag.Parse()
	index, err := loadLifecycleIndex(*indexPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration-mapping:", err)
		os.Exit(1)
	}
	schema, err := loadPhysicalSchema(*migrationsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration-mapping:", err)
		os.Exit(1)
	}
	ownership, err := loadOwnership(*ownershipPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration-mapping:", err)
		os.Exit(1)
	}
	file, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	rows, err := validate(file, index, ownership, schema, expected{rows: 316, physical: 217, framework: 1, columns: 3312})
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration-mapping:", err)
		os.Exit(1)
	}
	pending := 0
	for _, row := range rows {
		if row.Signoff == "PENDING_HUMAN_SIGNOFF" {
			pending++
		}
	}
	if *completion && pending > 0 {
		fmt.Fprintf(os.Stderr, "migration-mapping P1 completion: PENDING_HUMAN_SIGNOFF (%d rows)\n", pending)
		os.Exit(2)
	}
	fmt.Printf("migration-mapping: PASS (rows=%d physical=217 columns=3312 pending=%d)\n", len(rows), pending)
}

package main

import (
	"bufio"
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

const legacySHA = "6cb989c071255437d75953dabb943318a74eb8f4"

type column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Ordinal  int    `json:"ordinal"`
}

type fieldMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
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

var (
	namePattern         = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	targetTable, target = regexp.MustCompile(`^(planned|physical):[a-z][a-z0-9_]*$`), regexp.MustCompile(`^(planned|physical):[a-z][a-z0-9_]*[.][a-z][a-z0-9_]*$`)
	identityCol         = regexp.MustCompile(`(?i)(external_?userid|unionid|openid|mobile|phone)`)
	planned             = set("staff", "channels", "stages", "customers", "tag_groups", "tags", "customer_tags", "customer_events", "segments", "segment_members", "automations", "automation_enrollments", "outbound_batches", "outbound_tasks", "event_log", "surveys", "survey_submissions", "identities", "customer_merges", "pending_events", "ai_prompts", "ai_generations", "settings", "settings_audit", "admin_users", "stats_daily", "wecom_sync_state", "migration_import_ledger")
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

func validate(input io.Reader, want expected) ([]mappingRow, error) {
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
			if !validCandidateTarget(candidate) {
				return nil, fmt.Errorf("%s: invalid candidate target", row.MappingID)
			}
		}
		if len(row.CandidateTargets) > 0 && row.TargetSchemaStatus != "PENDING_TARGET_SCHEMA" {
			return nil, fmt.Errorf("%s: candidate target is not physical", row.MappingID)
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
			if err := validateFields(row); err != nil {
				return nil, err
			}
		}
		if row.Decision == "UNREVIEWED" && row.Signoff == "PENDING_HUMAN_SIGNOFF" && len(row.DecisionEvidence) == 0 {
			// Honest initial candidate.
		} else if row.Decision == "NOT_APPLICABLE" && row.Signoff == "NOT_REQUIRED" && row.Recommendation == "FRAMEWORK_ONLY" && len(row.DecisionEvidence) > 0 {
			// Framework metadata is outside the business migration decision set.
		} else if oneOf(row.Decision, "MIGRATE", "ARCHIVE_ONLY", "DROP", "MANUAL_REENTRY", "REBUILD", "RESET_RUNTIME", "DEFER") && row.Signoff == "APPROVED" && len(row.DecisionEvidence) > 0 {
			// Human-approved row.
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

func validateFields(row mappingRow) error {
	columns, mappings := map[string]bool{}, map[string]string{}
	previousOrdinal := 0
	for _, item := range row.LegacyColumns {
		if !namePattern.MatchString(item.Name) || item.Type == "" || item.Ordinal <= previousOrdinal || columns[item.Name] {
			return fmt.Errorf("%s: invalid legacy column inventory", row.MappingID)
		}
		columns[item.Name] = true
		previousOrdinal = item.Ordinal
	}
	for _, item := range row.FieldMappings {
		if !columns[item.Source] || mappings[item.Source] != "" || !validTarget(item.Target) {
			return fmt.Errorf("%s: invalid or duplicate field mapping", row.MappingID)
		}
		if identityCol.MatchString(item.Source) && strings.HasPrefix(item.Target, "planned:customers.") {
			return fmt.Errorf("%s: external identity cannot target customers", row.MappingID)
		}
		mappings[item.Source] = item.Target
	}
	if len(columns) != len(mappings) {
		return fmt.Errorf("%s: every legacy column must be mapped once", row.MappingID)
	}
	if strings.Contains(strings.Join(row.CandidateTargets, " "), "outbound_") && !strings.Contains(row.SafetyRule, "never reactivate legacy execution or sending") {
		return fmt.Errorf("%s: outbound history lacks no-reactivation rule", row.MappingID)
	}
	if row.LegacyTable == "user_ops_do_not_disturb_next" && !strings.Contains(row.SafetyRule, "active suppression must remain effective") {
		return fmt.Errorf("%s: active suppression lacks a launch blocker", row.MappingID)
	}
	return nil
}

func validCandidateTarget(value string) bool {
	if !targetTable.MatchString(value) {
		return false
	}
	parts := strings.SplitN(value, ":", 2)
	return planned[parts[1]] && (parts[0] == "planned" || parts[1] == "stages")
}

func validTarget(value string) bool {
	if special[value] {
		return true
	}
	if !target.MatchString(value) {
		return false
	}
	parts := strings.SplitN(strings.SplitN(value, ":", 2)[1], ".", 2)
	return planned[parts[0]] && (strings.HasPrefix(value, "planned:") || parts[0] == "stages")
}

func main() {
	path := flag.String("mapping", "../docs/migration-mapping.jsonl", "mapping JSONL")
	completion := flag.Bool("completion", false, "require human signoff")
	flag.Parse()
	file, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()
	rows, err := validate(file, expected{rows: 316, physical: 217, framework: 1, columns: 3312})
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

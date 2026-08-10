package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func validRow() mappingRow {
	return mappingRow{
		MappingID: "LEGACY-T14-001", LegacyTable: "contacts", SourcePresence: "HEAD_PHYSICAL",
		LegacyLifecycle: "legacy", LegacyDomain: "contacts", MigrationSource: "0001 baseline",
		LegacyColumns:  []column{{Name: "name", Type: "text", Nullable: false, Ordinal: 1}},
		Recommendation: "MIGRATE_CANDIDATE", CandidateTargets: []string{"planned:customers"},
		TargetSchemaStatus: "PENDING_TARGET_SCHEMA", FieldMappings: []fieldMapping{{Source: "name", Target: "planned:customers.name", Reason: "Map contacts.name to planned:customers.name through the frozen conversion_rule."}},
		ConversionRule: "trim text", DefaultStrategy: "preserve nullability after signoff", DropReason: "none",
		SafetyRule: "fail closed", LegacyKeyStrategy: "SOURCE_PK:name; IMPORT_LEDGER_REQUIRED", WatermarkStrategy: "FULL_ONLY", FKStrategy: "quarantine unresolved references",
		LegacySourceSHA: legacySHA, SourceEvidence: []string{"migrations/baselines/0001_post_legacy.sql:537"},
		Decision: "MIGRATE", Implementation: "NOT_STARTED", Verification: "NOT_RUN", Signoff: "APPROVED",
		DecisionEvidence: approvedEvidence("MIGRATE"), Notes: "candidate only",
	}
}

func validIndex(rows ...mappingRow) lifecycleIndex {
	tables := make([]lifecycleTable, 0, len(rows))
	for i, row := range rows {
		tables = append(tables, lifecycleTable{LegacyTable: row.LegacyTable, LegacyDomain: row.LegacyDomain, LegacyLifecycle: row.LegacyLifecycle, MigrationSource: row.MigrationSource, SourceLine: i + 1})
	}
	return lifecycleIndex{SchemaVersion: 1, LegacySourceSHA: legacySHA, LifecycleManifestSHA256: lifecycleManifestSHA, TableCount: len(tables), Tables: tables}
}

func encoded(t *testing.T, rows ...mappingRow) *bytes.Reader {
	t.Helper()
	var data bytes.Buffer
	for _, row := range rows {
		if err := json.NewEncoder(&data).Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	return bytes.NewReader(data.Bytes())
}

func TestFrozenMapping(t *testing.T) {
	index, err := loadLifecycleIndex("../../docs/evidence/p1/migration-lifecycle-index-6cb989c.json")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("../../docs/migration-mapping.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := validate(file, index, expected{rows: 316, physical: 217, framework: 1, columns: 3312}); err != nil {
		t.Fatal(err)
	}
}

func TestValidationRejectsUnsafeMutations(t *testing.T) {
	tests := map[string]func(*mappingRow){
		"missing field": func(row *mappingRow) { row.FieldMappings = nil },
		"identity in customers": func(row *mappingRow) {
			row.LegacyColumns[0].Name = "external_userid"
			row.FieldMappings[0] = fieldMapping{Source: "external_userid", Target: "planned:customers.name", Reason: "Map contacts.external_userid to planned:customers.name."}
		},
		"identity in unrelated target": func(row *mappingRow) {
			row.LegacyColumns[0].Name = "openid"
			row.FieldMappings[0] = fieldMapping{Source: "openid", Target: "planned:settings.value", Reason: "Map contacts.openid to planned:settings.value."}
		},
		"missing field reason": func(row *mappingRow) { row.FieldMappings[0].Reason = "" },
		"forged field reason":  func(row *mappingRow) { row.FieldMappings[0].Reason = "Map another.value to planned:customers.name." },
		"outbound reactivation": func(row *mappingRow) {
			row.CandidateTargets = []string{"planned:outbound_tasks"}
			row.SafetyRule = "retry pending rows"
		},
		"missing import ledger": func(row *mappingRow) { row.LegacyKeyStrategy = "SOURCE_PK:name" },
		"forged decision":       func(row *mappingRow) { row.Decision = "DEFER" },
		"forged evidence":       func(row *mappingRow) { row.DecisionEvidence[0] = "G1-D02-FORGED" },
		"implementation claim":  func(row *mappingRow) { row.Implementation = "IMPLEMENTED" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row, data := mappingRow{}, encoded(t, validRow())
			if err := json.NewDecoder(data).Decode(&row); err != nil {
				t.Fatal(err)
			}
			mutate(&row)
			if _, err := validate(encoded(t, row), validIndex(row), expected{rows: 1, physical: 1, columns: 1}); err == nil {
				t.Fatal("mutation was accepted")
			}
		})
	}
}

func TestAbsentSourceMustDefer(t *testing.T) {
	row := validRow()
	row.SourcePresence = "ABSENT_AT_HEAD"
	row.LegacyColumns = nil
	row.FieldMappings = nil
	row.Decision = "DROP"
	row.DecisionEvidence = approvedEvidence("DROP")
	if _, err := validate(encoded(t, row), validIndex(row), expected{rows: 1}); err == nil {
		t.Fatal("absent source table was allowed to drop")
	}
}

func TestValidationRejectsLifecycleIndexDrift(t *testing.T) {
	row := validRow()
	for name, mutate := range map[string]func(*lifecycleIndex){
		"missing table":   func(index *lifecycleIndex) { index.Tables = nil },
		"domain mismatch": func(index *lifecycleIndex) { index.Tables[0].LegacyDomain = "wrong" },
		"manifest hash":   func(index *lifecycleIndex) { index.LifecycleManifestSHA256 = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			index := validIndex(row)
			mutate(&index)
			if _, err := validate(encoded(t, row), index, expected{rows: 1, physical: 1, columns: 1}); err == nil {
				t.Fatal("lifecycle mutation was accepted")
			}
		})
	}
}

func TestValidationRejectsUnscopedIdentityMapping(t *testing.T) {
	row := validRow()
	row.LegacyColumns[0].Name = "external_userid"
	row.CandidateTargets = []string{"planned:identities"}
	row.ConversionRule = "normalize with signed scope"
	row.FieldMappings[0] = fieldMapping{
		Source: "external_userid", Target: "planned:identities.normalized_value",
		Reason: "Map contacts.external_userid to planned:identities.normalized_value with signed scope and provenance.",
	}
	if _, err := validate(encoded(t, row), validIndex(row), expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("identity mapping without a scope field was accepted")
	}
}

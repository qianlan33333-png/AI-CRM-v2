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
		TargetSchemaStatus: "PENDING_TARGET_SCHEMA", FieldMappings: []fieldMapping{{Source: "name", Target: "planned:customers.name"}},
		ConversionRule: "trim text", DefaultStrategy: "preserve nullability after signoff", DropReason: "none",
		SafetyRule: "fail closed", LegacyKeyStrategy: "SOURCE_PK:name; IMPORT_LEDGER_REQUIRED", WatermarkStrategy: "FULL_ONLY", FKStrategy: "quarantine unresolved references",
		LegacySourceSHA: legacySHA, SourceEvidence: []string{"migrations/baselines/0001_post_legacy.sql:537"},
		Decision: "UNREVIEWED", Implementation: "NOT_STARTED", Verification: "NOT_RUN", Signoff: "PENDING_HUMAN_SIGNOFF",
		DecisionEvidence: []string{}, Notes: "candidate only",
	}
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
	file, err := os.Open("../../docs/migration-mapping.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := validate(file, expected{rows: 316, physical: 217, framework: 1, columns: 3312}); err != nil {
		t.Fatal(err)
	}
}

func TestValidationRejectsUnsafeMutations(t *testing.T) {
	tests := map[string]func(*mappingRow){
		"missing field": func(row *mappingRow) { row.FieldMappings = nil },
		"identity in customers": func(row *mappingRow) {
			row.LegacyColumns[0].Name = "external_userid"
			row.FieldMappings[0] = fieldMapping{Source: "external_userid", Target: "planned:customers.name"}
		},
		"outbound reactivation": func(row *mappingRow) {
			row.CandidateTargets = []string{"planned:outbound_tasks"}
			row.SafetyRule = "retry pending rows"
		},
		"missing import ledger": func(row *mappingRow) { row.LegacyKeyStrategy = "SOURCE_PK:name" },
		"fake signoff":          func(row *mappingRow) { row.Decision = "MIGRATE"; row.Signoff = "APPROVED" },
		"implementation claim":  func(row *mappingRow) { row.Implementation = "IMPLEMENTED" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			row, data := mappingRow{}, encoded(t, validRow())
			if err := json.NewDecoder(data).Decode(&row); err != nil {
				t.Fatal(err)
			}
			mutate(&row)
			if _, err := validate(encoded(t, row), expected{rows: 1, physical: 1, columns: 1}); err == nil {
				t.Fatal("mutation was accepted")
			}
		})
	}
}

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

func testOwnership(t *testing.T) ownershipCatalog {
	t.Helper()
	path := t.TempDir() + "/table-ownership.yml"
	data := `version: 1
owners:
  contact:
    package: internal/contact
    tables: [customers]
  media:
    package: internal/media
    tables: [media_images]
  product:
    package: internal/product
    tables: [products]
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	ownership, err := loadOwnership(path)
	if err != nil {
		t.Fatal(err)
	}
	return ownership
}

func testPhysicalSchema(t *testing.T) physicalSchema {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"00001_customers.sql": `-- +goose Up
CREATE TABLE customers (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL
);
-- +goose Down
DROP TABLE customers;
`,
		"00002_media.sql": `-- +goose Up
CREATE TABLE media_images (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL
);
-- +goose Down
DROP TABLE media_images;
`,
		"00003_products.sql": `-- +goose Up
CREATE TABLE products (
  id BIGINT PRIMARY KEY,
  name TEXT NOT NULL
);
-- +goose Down
DROP TABLE products;
`,
	}
	for name, data := range files {
		if err := os.WriteFile(dir+"/"+name, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := loadPhysicalSchema(dir)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestNumberedDDLChainFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"duplicate", map[string]string{"00001_one.sql": "CREATE TABLE one (id BIGINT);", "00001_two.sql": "CREATE TABLE two (id BIGINT);"}},
		{"gap", map[string]string{"00001_one.sql": "CREATE TABLE one (id BIGINT);", "00003_three.sql": "CREATE TABLE three (id BIGINT);"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tc.files {
				if err := os.WriteFile(dir+"/"+name, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadPhysicalSchema(dir); err == nil {
				t.Fatal("invalid numbered DDL chain was accepted")
			}
		})
	}
}

func TestPhysicalTargetCannotComeFromDownDDL(t *testing.T) {
	dir := t.TempDir()
	data := `-- +goose Up
SELECT 1;
-- +goose Down
CREATE TABLE media_images (id BIGINT);
`
	if err := os.WriteFile(dir+"/00001_down_only.sql", []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	schema, err := loadPhysicalSchema(dir)
	if err != nil {
		t.Fatal(err)
	}
	row := validRow()
	row.CandidateTargets = []string{"physical:media_images"}
	row.TargetSchemaStatus = "FROZEN_PHYSICAL"
	row.FieldMappings[0].Target = "physical:media_images.name"
	row.FieldMappings[0].Reason = "Map contacts.name to physical:media_images.name through the frozen conversion_rule."
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), schema, expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("down-only DDL was accepted as a physical target")
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
	ownership, err := loadOwnership("../../docs/architecture/table-ownership.yml")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := loadPhysicalSchema("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validate(file, index, ownership, schema, expected{rows: 316, physical: 217, framework: 1, columns: 3312}); err != nil {
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
			if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
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
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1}); err == nil {
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
			if _, err := validate(encoded(t, row), index, testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
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
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("identity mapping without a scope field was accepted")
	}
}

func TestPhysicalTargetsUseDeclaredOwnership(t *testing.T) {
	for _, target := range []string{"physical:media_images", "physical:products"} {
		t.Run(target, func(t *testing.T) {
			row := validRow()
			row.CandidateTargets = []string{target}
			row.TargetSchemaStatus = "FROZEN_PHYSICAL"
			row.FieldMappings[0].Target = target + ".name"
			row.FieldMappings[0].Reason = "Map contacts.name to " + target + ".name through the frozen conversion_rule."
			if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err != nil {
				t.Fatalf("declared physical target was rejected: %v", err)
			}
		})
	}
}

func TestTargetValidationFailsClosed(t *testing.T) {
	for _, target := range []string{
		"unknown:media_images", "physical:media_images/../products", "physical:unknown_table", "planned:products..name",
	} {
		t.Run(target, func(t *testing.T) {
			row := validRow()
			row.CandidateTargets = []string{target}
			if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
				t.Fatal("unsafe target was accepted")
			}
		})
	}

	path := t.TempDir() + "/duplicate-owner.yml"
	data := `owners:
  media:
    package: internal/media
    tables: [media_images]
  product:
    package: internal/product
    tables: [media_images]
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOwnership(path); err == nil {
		t.Fatal("cross-owner table declaration was accepted")
	}

	row := validRow()
	row.CandidateTargets = []string{"physical:media_images"}
	row.TargetSchemaStatus = "FROZEN_PHYSICAL"
	row.FieldMappings[0].Target = "physical:media_images.name"
	row.FieldMappings[0].Reason = "Map contacts.name to physical:media_images.name through the frozen conversion_rule."
	schema := testPhysicalSchema(t)
	delete(schema.tables, "media_images")
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), schema, expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("physical target without numbered DDL was accepted")
	}

	row = validRow()
	row.CandidateTargets = []string{"physical:media_images"}
	row.TargetSchemaStatus = "FROZEN_PHYSICAL"
	row.FieldMappings[0].Target = "physical:media_images.unknown_column"
	row.FieldMappings[0].Reason = "Map contacts.name to physical:media_images.unknown_column through the frozen conversion_rule."
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("physical target without a numbered DDL column was accepted")
	}

	row = validRow()
	row.CandidateTargets = []string{"planned:media_images"}
	row.TargetSchemaStatus = "FROZEN_PHYSICAL"
	row.FieldMappings[0].Target = "planned:media_images.name"
	row.FieldMappings[0].Reason = "Map contacts.name to planned:media_images.name through the frozen conversion_rule."
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), testPhysicalSchema(t), expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("planned target was accepted as frozen physical")
	}

	dir := t.TempDir()
	if err := os.WriteFile(dir+"/00001_migration_import_ledger.sql", []byte(`-- +goose Up
CREATE TABLE migration_import_ledger (
  legacy_pk TEXT NOT NULL
);
-- +goose Down
DROP TABLE migration_import_ledger;
`), 0600); err != nil {
		t.Fatal(err)
	}
	ledgerSchema, err := loadPhysicalSchema(dir)
	if err != nil {
		t.Fatal(err)
	}
	row = validRow()
	row.CandidateTargets = []string{"physical:migration_import_ledger"}
	row.TargetSchemaStatus = "FROZEN_PHYSICAL"
	row.FieldMappings[0].Target = "physical:migration_import_ledger.legacy_pk"
	row.FieldMappings[0].Reason = "Map contacts.name to physical:migration_import_ledger.legacy_pk through the frozen conversion_rule."
	if _, err := validate(encoded(t, row), validIndex(row), testOwnership(t), ledgerSchema, expected{rows: 1, physical: 1, columns: 1}); err == nil {
		t.Fatal("numbered DDL without canonical ownership was accepted")
	}
}

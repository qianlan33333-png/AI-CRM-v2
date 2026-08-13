package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryIdentityMigrationPassesIndexPolicy(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "00010_identity_storage.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckMigration(string(source)); err != nil {
		t.Fatalf("CheckMigration() error = %v", err)
	}
}

func TestPermanentForbiddenReceiptIndexCases(t *testing.T) {
	tests := []struct {
		name    string
		ddl     string
		message string
	}{
		{
			name:    "unquoted gin",
			ddl:     `CREATE INDEX receipt_payload_gin ON identity_operation_receipts USING GIN (payload);`,
			message: "GIN index",
		},
		{
			name:    "quoted index and table gin",
			ddl:     `CREATE INDEX "receipt_payload_gin" ON "identity_operation_receipts" USING "gin" (payload);`,
			message: "GIN index",
		},
		{
			name:    "schema qualified quoted gin",
			ddl:     `CREATE INDEX "public"."receipt_payload_gin" ON "public"."identity_operation_receipts" USING gin (payload);`,
			message: "GIN index",
		},
		{
			name:    "unquoted state only",
			ddl:     `CREATE INDEX receipt_state_idx ON identity_operation_receipts (state);`,
			message: "state-only index",
		},
		{
			name:    "quoted state only",
			ddl:     `CREATE INDEX "receipt_state_idx" ON "identity_operation_receipts" ("state");`,
			message: "state-only index",
		},
		{
			name:    "schema qualified state only",
			ddl:     `CREATE INDEX public.receipt_state_idx ON public.identity_operation_receipts (state);`,
			message: "state-only index",
		},
		{
			name: "drop then create leaves forbidden final state",
			ddl: `DROP INDEX IF EXISTS public.receipt_state_idx;
CREATE INDEX public.receipt_state_idx ON public.identity_operation_receipts (state);`,
			message: "state-only index",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckMigration(fixture(test.ddl))
			if err == nil {
				t.Fatal("CheckMigration() unexpectedly passed")
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("CheckMigration() error = %q, want substring %q", err, test.message)
			}
		})
	}
}

func TestPermanentQuotedIdentifierFixturesFail(t *testing.T) {
	tests := []struct {
		path    string
		message string
	}{
		{"testdata/illegal_quoted_gin.sql", "GIN index"},
		{"testdata/illegal_schema_state.sql", "state-only index"},
	}
	for _, test := range tests {
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			source, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			err = CheckMigration(string(source))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("CheckMigration() error = %v, want %q rejection", err, test.message)
			}
		})
	}
}

func TestDroppedForbiddenReceiptIndexesDoNotAffectFinalState(t *testing.T) {
	tests := []struct {
		name string
		ddl  string
	}{
		{
			name: "unquoted create then schema drop",
			ddl: `CREATE INDEX transient_gin ON public.identity_operation_receipts USING gin (payload);
DROP INDEX public.transient_gin;`,
		},
		{
			name: "quoted create then unquoted drop",
			ddl: `CREATE INDEX "transient_state" ON "public"."identity_operation_receipts" ("state");
DROP INDEX "transient_state";`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := CheckMigration(fixture(test.ddl)); err != nil {
				t.Fatalf("CheckMigration() error = %v", err)
			}
		})
	}
}

func TestReceiptIndexPolicyIgnoresOtherTables(t *testing.T) {
	if err := CheckMigration(fixture(`CREATE INDEX other_state ON other_receipts USING gin (state);`)); err != nil {
		t.Fatalf("CheckMigration() error = %v", err)
	}
}

func fixture(indexDDL string) string {
	return `-- +goose Up
CREATE TABLE identity_operation_receipts (state text, payload jsonb);
CREATE TABLE other_receipts (state text);
` + indexDDL + `
-- +goose Down
DROP TABLE other_receipts;
DROP TABLE identity_operation_receipts;
`
}

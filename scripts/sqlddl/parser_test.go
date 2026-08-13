package sqlddl

import (
	"reflect"
	"testing"
)

func TestParseCreateTableMultilineConstraints(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE workflow.jobs (
			id uuid NOT NULL,
			state text NOT NULL,
			result jsonb,
			CONSTRAINT jobs_state_allowed
				CHECK (
					state IN ('queued', 'running', 'done')
				),
			CONSTRAINT jobs_done_result
				CHECK (state <> 'done' OR result IS NOT NULL)
		);
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	table, ok := catalog.Table("workflow.jobs")
	if !ok {
		t.Fatal("workflow.jobs table not found")
	}
	if got, want := table.ColumnNames(), []string{"id", "result", "state"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ColumnNames() = %#v, want %#v", got, want)
	}
	if got, want := table.ConstraintNames(), []string{"jobs_done_result", "jobs_state_allowed"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ConstraintNames() = %#v, want %#v", got, want)
	}
	constraint := table.Constraints["jobs_state_allowed"]
	if constraint.Kind != ConstraintCheck {
		t.Fatalf("constraint kind = %q, want %q", constraint.Kind, ConstraintCheck)
	}
	if constraint.Canonical == "" {
		t.Fatal("constraint canonical form is empty")
	}
	if _, exists := table.Columns["check"]; exists {
		t.Fatal("multiline CHECK continuation was misclassified as a column")
	}
}

func TestParseAppliesAddAndLaterDropConstraint(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE jobs (id bigint, state text);
		ALTER TABLE jobs ADD CONSTRAINT jobs_state_nonempty CHECK (state <> '');
		ALTER TABLE jobs DROP CONSTRAINT jobs_state_nonempty;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	table, ok := catalog.Table("jobs")
	if !ok {
		t.Fatal("jobs table not found")
	}
	if got := table.ConstraintNames(); len(got) != 0 {
		t.Fatalf("final constraints = %#v, want none", got)
	}
}

func TestParseAlterTableMultipleConstraintActions(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE jobs (id bigint, state text);
		ALTER TABLE jobs
			ADD CONSTRAINT jobs_id_positive CHECK (id > 0),
			ADD CONSTRAINT jobs_state_nonempty CHECK (state <> ''),
			DROP CONSTRAINT jobs_id_positive;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	table, _ := catalog.Table("jobs")
	if got, want := table.ConstraintNames(), []string{"jobs_state_nonempty"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ConstraintNames() = %#v, want %#v", got, want)
	}
}

func TestParseKeepsTopLevelStatementBoundariesPortable(t *testing.T) {
	catalog, err := Parse(`
		-- a semicolon in a comment ; is not a statement boundary
		CREATE TABLE "CaseSchema"."Jobs" (
			id bigint,
			note text DEFAULT 'a;b',
			escaped_note text DEFAULT E'a\';b',
			CONSTRAINT "Note;Check" CHECK (note <> $$x;y$$)
		);
		/* another ; comment */
		ALTER TABLE "CaseSchema"."Jobs" DROP CONSTRAINT "Note;Check";
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	table, ok := catalog.Table(`"CaseSchema"."Jobs"`)
	if !ok {
		t.Fatal("quoted table not found")
	}
	if got := table.ConstraintNames(); len(got) != 0 {
		t.Fatalf("final constraints = %#v, want none", got)
	}
}

func TestParseRejectsUnknownDropWithoutIfExists(t *testing.T) {
	_, err := Parse(`
		CREATE TABLE jobs (id bigint);
		ALTER TABLE jobs DROP CONSTRAINT missing_constraint;
	`)
	if err == nil {
		t.Fatal("Parse() error = nil, want unknown constraint error")
	}
}

func TestParseAllowsUnknownDropWithIfExists(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE jobs (id bigint);
		ALTER TABLE jobs DROP CONSTRAINT IF EXISTS missing_constraint;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, ok := catalog.Table("jobs"); !ok {
		t.Fatal("jobs table not found")
	}
}

func TestParseRejectsUnterminatedDDL(t *testing.T) {
	if _, err := Parse(`CREATE TABLE jobs (id bigint, CONSTRAINT jobs_id CHECK (id > 0);`); err == nil {
		t.Fatal("Parse() error = nil, want unterminated DDL error")
	}
}

func TestParseRejectsUnbalancedRecognizedAlterTable(t *testing.T) {
	_, err := Parse(`
		CREATE TABLE jobs (id bigint);
		ALTER TABLE jobs ADD CONSTRAINT jobs_id CHECK (id > 0));
	`)
	if err == nil {
		t.Fatal("Parse() error = nil, want unbalanced ALTER TABLE error")
	}
}

func TestParseModelsQualifiedQuotedIndex(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE "CaseSchema"."Jobs" ("state" text, payload jsonb);
		CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS "CaseSchema"."Jobs State"
			ON ONLY "CaseSchema"."Jobs" USING "gin" ("state")
			WHERE payload IS NOT NULL;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	index, ok := catalog.Index(`"CaseSchema"."Jobs State"`)
	if !ok {
		t.Fatal("quoted qualified index not found")
	}
	if !index.Unique || index.Table != `"CaseSchema"."Jobs"` || index.Method != "gin" {
		t.Fatalf("index = %#v", index)
	}
	if got, want := index.Keys, []IndexKey{{Column: "state", Canonical: `"state"`}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("index keys = %#v, want %#v", got, want)
	}
	if got, want := index.Predicate, "payload is not null"; got != want {
		t.Fatalf("index predicate = %q, want %q", got, want)
	}
}

func TestParseAppliesIndexDDLInStatementOrder(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE audit.jobs (state text, created_at timestamptz);
		CREATE INDEX jobs_state ON audit.jobs (state);
		DROP INDEX audit.jobs_state;
		DROP INDEX IF EXISTS audit.jobs_created;
		CREATE INDEX audit.jobs_created ON audit.jobs (created_at DESC);
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, want := catalog.IndexNames(), []string{"audit.jobs_created"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexNames() = %#v, want %#v", got, want)
	}
	indexes := catalog.IndexesForTable("audit.jobs")
	if len(indexes) != 1 || indexes[0].Keys[0].Canonical != "created_at desc" {
		t.Fatalf("IndexesForTable() = %#v", indexes)
	}
}

func TestParseUnqualifiedDropResolvesUniqueSchemaIndex(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE audit.jobs (state text);
		CREATE INDEX "jobs state" ON audit.jobs (state);
		DROP INDEX "jobs state";
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := catalog.IndexNames(); len(got) != 0 {
		t.Fatalf("IndexNames() = %#v, want none", got)
	}
}

func TestParseQuotedDotsRemainInsideIdentifierParts(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE "audit.schema"."jobs.table" (state text);
		CREATE INDEX "jobs.state" ON "audit.schema"."jobs.table" (state);
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got, want := catalog.IndexNames(), []string{`"audit.schema"."jobs.state"`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexNames() = %#v, want %#v", got, want)
	}
}

func TestParseRejectsUnknownDropIndexWithoutIfExists(t *testing.T) {
	if _, err := Parse(`DROP INDEX missing_index;`); err == nil {
		t.Fatal("Parse() error = nil, want unknown index error")
	}
}

func TestParseDropTableRemovesOwnedIndexes(t *testing.T) {
	catalog, err := Parse(`
		CREATE TABLE jobs (state text);
		CREATE INDEX jobs_state ON jobs (state);
		DROP TABLE jobs;
	`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := catalog.IndexNames(); len(got) != 0 {
		t.Fatalf("IndexNames() = %#v, want none", got)
	}
}

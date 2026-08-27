package manifest

import "testing"

func TestNormalizeDigestIgnoresCollectionOrder(t *testing.T) {
	first := fixtureManifest([]Table{fixtureTable("public", "orders", 2), fixtureTable("public", "contacts", 3)})
	second := fixtureManifest([]Table{fixtureTable("public", "contacts", 3), fixtureTable("public", "orders", 2)})
	if err := first.Normalize(); err != nil {
		t.Fatal(err)
	}
	if err := second.Normalize(); err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("digest differs: %s != %s", first.Digest, second.Digest)
	}
	if first.Tables[0].Table != "contacts" {
		t.Fatalf("tables not sorted: %#v", first.Tables)
	}
}

func TestCompareReportsMetadataAndAggregateChanges(t *testing.T) {
	left := fixtureManifest([]Table{fixtureTable("public", "contacts", 3), fixtureTable("public", "legacy_only", 1)})
	rightChanged := fixtureTable("public", "contacts", 4)
	rightChanged.Columns[1].Type = "integer"
	right := fixtureManifest([]Table{rightChanged, fixtureTable("public", "v2_only", 2)})
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if result.Equal || len(result.Added) != 1 || len(result.Removed) != 1 || len(result.Changed) != 1 {
		t.Fatalf("unexpected comparison: %#v", result)
	}
	if result.Changed[0].Table.String() != "public.contacts" || result.Changed[0].LeftRowCount != 3 || result.Changed[0].RightRowCount != 4 {
		t.Fatalf("unexpected delta: %#v", result.Changed[0])
	}
}

func TestReconcileRequiresAllSourceTablesToHaveTerminalDisposition(t *testing.T) {
	source := fixtureManifest([]Table{fixtureTable("public", "contacts", 3), fixtureTable("public", "event_log", 7)})
	target := fixtureManifest([]Table{fixtureTable("public", "customers", 3)})
	document := DispositionDocument{Tables: []TableDisposition{{Source: TableKey{Schema: "public", Table: "contacts"}, Target: &TableKey{Schema: "public", Table: "customers"}, Disposition: "imported"}, {Source: TableKey{Schema: "public", Table: "event_log"}, Disposition: "reset_runtime"}}}
	report, err := Reconcile(source, &target, document)
	if err != nil {
		t.Fatal(err)
	}
	if !report.SourceCountEqualsTerminalDisposition || report.TerminalDispositionSourceRowCount != 10 || len(report.Tables) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if !report.Tables[0].TargetPresent || report.Tables[0].TargetRowCount == nil || *report.Tables[0].TargetRowCount != 3 {
		t.Fatalf("target count missing: %#v", report.Tables[0])
	}

	document.Tables = document.Tables[:1]
	report, err = Reconcile(source, nil, document)
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceCountEqualsTerminalDisposition || len(report.Unclassified) != 1 {
		t.Fatalf("unclassified source must fail completion: %#v", report)
	}
}

func TestReconcileRejectsUnknownAndNonterminalDisposition(t *testing.T) {
	source := fixtureManifest([]Table{fixtureTable("public", "contacts", 3)})
	_, err := Reconcile(source, nil, DispositionDocument{Tables: []TableDisposition{{Source: TableKey{Schema: "public", Table: "contacts"}, Disposition: "in_progress"}}})
	if err == nil {
		t.Fatal("expected non-terminal disposition error")
	}
	report, err := Reconcile(source, nil, DispositionDocument{Tables: []TableDisposition{{Source: TableKey{Schema: "public", Table: "contacts"}, Disposition: "archived"}, {Source: TableKey{Schema: "public", Table: "unknown"}, Disposition: "archived"}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceCountEqualsTerminalDisposition || len(report.UnknownSources) != 1 {
		t.Fatalf("unknown source must fail completion: %#v", report)
	}
}

func fixtureManifest(tables []Table) Manifest {
	return Manifest{Version: Version, Schemas: []string{"public"}, Tables: tables}
}

func fixtureTable(schema, name string, rows int64) Table {
	return Table{TableKey: TableKey{Schema: schema, Table: name}, RowCount: rows, Columns: []Column{{Name: "id", Type: "bigint", Ordinal: 1}, {Name: "created_at", Type: "timestamp with time zone", Nullable: false, Ordinal: 2}}, PrimaryKey: []string{"id"}, Watermark: Watermark{Column: "created_at", Value: "2026-08-27 00:00:00+00"}}
}

package v1archive

import (
	"errors"
	"strings"
	"testing"
)

func TestTableRowsSQLQuotesCatalogIdentifiers(t *testing.T) {
	table := Table{Name: `odd";drop table x;--`, Columns: []Column{{Ordinal: 1, Name: `id";--`, Type: "bigint", NotNull: true}}, PrimaryKey: []string{`id";--`}}
	query := tableRowsSQL(table)
	if !strings.Contains(query, `"odd"";drop table x;--"`) || !strings.Contains(query, `"id"";--"`) {
		t.Fatalf("catalog identifiers not safely quoted: %s", query)
	}
}

func TestTableRowsSQLUsesStablePayloadAndDuplicateOrdinalForKeylessTable(t *testing.T) {
	table := Table{Name: "legacy_backup", Columns: []Column{{Ordinal: 1, Name: "value", Type: "text"}}}
	if err := table.Validate(); err != nil {
		t.Fatalf("keyless frozen table rejected: %v", err)
	}
	query := tableRowsSQL(table)
	for _, fragment := range []string{"__aicrm_keyless__", "payload, duplicate_ordinal", "PARTITION BY to_jsonb(source_row)::text", "source_row.ctid", `"public"."legacy_backup"`} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("keyless query missing %q: %s", fragment, query)
		}
	}
	if strings.Contains(query, "jsonb_build_array('__aicrm_keyless__', row_number") {
		t.Fatalf("keyless identity still depends on global physical row order: %s", query)
	}
}

func TestSourceSafetyRejectsWritableConfigurations(t *testing.T) {
	for name, safety := range map[string][]bool{
		"transaction":      {false, true, true, true},
		"table_privilege":  {true, false, true, true},
		"schema_privilege": {true, true, false, true},
		"super_role":       {true, true, true, false},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateSourceSafety(safety[0], safety[1], safety[2], safety[3]); !errors.Is(err, ErrSourceNotReadOnly) {
				t.Fatalf("safety error = %v", err)
			}
		})
	}
	if err := validateSourceSafety(true, true, true, true); err != nil {
		t.Fatalf("read-only source rejected: %v", err)
	}
}

package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
)

var reference = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

func TestCompileLeafFamiliesAndStableProgram(t *testing.T) {
	cases := []struct {
		name       string
		definition string
		wantLeaf   string
	}{
		{"stage equal", `{"field":"stage_id","op":"eq","value":1}`, `"stage.equal"`},
		{"stage any", `{"field":"stage_id","op":"in","value":[1,2]}`, `"stage.any"`},
		{"owner equal", `{"field":"owner_staff_id","op":"eq","value":1}`, `"owner.equal"`},
		{"owner any", `{"field":"owner_staff_id","op":"in","value":[1,2]}`, `"owner.any"`},
		{"channel equal", `{"field":"channel_id","op":"eq","value":1}`, `"channel.equal"`},
		{"channel any", `{"field":"channel_id","op":"in","value":[1,2]}`, `"channel.any"`},
		{"tag any", `{"field":"tag_id","op":"has_any","value":[1,2]}`, `"tag.has_any"`},
		{"added before", `{"field":"added_at","op":"before","value":"2026-08-12T00:00:00Z"}`, `"added.before"`},
		{"added after", `{"field":"added_at","op":"after","value":"2026-08-12T00:00:00Z"}`, `"added.after"`},
		{"last interact before", `{"field":"last_interact_at","op":"before","value":"2026-08-12T00:00:00Z"}`, `"last_interact.before"`},
		{"last interact after", `{"field":"last_interact_at","op":"after","value":"2026-08-12T00:00:00Z"}`, `"last_interact.after"`},
		{"deleted true", `{"field":"is_deleted","op":"eq","value":true}`, `"deleted.equal"`},
		{"deleted false", `{"field":"is_deleted","op":"eq","value":false}`, `"deleted.equal"`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ast := mustParse(t, test.definition)
			first := mustCompileJSON(t, ast, reference)
			second := mustCompileJSON(t, ast, reference)
			if !bytes.Equal(first, second) {
				t.Fatalf("Compile output is not stable: %s != %s", first, second)
			}
			if !bytes.Contains(first, []byte(test.wantLeaf)) || !bytes.Contains(first, []byte(`"universe":"all"`)) {
				t.Fatalf("program = %s, missing %s or universe", first, test.wantLeaf)
			}
			assertNoSQLText(t, first)
		})
	}
}

func TestCompileCanonicalASTsHaveSameProgram(t *testing.T) {
	first := mustParse(t, `{"and":[{"field":"stage_id","op":"in","value":[2,1]},{"field":"tag_id","op":"has_any","value":[4,3]}]}`)
	second := mustParse(t, `{"and":[{"field":"stage_id","op":"in","value":[1,2]},{"field":"tag_id","op":"has_any","value":[3,4]}]}`)
	if got, want := mustCompileJSON(t, first, reference), mustCompileJSON(t, second, reference); !bytes.Equal(got, want) {
		t.Fatalf("canonical AST programs differ:\n%s\n%s", got, want)
	}
}

func TestCompilePreservesGroupTraversalOrder(t *testing.T) {
	ast := mustParse(t, `{"or":[{"field":"channel_id","op":"eq","value":3},{"and":[{"field":"is_deleted","op":"eq","value":true},{"field":"stage_id","op":"eq","value":2}]}]}`)
	program := mustCompileJSON(t, ast, reference)
	channel := bytes.Index(program, []byte(`"channel.equal"`))
	deleted := bytes.Index(program, []byte(`"deleted.equal"`))
	stage := bytes.Index(program, []byte(`"stage.equal"`))
	if channel < 0 || deleted < channel || stage < deleted {
		t.Fatalf("program traversal order changed: %s", program)
	}
}

func TestCompileRelativeTimeUsesOneReferenceInstant(t *testing.T) {
	ast := mustParse(t, `{"field":"added_at","op":"after","value":"-7d"}`)
	program := mustCompileJSON(t, ast, reference)
	if !bytes.Contains(program, []byte(`"timestamp":"2026-08-06T12:00:00Z"`)) || bytes.Contains(program, []byte(`-7d`)) {
		t.Fatalf("relative program = %s", program)
	}
}

func TestCompileRejectsUnsafeReferenceAndUnrepresentableAST(t *testing.T) {
	valid := mustParse(t, `{"field":"stage_id","op":"eq","value":1}`)
	for _, badReference := range []time.Time{time.Time{}, reference.In(time.FixedZone("zero", 0))} {
		if _, err := Compile(valid, badReference); reasonOf(err) != dsl.ReasonCompileUnsafe {
			t.Fatalf("bad reference error = %v", err)
		}
	}
	cases := []dsl.AST{
		{},
		{Root: dsl.Predicate{Field: dsl.Field("unexpected"), Operator: dsl.OperatorEqual, Value: dsl.IntValue{Value: 1}}},
		{Root: dsl.Predicate{Field: dsl.FieldStageID, Operator: dsl.OperatorEqual, Value: dsl.IntValue{Value: 0}}},
		{Root: dsl.Predicate{Field: dsl.FieldStageID, Operator: dsl.OperatorIn, Value: dsl.IntListValue{Values: []int64{2, 1}}}},
		{Root: dsl.Predicate{Field: dsl.FieldAddedAt, Operator: dsl.OperatorBefore, Value: dsl.BoolValue{Value: true}}},
		{Root: dsl.And{}},
	}
	for _, ast := range cases {
		if _, err := Compile(ast, reference); reasonOf(err) != dsl.ReasonCompileUnrepresentable {
			t.Fatalf("Compile(%#v) error = %v", ast, err)
		}
	}
}

func TestCompileTableDrivenSafetyMatrix(t *testing.T) {
	// 50 independent legal definitions cover all opcodes, both set operators,
	// boolean values, absolute and relative time, and varied nesting shapes.
	cases := make([]string, 0, 50)
	for value := 1; value <= 10; value++ {
		cases = append(cases,
			fmt.Sprintf(`{"field":"stage_id","op":"eq","value":%d}`, value),
			fmt.Sprintf(`{"field":"owner_staff_id","op":"in","value":[%d,%d]}`, value, value+10),
			fmt.Sprintf(`{"field":"channel_id","op":"eq","value":%d}`, value),
			fmt.Sprintf(`{"and":[{"field":"tag_id","op":"has_any","value":[%d,%d]},{"field":"is_deleted","op":"eq","value":false}]}`, value, value+20),
			fmt.Sprintf(`{"or":[{"field":"added_at","op":"after","value":"-%dd"},{"field":"last_interact_at","op":"before","value":"2026-08-%02dT00:00:00Z"}]}`, value, value),
		)
	}
	if len(cases) != 50 {
		t.Fatal("test matrix must stay at 50 cases")
	}
	for index, definition := range cases {
		t.Run(fmt.Sprintf("case-%02d", index+1), func(t *testing.T) {
			program := mustCompileJSON(t, mustParse(t, definition), reference)
			assertNoSQLText(t, program)
			var decoded map[string]any
			if err := json.Unmarshal(program, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded["universe"] != "all" {
				t.Fatalf("universe = %#v", decoded["universe"])
			}
		})
	}
}

func FuzzCompileCanonicalASTIsDeterministicAndSQLFree(f *testing.F) {
	for _, seed := range []string{
		`{"field":"stage_id","op":"eq","value":1}`,
		`{"and":[{"field":"tag_id","op":"has_any","value":[1,2]},{"field":"is_deleted","op":"eq","value":false}]}`,
		`{"field":"last_interact_at","op":"before","value":"-7d"}`,
		`{"field":"stage_id; DROP TABLE customers","op":"eq","value":1}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		ast, err := dsl.Parse(input)
		if err != nil {
			return
		}
		first, err := Compile(ast, reference)
		if err != nil {
			t.Fatalf("validated AST rejected: %v", err)
		}
		firstJSON, err := first.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		secondJSON := mustCompileJSON(t, ast, reference)
		if !bytes.Equal(firstJSON, secondJSON) {
			t.Fatalf("program not deterministic")
		}
		assertNoSQLText(t, firstJSON)
	})
}

func mustParse(t *testing.T, definition string) dsl.AST {
	t.Helper()
	ast, err := dsl.Parse([]byte(definition))
	if err != nil {
		t.Fatalf("Parse(%s): %v", definition, err)
	}
	return ast
}
func mustCompileJSON(t *testing.T, ast dsl.AST, at time.Time) []byte {
	t.Helper()
	program, err := Compile(ast, at)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	result, err := program.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	return result
}
func reasonOf(err error) dsl.Reason {
	detail, ok := dsl.ErrorDetail(err)
	if !ok {
		return ""
	}
	return detail.Reason
}
func assertNoSQLText(t *testing.T, program []byte) {
	t.Helper()
	for _, forbidden := range []string{"select", "from", "where", "join", ";", "customers", "--", "/*", "$1"} {
		if strings.Contains(strings.ToLower(string(program)), forbidden) {
			t.Fatalf("program leaked SQL-shaped text %q: %s", forbidden, program)
		}
	}
}

package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestParseCanonicalAST(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "integer list is sorted while group order remains stable",
			input: `{"and":[{"field":"tag_id","op":"has_any","value":[22,15]},{"field":"stage_id","op":"in","value":[3,2]}]}`,
			want:  `{"and":[{"field":"tag_id","op":"has_any","value":[15,22]},{"field":"stage_id","op":"in","value":[2,3]}]}`,
		},
		{
			name:  "all scalar families",
			input: `{"or":[{"field":"owner_staff_id","op":"eq","value":9},{"field":"channel_id","op":"in","value":[4,2]},{"field":"added_at","op":"after","value":"2026-08-12T10:00:00+00:00"},{"field":"last_interact_at","op":"before","value":"-7d"},{"field":"is_deleted","op":"eq","value":false}]}`,
			want:  `{"or":[{"field":"owner_staff_id","op":"eq","value":9},{"field":"channel_id","op":"in","value":[2,4]},{"field":"added_at","op":"after","value":"2026-08-12T10:00:00Z"},{"field":"last_interact_at","op":"before","value":"-7d"},{"field":"is_deleted","op":"eq","value":false}]}`,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ast, err := Parse([]byte(testCase.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			actual, err := ast.CanonicalJSON()
			if err != nil {
				t.Fatalf("CanonicalJSON() error = %v", err)
			}
			if string(actual) != testCase.want {
				t.Fatalf("CanonicalJSON() = %s, want %s", actual, testCase.want)
			}
			if _, ok := ast.Root.(And); testCase.name == "integer list is sorted while group order remains stable" && !ok {
				t.Fatalf("root type = %T, want And", ast.Root)
			}
		})
	}
}

func TestParseRejectsClosedDSLViolations(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		reason Reason
		field  string
	}{
		{"malformed JSON", `{"field":`, ReasonInvalidJSON, ""},
		{"top-level array", `[]`, ReasonInvalidShape, ""},
		{"duplicate key", `{"field":"stage_id","field":"stage_id","op":"eq","value":1}`, ReasonDuplicateKey, "/field"},
		{"unknown group sibling", `{"and":[],"extra":true}`, ReasonInvalidShape, ""},
		{"empty group", `{"or":[]}`, ReasonInvalidShape, "/or"},
		{"unknown field", `{"field":"extra","op":"eq","value":1}`, ReasonUnknownField, "/field"},
		{"unsupported operator", `{"field":"tag_id","op":"in","value":[1]}`, ReasonUnsupportedOperator, "/op"},
		{"integer float", `{"field":"stage_id","op":"eq","value":1.0}`, ReasonInvalidValue, "/value"},
		{"integer zero", `{"field":"stage_id","op":"eq","value":0}`, ReasonInvalidValue, "/value"},
		{"integer exponent", `{"field":"stage_id","op":"eq","value":1e1}`, ReasonInvalidValue, "/value"},
		{"duplicate integer", `{"field":"tag_id","op":"has_any","value":[1,1]}`, ReasonNoncanonicalValue, "/value"},
		{"time offset is not UTC Z", `{"field":"added_at","op":"before","value":"2026-08-12T10:00:00+08:00"}`, ReasonInvalidValue, "/value"},
		{"relative zero", `{"field":"last_interact_at","op":"before","value":"-0d"}`, ReasonInvalidValue, "/value"},
		{"boolean has wrong type", `{"field":"is_deleted","op":"eq","value":1}`, ReasonInvalidValue, "/value"},
		{"SQL shaped input", `{"field":"stage_id; DROP TABLE customers","op":"eq","value":1}`, ReasonUnknownField, "/field"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(testCase.input))
			if err == nil {
				t.Fatal("Parse() error = nil")
			}
			detail, ok := ErrorDetail(err)
			if !ok {
				t.Fatalf("ErrorDetail(%v) = false", err)
			}
			if detail.Reason != testCase.reason || detail.Field != testCase.field {
				t.Fatalf("detail = %#v, want reason=%q field=%q", detail, testCase.reason, testCase.field)
			}
		})
	}
}

func TestParseLimits(t *testing.T) {
	t.Parallel()
	validDepth := predicateJSON()
	for range MaxDepth - 1 {
		validDepth = `{"and":[` + validDepth + `]}`
	}
	if _, err := Parse([]byte(validDepth)); err != nil {
		t.Fatalf("max depth Parse() error = %v", err)
	}
	overDepth := `{"and":[` + validDepth + `]}`
	assertReason(t, overDepth, ReasonLimitExceeded, "/and/0/and/0/and/0/and/0/and/0/and/0/and/0/and/0")

	values := make([]int, MaxListValues)
	for index := range values {
		values[index] = index + 1
	}
	validList, err := json.Marshal(map[string]any{"field": "tag_id", "op": "has_any", "value": values})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(validList); err != nil {
		t.Fatalf("max list Parse() error = %v", err)
	}
	values = append(values, MaxListValues+1)
	overList, err := json.Marshal(map[string]any{"field": "tag_id", "op": "has_any", "value": values})
	if err != nil {
		t.Fatal(err)
	}
	assertReason(t, string(overList), ReasonLimitExceeded, "/value")

	tooManyNodes := `{"and":[{"and":[` + repeatedPredicates(63) + `]},{"or":[` + repeatedPredicates(63) + `]}]}`
	assertReason(t, tooManyNodes, ReasonLimitExceeded, "/and/1/or/62")

	tooLarge := make([]byte, MaxDefinitionBytes+1)
	for index := range tooLarge {
		tooLarge[index] = ' '
	}
	if _, err := Parse(tooLarge); err == nil {
		t.Fatal("oversized Parse() error = nil")
	} else if detail, ok := ErrorDetail(err); !ok || detail.Reason != ReasonLimitExceeded {
		t.Fatalf("oversized detail = %#v, ok=%t", detail, ok)
	}
}

func TestCanonicalJSONRoundTrips(t *testing.T) {
	t.Parallel()
	input := []byte(`{"or":[{"field":"tag_id","op":"has_any","value":[3,1,2]},{"field":"is_deleted","op":"eq","value":true}]}`)
	ast, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := ast.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(canonical)
	if err != nil {
		t.Fatalf("Parse(canonical) error = %v", err)
	}
	recanonical, err := reparsed.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != string(recanonical) {
		t.Fatalf("canonical instability: %s != %s", canonical, recanonical)
	}
}

func FuzzParseNeverPanicsAndCanonicalRoundTrips(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"field":"stage_id","op":"eq","value":1}`),
		[]byte(`{"and":[{"field":"tag_id","op":"has_any","value":[2,1]}]}`),
		[]byte(`{"field":"field","field":"field"}`),
		[]byte{0xff, 0xfe},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		ast, err := Parse(input)
		if err != nil {
			if _, ok := ErrorDetail(err); !ok {
				t.Fatalf("unstructured error: %T %v", err, err)
			}
			return
		}
		canonical, err := ast.CanonicalJSON()
		if err != nil {
			t.Fatalf("CanonicalJSON() error = %v", err)
		}
		reparsed, err := Parse(canonical)
		if err != nil {
			t.Fatalf("Parse(canonical) error = %v", err)
		}
		recanonical, err := reparsed.CanonicalJSON()
		if err != nil || string(canonical) != string(recanonical) {
			t.Fatalf("canonical round-trip failed: %s, %v", canonical, err)
		}
	})
}

func assertReason(t *testing.T, input string, reason Reason, field string) {
	t.Helper()
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("Parse() error = nil")
	}
	detail, ok := ErrorDetail(err)
	if !ok || detail.Reason != reason || detail.Field != field {
		t.Fatalf("detail = %#v, ok=%t, want reason=%q field=%q", detail, ok, reason, field)
	}
}

func predicateJSON() string {
	return `{"field":"stage_id","op":"eq","value":1}`
}

func repeatedPredicates(count int) string {
	items := make([]string, count)
	for index := range items {
		items[index] = fmt.Sprintf(`{"field":"stage_id","op":"eq","value":%d}`, index+1)
	}
	return strings.Join(items, ",")
}

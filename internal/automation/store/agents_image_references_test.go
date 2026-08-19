package store

import (
	"reflect"
	"testing"
)

func TestAutomationImageReferenceIDsRejectsNonCanonicalArrayElements(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want []int64
		err  bool
	}{
		{name: "empty array", raw: `[]`, want: []int64{}},
		{name: "canonical numbers", raw: `[7,42]`, want: []int64{7, 42}},
		{name: "leading zero string", raw: `["042"]`, err: true},
		{name: "plus string", raw: `["+42"]`, err: true},
		{name: "whitespace string", raw: `[" 42"]`, err: true},
		{name: "object", raw: `[{}]`, err: true},
		{name: "illegal string", raw: `["image-42"]`, err: true},
		{name: "mixed valid and invalid", raw: `[42,"042"]`, err: true},
		{name: "duplicate", raw: `[42,42]`, err: true},
		{name: "json null", raw: `null`, err: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := automationImageReferenceIDs([]byte(test.raw))
			if test.err {
				if err == nil {
					t.Fatalf("automationImageReferenceIDs(%s) succeeded", test.raw)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("automationImageReferenceIDs(%s) = %#v, %v; want %#v, nil", test.raw, got, err, test.want)
			}
		})
	}
}

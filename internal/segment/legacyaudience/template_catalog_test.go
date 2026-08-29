package legacyaudience

import (
	"errors"
	"reflect"
	"testing"
)

func TestAudienceTemplateCatalogIsFixedAndDefensive(t *testing.T) {
	first := ListAudienceTemplates()
	if got, want := first, []AudienceTemplate{
		{Key: TemplateActiveContacts, Version: 1},
		{Key: TemplateStageAny, Version: 1, Parameters: []TemplateParameter{{Key: "stage_ids", Required: true}}},
		{Key: TemplateTagAny, Version: 1, Parameters: []TemplateParameter{{Key: "tag_ids", Required: true}}},
		{Key: TemplateOwnerAny, Version: 1, Parameters: []TemplateParameter{{Key: "owner_staff_ids", Required: true}}},
		{Key: TemplateChannelAny, Version: 1, Parameters: []TemplateParameter{{Key: "channel_ids", Required: true}}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog=%+v want=%+v", got, want)
	}
	first[1].Parameters[0].Key = "mutated"
	if got := ListAudienceTemplates()[1].Parameters[0].Key; got != "stage_ids" {
		t.Fatalf("catalog leaked mutable parameter=%q", got)
	}
}

func TestBuildAudienceTemplateDefinitionUsesClosedLocalFilters(t *testing.T) {
	cases := []struct {
		name      string
		selection TemplateSelection
		want      string
	}{
		{"active contacts", TemplateSelection{Key: TemplateActiveContacts, Version: 1}, `{"field":"is_deleted","op":"eq","value":false}`},
		{"stage IDs are sorted", TemplateSelection{Key: TemplateStageAny, Version: 1, Parameters: map[string][]int64{"stage_ids": {9, 2}}}, `{"and":[{"field":"is_deleted","op":"eq","value":false},{"field":"stage_id","op":"in","value":[2,9]}]}`},
		{"tag IDs", TemplateSelection{Key: TemplateTagAny, Version: 1, Parameters: map[string][]int64{"tag_ids": {4, 2}}}, `{"and":[{"field":"is_deleted","op":"eq","value":false},{"field":"tag_id","op":"has_any","value":[2,4]}]}`},
		{"owner IDs", TemplateSelection{Key: TemplateOwnerAny, Version: 1, Parameters: map[string][]int64{"owner_staff_ids": {3}}}, `{"and":[{"field":"is_deleted","op":"eq","value":false},{"field":"owner_staff_id","op":"in","value":[3]}]}`},
		{"channel IDs", TemplateSelection{Key: TemplateChannelAny, Version: 1, Parameters: map[string][]int64{"channel_ids": {6}}}, `{"and":[{"field":"is_deleted","op":"eq","value":false},{"field":"channel_id","op":"in","value":[6]}]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := BuildAudienceTemplateDefinition(testCase.selection)
			if err != nil || string(got) != testCase.want {
				t.Fatalf("definition=%s err=%v want=%s", got, err, testCase.want)
			}
		})
	}
}

func TestBuildAudienceTemplateDefinitionRejectsUnknownOrUnsafeParameters(t *testing.T) {
	cases := []TemplateSelection{
		{Key: "unknown", Version: 1},
		{Key: TemplateStageAny, Version: 2, Parameters: map[string][]int64{"stage_ids": {1}}},
		{Key: TemplateStageAny, Version: 1},
		{Key: TemplateStageAny, Version: 1, Parameters: map[string][]int64{"stage_ids": {}}},
		{Key: TemplateStageAny, Version: 1, Parameters: map[string][]int64{"stage_ids": {1}, "extra": {2}}},
		{Key: TemplateStageAny, Version: 1, Parameters: map[string][]int64{"stage_ids": {0}}},
		{Key: TemplateStageAny, Version: 1, Parameters: map[string][]int64{"stage_ids": {2, 2}}},
		{Key: TemplateActiveContacts, Version: 1, Parameters: map[string][]int64{"stage_ids": {1}}},
	}
	for _, selection := range cases {
		if _, err := BuildAudienceTemplateDefinition(selection); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("selection=%+v error=%v, want invalid input", selection, err)
		}
	}
}

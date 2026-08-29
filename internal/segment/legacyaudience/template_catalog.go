package legacyaudience

import (
	"encoding/json"
	"sort"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

// TemplateParameter describes one closed, local-only template input. It is
// deliberately not a SQL fragment, prompt, Provider payload, or executable
// expression.
type TemplateParameter struct {
	Key      string `json:"key"`
	Required bool   `json:"required"`
}

// AudienceTemplate is the small catalog exposed by the future HTTP adapter.
// The catalog compiles only into the existing Segment DSL.
type AudienceTemplate struct {
	Key        string              `json:"key"`
	Version    int64               `json:"version"`
	Parameters []TemplateParameter `json:"parameters"`
}

type AudienceTemplateCatalogResponse struct {
	Items []AudienceTemplate `json:"items"`
	Projection
}

// TemplateSelection contains one catalog key, its fixed version, and typed
// positive-ID-list parameters. JSON decoding and CSRF/idempotency belong to a
// later HTTP adapter; this leaf only validates local selection semantics.
type TemplateSelection struct {
	Key        string             `json:"key"`
	Version    int64              `json:"version"`
	Parameters map[string][]int64 `json:"parameters"`
}

const (
	TemplateActiveContacts = "active_contacts"
	TemplateStageAny       = "stage_any"
	TemplateTagAny         = "tag_any"
	TemplateOwnerAny       = "owner_any"
	TemplateChannelAny     = "channel_any"
)

var audienceTemplateCatalog = []AudienceTemplate{
	{Key: TemplateActiveContacts, Version: 1},
	{Key: TemplateStageAny, Version: 1, Parameters: []TemplateParameter{{Key: "stage_ids", Required: true}}},
	{Key: TemplateTagAny, Version: 1, Parameters: []TemplateParameter{{Key: "tag_ids", Required: true}}},
	{Key: TemplateOwnerAny, Version: 1, Parameters: []TemplateParameter{{Key: "owner_staff_ids", Required: true}}},
	{Key: TemplateChannelAny, Version: 1, Parameters: []TemplateParameter{{Key: "channel_ids", Required: true}}},
}

// ListAudienceTemplates returns a defensive copy of the fixed V2 catalog.
func ListAudienceTemplates() []AudienceTemplate {
	result := make([]AudienceTemplate, len(audienceTemplateCatalog))
	for index, template := range audienceTemplateCatalog {
		result[index] = AudienceTemplate{
			Key:        template.Key,
			Version:    template.Version,
			Parameters: append([]TemplateParameter(nil), template.Parameters...),
		}
	}
	return result
}

// BuildAudienceTemplateDefinition validates a selection and returns a
// canonical local Segment definition. Every template deliberately includes
// is_deleted=false, so it cannot select deleted contacts. It never emits SQL
// or performs a Provider call.
func BuildAudienceTemplateDefinition(selection TemplateSelection) (segmentport.Definition, error) {
	template, ok := audienceTemplateByKey(selection.Key)
	if !ok || selection.Version != template.Version {
		return nil, ErrInvalidInput
	}
	values, err := templateParameterValues(template, selection.Parameters)
	if err != nil {
		return nil, err
	}

	children := []any{map[string]any{"field": string(dsl.FieldIsDeleted), "op": string(dsl.OperatorEqual), "value": false}}
	switch template.Key {
	case TemplateActiveContacts:
	case TemplateStageAny:
		children = append(children, predicate(dsl.FieldStageID, dsl.OperatorIn, values["stage_ids"]))
	case TemplateTagAny:
		children = append(children, predicate(dsl.FieldTagID, dsl.OperatorHasAny, values["tag_ids"]))
	case TemplateOwnerAny:
		children = append(children, predicate(dsl.FieldOwnerStaffID, dsl.OperatorIn, values["owner_staff_ids"]))
	case TemplateChannelAny:
		children = append(children, predicate(dsl.FieldChannelID, dsl.OperatorIn, values["channel_ids"]))
	default:
		return nil, ErrInvalidInput
	}

	definition := any(children[0])
	if len(children) > 1 {
		definition = map[string]any{"and": children}
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		return nil, ErrUnavailable
	}
	return canonicalDefinition(segmentport.Definition(raw))
}

func audienceTemplateByKey(key string) (AudienceTemplate, bool) {
	for _, template := range audienceTemplateCatalog {
		if template.Key == key {
			return template, true
		}
	}
	return AudienceTemplate{}, false
}

func templateParameterValues(template AudienceTemplate, parameters map[string][]int64) (map[string][]int64, error) {
	if len(parameters) != len(template.Parameters) {
		return nil, ErrInvalidInput
	}
	values := make(map[string][]int64, len(parameters))
	for _, parameter := range template.Parameters {
		raw, ok := parameters[parameter.Key]
		if !ok || parameter.Required && len(raw) == 0 || len(raw) > dsl.MaxListValues {
			return nil, ErrInvalidInput
		}
		copy := append([]int64(nil), raw...)
		sort.Slice(copy, func(left, right int) bool { return copy[left] < copy[right] })
		for index, value := range copy {
			if value <= 0 || index > 0 && copy[index-1] == value {
				return nil, ErrInvalidInput
			}
		}
		values[parameter.Key] = copy
	}
	return values, nil
}

func predicate(field dsl.Field, operator dsl.Operator, values []int64) map[string]any {
	return map[string]any{"field": string(field), "op": string(operator), "value": values}
}

func cloneTemplateSelection(value TemplateSelection) TemplateSelection {
	result := TemplateSelection{Key: value.Key, Version: value.Version, Parameters: make(map[string][]int64, len(value.Parameters))}
	for key, items := range value.Parameters {
		result.Parameters[key] = append([]int64(nil), items...)
	}
	return result
}

// Package compiler turns the closed Segment AST into a safe query program.
// It deliberately contains no SQL, database, sqlc, or executor dependency.
package compiler

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
)

// Opcode is a closed, declarative selector. It is not a column name or SQL
// fragment; S03 alone maps these opcodes to fixed sqlc calls.
type Opcode string

const (
	StageEqual                    Opcode = "stage.equal"
	StageAny                      Opcode = "stage.any"
	OwnerEqual                    Opcode = "owner.equal"
	OwnerAny                      Opcode = "owner.any"
	ChannelEqual                  Opcode = "channel.equal"
	ChannelAny                    Opcode = "channel.any"
	TagHasAny                     Opcode = "tag.has_any"
	AddedBefore                   Opcode = "added.before"
	AddedAfter                    Opcode = "added.after"
	LastInteractBefore            Opcode = "last_interact.before"
	LastInteractAfter             Opcode = "last_interact.after"
	DeletedEqual                  Opcode = "deleted.equal"
	LegacyAudiencePackageSnapshot Opcode = "legacy_audience_package.snapshot"
	HXCSubscriptionTierEqual      Opcode = "hxc.subscription_tier.equal"
	HXCSubscriptionActiveEqual    Opcode = "hxc.subscription_active.equal"
	HXCDaysRemainingGTE           Opcode = "hxc.days_remaining.gte"
	HXCDaysRemainingLTE           Opcode = "hxc.days_remaining.lte"
	HXCUserMessages7DGTE          Opcode = "hxc.user_messages_7d.gte"
	HXCUserMessages7DLTE          Opcode = "hxc.user_messages_7d.lte"
	HXCUserMessages30DGTE         Opcode = "hxc.user_messages_30d.gte"
	HXCUserMessages30DLTE         Opcode = "hxc.user_messages_30d.lte"
	HXCLastCapabilityEqual        Opcode = "hxc.last_capability.equal"
	HXCBusinessStageEqual         Opcode = "hxc.business_stage.equal"
	HXCMainLineTypeEqual          Opcode = "hxc.main_line_type.equal"
	HXCUserSegmentEqual           Opcode = "hxc.user_segment.equal"
	HXCFocusTopicAny              Opcode = "hxc.focus_topic.has_any"
	HXCPainTagEqual               Opcode = "hxc.pain_tag.equal"
)

// Program is immutable after Compile returns. Its representation is private;
// CanonicalJSON is the transport-neutral, stable form for later S03 use.
type Program struct{ root node }

// CanonicalJSON serializes the closed query program without SQL text or raw
// identifiers. Every filter is evaluated against the explicit all-customers
// universe; S03 must preserve that universe before applying set operations.
// The result is deterministic for a canonical input AST.
func (program Program) CanonicalJSON() ([]byte, error) {
	if program.root == nil {
		return nil, dsl.FieldError{Reason: dsl.ReasonCompileUnrepresentable}
	}
	return json.Marshal(map[string]any{
		"filter":   canonicalNode(program.root),
		"universe": "all",
	})
}

type node interface{ queryProgramNode() }
type intersect struct{ children []node }
type union struct{ children []node }
type complement struct{ child node }
type leaf struct {
	opcode Opcode
	bind   bind
}

func (intersect) queryProgramNode()  {}
func (union) queryProgramNode()      {}
func (complement) queryProgramNode() {}
func (leaf) queryProgramNode()       {}

// bind is a tagged, strongly typed value slot. No string slot exists: parsed
// relative dates are resolved once to an absolute UTC timestamp here.
type bind struct {
	integer   *int64
	integers  []int64
	timestamp *time.Time
	boolean   *bool
	text      *string
	texts     []string
}

type budget struct{ nodes int }

// Compile maps a validated S01 AST to a closed set-algebra program. reference
// must be a non-zero UTC instant when relative dates are present. It never
// renders SQL or accesses a database.
func Compile(ast dsl.AST, reference time.Time) (Program, error) {
	if ast.Root == nil {
		return Program{}, unrepresentable()
	}
	if reference.IsZero() || reference.Location() != time.UTC {
		return Program{}, unsafe()
	}
	root, err := compileNode(ast.Root, reference, 1, &budget{})
	if err != nil {
		return Program{}, err
	}
	return Program{root: root}, nil
}

func compileNode(input dsl.Node, reference time.Time, depth int, used *budget) (node, error) {
	if depth > dsl.MaxDepth {
		return nil, unrepresentable()
	}
	used.nodes++
	if used.nodes > dsl.MaxNodes {
		return nil, unrepresentable()
	}
	switch typed := input.(type) {
	case dsl.And:
		children, err := compileChildren(typed.Children, reference, depth, used)
		return intersect{children: children}, err
	case dsl.Or:
		children, err := compileChildren(typed.Children, reference, depth, used)
		return union{children: children}, err
	case dsl.Predicate:
		return compilePredicate(typed, reference)
	default:
		return nil, unrepresentable()
	}
}

func compileChildren(children []dsl.Node, reference time.Time, depth int, used *budget) ([]node, error) {
	if len(children) == 0 || len(children) > dsl.MaxGroupChildren {
		return nil, unrepresentable()
	}
	result := make([]node, len(children))
	for index, child := range children {
		compiled, err := compileNode(child, reference, depth+1, used)
		if err != nil {
			return nil, err
		}
		result[index] = compiled
	}
	return result, nil
}

func compilePredicate(predicate dsl.Predicate, reference time.Time) (node, error) {
	switch value := predicate.Value.(type) {
	case dsl.IntValue:
		if value.Value < 0 || (value.Value == 0 && !hxcNumericField(predicate.Field)) {
			return nil, unrepresentable()
		}
		opcode, ok := integerOpcode(predicate.Field, predicate.Operator)
		if !ok {
			return nil, unrepresentable()
		}
		item := value.Value
		return leaf{opcode: opcode, bind: bind{integer: &item}}, nil
	case dsl.IntListValue:
		if !canonicalPositiveList(value.Values) {
			return nil, unrepresentable()
		}
		opcode, ok := listOpcode(predicate.Field, predicate.Operator)
		if !ok {
			return nil, unrepresentable()
		}
		return leaf{opcode: opcode, bind: bind{integers: append([]int64(nil), value.Values...)}}, nil
	case dsl.TimestampValue:
		if _, offset := value.Value.Zone(); offset != 0 {
			return nil, unrepresentable()
		}
		opcode, ok := timeOpcode(predicate.Field, predicate.Operator)
		if !ok {
			return nil, unrepresentable()
		}
		instant := value.Value.UTC()
		return leaf{opcode: opcode, bind: bind{timestamp: &instant}}, nil
	case dsl.RelativeDaysValue:
		if value.Days < 1 || value.Days > 9999 {
			return nil, unrepresentable()
		}
		opcode, ok := timeOpcode(predicate.Field, predicate.Operator)
		if !ok {
			return nil, unrepresentable()
		}
		instant := reference.AddDate(0, 0, -value.Days).UTC()
		return leaf{opcode: opcode, bind: bind{timestamp: &instant}}, nil
	case dsl.BoolValue:
		opcode := DeletedEqual
		if predicate.Field == dsl.FieldHXCSubscriptionActive && predicate.Operator == dsl.OperatorEqual {
			opcode = HXCSubscriptionActiveEqual
		} else if predicate.Field != dsl.FieldIsDeleted || predicate.Operator != dsl.OperatorEqual {
			return nil, unrepresentable()
		}
		boolean := value.Value
		return leaf{opcode: opcode, bind: bind{boolean: &boolean}}, nil
	case dsl.StringValue:
		opcode, ok := stringOpcode(predicate.Field, predicate.Operator)
		if !ok || value.Value == "" {
			return nil, unrepresentable()
		}
		text := value.Value
		return leaf{opcode: opcode, bind: bind{text: &text}}, nil
	case dsl.StringListValue:
		if predicate.Field != dsl.FieldHXCFocusTopic || predicate.Operator != dsl.OperatorHasAny || !canonicalStringList(value.Values) {
			return nil, unrepresentable()
		}
		return leaf{opcode: HXCFocusTopicAny, bind: bind{texts: append([]string(nil), value.Values...)}}, nil
	default:
		return nil, unrepresentable()
	}
}

func integerOpcode(field dsl.Field, operator dsl.Operator) (Opcode, bool) {
	if field == dsl.FieldHXCDaysRemaining {
		if operator == dsl.OperatorGTE {
			return HXCDaysRemainingGTE, true
		}
		if operator == dsl.OperatorLTE {
			return HXCDaysRemainingLTE, true
		}
	}
	if field == dsl.FieldHXCUserMessages7D {
		if operator == dsl.OperatorGTE {
			return HXCUserMessages7DGTE, true
		}
		if operator == dsl.OperatorLTE {
			return HXCUserMessages7DLTE, true
		}
	}
	if field == dsl.FieldHXCUserMessages30D {
		if operator == dsl.OperatorGTE {
			return HXCUserMessages30DGTE, true
		}
		if operator == dsl.OperatorLTE {
			return HXCUserMessages30DLTE, true
		}
	}
	if operator != dsl.OperatorEqual {
		return "", false
	}
	switch field {
	case dsl.FieldStageID:
		return StageEqual, true
	case dsl.FieldOwnerStaffID:
		return OwnerEqual, true
	case dsl.FieldChannelID:
		return ChannelEqual, true
	case dsl.FieldLegacyAudiencePackageSourceID:
		return LegacyAudiencePackageSnapshot, true
	default:
		return "", false
	}
}

func hxcNumericField(field dsl.Field) bool {
	return field == dsl.FieldHXCDaysRemaining || field == dsl.FieldHXCUserMessages7D || field == dsl.FieldHXCUserMessages30D
}

func stringOpcode(field dsl.Field, operator dsl.Operator) (Opcode, bool) {
	if operator != dsl.OperatorEqual {
		return "", false
	}
	switch field {
	case dsl.FieldHXCSubscriptionTier:
		return HXCSubscriptionTierEqual, true
	case dsl.FieldHXCLastCapability:
		return HXCLastCapabilityEqual, true
	case dsl.FieldHXCBusinessStage:
		return HXCBusinessStageEqual, true
	case dsl.FieldHXCMainLineType:
		return HXCMainLineTypeEqual, true
	case dsl.FieldHXCUserSegment:
		return HXCUserSegmentEqual, true
	case dsl.FieldHXCPainTag:
		return HXCPainTagEqual, true
	default:
		return "", false
	}
}

func canonicalStringList(values []string) bool {
	if len(values) == 0 || len(values) > dsl.MaxListValues || !sort.StringsAreSorted(values) {
		return false
	}
	for index, value := range values {
		if value == "" || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}
func listOpcode(field dsl.Field, operator dsl.Operator) (Opcode, bool) {
	switch field {
	case dsl.FieldStageID:
		if operator == dsl.OperatorIn {
			return StageAny, true
		}
	case dsl.FieldOwnerStaffID:
		if operator == dsl.OperatorIn {
			return OwnerAny, true
		}
	case dsl.FieldChannelID:
		if operator == dsl.OperatorIn {
			return ChannelAny, true
		}
	case dsl.FieldTagID:
		if operator == dsl.OperatorHasAny {
			return TagHasAny, true
		}
	}
	return "", false
}
func timeOpcode(field dsl.Field, operator dsl.Operator) (Opcode, bool) {
	switch field {
	case dsl.FieldAddedAt:
		if operator == dsl.OperatorBefore {
			return AddedBefore, true
		}
		if operator == dsl.OperatorAfter {
			return AddedAfter, true
		}
	case dsl.FieldLastInteractAt:
		if operator == dsl.OperatorBefore {
			return LastInteractBefore, true
		}
		if operator == dsl.OperatorAfter {
			return LastInteractAfter, true
		}
	}
	return "", false
}
func canonicalPositiveList(values []int64) bool {
	if len(values) == 0 || len(values) > dsl.MaxListValues {
		return false
	}
	return sort.SliceIsSorted(values, func(left, right int) bool { return values[left] < values[right] }) && allUniquePositive(values)
}
func allUniquePositive(values []int64) bool {
	for index, value := range values {
		if value <= 0 || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}
func unrepresentable() error { return dsl.FieldError{Reason: dsl.ReasonCompileUnrepresentable} }
func unsafe() error          { return dsl.FieldError{Reason: dsl.ReasonCompileUnsafe} }

func canonicalNode(input node) any {
	switch typed := input.(type) {
	case intersect:
		children := make([]any, len(typed.children))
		for index, child := range typed.children {
			children[index] = canonicalNode(child)
		}
		return map[string]any{"and": children}
	case union:
		children := make([]any, len(typed.children))
		for index, child := range typed.children {
			children[index] = canonicalNode(child)
		}
		return map[string]any{"or": children}
	case complement:
		return map[string]any{"not": canonicalNode(typed.child)}
	case leaf:
		return map[string]any{"leaf": string(typed.opcode), "bind": canonicalBind(typed.bind)}
	default:
		return nil
	}
}
func canonicalBind(value bind) any {
	switch {
	case value.integer != nil:
		return map[string]any{"integer": *value.integer}
	case value.integers != nil:
		return map[string]any{"integers": append([]int64(nil), value.integers...)}
	case value.timestamp != nil:
		return map[string]any{"timestamp": value.timestamp.UTC().Format(time.RFC3339Nano)}
	case value.boolean != nil:
		return map[string]any{"boolean": *value.boolean}
	case value.text != nil:
		return map[string]any{"text": *value.text}
	case value.texts != nil:
		return map[string]any{"texts": append([]string(nil), value.texts...)}
	default:
		return nil
	}
}

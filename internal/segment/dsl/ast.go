// Package dsl parses the closed Segment definition language frozen by P3-S00.
// It intentionally has no SQL, store, scheduler, or HTTP dependency.
package dsl

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	MaxDefinitionBytes = 64 << 10
	MaxDepth           = 8
	MaxNodes           = 128
	MaxGroupChildren   = 64
	MaxListValues      = 1000
	MaxStringBytes     = 64
)

type Reason string

const (
	ReasonInvalidJSON            Reason = "segment_definition_invalid_json"
	ReasonInvalidShape           Reason = "segment_definition_invalid_shape"
	ReasonDuplicateKey           Reason = "segment_definition_duplicate_key"
	ReasonLimitExceeded          Reason = "segment_definition_limit_exceeded"
	ReasonUnknownField           Reason = "segment_definition_unknown_field"
	ReasonUnsupportedOperator    Reason = "segment_definition_unsupported_operator"
	ReasonInvalidValue           Reason = "segment_definition_invalid_value"
	ReasonNoncanonicalValue      Reason = "segment_definition_noncanonical_value"
	ReasonCompileUnrepresentable Reason = "segment_compile_unrepresentable"
	ReasonCompileUnsafe          Reason = "segment_compile_unsafe"
)

// FieldError is deliberately transport-neutral. A future HTTP adapter maps it
// to the platform error envelope without exposing the definition or SQL.
type FieldError struct {
	Field  string
	Reason Reason
}

func (errorDetail FieldError) Error() string {
	return string(errorDetail.Reason)
}

func asFieldError(err error) (FieldError, bool) {
	var detail FieldError
	if !errors.As(err, &detail) {
		return FieldError{}, false
	}
	return detail, true
}

// ErrorDetail returns the stable error category and RFC 6901 JSON pointer.
func ErrorDetail(err error) (FieldError, bool) {
	return asFieldError(err)
}

type Field string

const (
	FieldStageID        Field = "stage_id"
	FieldOwnerStaffID   Field = "owner_staff_id"
	FieldChannelID      Field = "channel_id"
	FieldTagID          Field = "tag_id"
	FieldAddedAt        Field = "added_at"
	FieldLastInteractAt Field = "last_interact_at"
	FieldIsDeleted      Field = "is_deleted"
)

type Operator string

const (
	OperatorEqual  Operator = "eq"
	OperatorIn     Operator = "in"
	OperatorHasAny Operator = "has_any"
	OperatorBefore Operator = "before"
	OperatorAfter  Operator = "after"
)

// AST has exactly one root node. Node and Value are sealed so later slices
// cannot accept a caller-defined predicate or executable value type.
type AST struct {
	Root Node
}

type Node interface {
	node()
}

type And struct{ Children []Node }
type Or struct{ Children []Node }
type Predicate struct {
	Field    Field
	Operator Operator
	Value    Value
}

func (And) node()       {}
func (Or) node()        {}
func (Predicate) node() {}

type Value interface {
	value()
}

type IntValue struct{ Value int64 }
type IntListValue struct{ Values []int64 }
type TimestampValue struct{ Value time.Time }
type RelativeDaysValue struct{ Days int }
type BoolValue struct{ Value bool }

func (IntValue) value()          {}
func (IntListValue) value()      {}
func (TimestampValue) value()    {}
func (RelativeDaysValue) value() {}
func (BoolValue) value()         {}

// CanonicalJSON returns the normalized AST with sorted integer lists and
// normalized UTC timestamps. Group child order is intentionally preserved.
func (ast AST) CanonicalJSON() ([]byte, error) {
	if ast.Root == nil {
		return nil, FieldError{Reason: ReasonInvalidShape}
	}
	return json.Marshal(canonicalNode(ast.Root))
}

func canonicalNode(node Node) any {
	switch typed := node.(type) {
	case And:
		children := make([]any, len(typed.Children))
		for index, child := range typed.Children {
			children[index] = canonicalNode(child)
		}
		return map[string]any{"and": children}
	case Or:
		children := make([]any, len(typed.Children))
		for index, child := range typed.Children {
			children[index] = canonicalNode(child)
		}
		return map[string]any{"or": children}
	case Predicate:
		return map[string]any{
			"field": string(typed.Field),
			"op":    string(typed.Operator),
			"value": canonicalValue(typed.Value),
		}
	default:
		return nil
	}
}

func canonicalValue(value Value) any {
	switch typed := value.(type) {
	case IntValue:
		return typed.Value
	case IntListValue:
		return append([]int64(nil), typed.Values...)
	case TimestampValue:
		return typed.Value.UTC().Format(time.RFC3339Nano)
	case RelativeDaysValue:
		return fmt.Sprintf("-%dd", typed.Days)
	case BoolValue:
		return typed.Value
	default:
		return nil
	}
}

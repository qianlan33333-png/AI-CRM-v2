package dsl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var relativeDaysPattern = regexp.MustCompile(`^-[1-9][0-9]{0,3}d$`)

type rawObject map[string]any
type rawArray []any

type parseBudget struct {
	nodes      int
	listValues int
}

// Parse validates and canonicalizes a closed DSL v1 document. It never emits
// SQL and never accepts executable input outside the fixed field/operator set.
func Parse(input []byte) (AST, error) {
	if len(input) > MaxDefinitionBytes {
		return AST{}, fieldError("", ReasonLimitExceeded)
	}
	if !utf8.Valid(input) {
		return AST{}, fieldError("", ReasonInvalidJSON)
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	raw, err := decodeRaw(decoder, "")
	if err != nil {
		return AST{}, normalizeDecodeError(err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return AST{}, fieldError("", ReasonInvalidJSON)
	}

	budget := &parseBudget{}
	root, err := parseNode(raw, "", 1, budget)
	if err != nil {
		return AST{}, err
	}
	return AST{Root: root}, nil
}

func decodeRaw(decoder *json.Decoder, pointer string) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch typed := token.(type) {
	case json.Delim:
		switch typed {
		case '{':
			object := rawObject{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is not a string")
				}
				keyPointer := appendPointer(pointer, key)
				if _, exists := object[key]; exists {
					return nil, fieldError(keyPointer, ReasonDuplicateKey)
				}
				value, err := decodeRaw(decoder, keyPointer)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return object, nil
		case '[':
			array := rawArray{}
			for index := 0; decoder.More(); index++ {
				value, err := decodeRaw(decoder, appendPointer(pointer, strconv.Itoa(index)))
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter")
		}
	default:
		return typed, nil
	}
}

func parseNode(raw any, pointer string, depth int, budget *parseBudget) (Node, error) {
	if depth > MaxDepth {
		return nil, fieldError(pointer, ReasonLimitExceeded)
	}
	budget.nodes++
	if budget.nodes > MaxNodes {
		return nil, fieldError(pointer, ReasonLimitExceeded)
	}

	object, ok := raw.(rawObject)
	if !ok {
		return nil, fieldError(pointer, ReasonInvalidShape)
	}
	if group, exists := object["and"]; exists {
		if len(object) != 1 {
			return nil, fieldError(pointer, ReasonInvalidShape)
		}
		return parseGroup(group, appendPointer(pointer, "and"), depth, budget, true)
	}
	if group, exists := object["or"]; exists {
		if len(object) != 1 {
			return nil, fieldError(pointer, ReasonInvalidShape)
		}
		return parseGroup(group, appendPointer(pointer, "or"), depth, budget, false)
	}
	if len(object) != 3 || object["field"] == nil || object["op"] == nil || object["value"] == nil {
		return nil, fieldError(pointer, ReasonInvalidShape)
	}
	if !hasOnlyKeys(object, "field", "op", "value") {
		return nil, fieldError(pointer, ReasonInvalidShape)
	}
	fieldText, ok := object["field"].(string)
	if !ok {
		return nil, fieldError(appendPointer(pointer, "field"), ReasonInvalidValue)
	}
	if exceedsStringLimit(fieldText) {
		return nil, fieldError(appendPointer(pointer, "field"), ReasonLimitExceeded)
	}
	field, ok := knownField(fieldText)
	if !ok {
		return nil, fieldError(appendPointer(pointer, "field"), ReasonUnknownField)
	}
	opText, ok := object["op"].(string)
	if !ok {
		return nil, fieldError(appendPointer(pointer, "op"), ReasonInvalidValue)
	}
	if exceedsStringLimit(opText) {
		return nil, fieldError(appendPointer(pointer, "op"), ReasonLimitExceeded)
	}
	op, ok := allowedOperator(field, opText)
	if !ok {
		return nil, fieldError(appendPointer(pointer, "op"), ReasonUnsupportedOperator)
	}
	value, err := parseValue(field, op, object["value"], appendPointer(pointer, "value"), budget)
	if err != nil {
		return nil, err
	}
	return Predicate{Field: field, Operator: op, Value: value}, nil
}

func parseGroup(raw any, pointer string, depth int, budget *parseBudget, isAnd bool) (Node, error) {
	array, ok := raw.(rawArray)
	if !ok || len(array) == 0 {
		return nil, fieldError(pointer, ReasonInvalidShape)
	}
	if len(array) > MaxGroupChildren {
		return nil, fieldError(pointer, ReasonLimitExceeded)
	}
	children := make([]Node, len(array))
	for index, child := range array {
		parsed, err := parseNode(child, appendPointer(pointer, strconv.Itoa(index)), depth+1, budget)
		if err != nil {
			return nil, err
		}
		children[index] = parsed
	}
	if isAnd {
		return And{Children: children}, nil
	}
	return Or{Children: children}, nil
}

func parseValue(field Field, operator Operator, raw any, pointer string, budget *parseBudget) (Value, error) {
	switch field {
	case FieldStageID, FieldOwnerStaffID, FieldChannelID:
		if operator == OperatorEqual {
			value, err := parsePositiveInt(raw, pointer)
			return IntValue{Value: value}, err
		}
		return parsePositiveIntList(raw, pointer, budget)
	case FieldTagID:
		return parsePositiveIntList(raw, pointer, budget)
	case FieldAddedAt, FieldLastInteractAt:
		return parseTimeValue(raw, pointer)
	case FieldIsDeleted:
		value, ok := raw.(bool)
		if !ok {
			return nil, fieldError(pointer, ReasonInvalidValue)
		}
		return BoolValue{Value: value}, nil
	default:
		return nil, fieldError(pointer, ReasonUnknownField)
	}
}

func parsePositiveInt(raw any, pointer string) (int64, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, fieldError(pointer, ReasonInvalidValue)
	}
	if strings.ContainsAny(number.String(), ".eE") {
		return 0, fieldError(pointer, ReasonInvalidValue)
	}
	value, err := number.Int64()
	if err != nil || value <= 0 {
		return 0, fieldError(pointer, ReasonInvalidValue)
	}
	return value, nil
}

func parsePositiveIntList(raw any, pointer string, budget *parseBudget) (Value, error) {
	array, ok := raw.(rawArray)
	if !ok || len(array) == 0 {
		return nil, fieldError(pointer, ReasonInvalidValue)
	}
	if len(array) > MaxListValues || budget.listValues+len(array) > MaxListValues {
		return nil, fieldError(pointer, ReasonLimitExceeded)
	}
	budget.listValues += len(array)
	values := make([]int64, len(array))
	for index, rawValue := range array {
		value, err := parsePositiveInt(rawValue, appendPointer(pointer, strconv.Itoa(index)))
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return nil, fieldError(pointer, ReasonNoncanonicalValue)
		}
	}
	return IntListValue{Values: values}, nil
}

func parseTimeValue(raw any, pointer string) (Value, error) {
	value, ok := raw.(string)
	if !ok {
		return nil, fieldError(pointer, ReasonInvalidValue)
	}
	if exceedsStringLimit(value) {
		return nil, fieldError(pointer, ReasonLimitExceeded)
	}
	if relativeDaysPattern.MatchString(value) {
		days, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "-"), "d"))
		if err != nil {
			return nil, fieldError(pointer, ReasonInvalidValue)
		}
		return RelativeDaysValue{Days: days}, nil
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, fieldError(pointer, ReasonInvalidValue)
	}
	if _, offset := timestamp.Zone(); offset != 0 {
		return nil, fieldError(pointer, ReasonInvalidValue)
	}
	return TimestampValue{Value: timestamp.UTC()}, nil
}

func knownField(value string) (Field, bool) {
	field := Field(value)
	switch field {
	case FieldStageID, FieldOwnerStaffID, FieldChannelID, FieldTagID, FieldAddedAt, FieldLastInteractAt, FieldIsDeleted:
		return field, true
	default:
		return "", false
	}
}

func allowedOperator(field Field, value string) (Operator, bool) {
	operator := Operator(value)
	switch field {
	case FieldStageID, FieldOwnerStaffID, FieldChannelID:
		return operator, operator == OperatorEqual || operator == OperatorIn
	case FieldTagID:
		return operator, operator == OperatorHasAny
	case FieldAddedAt, FieldLastInteractAt:
		return operator, operator == OperatorBefore || operator == OperatorAfter
	case FieldIsDeleted:
		return operator, operator == OperatorEqual
	default:
		return "", false
	}
}

func hasOnlyKeys(object rawObject, expected ...string) bool {
	if len(object) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

func appendPointer(pointer, token string) string {
	escaped := strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	return pointer + "/" + escaped
}

func exceedsStringLimit(value string) bool {
	return len(value) > MaxStringBytes
}

func fieldError(pointer string, reason Reason) FieldError {
	return FieldError{Field: pointer, Reason: reason}
}

func normalizeDecodeError(err error) error {
	if detail, ok := asFieldError(err); ok {
		return detail
	}
	return fieldError("", ReasonInvalidJSON)
}

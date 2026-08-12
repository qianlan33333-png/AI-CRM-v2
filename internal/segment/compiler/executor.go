package compiler

import (
	"context"
	"sort"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
)

// QuerySet is the closed database boundary for S03. Each method is backed by
// exactly one fixed sqlc query; callers cannot supply SQL or identifiers.
type QuerySet interface {
	Universe(context.Context) ([]int64, error)
	StageEqual(context.Context, int64) ([]int64, error)
	StageAny(context.Context, []int64) ([]int64, error)
	OwnerEqual(context.Context, int64) ([]int64, error)
	OwnerAny(context.Context, []int64) ([]int64, error)
	ChannelEqual(context.Context, int64) ([]int64, error)
	ChannelAny(context.Context, []int64) ([]int64, error)
	TagAny(context.Context, []int64) ([]int64, error)
	AddedBefore(context.Context, time.Time) ([]int64, error)
	AddedAfter(context.Context, time.Time) ([]int64, error)
	LastInteractBefore(context.Context, time.Time) ([]int64, error)
	LastInteractAfter(context.Context, time.Time) ([]int64, error)
	DeletedEqual(context.Context, bool) ([]int64, error)
}

type execution struct {
	queries  QuerySet
	universe []int64
}

// Execute is the repository's only QueryProgram-to-customer-set conversion.
// It evaluates set algebra in stable traversal order and returns sorted unique
// OneIDs. Program values never become SQL text.
func Execute(ctx context.Context, program Program, queries QuerySet) ([]int64, error) {
	if program.root == nil || queries == nil {
		return nil, unsafe()
	}
	universe, err := queries.Universe(ctx)
	if err != nil {
		return nil, err
	}
	universe, err = normalizeIDs(universe)
	if err != nil {
		return nil, err
	}
	run := execution{queries: queries, universe: universe}
	selected, err := run.node(ctx, program.root)
	if err != nil {
		return nil, err
	}
	return intersectIDs(universe, selected), nil
}

func (run execution) node(ctx context.Context, input node) ([]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch typed := input.(type) {
	case intersect:
		if len(typed.children) == 0 {
			return nil, unsafe()
		}
		result := append([]int64(nil), run.universe...)
		for _, child := range typed.children {
			ids, err := run.node(ctx, child)
			if err != nil {
				return nil, err
			}
			result = intersectIDs(result, ids)
		}
		return result, nil
	case union:
		if len(typed.children) == 0 {
			return nil, unsafe()
		}
		var result []int64
		for _, child := range typed.children {
			ids, err := run.node(ctx, child)
			if err != nil {
				return nil, err
			}
			result = unionIDs(result, ids)
		}
		return result, nil
	case complement:
		ids, err := run.node(ctx, typed.child)
		if err != nil {
			return nil, err
		}
		return differenceIDs(run.universe, ids), nil
	case leaf:
		return run.leaf(ctx, typed)
	default:
		return nil, unsafe()
	}
}

func (run execution) leaf(ctx context.Context, input leaf) ([]int64, error) {
	if !validLeafBind(input) {
		return nil, unsafe()
	}
	var (
		ids []int64
		err error
	)
	switch input.opcode {
	case StageEqual:
		ids, err = run.queries.StageEqual(ctx, requiredInteger(input.bind))
	case StageAny:
		ids, err = run.queries.StageAny(ctx, requiredIntegers(input.bind))
	case OwnerEqual:
		ids, err = run.queries.OwnerEqual(ctx, requiredInteger(input.bind))
	case OwnerAny:
		ids, err = run.queries.OwnerAny(ctx, requiredIntegers(input.bind))
	case ChannelEqual:
		ids, err = run.queries.ChannelEqual(ctx, requiredInteger(input.bind))
	case ChannelAny:
		ids, err = run.queries.ChannelAny(ctx, requiredIntegers(input.bind))
	case TagHasAny:
		ids, err = run.queries.TagAny(ctx, requiredIntegers(input.bind))
	case AddedBefore:
		ids, err = run.queries.AddedBefore(ctx, requiredTimestamp(input.bind))
	case AddedAfter:
		ids, err = run.queries.AddedAfter(ctx, requiredTimestamp(input.bind))
	case LastInteractBefore:
		ids, err = run.queries.LastInteractBefore(ctx, requiredTimestamp(input.bind))
	case LastInteractAfter:
		ids, err = run.queries.LastInteractAfter(ctx, requiredTimestamp(input.bind))
	case DeletedEqual:
		ids, err = run.queries.DeletedEqual(ctx, requiredBoolean(input.bind))
	default:
		return nil, unsafe()
	}
	if err != nil {
		return nil, err
	}
	return normalizeIDs(ids)
}

func validLeafBind(input leaf) bool {
	switch input.opcode {
	case StageEqual, OwnerEqual, ChannelEqual:
		return input.bind.integer != nil && input.bind.integers == nil && input.bind.timestamp == nil && input.bind.boolean == nil && *input.bind.integer > 0
	case StageAny, OwnerAny, ChannelAny, TagHasAny:
		return input.bind.integer == nil && canonicalPositiveList(input.bind.integers) && input.bind.timestamp == nil && input.bind.boolean == nil
	case AddedBefore, AddedAfter, LastInteractBefore, LastInteractAfter:
		return input.bind.integer == nil && input.bind.integers == nil && input.bind.timestamp != nil && input.bind.boolean == nil && !input.bind.timestamp.IsZero() && input.bind.timestamp.Location() == time.UTC
	case DeletedEqual:
		return input.bind.integer == nil && input.bind.integers == nil && input.bind.timestamp == nil && input.bind.boolean != nil
	default:
		return false
	}
}

func requiredInteger(value bind) int64 {
	if value.integer == nil || value.integers != nil || value.timestamp != nil || value.boolean != nil || *value.integer <= 0 {
		return 0
	}
	return *value.integer
}

func requiredIntegers(value bind) []int64 {
	if value.integer != nil || value.integers == nil || value.timestamp != nil || value.boolean != nil || !canonicalPositiveList(value.integers) {
		return nil
	}
	return append([]int64(nil), value.integers...)
}

func requiredTimestamp(value bind) time.Time {
	if value.integer != nil || value.integers != nil || value.timestamp == nil || value.boolean != nil || value.timestamp.IsZero() || value.timestamp.Location() != time.UTC {
		return time.Time{}
	}
	return *value.timestamp
}

func requiredBoolean(value bind) bool {
	if value.integer != nil || value.integers != nil || value.timestamp != nil || value.boolean == nil {
		return false
	}
	return *value.boolean
}

func normalizeIDs(ids []int64) ([]int64, error) {
	result := append([]int64(nil), ids...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	write := 0
	for _, id := range result {
		if id <= 0 {
			return nil, dsl.FieldError{Reason: dsl.ReasonCompileUnsafe}
		}
		if write == 0 || result[write-1] != id {
			result[write] = id
			write++
		}
	}
	return result[:write], nil
}

func intersectIDs(left, right []int64) []int64 {
	result := make([]int64, 0, min(len(left), len(right)))
	for i, j := 0, 0; i < len(left) && j < len(right); {
		switch {
		case left[i] < right[j]:
			i++
		case left[i] > right[j]:
			j++
		default:
			result = append(result, left[i])
			i++
			j++
		}
	}
	return result
}

func unionIDs(left, right []int64) []int64 {
	result := make([]int64, 0, len(left)+len(right))
	for i, j := 0, 0; i < len(left) || j < len(right); {
		switch {
		case i == len(left):
			result = append(result, right[j:]...)
			return result
		case j == len(right):
			result = append(result, left[i:]...)
			return result
		case left[i] < right[j]:
			result = append(result, left[i])
			i++
		case left[i] > right[j]:
			result = append(result, right[j])
			j++
		default:
			result = append(result, left[i])
			i++
			j++
		}
	}
	return result
}

func differenceIDs(universe, excluded []int64) []int64 {
	result := make([]int64, 0, len(universe))
	for i, j := 0, 0; i < len(universe); i++ {
		for j < len(excluded) && excluded[j] < universe[i] {
			j++
		}
		if j == len(excluded) || excluded[j] != universe[i] {
			result = append(result, universe[i])
		}
	}
	return result
}

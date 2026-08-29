package membergrid

import (
	"context"
	"time"
)

// querySelection is the closed native V2 query grammar. It is deliberately
// separate from SavedView: saved views retain their legacy field semantics and
// must not be projected onto canonical member rows.
type querySelection struct {
	Sort    querySort
	GroupBy queryGroupBy
	ViewID  string
}

type querySort string

const (
	querySortUpdatedAtDesc querySort = "updated_at_desc"
	querySortStartsAtDesc  querySort = "starts_at_desc"
)

func (sort querySort) valid() bool {
	return sort == querySortUpdatedAtDesc || sort == querySortStartsAtDesc
}

type queryGroupBy string

const (
	queryGroupNone  queryGroupBy = ""
	queryGroupState queryGroupBy = "state"
)

func (groupBy queryGroupBy) valid() bool {
	return groupBy == queryGroupNone || groupBy == queryGroupState
}

func defaultQuerySelection() querySelection {
	return querySelection{Sort: querySortUpdatedAtDesc, GroupBy: queryGroupNone}
}

func normalizeQuerySelection(selection querySelection, input QueryInput) (querySelection, error) {
	if selection.Sort == "" {
		selection.Sort = querySortUpdatedAtDesc
	}
	if !selection.Sort.valid() || !selection.GroupBy.valid() {
		return querySelection{}, ErrInvalidQuery
	}
	if selection.ViewID == "" {
		return selection, nil
	}
	defaultSelection := defaultQuerySelection()
	if selection.ViewID != "default" || selection.Sort != defaultSelection.Sort || selection.GroupBy != defaultSelection.GroupBy ||
		input.State != StateAll || input.Source != SourceAny {
		return querySelection{}, ErrInvalidQuery
	}
	return selection, nil
}

// selectedPosition is the opaque continuation position for a non-default
// canonical selection. GroupState is populated only for a state-grouped page.
type selectedPosition struct {
	SortAt     time.Time
	MemberRef  string
	GroupState StateFilter
}

type selectedStoreQuery struct {
	ProductID int64
	State     StateFilter
	Source    SourceFilter
	Limit     int
	Selection querySelection
	After     *selectedPosition
}

type selectedStore interface {
	QuerySelectedMembers(context.Context, selectedStoreQuery) ([]MemberRecord, error)
}

func selectedPositionFor(record MemberRecord, selection querySelection) selectedPosition {
	position := selectedPosition{MemberRef: record.MemberRef}
	if selection.Sort == querySortStartsAtDesc {
		position.SortAt = record.StartsAt.UTC()
	} else {
		position.SortAt = record.UpdatedAt.UTC()
	}
	if selection.GroupBy == queryGroupState {
		position.GroupState = record.State
	}
	return position
}

func stateGroupRank(state StateFilter) int {
	switch state {
	case StateActive:
		return 1
	case StateExpired:
		return 2
	case StateRemoved:
		return 3
	default:
		return 0
	}
}

func recordBeforeSelection(current, previous MemberRecord, selection querySelection) bool {
	if selection.GroupBy == queryGroupState {
		currentRank, previousRank := stateGroupRank(current.State), stateGroupRank(previous.State)
		if currentRank != previousRank {
			return currentRank > previousRank
		}
	}
	currentPosition, previousPosition := selectedPositionFor(current, selection), selectedPositionFor(previous, selection)
	if currentPosition.SortAt.Before(previousPosition.SortAt) {
		return true
	}
	return currentPosition.SortAt.Equal(previousPosition.SortAt) && currentPosition.MemberRef < previousPosition.MemberRef
}

func recordAfterSelection(record MemberRecord, position selectedPosition, selection querySelection) bool {
	if selection.GroupBy == queryGroupState {
		currentRank, positionRank := stateGroupRank(record.State), stateGroupRank(position.GroupState)
		if currentRank != positionRank {
			return currentRank > positionRank
		}
	}
	current := selectedPositionFor(record, selection)
	if current.SortAt.Before(position.SortAt) {
		return true
	}
	return current.SortAt.Equal(position.SortAt) && current.MemberRef < position.MemberRef
}

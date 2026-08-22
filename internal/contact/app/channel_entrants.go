package app

import (
	"context"
	"errors"
	"reflect"
	"time"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	ChannelEntrantsDefaultLimit = 20
	ChannelEntrantsMaximumLimit = 50
)

var (
	ErrInvalidChannelEntrantsQuery  = errors.New("invalid channel entrants query")
	ErrInvalidChannelEntrantsCursor = errors.New("invalid channel entrants cursor")
	ErrChannelEntrantsNotFound      = errors.New("channel entrants channel not found")
	ErrChannelEntrantsUnavailable   = errors.New("channel entrants unavailable")
)

// ChannelEntrantItem is the complete public item projection. It deliberately
// excludes customer identity attributes, owner facts, opaque extensions and
// provider state.
type ChannelEntrantItem struct {
	CustomerID     int64      `json:"customer_id"`
	DisplayName    string     `json:"display_name"`
	AddedAt        time.Time  `json:"added_at"`
	LastInteractAt *time.Time `json:"last_interact_at"`
}

// ChannelEntrantsInput contains only local keyset-pagination inputs.
type ChannelEntrantsInput struct {
	ChannelID int64
	Limit     int
	Cursor    string
}

// ChannelEntrantsResponse states exactly what this read proves: a local Contact
// projection was read and no real external call was executed.
type ChannelEntrantsResponse struct {
	ChannelID                int64                `json:"channel_id"`
	Items                    []ChannelEntrantItem `json:"items"`
	Limit                    int                  `json:"limit"`
	HasMore                  bool                 `json:"has_more"`
	NextCursor               string               `json:"next_cursor"`
	LocalProjection          bool                 `json:"local_projection"`
	RealExternalCallExecuted bool                 `json:"real_external_call_executed"`
}

type ChannelEntrantsChannelState string

const (
	ChannelEntrantsChannelActive   ChannelEntrantsChannelState = "active"
	ChannelEntrantsChannelInactive ChannelEntrantsChannelState = "inactive"
	ChannelEntrantsChannelArchived ChannelEntrantsChannelState = "archived"
)

// ChannelEntrantsPosition is bound into the authenticated cursor.
type ChannelEntrantsPosition struct {
	AddedAt    time.Time
	CustomerID int64
}

// ChannelEntrantsRecord is an internal closed store projection. Store
// implementations must not populate it from customers.extra or identity tables.
type ChannelEntrantsRecord struct {
	CustomerID     int64
	ChannelID      int64
	DisplayName    string
	AddedAt        time.Time
	LastInteractAt *time.Time
}

type ChannelEntrantsStoreQuery struct {
	ChannelID int64
	Limit     int
	After     *ChannelEntrantsPosition
}

// ChannelEntrantsStore is Contact-internal and requires a transaction-bound
// context supplied by the UnitOfWork.
type ChannelEntrantsStore interface {
	ReadChannelEntrantsChannelState(context.Context, int64) (ChannelEntrantsChannelState, error)
	ListChannelEntrants(context.Context, ChannelEntrantsStoreQuery) ([]ChannelEntrantsRecord, error)
}

type ChannelEntrantsService struct {
	uow   platformport.UnitOfWork
	store ChannelEntrantsStore
	codec *ChannelEntrantsCursorCodec
}

func NewChannelEntrantsService(
	uow platformport.UnitOfWork,
	store ChannelEntrantsStore,
	codec *ChannelEntrantsCursorCodec,
) (*ChannelEntrantsService, error) {
	if channelEntrantsNilDependency(uow) || channelEntrantsNilDependency(store) ||
		!channelEntrantsCursorCodecReady(codec) {
		return nil, errors.New("channel entrants dependencies are required")
	}
	return &ChannelEntrantsService{uow: uow, store: store, codec: codec}, nil
}

func (service *ChannelEntrantsService) List(
	ctx context.Context,
	input ChannelEntrantsInput,
) (ChannelEntrantsResponse, error) {
	if service == nil || channelEntrantsNilDependency(service.uow) ||
		channelEntrantsNilDependency(service.store) || !channelEntrantsCursorCodecReady(service.codec) {
		return ChannelEntrantsResponse{}, ErrChannelEntrantsUnavailable
	}
	if ctx == nil {
		return ChannelEntrantsResponse{}, ErrInvalidChannelEntrantsQuery
	}
	if err := ctx.Err(); err != nil {
		return ChannelEntrantsResponse{}, errors.Join(ErrChannelEntrantsUnavailable, err)
	}

	query, err := service.normalize(input)
	if err != nil {
		return ChannelEntrantsResponse{}, err
	}

	var state ChannelEntrantsChannelState
	var records []ChannelEntrantsRecord
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		state, storeErr = service.store.ReadChannelEntrantsChannelState(txCtx, query.ChannelID)
		if storeErr != nil {
			return storeErr
		}
		switch state {
		case ChannelEntrantsChannelActive, ChannelEntrantsChannelInactive:
		case ChannelEntrantsChannelArchived:
			return ErrChannelEntrantsNotFound
		default:
			return ErrChannelEntrantsUnavailable
		}
		records, storeErr = service.store.ListChannelEntrants(txCtx, ChannelEntrantsStoreQuery{
			ChannelID: query.ChannelID,
			Limit:     query.Limit + 1,
			After:     channelEntrantsClonePosition(query.After),
		})
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrChannelEntrantsNotFound) {
			return ChannelEntrantsResponse{}, ErrChannelEntrantsNotFound
		}
		return ChannelEntrantsResponse{}, errors.Join(ErrChannelEntrantsUnavailable, err)
	}
	if len(records) > query.Limit+1 || !validChannelEntrantsRecords(records, query) {
		return ChannelEntrantsResponse{}, ErrChannelEntrantsUnavailable
	}

	hasMore := len(records) > query.Limit
	visible := records
	if hasMore {
		visible = records[:query.Limit]
	}
	response := ChannelEntrantsResponse{
		ChannelID:                query.ChannelID,
		Items:                    make([]ChannelEntrantItem, len(visible)),
		Limit:                    query.Limit,
		HasMore:                  hasMore,
		LocalProjection:          true,
		RealExternalCallExecuted: false,
	}
	for index, record := range visible {
		response.Items[index] = ChannelEntrantItem{
			CustomerID:     record.CustomerID,
			DisplayName:    record.DisplayName,
			AddedAt:        record.AddedAt.UTC(),
			LastInteractAt: channelEntrantsCloneTime(record.LastInteractAt),
		}
	}
	if hasMore {
		last := visible[len(visible)-1]
		response.NextCursor, err = service.codec.Encode(query.ChannelID, ChannelEntrantsPosition{
			AddedAt: last.AddedAt, CustomerID: last.CustomerID,
		})
		if err != nil {
			return ChannelEntrantsResponse{}, errors.Join(ErrChannelEntrantsUnavailable, err)
		}
	}
	return response, nil
}

type normalizedChannelEntrantsQuery struct {
	ChannelID int64
	Limit     int
	After     *ChannelEntrantsPosition
}

func (service *ChannelEntrantsService) normalize(input ChannelEntrantsInput) (normalizedChannelEntrantsQuery, error) {
	limit := input.Limit
	if limit == 0 {
		limit = ChannelEntrantsDefaultLimit
	}
	if input.ChannelID < 1 || limit < 1 || limit > ChannelEntrantsMaximumLimit ||
		len(input.Cursor) > channelEntrantsMaximumCursorLength {
		return normalizedChannelEntrantsQuery{}, ErrInvalidChannelEntrantsQuery
	}
	query := normalizedChannelEntrantsQuery{ChannelID: input.ChannelID, Limit: limit}
	if input.Cursor == "" {
		return query, nil
	}
	position, err := service.codec.Decode(input.Cursor, input.ChannelID)
	if err != nil {
		if errors.Is(err, ErrInvalidChannelEntrantsCursor) {
			return normalizedChannelEntrantsQuery{}, ErrInvalidChannelEntrantsCursor
		}
		return normalizedChannelEntrantsQuery{}, errors.Join(ErrChannelEntrantsUnavailable, err)
	}
	query.After = &position
	return query, nil
}

func validChannelEntrantsRecords(records []ChannelEntrantsRecord, query normalizedChannelEntrantsQuery) bool {
	seen := make(map[int64]struct{}, len(records))
	var previous *ChannelEntrantsRecord
	for index := range records {
		record := records[index]
		if record.CustomerID < 1 || record.ChannelID != query.ChannelID || record.AddedAt.IsZero() ||
			!utf8.ValidString(record.DisplayName) ||
			(record.LastInteractAt != nil && record.LastInteractAt.IsZero()) {
			return false
		}
		if _, duplicate := seen[record.CustomerID]; duplicate {
			return false
		}
		seen[record.CustomerID] = struct{}{}
		if index == 0 && query.After != nil && !channelEntrantsRecordBeforePosition(record, *query.After) {
			return false
		}
		if previous != nil && !channelEntrantsRecordBeforeRecord(record, *previous) {
			return false
		}
		previous = &records[index]
	}
	return true
}

func channelEntrantsRecordBeforeRecord(current, previous ChannelEntrantsRecord) bool {
	if current.AddedAt.Before(previous.AddedAt) {
		return true
	}
	return current.AddedAt.Equal(previous.AddedAt) && current.CustomerID < previous.CustomerID
}

func channelEntrantsRecordBeforePosition(record ChannelEntrantsRecord, position ChannelEntrantsPosition) bool {
	if record.AddedAt.Before(position.AddedAt) {
		return true
	}
	return record.AddedAt.Equal(position.AddedAt) && record.CustomerID < position.CustomerID
}

func channelEntrantsClonePosition(value *ChannelEntrantsPosition) *ChannelEntrantsPosition {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.AddedAt = cloned.AddedAt.UTC()
	return &cloned
}

func channelEntrantsCloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func channelEntrantsNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

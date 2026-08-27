package membergrid

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type Service struct {
	uow   platformport.UnitOfWork
	store Store
	codec *CursorCodec
}

func NewService(uow platformport.UnitOfWork, store Store, codec *CursorCodec) (*Service, error) {
	if nilDependency(uow) || nilDependency(store) || codec == nil || codec.aead == nil {
		return nil, errors.New("member grid dependencies are required")
	}
	return &Service{uow: uow, store: store, codec: codec}, nil
}

func (service *Service) Access(ctx context.Context, productID int64) (AccessResponse, error) {
	if err := service.requireProduct(ctx, productID); err != nil {
		return AccessResponse{}, err
	}
	return AccessResponse{
		ProductID: productID, CanView: true, CanQuery: true,
		CanManageViews: false, CanShare: false,
	}, nil
}

func (service *Service) Schema(ctx context.Context, productID int64) (SchemaResponse, error) {
	if err := service.requireProduct(ctx, productID); err != nil {
		return SchemaResponse{}, err
	}
	return SchemaResponse{ServiceProductID: productID, Columns: cloneColumns(safeColumns)}, nil
}

func (service *Service) MemberViews(ctx context.Context, productID int64) (MemberViewsResponse, error) {
	if err := service.requireProduct(ctx, productID); err != nil {
		return MemberViewsResponse{}, err
	}
	return MemberViewsResponse{ProductID: productID, Views: append([]MemberView(nil), builtInViews...)}, nil
}

func (service *Service) Query(ctx context.Context, input QueryInput) (QueryResponse, error) {
	if service == nil || nilDependency(service.uow) || nilDependency(service.store) || service.codec == nil {
		return QueryResponse{}, ErrUnavailable
	}
	if ctx == nil || input.ProductID < 1 || !input.State.validCanonicalGridState() || !input.Source.valid() || input.Limit < 1 || input.Limit > MaximumLimit {
		return QueryResponse{}, ErrInvalidQuery
	}
	if err := ctx.Err(); err != nil {
		return QueryResponse{}, errors.Join(ErrUnavailable, err)
	}

	var after *Position
	if input.Cursor != "" {
		decoded, err := service.codec.Decode(input.Cursor, input.ProductID, input.State, input.Source, input.Limit)
		if err != nil {
			return QueryResponse{}, ErrInvalidCursor
		}
		after = &decoded
	}

	var exists bool
	var records []MemberRecord
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		exists, storeErr = service.store.ProductExists(txCtx, input.ProductID)
		if storeErr != nil || !exists {
			return storeErr
		}
		records, storeErr = service.store.QueryMembers(txCtx, StoreQuery{
			ProductID: input.ProductID,
			State:     input.State,
			Source:    input.Source,
			Limit:     input.Limit + 1,
			After:     clonePosition(after),
		})
		return storeErr
	})
	if err != nil {
		return QueryResponse{}, errors.Join(ErrUnavailable, err)
	}
	if !exists {
		return QueryResponse{}, ErrNotFound
	}
	if len(records) > input.Limit+1 || !validRecords(records, input, after) {
		return QueryResponse{}, ErrUnavailable
	}

	hasMore := len(records) > input.Limit
	visible := records
	if hasMore {
		visible = records[:input.Limit]
	}
	response := QueryResponse{
		Rows:    make([]MemberRow, len(visible)),
		Limit:   input.Limit,
		HasMore: hasMore,
	}
	for index, record := range visible {
		response.Rows[index] = mapMember(record)
	}
	if hasMore && len(visible) > 0 {
		last := visible[len(visible)-1]
		response.NextCursor, err = service.codec.Encode(input.ProductID, input.State, input.Source, input.Limit, Position{
			UpdatedAt: last.UpdatedAt, MemberRef: last.MemberRef,
		})
		if err != nil {
			return QueryResponse{}, errors.Join(ErrUnavailable, err)
		}
	}
	return response, nil
}

func (service *Service) requireProduct(ctx context.Context, productID int64) error {
	if service == nil || nilDependency(service.uow) || nilDependency(service.store) {
		return ErrUnavailable
	}
	if ctx == nil || productID < 1 {
		return ErrInvalidProductID
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	var exists bool
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		exists, storeErr = service.store.ProductExists(txCtx, productID)
		return storeErr
	})
	if err != nil {
		return errors.Join(ErrUnavailable, err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func validRecords(records []MemberRecord, input QueryInput, after *Position) bool {
	seen := make(map[string]struct{}, len(records))
	var previous *MemberRecord
	for index := range records {
		record := records[index]
		if !validMemberRef(record.MemberRef) || record.ServiceProductID != input.ProductID || record.CustomerID < 1 || record.Version < 1 ||
			record.StartsAt.IsZero() || record.UpdatedAt.IsZero() || !record.State.validCanonicalGridState() || record.State == StateAll ||
			!record.Source.valid() || record.Source == SourceAny ||
			(input.State != StateAll && record.State != input.State) || (input.Source != SourceAny && record.Source != input.Source) || !utf8.ValidString(record.DisplayName) {
			return false
		}
		if _, duplicate := seen[record.MemberRef]; duplicate {
			return false
		}
		seen[record.MemberRef] = struct{}{}
		if record.ExpiresAt != nil && (record.ExpiresAt.IsZero() || record.ExpiresAt.Before(record.StartsAt)) {
			return false
		}
		if record.State == StateActive && (record.ExpiredAt != nil || record.RemovedAt != nil) {
			return false
		}
		if record.State == StateExpired && (record.ExpiredAt == nil || record.ExpiredAt.IsZero() || record.ExpiredAt.Before(record.StartsAt) || record.RemovedAt != nil) {
			return false
		}
		if record.State == StateRemoved && (record.RemovedAt == nil || record.RemovedAt.IsZero() || record.RemovedAt.Before(record.StartsAt)) {
			return false
		}
		if index == 0 && after != nil && !positionBefore(record, *after) {
			return false
		}
		if previous != nil && !recordBefore(record, *previous) {
			return false
		}
		previous = &records[index]
	}
	return true
}

func recordBefore(current, previous MemberRecord) bool {
	if current.UpdatedAt.Before(previous.UpdatedAt) {
		return true
	}
	return current.UpdatedAt.Equal(previous.UpdatedAt) && current.MemberRef < previous.MemberRef
}

func positionBefore(record MemberRecord, position Position) bool {
	if record.UpdatedAt.Before(position.UpdatedAt) {
		return true
	}
	return record.UpdatedAt.Equal(position.UpdatedAt) && record.MemberRef < position.MemberRef
}

func mapMember(record MemberRecord) MemberRow {
	return MemberRow{
		MemberRef:        record.MemberRef,
		ServiceProductID: record.ServiceProductID,
		CustomerID:       record.CustomerID,
		State:            string(record.State),
		Source:           string(record.Source),
		StartsAt:         record.StartsAt.UTC(),
		ExpiresAt:        cloneTime(record.ExpiresAt),
		ExpiredAt:        cloneTime(record.ExpiredAt),
		RemovedAt:        cloneTime(record.RemovedAt),
		Version:          record.Version,
		UpdatedAt:        record.UpdatedAt.UTC(),
		DisplayName:      record.DisplayName,
	}
}

func cloneColumns(columns []ColumnDefinition) []ColumnDefinition {
	return append([]ColumnDefinition(nil), columns...)
}

func clonePosition(position *Position) *Position {
	if position == nil {
		return nil
	}
	cloned := *position
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func validMemberRef(value string) bool {
	if len(value) != 26 || !strings.HasPrefix(value, "spm_") {
		return false
	}
	for _, character := range value[4:] {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' || character == '_') {
			return false
		}
	}
	return true
}

func nilDependency(value any) bool {
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

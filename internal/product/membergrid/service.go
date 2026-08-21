package membergrid

import (
	"context"
	"errors"
	"reflect"
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
	return SchemaResponse{ProductID: productID, Columns: cloneColumns(safeColumns)}, nil
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
	if ctx == nil || input.ProductID < 1 || !input.State.valid() || input.Limit < 1 || input.Limit > MaximumLimit {
		return QueryResponse{}, ErrInvalidQuery
	}
	if err := ctx.Err(); err != nil {
		return QueryResponse{}, errors.Join(ErrUnavailable, err)
	}

	var after *Position
	if input.Cursor != "" {
		decoded, err := service.codec.Decode(input.Cursor, input.ProductID, input.State)
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
		response.NextCursor, err = service.codec.Encode(input.ProductID, input.State, Position{
			GrantedAt: last.GrantedAt, EntitlementID: last.EntitlementID,
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
	seen := make(map[int64]struct{}, len(records))
	var previous *MemberRecord
	for index := range records {
		record := records[index]
		if record.EntitlementID < 1 || record.ProductID != input.ProductID || record.Version < 1 ||
			record.GrantedAt.IsZero() || !record.State.valid() || record.State == StateAll ||
			(input.State != StateAll && record.State != input.State) || !utf8.ValidString(record.DisplayName) {
			return false
		}
		if _, duplicate := seen[record.EntitlementID]; duplicate {
			return false
		}
		seen[record.EntitlementID] = struct{}{}
		if record.State == StateActive && record.RevokedAt != nil {
			return false
		}
		if record.State == StateRevoked && (record.RevokedAt == nil || record.RevokedAt.IsZero() || record.RevokedAt.Before(record.GrantedAt)) {
			return false
		}
		if record.MaskedMobile != nil && !validMaskedMobile(*record.MaskedMobile) {
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
	if current.GrantedAt.Before(previous.GrantedAt) {
		return true
	}
	return current.GrantedAt.Equal(previous.GrantedAt) && current.EntitlementID < previous.EntitlementID
}

func positionBefore(record MemberRecord, position Position) bool {
	if record.GrantedAt.Before(position.GrantedAt) {
		return true
	}
	return record.GrantedAt.Equal(position.GrantedAt) && record.EntitlementID < position.EntitlementID
}

func validMaskedMobile(value string) bool {
	if len(value) < 7 || len(value) > 32 || !utf8.ValidString(value) {
		return false
	}
	digits := 0
	masked := 0
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			digits++
		case character == '*':
			masked++
		default:
			return false
		}
	}
	return digits >= 2 && digits <= 7 && masked >= 3
}

func mapMember(record MemberRecord) MemberRow {
	return MemberRow{
		EntitlementID: record.EntitlementID,
		ProductID:     record.ProductID,
		State:         string(record.State),
		Version:       record.Version,
		GrantedAt:     record.GrantedAt.UTC(),
		RevokedAt:     cloneTime(record.RevokedAt),
		DisplayName:   record.DisplayName,
		MaskedMobile:  cloneString(record.MaskedMobile),
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

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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

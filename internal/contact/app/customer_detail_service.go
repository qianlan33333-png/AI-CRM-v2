package app

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"time"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var (
	ErrInvalidCustomerDetailQuery = errors.New("invalid customer detail query")
	ErrCustomerDetailUnavailable  = errors.New("customer detail unavailable")
)

// CustomerTagRecord is a local CRM tag. WeCom tag identifiers remain private
// to the wecom domain and must never be exposed through this read model.
type CustomerTagRecord struct {
	ID             int64
	GroupID        *int64
	GroupName      *string
	GroupSortOrder int32
	Name           string
	SortOrder      int32
}

type CustomerDetailInput struct {
	ID           contactport.CustomerID
	OwnerStaffID *int64
}

type CustomerDetailStoreResult struct {
	Customer CustomerRecord
	Tags     []CustomerTagRecord
}

// CustomerDetailStore is contact-internal and requires a transaction-bound
// context. OwnerStaffID is a mandatory SQL predicate when it is non-nil.
type CustomerDetailStore interface {
	GetCustomerDetail(context.Context, CustomerDetailInput) (CustomerDetailStoreResult, error)
}

type CustomerDetailService struct {
	uow   platformport.UnitOfWork
	store CustomerDetailStore
}

func NewCustomerDetailService(uow platformport.UnitOfWork, store CustomerDetailStore) *CustomerDetailService {
	return &CustomerDetailService{uow: uow, store: store}
}

func (service *CustomerDetailService) Get(
	ctx context.Context,
	input CustomerDetailInput,
) (CustomerDetailStoreResult, error) {
	if ctx == nil || input.ID <= 0 || (input.OwnerStaffID != nil && *input.OwnerStaffID <= 0) {
		return CustomerDetailStoreResult{}, ErrInvalidCustomerDetailQuery
	}
	if service == nil || nilCustomerDetailDependency(service.uow) || nilCustomerDetailDependency(service.store) {
		return CustomerDetailStoreResult{}, ErrCustomerDetailUnavailable
	}
	if err := ctx.Err(); err != nil {
		return CustomerDetailStoreResult{}, errors.Join(ErrCustomerDetailUnavailable, err)
	}

	var result CustomerDetailStoreResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var readErr error
		result, readErr = service.GetInTransaction(txCtx, input)
		return readErr
	})
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return CustomerDetailStoreResult{}, ErrCustomerNotFound
		}
		return CustomerDetailStoreResult{}, errors.Join(ErrCustomerDetailUnavailable, err)
	}
	return result, nil
}

// GetInTransaction preserves the ordinary detail validation while reusing an
// already-open UnitOfWork owned by a composed local workflow.
func (service *CustomerDetailService) GetInTransaction(ctx context.Context, input CustomerDetailInput) (CustomerDetailStoreResult, error) {
	if ctx == nil || input.ID <= 0 || (input.OwnerStaffID != nil && *input.OwnerStaffID <= 0) {
		return CustomerDetailStoreResult{}, ErrInvalidCustomerDetailQuery
	}
	if service == nil || nilCustomerDetailDependency(service.store) || ctx.Err() != nil {
		return CustomerDetailStoreResult{}, ErrCustomerDetailUnavailable
	}
	stored, err := service.store.GetCustomerDetail(ctx, cloneCustomerDetailInput(input))
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return CustomerDetailStoreResult{}, ErrCustomerNotFound
		}
		return CustomerDetailStoreResult{}, errors.Join(ErrCustomerDetailUnavailable, err)
	}
	if err = validateCustomerDetailStoreResult(stored, input); err != nil {
		return CustomerDetailStoreResult{}, errors.Join(ErrCustomerDetailUnavailable, err)
	}
	return cloneCustomerDetailResult(stored), nil
}

func validateCustomerDetailStoreResult(result CustomerDetailStoreResult, input CustomerDetailInput) error {
	customer := result.Customer
	if customer.ID != input.ID || customer.CreatedAt.IsZero() || customer.UpdatedAt.IsZero() ||
		customer.CreatedAt.After(customer.UpdatedAt) || invalidCustomerDetailID(customer.StageID) ||
		invalidCustomerDetailID(customer.OwnerStaffID) || invalidCustomerDetailID(customer.ChannelID) ||
		invalidCustomerDetailTime(customer.AddedAt) || invalidCustomerDetailTime(customer.LastInteractAt) ||
		!utf8.ValidString(customer.Name) || !IsChannelNeutralCustomerExtra(customer.Extra) ||
		(customer.AvatarURL != nil && !validCustomerDetailAvatarURL(*customer.AvatarURL)) {
		return errors.New("customer detail store returned an invalid customer")
	}
	if input.OwnerStaffID != nil && (customer.OwnerStaffID == nil || *customer.OwnerStaffID != *input.OwnerStaffID) {
		return errors.New("customer detail store escaped the owner scope")
	}

	seen := make(map[int64]struct{}, len(result.Tags))
	for index, tag := range result.Tags {
		if !validCustomerTagRecord(tag) {
			return errors.New("customer detail store returned an invalid tag")
		}
		if _, exists := seen[tag.ID]; exists {
			return errors.New("customer detail store returned a duplicate tag")
		}
		seen[tag.ID] = struct{}{}
		if index > 0 && customerTagLess(tag, result.Tags[index-1]) {
			return errors.New("customer detail store returned unstable tag order")
		}
	}
	return nil
}

func validCustomerTagRecord(tag CustomerTagRecord) bool {
	if tag.ID <= 0 || !validCustomerTagName(tag.Name) {
		return false
	}
	if tag.GroupID == nil || tag.GroupName == nil {
		return tag.GroupID == nil && tag.GroupName == nil && tag.GroupSortOrder == 0
	}
	return *tag.GroupID > 0 && utf8.ValidString(*tag.GroupName)
}

func validCustomerTagName(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200
}

func customerTagLess(left, right CustomerTagRecord) bool {
	if left.GroupSortOrder != right.GroupSortOrder {
		return left.GroupSortOrder < right.GroupSortOrder
	}
	if left.SortOrder != right.SortOrder {
		return left.SortOrder < right.SortOrder
	}
	return left.ID < right.ID
}

func cloneCustomerDetailInput(input CustomerDetailInput) CustomerDetailInput {
	cloned := input
	cloned.OwnerStaffID = cloneInt64(input.OwnerStaffID)
	return cloned
}

func cloneCustomerDetailResult(result CustomerDetailStoreResult) CustomerDetailStoreResult {
	cloned := result
	cloned.Customer.Extra = append([]byte(nil), result.Customer.Extra...)
	cloned.Customer.AvatarURL = cloneCustomerDetailString(result.Customer.AvatarURL)
	cloned.Customer.Gender = cloneInt16(result.Customer.Gender)
	cloned.Customer.StageID = cloneInt64(result.Customer.StageID)
	cloned.Customer.OwnerStaffID = cloneInt64(result.Customer.OwnerStaffID)
	cloned.Customer.ChannelID = cloneInt64(result.Customer.ChannelID)
	cloned.Customer.AddedAt = cloneCustomerDetailTime(result.Customer.AddedAt)
	cloned.Customer.LastInteractAt = cloneCustomerDetailTime(result.Customer.LastInteractAt)
	cloned.Tags = make([]CustomerTagRecord, len(result.Tags))
	for index, tag := range result.Tags {
		cloned.Tags[index] = tag
		cloned.Tags[index].GroupID = cloneInt64(tag.GroupID)
		cloned.Tags[index].GroupName = cloneCustomerDetailString(tag.GroupName)
	}
	return cloned
}

func invalidCustomerDetailID(value *int64) bool {
	return value != nil && *value <= 0
}

func invalidCustomerDetailTime(value *time.Time) bool {
	return value != nil && value.IsZero()
}

func cloneInt16(value *int16) *int16 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerDetailString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCustomerDetailTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func validCustomerDetailAvatarURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" && parsed.User == nil
}

func nilCustomerDetailDependency(value any) bool {
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

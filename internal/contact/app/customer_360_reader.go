package app

import (
	"context"
	"errors"
	"reflect"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

type customer360DetailReader interface {
	Get(context.Context, CustomerDetailInput) (CustomerDetailStoreResult, error)
}

type customer360TimelineReader interface {
	List(context.Context, CustomerEventInput) (CustomerEventResult, error)
}

// Customer360ReaderService composes the existing scoped contact detail and
// timeline readers. It does not introduce a store, mutation, or identity join.
type Customer360ReaderService struct {
	detail   customer360DetailReader
	timeline customer360TimelineReader
}

func NewCustomer360ReaderService(
	detail customer360DetailReader,
	timeline customer360TimelineReader,
) *Customer360ReaderService {
	return &Customer360ReaderService{detail: detail, timeline: timeline}
}

func (service *Customer360ReaderService) ReadCustomer360(
	ctx context.Context,
	input contactport.Customer360ReadInput,
) (contactport.Customer360Read, error) {
	if ctx == nil || input.CustomerID <= 0 || (input.OwnerStaffID != nil && *input.OwnerStaffID <= 0) ||
		input.TimelineLimit < 0 || input.TimelineLimit > CustomerListMaximumLimit || len(input.TimelineCursor) > customerEventMaximumCursor {
		return contactport.Customer360Read{}, contactport.ErrInvalidCustomer360Read
	}
	if service == nil || nilCustomer360Dependency(service.detail) || nilCustomer360Dependency(service.timeline) {
		return contactport.Customer360Read{}, contactport.ErrCustomer360ReadUnavailable
	}
	if err := ctx.Err(); err != nil {
		return contactport.Customer360Read{}, errors.Join(contactport.ErrCustomer360ReadUnavailable, err)
	}

	detail, err := service.detail.Get(ctx, CustomerDetailInput{ID: input.CustomerID, OwnerStaffID: cloneInt64(input.OwnerStaffID)})
	if err != nil {
		return contactport.Customer360Read{}, customer360ReadError(err)
	}
	timeline, err := service.timeline.List(ctx, CustomerEventInput{
		CustomerID: input.CustomerID, OwnerStaffID: cloneInt64(input.OwnerStaffID), Cursor: input.TimelineCursor, Limit: input.TimelineLimit,
	})
	if err != nil {
		return contactport.Customer360Read{}, customer360ReadError(err)
	}
	if detail.Customer.ID != input.CustomerID {
		return contactport.Customer360Read{}, contactport.ErrCustomer360ReadUnavailable
	}

	result := contactport.Customer360Read{
		Customer: contactport.Customer360Customer{
			ID:             detail.Customer.ID,
			Name:           detail.Customer.Name,
			StageID:        cloneInt64(detail.Customer.StageID),
			OwnerStaffID:   cloneInt64(detail.Customer.OwnerStaffID),
			ChannelID:      cloneInt64(detail.Customer.ChannelID),
			AddedAt:        cloneCustomerDetailTime(detail.Customer.AddedAt),
			LastInteractAt: cloneCustomerDetailTime(detail.Customer.LastInteractAt),
		},
		Tags:               make([]contactport.Customer360Tag, len(detail.Tags)),
		Timeline:           make([]contactport.Customer360TimelineEntry, len(timeline.Items)),
		TimelineNextCursor: cloneCustomer360String(timeline.NextCursor),
	}
	for index, tag := range detail.Tags {
		result.Tags[index] = contactport.Customer360Tag{
			ID: tag.ID, GroupID: cloneInt64(tag.GroupID), GroupName: cloneCustomerDetailString(tag.GroupName),
			GroupSortOrder: tag.GroupSortOrder, Name: tag.Name, SortOrder: tag.SortOrder,
		}
	}
	for index, event := range timeline.Items {
		if event.CustomerID != input.CustomerID || event.ID <= 0 || event.EventType == "" || event.OccurredAt.IsZero() {
			return contactport.Customer360Read{}, contactport.ErrCustomer360ReadUnavailable
		}
		result.Timeline[index] = contactport.Customer360TimelineEntry{ID: event.ID, EventType: event.EventType, OccurredAt: event.OccurredAt.UTC()}
	}
	return result, nil
}

func customer360ReadError(err error) error {
	if errors.Is(err, ErrCustomerNotFound) {
		return contactport.ErrCustomerReadNotFound
	}
	return errors.Join(contactport.ErrCustomer360ReadUnavailable, err)
}

func cloneCustomer360String(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func nilCustomer360Dependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

var _ contactport.Customer360Reader = (*Customer360ReaderService)(nil)

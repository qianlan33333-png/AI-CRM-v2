package app

import (
	"context"
	"errors"
	"reflect"
	"unicode/utf8"

	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrTagCatalogUnavailable = errors.New("tag catalog unavailable")

type TagCatalogRecord struct {
	ID             int64
	GroupID        *int64
	GroupName      *string
	GroupSortOrder *int32
	Name           string
	SortOrder      int32
}

type TagCatalogStore interface {
	ListTags(context.Context) ([]TagCatalogRecord, error)
}

type TagCatalogService struct {
	uow   platformport.UnitOfWork
	store TagCatalogStore
}

func NewTagCatalogService(uow platformport.UnitOfWork, store TagCatalogStore) *TagCatalogService {
	return &TagCatalogService{uow: uow, store: store}
}

func (service *TagCatalogService) List(ctx context.Context) ([]TagCatalogRecord, error) {
	if ctx == nil || service == nil || nilTagCatalogDependency(service.uow) || nilTagCatalogDependency(service.store) {
		return nil, ErrTagCatalogUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrTagCatalogUnavailable, err)
	}

	var records []TagCatalogRecord
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		var storeErr error
		records, storeErr = service.store.ListTags(txCtx)
		return storeErr
	})
	if err != nil {
		return nil, errors.Join(ErrTagCatalogUnavailable, err)
	}
	if records == nil {
		return nil, ErrTagCatalogUnavailable
	}
	if err = validateTagCatalog(records); err != nil {
		return nil, errors.Join(ErrTagCatalogUnavailable, err)
	}
	return cloneTagCatalog(records), nil
}

func validateTagCatalog(records []TagCatalogRecord) error {
	seenTags := make(map[int64]struct{}, len(records))
	groups := make(map[int64]tagCatalogGroup)
	for index, record := range records {
		if !validTagCatalogRecord(record) {
			return errors.New("tag catalog store returned an invalid tag")
		}
		if _, duplicate := seenTags[record.ID]; duplicate {
			return errors.New("tag catalog store returned a duplicate tag")
		}
		seenTags[record.ID] = struct{}{}
		if record.GroupID != nil {
			group := tagCatalogGroup{name: *record.GroupName, sortOrder: *record.GroupSortOrder}
			if previous, exists := groups[*record.GroupID]; exists && previous != group {
				return errors.New("tag catalog store returned an inconsistent group")
			}
			groups[*record.GroupID] = group
		}
		if index > 0 && tagCatalogLess(record, records[index-1]) {
			return errors.New("tag catalog store returned unstable order")
		}
	}
	return nil
}

type tagCatalogGroup struct {
	name      string
	sortOrder int32
}

func validTagCatalogRecord(record TagCatalogRecord) bool {
	if record.ID <= 0 || !validTagCatalogText(record.Name) {
		return false
	}
	grouped := record.GroupID != nil || record.GroupName != nil || record.GroupSortOrder != nil
	if !grouped {
		return true
	}
	return record.GroupID != nil && *record.GroupID > 0 && record.GroupName != nil &&
		validTagCatalogText(*record.GroupName) && record.GroupSortOrder != nil
}

func validTagCatalogText(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 200
}

func tagCatalogLess(left, right TagCatalogRecord) bool {
	leftGrouped, rightGrouped := left.GroupID != nil, right.GroupID != nil
	if leftGrouped != rightGrouped {
		return leftGrouped
	}
	if leftGrouped {
		if *left.GroupSortOrder != *right.GroupSortOrder {
			return *left.GroupSortOrder < *right.GroupSortOrder
		}
		if *left.GroupID != *right.GroupID {
			return *left.GroupID < *right.GroupID
		}
	}
	if left.SortOrder != right.SortOrder {
		return left.SortOrder < right.SortOrder
	}
	return left.ID < right.ID
}

func cloneTagCatalog(records []TagCatalogRecord) []TagCatalogRecord {
	cloned := make([]TagCatalogRecord, len(records))
	for index, record := range records {
		cloned[index] = record
		cloned[index].GroupID = cloneInt64(record.GroupID)
		cloned[index].GroupName = cloneCustomerDetailString(record.GroupName)
		if record.GroupSortOrder != nil {
			value := *record.GroupSortOrder
			cloned[index].GroupSortOrder = &value
		}
	}
	return cloned
}

func nilTagCatalogDependency(value any) bool {
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

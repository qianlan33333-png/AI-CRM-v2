package app

import (
	"context"
	"errors"
	"sort"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// CustomerReferenceRecord proves only an active canonical customer ID while
// retaining a transactional reference lock. All customer detail projections
// remain owned by the existing Contact read services.
type CustomerReferenceRecord struct {
	ID contactport.CustomerID
}

// CustomerReferenceStore owns a single sorted batch query over active
// customers. Its implementation must retain a SHARE lock until the
// caller's UnitOfWork commits or rolls back.
type CustomerReferenceStore interface {
	ReadActiveCustomerReferences(context.Context, []contactport.CustomerID) ([]CustomerReferenceRecord, error)
}

// CustomerReferenceReader preserves Contact validation at an already-open
// transaction seam. It intentionally does not start a nested UnitOfWork.
type CustomerReferenceReader struct {
	store CustomerReferenceStore
}

func NewCustomerReferenceReader(store CustomerReferenceStore) *CustomerReferenceReader {
	return &CustomerReferenceReader{store: store}
}

func (reader *CustomerReferenceReader) ReadInTransaction(ctx context.Context, customerIDs []contactport.CustomerID) ([]CustomerReferenceRecord, error) {
	if reader == nil || reader.store == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrCustomerListUnavailable
	}
	normalized, err := normalizeCustomerReferenceIDs(customerIDs)
	if err != nil {
		return nil, err
	}
	items, err := reader.store.ReadActiveCustomerReferences(ctx, normalized)
	if err != nil {
		return nil, errors.Join(ErrCustomerListUnavailable, err)
	}
	if len(items) != len(normalized) {
		return nil, ErrCustomerNotFound
	}
	for index, item := range items {
		if item.ID <= 0 {
			return nil, ErrCustomerListUnavailable
		}
		if item.ID != normalized[index] {
			if item.ID < normalized[index] {
				return nil, ErrCustomerListUnavailable
			}
			return nil, ErrCustomerNotFound
		}
	}
	return append([]CustomerReferenceRecord(nil), items...), nil
}

func normalizeCustomerReferenceIDs(customerIDs []contactport.CustomerID) ([]contactport.CustomerID, error) {
	if len(customerIDs) == 0 || len(customerIDs) > int(CustomerListMaximumLimit)*5 {
		return nil, ErrInvalidCustomerListQuery
	}
	result := append([]contactport.CustomerID(nil), customerIDs...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	write := 0
	for _, customerID := range result {
		if customerID <= 0 {
			return nil, ErrInvalidCustomerListQuery
		}
		if write > 0 && result[write-1] == customerID {
			continue
		}
		result[write] = customerID
		write++
	}
	return result[:write], nil
}

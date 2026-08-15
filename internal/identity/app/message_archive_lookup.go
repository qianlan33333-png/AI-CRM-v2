package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

var ErrMessageArchiveUnionLookupFailed = errors.New("message archive unionid lookup failed")

// MessageArchiveUnionIDStore returns no identifier or scope. The two-row cap
// makes cross-scope ambiguity fail closed instead of selecting an arbitrary
// identity binding.
type MessageArchiveUnionIDStore interface {
	LookupMessageArchiveUnionIDCustomers(context.Context, string) ([]int64, error)
}

type MessageArchiveUnionIDResolver struct {
	uow   platformport.UnitOfWork
	store MessageArchiveUnionIDStore
}

func NewMessageArchiveUnionIDResolver(uow platformport.UnitOfWork, store MessageArchiveUnionIDStore) *MessageArchiveUnionIDResolver {
	return &MessageArchiveUnionIDResolver{uow: uow, store: store}
}

func (resolver *MessageArchiveUnionIDResolver) ResolveUnionID(ctx context.Context, unionID string) (identityport.ResolveResult, error) {
	if !validMessageArchiveUnionID(unionID) {
		return identityport.ResolveResult{}, ErrInvalidIdentity
	}
	if resolver == nil || ctx == nil || nilMessageArchiveUnionDependency(resolver.uow) || nilMessageArchiveUnionDependency(resolver.store) {
		return identityport.ResolveResult{}, ErrMessageArchiveUnionLookupFailed
	}
	var customerIDs []int64
	err := resolver.uow.Within(ctx, func(tx context.Context) error {
		var err error
		customerIDs, err = resolver.store.LookupMessageArchiveUnionIDCustomers(tx, unionID)
		return err
	})
	if err != nil || len(customerIDs) > 2 || !validMessageArchiveCustomerIDs(customerIDs) {
		return identityport.ResolveResult{}, errors.Join(ErrMessageArchiveUnionLookupFailed, err)
	}
	switch len(customerIDs) {
	case 0:
		return identityport.ResolveResult{Status: identityport.ResolveNotFound}, nil
	case 1:
		return identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(customerIDs[0])}, nil
	default:
		return identityport.ResolveResult{Status: identityport.ResolveConflict}, nil
	}
}

func validMessageArchiveUnionID(value string) bool {
	return value != "" && len(value) <= 1024 && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}

func validMessageArchiveCustomerIDs(values []int64) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func nilMessageArchiveUnionDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return (reflected.Kind() == reflect.Chan || reflected.Kind() == reflect.Func || reflected.Kind() == reflect.Interface ||
		reflected.Kind() == reflect.Map || reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Slice) && reflected.IsNil()
}

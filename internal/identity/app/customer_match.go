package app

import (
	"context"
	"errors"
	"reflect"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const CustomerMatchMaximumBatch = 500

var ErrCustomerIdentityMatchFailed = errors.New("customer identity match failed")

type customerMatchStore interface {
	ResolveStore
	MessageArchiveUnionIDStore
}

// CustomerMatcherService resolves a bounded batch inside one UoW that only
// performs reads. It never returns identity values or creates/binds an identity.
// Any malformed, ambiguous, conflicting, or unavailable hint fails the complete
// batch closed.
type CustomerMatcherService struct {
	uow   platformport.UnitOfWork
	store customerMatchStore
}

func NewCustomerMatcherService(uow platformport.UnitOfWork, store customerMatchStore) *CustomerMatcherService {
	return &CustomerMatcherService{uow: uow, store: store}
}

type preparedCustomerMatch struct {
	CustomerID    contactport.CustomerID
	Refs          []NormalizedIdentity
	LegacyUnionID string
}

func (service *CustomerMatcherService) MatchCustomers(ctx context.Context, requests []identityport.CustomerMatchRequest) ([]bool, error) {
	if ctx == nil || len(requests) < 1 || len(requests) > CustomerMatchMaximumBatch || service == nil ||
		nilCustomerMatchDependency(service.uow) || nilCustomerMatchDependency(service.store) {
		return nil, ErrCustomerIdentityMatchFailed
	}
	prepared := make([]preparedCustomerMatch, len(requests))
	for index, request := range requests {
		if request.CustomerID <= 0 || len(request.Refs) > 3 ||
			(len(request.Refs) == 0 && request.LegacyUnionID == "") ||
			(request.LegacyUnionID != "" && !validMessageArchiveUnionID(request.LegacyUnionID)) {
			return nil, ErrCustomerIdentityMatchFailed
		}
		prepared[index] = preparedCustomerMatch{CustomerID: request.CustomerID, LegacyUnionID: request.LegacyUnionID}
		prepared[index].Refs = make([]NormalizedIdentity, len(request.Refs))
		for refIndex, ref := range request.Refs {
			normalized, err := Normalize(ref)
			if err != nil {
				return nil, errors.Join(ErrCustomerIdentityMatchFailed, err)
			}
			prepared[index].Refs[refIndex] = normalized
		}
	}

	results := make([]bool, len(prepared))
	err := service.uow.Within(ctx, func(tx context.Context) error {
		for index, request := range prepared {
			matched, matchErr := service.matchWithin(tx, request)
			if matchErr != nil {
				return matchErr
			}
			results[index] = matched
		}
		return nil
	})
	if err != nil {
		return nil, errors.Join(ErrCustomerIdentityMatchFailed, err)
	}
	return append([]bool(nil), results...), nil
}

func (service *CustomerMatcherService) matchWithin(ctx context.Context, request preparedCustomerMatch) (bool, error) {
	results := make([]identityport.ResolveResult, 0, len(request.Refs)+1)
	for _, ref := range request.Refs {
		record, err := service.store.LookupNormalized(ctx, ref)
		if err != nil {
			return false, err
		}
		result := identityport.ResolveResult{Status: identityport.ResolveNotFound}
		switch {
		case record.Conflict:
			result = identityport.ResolveResult{Status: identityport.ResolveConflict}
		case record.CustomerID == 0:
		case record.CustomerID > 0:
			result = identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(record.CustomerID)}
		default:
			return false, ErrCustomerIdentityMatchFailed
		}
		results = append(results, result)
	}
	if request.LegacyUnionID != "" {
		customerIDs, err := service.store.LookupMessageArchiveUnionIDCustomers(ctx, request.LegacyUnionID)
		if err != nil || len(customerIDs) > 2 || !validMessageArchiveCustomerIDs(customerIDs) {
			return false, errors.Join(ErrCustomerIdentityMatchFailed, err)
		}
		result := identityport.ResolveResult{Status: identityport.ResolveNotFound}
		switch len(customerIDs) {
		case 0:
		case 1:
			result = identityport.ResolveResult{Status: identityport.ResolveFound, CustomerID: contactport.CustomerID(customerIDs[0])}
		default:
			result = identityport.ResolveResult{Status: identityport.ResolveConflict}
		}
		results = append(results, result)
	}

	var found identityport.ResolveResult
	for _, result := range results {
		switch result.Status {
		case identityport.ResolveNotFound:
			if result.CustomerID != 0 {
				return false, ErrCustomerIdentityMatchFailed
			}
		case identityport.ResolveFound:
			if result.CustomerID <= 0 || found.Status == identityport.ResolveFound && found.CustomerID != result.CustomerID {
				return false, ErrCustomerIdentityMatchFailed
			}
			found = result
		case identityport.ResolveConflict:
			return false, ErrCustomerIdentityMatchFailed
		default:
			return false, ErrCustomerIdentityMatchFailed
		}
	}
	return found.Status == identityport.ResolveFound && found.CustomerID == request.CustomerID, nil
}

func nilCustomerMatchDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

var _ identityport.CustomerMatcher = (*CustomerMatcherService)(nil)

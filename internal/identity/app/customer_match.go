package app

import (
	"context"
	"errors"
	"reflect"

	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
)

var ErrCustomerIdentityMatchFailed = errors.New("customer identity match failed")

type customerIdentityResolver interface {
	Resolve(context.Context, identityport.IDRef) (identityport.ResolveResult, error)
}

type legacyUnionIDResolver interface {
	ResolveUnionID(context.Context, string) (identityport.ResolveResult, error)
}

// CustomerMatcherService resolves every usable hint and fails closed when two
// found hints disagree or any hint is ambiguous. It never returns an identity
// value or creates/binds an identity.
type CustomerMatcherService struct {
	resolver customerIdentityResolver
	union    legacyUnionIDResolver
}

func NewCustomerMatcherService(resolver customerIdentityResolver, union legacyUnionIDResolver) *CustomerMatcherService {
	return &CustomerMatcherService{resolver: resolver, union: union}
}

func (service *CustomerMatcherService) MatchesCustomer(ctx context.Context, request identityport.CustomerMatchRequest) (bool, error) {
	if ctx == nil || request.CustomerID <= 0 || len(request.Refs) > 3 || len(request.LegacyUnionID) > 1024 ||
		(len(request.Refs) == 0 && request.LegacyUnionID == "") || service == nil || nilMatchDependency(service.resolver) {
		return false, ErrCustomerIdentityMatchFailed
	}

	results := make([]identityport.ResolveResult, 0, len(request.Refs)+1)
	for _, ref := range request.Refs {
		result, err := service.resolver.Resolve(ctx, ref)
		if err != nil {
			return false, errors.Join(ErrCustomerIdentityMatchFailed, err)
		}
		results = append(results, result)
	}
	if request.LegacyUnionID != "" {
		if nilMatchDependency(service.union) {
			return false, ErrCustomerIdentityMatchFailed
		}
		result, err := service.union.ResolveUnionID(ctx, request.LegacyUnionID)
		if err != nil {
			return false, errors.Join(ErrCustomerIdentityMatchFailed, err)
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

func nilMatchDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	return reflected.Kind() == reflect.Pointer && reflected.IsNil()
}

var _ identityport.CustomerMatcher = (*CustomerMatcherService)(nil)

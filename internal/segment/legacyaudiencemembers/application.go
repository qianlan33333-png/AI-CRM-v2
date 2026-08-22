package legacyaudiencemembers

import (
	"context"
	"errors"
	"reflect"
	"strings"
)

var _ Application = (*Service)(nil)

type Service struct {
	packages   PackageExistenceReader
	members    MemberRepository
	identities TrustedIdentityReader
}

func NewService(
	packages PackageExistenceReader,
	members MemberRepository,
	identities TrustedIdentityReader,
) (*Service, error) {
	if nilInterface(packages) || nilInterface(members) || nilInterface(identities) {
		return nil, ErrUnavailable
	}
	return &Service{packages: packages, members: members, identities: identities}, nil
}

func (service *Service) ListMembers(ctx context.Context, input ListInput) (ListResponse, error) {
	if service == nil || nilInterface(service.packages) || nilInterface(service.members) ||
		nilInterface(service.identities) || ctx == nil {
		return ListResponse{}, ErrUnavailable
	}
	if input.PackageID <= 0 {
		return ListResponse{}, ErrNotFound
	}
	if input.Limit < 1 || input.Limit > MaximumLimit || input.Offset < 0 {
		return ListResponse{}, ErrInvalidInput
	}

	exists, err := service.packages.PackageExists(ctx, input.PackageID)
	if err != nil {
		return ListResponse{}, errors.Join(ErrUnavailable, err)
	}
	if !exists {
		return ListResponse{}, ErrNotFound
	}

	page, err := service.members.ListMembers(ctx, input.PackageID, input.Limit, input.Offset)
	if err != nil {
		return ListResponse{}, errors.Join(ErrUnavailable, err)
	}
	if page.Total < 0 || len(page.Items) > input.Limit || page.Total < int64(len(page.Items)) || !stableMemberPage(page.Items) {
		return ListResponse{}, ErrUnavailable
	}

	customerIDs := make([]int64, len(page.Items))
	known := make(map[int64]struct{}, len(page.Items))
	for index, record := range page.Items {
		if record.CustomerID <= 0 || record.EnteredAt.IsZero() {
			return ListResponse{}, ErrUnavailable
		}
		if _, duplicate := known[record.CustomerID]; duplicate {
			return ListResponse{}, ErrUnavailable
		}
		known[record.CustomerID] = struct{}{}
		customerIDs[index] = record.CustomerID
	}

	resolved := make(map[int64]string, len(page.Items))
	seenIdentityCustomers := make(map[int64]struct{}, len(page.Items))
	if len(customerIDs) > 0 {
		identities, identityErr := service.identities.ListPrimaryExternalUserIDs(ctx, customerIDs)
		if identityErr != nil {
			return ListResponse{}, errors.Join(ErrUnavailable, identityErr)
		}
		for _, identity := range identities {
			if identity.CustomerID <= 0 {
				return ListResponse{}, ErrUnavailable
			}
			if _, allowed := known[identity.CustomerID]; !allowed {
				return ListResponse{}, ErrUnavailable
			}
			if _, duplicate := seenIdentityCustomers[identity.CustomerID]; duplicate {
				return ListResponse{}, ErrUnavailable
			}
			seenIdentityCustomers[identity.CustomerID] = struct{}{}
			if strings.TrimSpace(identity.ExternalUserID) == "" {
				continue
			}
			resolved[identity.CustomerID] = identity.ExternalUserID
		}
	}

	items := make([]MemberItem, 0, len(page.Items))
	for _, record := range page.Items {
		nickname := record.Nickname
		if strings.TrimSpace(nickname) == "" {
			nickname = UnnamedCustomer
		}
		items = append(items, MemberItem{
			CustomerID:     record.CustomerID,
			Nickname:       nickname,
			ExternalUserID: resolved[record.CustomerID],
			EnteredAt:      record.EnteredAt.UTC(),
		})
	}

	return ListResponse{
		OK:                       true,
		Items:                    items,
		Total:                    page.Total,
		Count:                    len(items),
		Limit:                    input.Limit,
		Offset:                   input.Offset,
		RealExternalCallExecuted: false,
	}, nil
}

func stableMemberPage(items []MemberRecord) bool {
	for index := 1; index < len(items); index++ {
		previous, current := items[index-1], items[index]
		if previous.EnteredAt.Before(current.EnteredAt) {
			return false
		}
		if previous.EnteredAt.Equal(current.EnteredAt) && previous.CustomerID <= current.CustomerID {
			return false
		}
	}
	return true
}

func nilInterface(value any) bool {
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

package main

import (
	"context"
	"errors"
	"net/http"
	"sort"

	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/legacyaudiencemembers"
)

type legacyAIAudienceMembersSecurity struct{}

func (legacyAIAudienceMembersSecurity) Authorize(request *http.Request, requirement legacyaudiencemembers.AccessRequirement) error {
	if request == nil {
		return legacyaudiencemembers.ErrUnauthenticated
	}
	principal, principalOK := authport.PrincipalFromContext(request.Context())
	authorization, authorizationOK := authport.AuthorizationFromContext(request.Context())
	if !principalOK || principal.AdminUserID < 1 {
		return legacyaudiencemembers.ErrUnauthenticated
	}
	if !authorizationOK || authorization.Scope != authport.ScopeGlobal ||
		string(authorization.Capability) != requirement.Capability ||
		requirement.Capability != legacyaudiencemembers.CapabilitySegmentsRead || requirement.RequireCSRF {
		return legacyaudiencemembers.ErrForbidden
	}
	return nil
}

// legacyAIAudienceMembersApplication keeps the Segment snapshot and Identity
// projection inside one root-owned transaction. Owner repositories receive the
// same transaction-bound context and must not open nested units of work.
type legacyAIAudienceMembersApplication struct {
	uow         platformport.UnitOfWork
	application legacyaudiencemembers.Application
}

func (application legacyAIAudienceMembersApplication) ListMembers(
	ctx context.Context,
	input legacyaudiencemembers.ListInput,
) (legacyaudiencemembers.ListResponse, error) {
	if application.uow == nil || application.application == nil || ctx == nil {
		return legacyaudiencemembers.ListResponse{}, legacyaudiencemembers.ErrUnavailable
	}
	var response legacyaudiencemembers.ListResponse
	err := application.uow.Within(ctx, func(txCtx context.Context) error {
		attemptResponse, readErr := application.application.ListMembers(txCtx, input)
		if readErr != nil {
			return readErr
		}
		response = attemptResponse
		return nil
	})
	if err != nil {
		return legacyaudiencemembers.ListResponse{}, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
	}
	return response, nil
}

// legacyAIAudienceMembersIdentityReader exposes only a unique verified WeCom
// external_userid. Customers with no verified identity or conflicting values
// across scopes are deliberately omitted from the projection.
type legacyAIAudienceMembersIdentityReader struct {
	reader identityport.TrustedWeComIdentityReader
}

func (reader legacyAIAudienceMembersIdentityReader) ListPrimaryExternalUserIDs(
	ctx context.Context,
	customerIDs []int64,
) ([]legacyaudiencemembers.TrustedExternalIdentity, error) {
	if reader.reader == nil || ctx == nil ||
		len(customerIDs) > identityport.MaximumTrustedWeComIdentityCustomerIDs {
		return nil, legacyaudiencemembers.ErrUnavailable
	}
	if len(customerIDs) == 0 {
		return []legacyaudiencemembers.TrustedExternalIdentity{}, nil
	}
	ids := append([]int64(nil), customerIDs...)
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for index, id := range ids {
		if id <= 0 || (index > 0 && id == ids[index-1]) {
			return nil, legacyaudiencemembers.ErrUnavailable
		}
	}
	trustedIDs := make([]contactport.CustomerID, len(ids))
	for index, id := range ids {
		trustedIDs[index] = contactport.CustomerID(id)
	}
	identities, err := reader.reader.ListPrimaryWeComExternalUserIDs(ctx, trustedIDs)
	if err != nil {
		return nil, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
	}
	items := make([]legacyaudiencemembers.TrustedExternalIdentity, 0, len(identities))
	for _, identity := range identities {
		items = append(items, legacyaudiencemembers.TrustedExternalIdentity{
			CustomerID: int64(identity.CustomerID), ExternalUserID: identity.ExternalUserID,
		})
	}
	return items, nil
}

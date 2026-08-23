package main

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
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

type legacyAIAudienceMembersSQLProvider struct{ pool *pgxpool.Pool }

func (provider legacyAIAudienceMembersSQLProvider) Reader(context.Context) (legacyaudiencemembers.SQLReader, error) {
	if provider.pool == nil {
		return nil, legacyaudiencemembers.ErrUnavailable
	}
	return legacyAIAudienceMembersSQLReader{queryer: provider.pool}, nil
}

type legacyAIAudienceMembersSQLReader struct{ queryer legacyAIAudienceQueryer }

func (reader legacyAIAudienceMembersSQLReader) QueryRow(ctx context.Context, query string, arguments ...any) legacyaudiencemembers.SQLRow {
	return reader.queryer.QueryRow(ctx, query, arguments...)
}

func (reader legacyAIAudienceMembersSQLReader) Query(ctx context.Context, query string, arguments ...any) (legacyaudiencemembers.SQLRows, error) {
	return reader.queryer.Query(ctx, query, arguments...)
}

// legacyAIAudienceMembersIdentityReader exposes only a unique verified WeCom
// external_userid. Customers with no verified identity or conflicting values
// across scopes are deliberately omitted from the projection.
type legacyAIAudienceMembersIdentityReader struct {
	uow    platformport.UnitOfWork
	reader identityport.TrustedWeComIdentityReader
}

func (reader legacyAIAudienceMembersIdentityReader) ListPrimaryExternalUserIDs(
	ctx context.Context,
	customerIDs []int64,
) ([]legacyaudiencemembers.TrustedExternalIdentity, error) {
	if reader.uow == nil || reader.reader == nil || ctx == nil ||
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
	items := make([]legacyaudiencemembers.TrustedExternalIdentity, 0, len(ids))
	err := reader.uow.Within(ctx, func(txCtx context.Context) error {
		identities, readErr := reader.reader.ListPrimaryWeComExternalUserIDs(txCtx, trustedIDs)
		if readErr != nil {
			return readErr
		}
		attemptItems := make([]legacyaudiencemembers.TrustedExternalIdentity, 0, len(identities))
		for _, identity := range identities {
			attemptItems = append(attemptItems, legacyaudiencemembers.TrustedExternalIdentity{
				CustomerID: int64(identity.CustomerID), ExternalUserID: identity.ExternalUserID,
			})
		}
		items = attemptItems
		return nil
	})
	if err != nil {
		return nil, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
	}
	return items, nil
}

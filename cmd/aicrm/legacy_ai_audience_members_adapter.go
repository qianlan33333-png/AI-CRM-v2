package main

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
	authport "github.com/qianlan33333-png/AI-CRM-v2/internal/auth/port"
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

const legacyAIAudienceMembersExternalIdentitySQL = `SELECT
  customer_id,
  min(normalized_value) AS external_userid
FROM public.identities
WHERE customer_id = ANY($1::bigint[])
  AND kind = 'wecom_external_userid'
  AND assurance = 'verified'
GROUP BY customer_id
HAVING count(DISTINCT normalized_value) = 1
ORDER BY customer_id`

// legacyAIAudienceMembersIdentityReader exposes only a unique verified WeCom
// external_userid. Customers with no verified identity or conflicting values
// across scopes are deliberately omitted from the projection.
type legacyAIAudienceMembersIdentityReader struct{ queryer legacyAIAudienceQueryer }

func (reader legacyAIAudienceMembersIdentityReader) ListPrimaryExternalUserIDs(
	ctx context.Context,
	customerIDs []int64,
) ([]legacyaudiencemembers.TrustedExternalIdentity, error) {
	if reader.queryer == nil || ctx == nil {
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
	rows, err := reader.queryer.Query(ctx, legacyAIAudienceMembersExternalIdentitySQL, ids)
	if err != nil {
		return nil, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
	}
	defer rows.Close()

	items := make([]legacyaudiencemembers.TrustedExternalIdentity, 0, len(ids))
	for rows.Next() {
		var item legacyaudiencemembers.TrustedExternalIdentity
		if err = rows.Scan(&item.CustomerID, &item.ExternalUserID); err != nil {
			return nil, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(legacyaudiencemembers.ErrUnavailable, err)
	}
	return items, nil
}

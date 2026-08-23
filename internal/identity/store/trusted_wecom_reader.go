package store

import (
	"context"
	"sort"
	"strings"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	identityapp "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/app"
	identityport "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/port"
	identitydb "github.com/qianlan33333-png/AI-CRM-v2/internal/identity/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

var _ identityport.TrustedWeComIdentityReader = (*Repository)(nil)

func (repository *Repository) ListPrimaryWeComExternalUserIDs(
	ctx context.Context,
	customerIDs []contactport.CustomerID,
) ([]identityport.TrustedWeComExternalIdentity, error) {
	if repository == nil || ctx == nil || len(customerIDs) > identityport.MaximumTrustedWeComIdentityCustomerIDs {
		return nil, identityapp.ErrInvalidIdentity
	}
	ids := append([]contactport.CustomerID(nil), customerIDs...)
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	queryIDs := make([]int64, len(ids))
	for index, id := range ids {
		if id <= 0 || (index > 0 && id == ids[index-1]) {
			return nil, identityapp.ErrInvalidIdentity
		}
		queryIDs[index] = int64(id)
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(queryIDs) == 0 {
		return []identityport.TrustedWeComExternalIdentity{}, nil
	}
	known := make(map[int64]struct{}, len(queryIDs))
	for _, id := range queryIDs {
		known[id] = struct{}{}
	}
	rows, err := identitydb.New(tx).ListPrimaryWeComExternalUserIDs(ctx, queryIDs)
	if err != nil {
		return nil, err
	}
	items := make([]identityport.TrustedWeComExternalIdentity, 0, len(rows))
	var previousID int64
	for _, row := range rows {
		if !row.CustomerID.Valid || row.CustomerID.Int64 <= previousID || strings.TrimSpace(row.ExternalUserid) == "" {
			return nil, identityapp.ErrInvalidIdentity
		}
		if _, exists := known[row.CustomerID.Int64]; !exists {
			return nil, identityapp.ErrInvalidIdentity
		}
		previousID = row.CustomerID.Int64
		items = append(items, identityport.TrustedWeComExternalIdentity{
			CustomerID: contactport.CustomerID(row.CustomerID.Int64), ExternalUserID: row.ExternalUserid,
		})
	}
	return items, nil
}

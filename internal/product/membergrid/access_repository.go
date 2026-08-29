package membergrid

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

type AccessRepository struct{}

type DataStore struct{ base Store }

var _ AccessStore = (*AccessRepository)(nil)
var _ Store = (*DataStore)(nil)
var _ selectedStore = (*DataStore)(nil)

func NewAccessRepository() *AccessRepository { return &AccessRepository{} }
func NewDataStore(base Store) *DataStore     { return &DataStore{base: base} }

func (store *DataStore) ProductExists(ctx context.Context, productID int64) (bool, error) {
	if store == nil || nilDependency(store.base) {
		return false, ErrUnavailable
	}
	return store.base.ProductExists(ctx, productID)
}

func (store *DataStore) QueryMembers(ctx context.Context, query StoreQuery) ([]MemberRecord, error) {
	if store == nil || nilDependency(store.base) {
		return nil, ErrUnavailable
	}
	return store.base.QueryMembers(ctx, query)
}

func (store *DataStore) QuerySelectedMembers(ctx context.Context, query selectedStoreQuery) ([]MemberRecord, error) {
	if store == nil || nilDependency(store.base) || ctx == nil || query.ProductID < 1 || !query.State.validCanonicalGridState() ||
		!query.Source.valid() || query.Limit < 1 || query.Limit > MaximumLimit+1 || !query.Selection.Sort.valid() || !query.Selection.GroupBy.valid() ||
		(query.After != nil && (!validMemberRef(query.After.MemberRef) || query.After.SortAt.IsZero() ||
			(query.Selection.GroupBy == queryGroupState && stateGroupRank(query.After.GroupState) == 0) ||
			(query.Selection.GroupBy == queryGroupNone && query.After.GroupState != ""))) {
		return nil, ErrUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	params := productdb.ListSelectedMemberGridMembersParams{
		RowLimit: int32(query.Limit), GroupBy: string(query.Selection.GroupBy), Sort: string(query.Selection.Sort),
		ServiceProductID: query.ProductID, State: string(query.State), Source: string(query.Source),
	}
	if query.After != nil {
		params.HasAfter = true
		params.AfterGroupRank = int32(stateGroupRank(query.After.GroupState))
		params.AfterSortAt = pgtype.Timestamptz{Time: query.After.SortAt.UTC(), Valid: true}
		params.AfterMemberRef = query.After.MemberRef
	}
	rows, err := productdb.New(tx).ListSelectedMemberGridMembers(ctx, params)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	records := make([]MemberRecord, len(rows))
	for index, row := range rows {
		if !row.StartsAt.Valid || !row.UpdatedAt.Valid {
			return nil, ErrUnavailable
		}
		records[index] = MemberRecord{
			MemberRef: row.MemberRef, ServiceProductID: row.ServiceProductID, CustomerID: row.CustomerID,
			State: StateFilter(row.State), Source: SourceFilter(row.Source), StartsAt: row.StartsAt.Time.UTC(),
			ExpiresAt: selectedNullableTime(row.ExpiresAt), ExpiredAt: selectedNullableTime(row.ExpiredAt),
			RemovedAt: selectedNullableTime(row.RemovedAt), Version: row.Version, UpdatedAt: row.UpdatedAt.Time.UTC(), DisplayName: row.DisplayName,
		}
	}
	return records, nil
}

func selectedNullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func (*AccessRepository) CollaboratorPermission(ctx context.Context, productID, staffID int64) (CollaboratorPermission, bool, error) {
	if ctx == nil || productID < 1 || staffID < 1 {
		return "", false, ErrUnavailable
	}
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return "", false, errors.Join(ErrUnavailable, err)
	}
	raw, err := productdb.New(tx).LookupActiveMemberGridCollaboratorPermission(ctx, productdb.LookupActiveMemberGridCollaboratorPermissionParams{
		ServiceProductID: productID,
		StaffID:          staffID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errors.Join(ErrUnavailable, err)
	}
	permission := CollaboratorPermission(raw)
	if !permission.valid() {
		return "", false, ErrUnavailable
	}
	return permission, true, nil
}

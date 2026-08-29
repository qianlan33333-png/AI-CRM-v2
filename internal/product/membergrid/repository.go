package membergrid

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
	productdb "github.com/qianlan33333-png/AI-CRM-v2/internal/product/store/generated"
)

const productExistsSQL = `SELECT EXISTS (
  SELECT 1
  FROM public.products AS p
  WHERE p.id = $1
    AND (
      p.legacy_admin_projection->>'status' = 'service_period_enabled'
      AND p.legacy_admin_projection->>'enabled' = 'true'
      OR p.legacy_admin_projection->>'status' IN (
        'service_period_draft',
        'service_period_disabled',
        'service_period_archived'
      ) AND p.legacy_admin_projection->>'enabled' = 'false'
    )
)`

const memberProjection = `SELECT
  m.member_ref,
  m.service_product_id,
  m.customer_id,
  m.state,
  m.source,
  m.starts_at,
  COALESCE(m.expires_at, TIMESTAMPTZ 'epoch') AS expires_at_value,
  (m.expires_at IS NOT NULL) AS has_expires_at,
  COALESCE(m.expired_at, TIMESTAMPTZ 'epoch') AS expired_at_value,
  (m.expired_at IS NOT NULL) AS has_expired_at,
  COALESCE(m.removed_at, TIMESTAMPTZ 'epoch') AS removed_at_value,
  (m.removed_at IS NOT NULL) AS has_removed_at,
  m.version,
  m.updated_at,
  c.name AS display_name
FROM public.service_period_members AS m
JOIN public.customers AS c ON c.id = m.customer_id`

const firstPageSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT $4`

const afterPageSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
  AND (m.updated_at, m.member_ref) < ($4::timestamptz, $5::text)
ORDER BY m.updated_at DESC, m.member_ref DESC
LIMIT $6`

const firstPageStartsAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
ORDER BY m.starts_at DESC, m.member_ref DESC
LIMIT $4`

const afterPageStartsAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
  AND (m.starts_at, m.member_ref) < ($4::timestamptz, $5::text)
ORDER BY m.starts_at DESC, m.member_ref DESC
LIMIT $6`

const stateGroupOrder = `CASE m.state
  WHEN 'active' THEN 1
  WHEN 'expired' THEN 2
  WHEN 'removed' THEN 3
  ELSE 4
END`

const firstPageStateGroupedUpdatedAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
ORDER BY ` + stateGroupOrder + ` ASC, m.updated_at DESC, m.member_ref DESC
LIMIT $4`

const afterPageStateGroupedUpdatedAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
  AND (` + stateGroupOrder + ` > $4
    OR (` + stateGroupOrder + ` = $4 AND (m.updated_at, m.member_ref) < ($5::timestamptz, $6::text)))
ORDER BY ` + stateGroupOrder + ` ASC, m.updated_at DESC, m.member_ref DESC
LIMIT $7`

const firstPageStateGroupedStartsAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
ORDER BY ` + stateGroupOrder + ` ASC, m.starts_at DESC, m.member_ref DESC
LIMIT $4`

const afterPageStateGroupedStartsAtSQL = memberProjection + `
WHERE m.service_product_id = $1
  AND ($2::text = 'all' OR m.state = $2::text)
  AND ($3::text = '' OR m.source = $3::text)
  AND (` + stateGroupOrder + ` > $4
    OR (` + stateGroupOrder + ` = $4 AND (m.starts_at, m.member_ref) < ($5::timestamptz, $6::text)))
ORDER BY ` + stateGroupOrder + ` ASC, m.starts_at DESC, m.member_ref DESC
LIMIT $7`

type sqlRow interface {
	Scan(...any) error
}

type sqlRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type sqlExecutor interface {
	QueryRow(context.Context, string, ...any) sqlRow
	Query(context.Context, string, ...any) (sqlRows, error)
}

type executorProvider func(context.Context) (sqlExecutor, error)

type shareQueries interface {
	CurrentMemberGridExternalShare(context.Context, int64) (productdb.CurrentMemberGridExternalShareRow, error)
	SetMemberGridExternalShare(context.Context, productdb.SetMemberGridExternalShareParams) (productdb.SetMemberGridExternalShareRow, error)
	LookupEnabledMemberGridExternalShare(context.Context, string) (productdb.LookupEnabledMemberGridExternalShareRow, error)
	SummarizePublicServicePeriodMembers(context.Context, int64) ([]productdb.SummarizePublicServicePeriodMembersRow, error)
	ListPublicServicePeriodMembersFirstPage(context.Context, productdb.ListPublicServicePeriodMembersFirstPageParams) ([]productdb.ListPublicServicePeriodMembersFirstPageRow, error)
	ListPublicServicePeriodMembersAfter(context.Context, productdb.ListPublicServicePeriodMembersAfterParams) ([]productdb.ListPublicServicePeriodMembersAfterRow, error)
}

type shareQueriesProvider func(context.Context) (shareQueries, error)

type Repository struct {
	executor     executorProvider
	shareQueries shareQueriesProvider
}

func NewRepository() *Repository {
	return &Repository{executor: transactionExecutor, shareQueries: transactionShareQueries}
}

func transactionShareQueries(ctx context.Context) (shareQueries, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return productdb.New(tx), nil
}

func transactionExecutor(ctx context.Context) (sqlExecutor, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return pgxExecutor{tx: tx}, nil
}

type pgxExecutor struct {
	tx pgx.Tx
}

func (executor pgxExecutor) QueryRow(ctx context.Context, sql string, arguments ...any) sqlRow {
	return executor.tx.QueryRow(ctx, sql, arguments...)
}

func (executor pgxExecutor) Query(ctx context.Context, sql string, arguments ...any) (sqlRows, error) {
	return executor.tx.Query(ctx, sql, arguments...)
}

func (repository *Repository) ProductExists(ctx context.Context, productID int64) (bool, error) {
	if repository == nil || repository.executor == nil || ctx == nil || productID < 1 {
		return false, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	var exists bool
	if err = executor.QueryRow(ctx, productExistsSQL, productID).Scan(&exists); err != nil {
		return false, errors.Join(ErrUnavailable, err)
	}
	return exists, nil
}

func (repository *Repository) QueryMembers(ctx context.Context, query StoreQuery) ([]MemberRecord, error) {
	if repository == nil || repository.executor == nil || ctx == nil || query.ProductID < 1 ||
		!query.State.validCanonicalGridState() || !query.Source.valid() || query.Limit < 1 || query.Limit > MaximumLimit+1 ||
		(query.After != nil && (!validMemberRef(query.After.MemberRef) || query.After.UpdatedAt.IsZero())) {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	var rows sqlRows
	if query.After == nil {
		rows, err = executor.Query(ctx, firstPageSQL, query.ProductID, string(query.State), string(query.Source), query.Limit)
	} else {
		rows, err = executor.Query(ctx, afterPageSQL, query.ProductID, string(query.State),
			string(query.Source), query.After.UpdatedAt.UTC(), query.After.MemberRef, query.Limit)
	}
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	defer rows.Close()

	members := make([]MemberRecord, 0, query.Limit)
	for rows.Next() {
		var (
			record                                   MemberRecord
			state, source                            string
			expiresValue, expiredValue, removedValue time.Time
			hasExpires, hasExpired, hasRemoved       bool
		)
		if err = rows.Scan(
			&record.MemberRef,
			&record.ServiceProductID,
			&record.CustomerID,
			&state,
			&source,
			&record.StartsAt,
			&expiresValue,
			&hasExpires,
			&expiredValue,
			&hasExpired,
			&removedValue,
			&hasRemoved,
			&record.Version,
			&record.UpdatedAt,
			&record.DisplayName,
		); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		record.State = StateFilter(state)
		record.Source = SourceFilter(source)
		record.StartsAt = record.StartsAt.UTC()
		record.UpdatedAt = record.UpdatedAt.UTC()
		if hasExpires {
			value := expiresValue.UTC()
			record.ExpiresAt = &value
		}
		if hasExpired {
			value := expiredValue.UTC()
			record.ExpiredAt = &value
		}
		if hasRemoved {
			value := removedValue.UTC()
			record.RemovedAt = &value
		}
		members = append(members, record)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return members, nil
}

func (repository *Repository) QuerySelectedMembers(ctx context.Context, query selectedStoreQuery) ([]MemberRecord, error) {
	if repository == nil || repository.executor == nil || ctx == nil || query.ProductID < 1 || !query.State.validCanonicalGridState() ||
		!query.Source.valid() || query.Limit < 1 || query.Limit > MaximumLimit+1 || !query.Selection.Sort.valid() || !query.Selection.GroupBy.valid() ||
		(query.After != nil && (!validMemberRef(query.After.MemberRef) || query.After.SortAt.IsZero() ||
			(query.Selection.GroupBy == queryGroupState && stateGroupRank(query.After.GroupState) == 0) ||
			(query.Selection.GroupBy == queryGroupNone && query.After.GroupState != ""))) {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	querySQL, arguments := selectedPageQuery(query)
	rows, err := executor.Query(ctx, querySQL, arguments...)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	defer rows.Close()

	members := make([]MemberRecord, 0, query.Limit)
	for rows.Next() {
		var (
			record                                   MemberRecord
			state, source                            string
			expiresValue, expiredValue, removedValue time.Time
			hasExpires, hasExpired, hasRemoved       bool
		)
		if err = rows.Scan(
			&record.MemberRef,
			&record.ServiceProductID,
			&record.CustomerID,
			&state,
			&source,
			&record.StartsAt,
			&expiresValue,
			&hasExpires,
			&expiredValue,
			&hasExpired,
			&removedValue,
			&hasRemoved,
			&record.Version,
			&record.UpdatedAt,
			&record.DisplayName,
		); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		record.State = StateFilter(state)
		record.Source = SourceFilter(source)
		record.StartsAt = record.StartsAt.UTC()
		record.UpdatedAt = record.UpdatedAt.UTC()
		if hasExpires {
			value := expiresValue.UTC()
			record.ExpiresAt = &value
		}
		if hasExpired {
			value := expiredValue.UTC()
			record.ExpiredAt = &value
		}
		if hasRemoved {
			value := removedValue.UTC()
			record.RemovedAt = &value
		}
		members = append(members, record)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return members, nil
}

func selectedPageQuery(query selectedStoreQuery) (string, []any) {
	base := []any{query.ProductID, string(query.State), string(query.Source)}
	if query.After == nil {
		base = append(base, query.Limit)
		switch {
		case query.Selection.GroupBy == queryGroupState && query.Selection.Sort == querySortStartsAtDesc:
			return firstPageStateGroupedStartsAtSQL, base
		case query.Selection.GroupBy == queryGroupState:
			return firstPageStateGroupedUpdatedAtSQL, base
		case query.Selection.Sort == querySortStartsAtDesc:
			return firstPageStartsAtSQL, base
		default:
			return firstPageSQL, base
		}
	}
	if query.Selection.GroupBy == queryGroupState {
		base = append(base, stateGroupRank(query.After.GroupState), query.After.SortAt.UTC(), query.After.MemberRef, query.Limit)
		if query.Selection.Sort == querySortStartsAtDesc {
			return afterPageStateGroupedStartsAtSQL, base
		}
		return afterPageStateGroupedUpdatedAtSQL, base
	}
	base = append(base, query.After.SortAt.UTC(), query.After.MemberRef, query.Limit)
	if query.Selection.Sort == querySortStartsAtDesc {
		return afterPageStartsAtSQL, base
	}
	return afterPageSQL, base
}

package membergrid

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const productExistsSQL = `SELECT EXISTS (
  SELECT 1
  FROM public.products AS p
  WHERE p.id = $1
)`

const memberProjection = `SELECT
  e.id,
  e.product_id,
  e.state,
  e.version,
  e.granted_at,
  COALESCE(e.revoked_at, TIMESTAMPTZ 'epoch') AS revoked_at_value,
  (e.revoked_at IS NOT NULL) AS has_revoked_at,
  c.name AS display_name
FROM public.product_local_entitlements AS e
JOIN public.customers AS c ON c.id = e.customer_id`

const firstPageSQL = memberProjection + `
WHERE e.product_id = $1
  AND ($2::text = 'all' OR e.state = $2::text)
ORDER BY e.granted_at DESC, e.id DESC
LIMIT $3`

const afterPageSQL = memberProjection + `
WHERE e.product_id = $1
  AND ($2::text = 'all' OR e.state = $2::text)
  AND (e.granted_at, e.id) < ($3::timestamptz, $4::bigint)
ORDER BY e.granted_at DESC, e.id DESC
LIMIT $5`

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

type Repository struct {
	executor executorProvider
}

func NewRepository() *Repository {
	return &Repository{executor: transactionExecutor}
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
		!query.State.valid() || query.Limit < 1 || query.Limit > MaximumLimit+1 ||
		(query.After != nil && (query.After.EntitlementID < 1 || query.After.GrantedAt.IsZero())) {
		return nil, ErrUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	var rows sqlRows
	if query.After == nil {
		rows, err = executor.Query(ctx, firstPageSQL, query.ProductID, string(query.State), query.Limit)
	} else {
		rows, err = executor.Query(ctx, afterPageSQL, query.ProductID, string(query.State),
			query.After.GrantedAt.UTC(), query.After.EntitlementID, query.Limit)
	}
	if err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	defer rows.Close()

	members := make([]MemberRecord, 0, query.Limit)
	for rows.Next() {
		var (
			record       MemberRecord
			state        string
			revokedValue time.Time
			hasRevoked   bool
		)
		if err = rows.Scan(
			&record.EntitlementID,
			&record.ProductID,
			&state,
			&record.Version,
			&record.GrantedAt,
			&revokedValue,
			&hasRevoked,
			&record.DisplayName,
		); err != nil {
			return nil, errors.Join(ErrUnavailable, err)
		}
		record.State = StateFilter(state)
		record.GrantedAt = record.GrantedAt.UTC()
		if hasRevoked {
			revokedAt := revokedValue.UTC()
			record.RevokedAt = &revokedAt
		}
		// Migration 00005 has no mobile column. Do not inspect customers.extra or
		// identity tables to manufacture a value; masked_mobile stays omitted.
		record.MaskedMobile = nil
		members = append(members, record)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(ErrUnavailable, err)
	}
	return members, nil
}

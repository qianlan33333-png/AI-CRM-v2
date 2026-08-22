package store

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const channelEntrantsChannelStateSQL = `SELECT ch.status
FROM public.channels AS ch
WHERE ch.id = $1`

const channelEntrantsProjectionSQL = `SELECT
  c.id,
  c.channel_id,
  c.name,
  c.added_at,
  c.last_interact_at
FROM public.customers AS c`

const channelEntrantsFirstPageSQL = channelEntrantsProjectionSQL + `
WHERE c.channel_id = $1
  AND c.is_deleted = FALSE
ORDER BY c.added_at DESC, c.id DESC
LIMIT $2`

const channelEntrantsAfterPageSQL = channelEntrantsProjectionSQL + `
WHERE c.channel_id = $1
  AND c.is_deleted = FALSE
  AND (c.added_at, c.id) < ($2::timestamptz, $3::bigint)
ORDER BY c.added_at DESC, c.id DESC
LIMIT $4`

type channelEntrantsSQLRow interface {
	Scan(...any) error
}

type channelEntrantsSQLRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type channelEntrantsSQLExecutor interface {
	QueryRow(context.Context, string, ...any) channelEntrantsSQLRow
	Query(context.Context, string, ...any) (channelEntrantsSQLRows, error)
}

type channelEntrantsExecutorProvider func(context.Context) (channelEntrantsSQLExecutor, error)

type ChannelEntrantsRepository struct {
	executor channelEntrantsExecutorProvider
}

var _ contactapp.ChannelEntrantsStore = (*ChannelEntrantsRepository)(nil)

func NewChannelEntrantsRepository() *ChannelEntrantsRepository {
	return &ChannelEntrantsRepository{executor: channelEntrantsTransactionExecutor}
}

func channelEntrantsTransactionExecutor(ctx context.Context) (channelEntrantsSQLExecutor, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return channelEntrantsPGXExecutor{tx: tx}, nil
}

type channelEntrantsPGXExecutor struct {
	tx pgx.Tx
}

func (executor channelEntrantsPGXExecutor) QueryRow(
	ctx context.Context,
	sql string,
	arguments ...any,
) channelEntrantsSQLRow {
	return executor.tx.QueryRow(ctx, sql, arguments...)
}

func (executor channelEntrantsPGXExecutor) Query(
	ctx context.Context,
	sql string,
	arguments ...any,
) (channelEntrantsSQLRows, error) {
	return executor.tx.Query(ctx, sql, arguments...)
}

func (repository *ChannelEntrantsRepository) ReadChannelEntrantsChannelState(
	ctx context.Context,
	channelID int64,
) (contactapp.ChannelEntrantsChannelState, error) {
	if repository == nil || repository.executor == nil || ctx == nil || channelID < 1 {
		return "", contactapp.ErrChannelEntrantsUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil || channelEntrantsStoreNilDependency(executor) {
		return "", errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
	}
	var state string
	if err = executor.QueryRow(ctx, channelEntrantsChannelStateSQL, channelID).Scan(&state); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", contactapp.ErrChannelEntrantsNotFound
		}
		return "", errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
	}
	return contactapp.ChannelEntrantsChannelState(state), nil
}

func (repository *ChannelEntrantsRepository) ListChannelEntrants(
	ctx context.Context,
	query contactapp.ChannelEntrantsStoreQuery,
) ([]contactapp.ChannelEntrantsRecord, error) {
	if repository == nil || repository.executor == nil || ctx == nil || query.ChannelID < 1 ||
		query.Limit < 1 || query.Limit > contactapp.ChannelEntrantsMaximumLimit+1 ||
		(query.After != nil && (query.After.CustomerID < 1 || query.After.AddedAt.IsZero())) {
		return nil, contactapp.ErrChannelEntrantsUnavailable
	}
	executor, err := repository.executor(ctx)
	if err != nil || channelEntrantsStoreNilDependency(executor) {
		return nil, errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
	}

	var rows channelEntrantsSQLRows
	if query.After == nil {
		rows, err = executor.Query(ctx, channelEntrantsFirstPageSQL, query.ChannelID, query.Limit)
	} else {
		rows, err = executor.Query(
			ctx,
			channelEntrantsAfterPageSQL,
			query.ChannelID,
			query.After.AddedAt.UTC(),
			query.After.CustomerID,
			query.Limit,
		)
	}
	if err != nil || channelEntrantsStoreNilDependency(rows) {
		return nil, errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
	}
	defer rows.Close()

	records := make([]contactapp.ChannelEntrantsRecord, 0, query.Limit)
	for rows.Next() {
		var (
			record       contactapp.ChannelEntrantsRecord
			addedAt      pgtype.Timestamptz
			lastInteract pgtype.Timestamptz
		)
		if err = rows.Scan(
			&record.CustomerID,
			&record.ChannelID,
			&record.DisplayName,
			&addedAt,
			&lastInteract,
		); err != nil {
			return nil, errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
		}
		if addedAt.Valid {
			record.AddedAt = addedAt.Time.UTC()
		}
		if lastInteract.Valid {
			value := lastInteract.Time.UTC()
			record.LastInteractAt = &value
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, errors.Join(contactapp.ErrChannelEntrantsUnavailable, err)
	}
	return records, nil
}

func channelEntrantsStoreNilDependency(value any) bool {
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

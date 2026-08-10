package platformstore

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const maxTransactionAttempts = 3

type txBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

type txContextKey struct{}

type txBinding struct {
	tx     pgx.Tx
	active atomic.Bool
}

// UnitOfWork executes a callback in one PostgreSQL transaction. Retryable
// PostgreSQL failures rerun the whole callback in a fresh transaction.
type UnitOfWork struct {
	db txBeginner
}

var _ platformport.UnitOfWork = (*UnitOfWork)(nil)

func NewUnitOfWork(db txBeginner) *UnitOfWork {
	return &UnitOfWork{db: db}
}

func (uow *UnitOfWork) Within(ctx context.Context, callback func(context.Context) error) error {
	if ctx == nil {
		return errors.New("unit of work context is required")
	}
	if binding, ok := bindingFromContext(ctx); ok {
		if binding.active.Load() {
			return platformport.ErrNestedTransaction
		}
		return platformport.ErrTransactionRequired
	}
	if uow == nil || uow.db == nil {
		return errors.New("unit of work database is required")
	}
	if callback == nil {
		return errors.New("unit of work callback is required")
	}

	var err error
	for attempt := 1; attempt <= maxTransactionAttempts; attempt++ {
		err = uow.withinOnce(ctx, callback)
		if err == nil || !isRetryableTransactionError(err) || ctx.Err() != nil {
			return err
		}
	}
	return err
}

func (uow *UnitOfWork) withinOnce(ctx context.Context, callback func(context.Context) error) (err error) {
	tx, err := uow.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin unit of work: %w", err)
	}

	binding := &txBinding{tx: tx}
	binding.active.Store(true)
	txCtx, cancel := context.WithCancel(context.WithValue(ctx, txContextKey{}, binding))
	defer func() {
		binding.active.Store(false)
		cancel()
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(recovered)
		}
	}()

	if callbackErr := callback(txCtx); callbackErr != nil {
		return rollbackWithError(ctx, tx, callbackErr)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		_ = tx.Rollback(context.WithoutCancel(ctx))
		return fmt.Errorf("commit unit of work: %w", commitErr)
	}
	return nil
}

// TxFromContext returns the transaction only while its UoW callback is active.
func TxFromContext(ctx context.Context) (pgx.Tx, error) {
	binding, ok := bindingFromContext(ctx)
	if !ok || !binding.active.Load() || binding.tx == nil {
		return nil, platformport.ErrTransactionRequired
	}
	return binding.tx, nil
}

func bindingFromContext(ctx context.Context) (*txBinding, bool) {
	if ctx == nil {
		return nil, false
	}
	binding, ok := ctx.Value(txContextKey{}).(*txBinding)
	return binding, ok && binding != nil
}

func rollbackWithError(ctx context.Context, tx pgx.Tx, cause error) error {
	if rollbackErr := tx.Rollback(context.WithoutCancel(ctx)); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
		return errors.Join(cause, fmt.Errorf("rollback unit of work: %w", rollbackErr))
	}
	return cause
}

func isRetryableTransactionError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

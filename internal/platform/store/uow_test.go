package platformstore

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

type fakeBeginner struct {
	txs        []*fakeTx
	beginCalls int
	beginError error
}

func (beginner *fakeBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	beginner.beginCalls++
	if beginner.beginError != nil {
		return nil, beginner.beginError
	}
	tx := &fakeTx{}
	beginner.txs = append(beginner.txs, tx)
	return tx, nil
}

type fakeTx struct {
	commits     int
	rollbacks   int
	commitError error
	rollbackErr error
}

func (*fakeTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("not implemented") }
func (tx *fakeTx) Commit(context.Context) error {
	tx.commits++
	return tx.commitError
}
func (tx *fakeTx) Rollback(context.Context) error {
	tx.rollbacks++
	return tx.rollbackErr
}
func (*fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not implemented")
}
func (*fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (*fakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (*fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not implemented")
}
func (*fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not implemented")
}
func (*fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (*fakeTx) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (*fakeTx) Conn() *pgx.Conn                                  { return nil }

func TestUnitOfWorkCommitAndContextLifetime(t *testing.T) {
	beginner := &fakeBeginner{}
	uow := NewUnitOfWork(beginner)
	var captured context.Context

	err := uow.Within(context.Background(), func(txCtx context.Context) error {
		captured = txCtx
		tx, txErr := TxFromContext(txCtx)
		if txErr != nil {
			t.Fatalf("TxFromContext() error = %v", txErr)
		}
		if tx != beginner.txs[0] {
			t.Fatal("TxFromContext() returned a different transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Within() error = %v", err)
	}
	if beginner.txs[0].commits != 1 || beginner.txs[0].rollbacks != 0 {
		t.Fatalf("transaction calls = commit:%d rollback:%d, want 1/0", beginner.txs[0].commits, beginner.txs[0].rollbacks)
	}
	if _, err := TxFromContext(captured); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("expired TxFromContext() error = %v, want ErrTransactionRequired", err)
	}
	if err := uow.Within(captured, func(context.Context) error { return nil }); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("expired Within() error = %v, want ErrTransactionRequired", err)
	}
}

func TestUnitOfWorkRollbackOnErrorAndPanic(t *testing.T) {
	t.Run("callback error", func(t *testing.T) {
		beginner := &fakeBeginner{}
		uow := NewUnitOfWork(beginner)
		sentinel := errors.New("callback failed")
		if err := uow.Within(context.Background(), func(context.Context) error { return sentinel }); !errors.Is(err, sentinel) {
			t.Fatalf("Within() error = %v, want callback sentinel", err)
		}
		if beginner.txs[0].commits != 0 || beginner.txs[0].rollbacks != 1 {
			t.Fatalf("transaction calls = commit:%d rollback:%d, want 0/1", beginner.txs[0].commits, beginner.txs[0].rollbacks)
		}
	})

	t.Run("panic", func(t *testing.T) {
		beginner := &fakeBeginner{}
		uow := NewUnitOfWork(beginner)
		defer func() {
			if recovered := recover(); recovered != "boom" {
				t.Fatalf("recovered = %v, want boom", recovered)
			}
			if beginner.txs[0].commits != 0 || beginner.txs[0].rollbacks != 1 {
				t.Fatalf("transaction calls = commit:%d rollback:%d, want 0/1", beginner.txs[0].commits, beginner.txs[0].rollbacks)
			}
		}()
		_ = uow.Within(context.Background(), func(context.Context) error { panic("boom") })
	})
}

func TestUnitOfWorkRejectsInvalidAndNestedCalls(t *testing.T) {
	if _, err := TxFromContext(nil); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("nil TxFromContext() error = %v, want ErrTransactionRequired", err)
	}
	if err := NewUnitOfWork(nil).Within(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("Within() with nil database error = nil")
	}
	if err := NewUnitOfWork(&fakeBeginner{}).Within(nil, func(context.Context) error { return nil }); err == nil {
		t.Fatal("Within() with nil context error = nil")
	}
	if err := NewUnitOfWork(&fakeBeginner{}).Within(context.Background(), nil); err == nil {
		t.Fatal("Within() with nil callback error = nil")
	}
	if _, err := TxFromContext(context.Background()); !errors.Is(err, platformport.ErrTransactionRequired) {
		t.Fatalf("plain TxFromContext() error = %v, want ErrTransactionRequired", err)
	}

	beginner := &fakeBeginner{}
	uow := NewUnitOfWork(beginner)
	if err := uow.Within(context.Background(), func(txCtx context.Context) error {
		return uow.Within(txCtx, func(context.Context) error { return nil })
	}); !errors.Is(err, platformport.ErrNestedTransaction) {
		t.Fatalf("nested Within() error = %v, want ErrNestedTransaction", err)
	}
	if beginner.beginCalls != 1 {
		t.Fatalf("BeginTx calls = %d, want 1", beginner.beginCalls)
	}
}

func TestUnitOfWorkTransactionFailures(t *testing.T) {
	t.Run("begin", func(t *testing.T) {
		sentinel := errors.New("begin failed")
		err := NewUnitOfWork(&fakeBeginner{beginError: sentinel}).Within(context.Background(), func(context.Context) error { return nil })
		if !errors.Is(err, sentinel) {
			t.Fatalf("Within() error = %v, want begin sentinel", err)
		}
	})

	t.Run("commit", func(t *testing.T) {
		beginner := &fakeBeginner{}
		sentinel := errors.New("commit failed")
		err := NewUnitOfWork(beginner).Within(context.Background(), func(context.Context) error {
			beginner.txs[0].commitError = sentinel
			return nil
		})
		if !errors.Is(err, sentinel) || beginner.txs[0].rollbacks != 1 {
			t.Fatalf("Within() error/rollbacks = %v/%d, want commit sentinel/1", err, beginner.txs[0].rollbacks)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		beginner := &fakeBeginner{}
		callbackErr := errors.New("callback failed")
		rollbackErr := errors.New("rollback failed")
		err := NewUnitOfWork(beginner).Within(context.Background(), func(context.Context) error {
			beginner.txs[0].rollbackErr = rollbackErr
			return callbackErr
		})
		if !errors.Is(err, callbackErr) || !errors.Is(err, rollbackErr) {
			t.Fatalf("Within() error = %v, want joined callback and rollback errors", err)
		}
	})
}

func TestUnitOfWorkRetriesWholeCallback(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		t.Run(code, func(t *testing.T) {
			beginner := &fakeBeginner{}
			uow := NewUnitOfWork(beginner)
			calls := 0
			err := uow.Within(context.Background(), func(context.Context) error {
				calls++
				if calls < maxTransactionAttempts {
					return &pgconn.PgError{Code: code, Message: "retry"}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("Within() error = %v", err)
			}
			if calls != maxTransactionAttempts || beginner.beginCalls != maxTransactionAttempts {
				t.Fatalf("calls = callback:%d begin:%d, want %d/%d", calls, beginner.beginCalls, maxTransactionAttempts, maxTransactionAttempts)
			}
			for index, tx := range beginner.txs {
				if index < maxTransactionAttempts-1 && (tx.commits != 0 || tx.rollbacks != 1) {
					t.Fatalf("retry tx %d calls = commit:%d rollback:%d, want 0/1", index, tx.commits, tx.rollbacks)
				}
			}
			last := beginner.txs[maxTransactionAttempts-1]
			if last.commits != 1 || last.rollbacks != 0 {
				t.Fatalf("final tx calls = commit:%d rollback:%d, want 1/0", last.commits, last.rollbacks)
			}
		})
	}

	t.Run("stops after bounded attempts", func(t *testing.T) {
		beginner := &fakeBeginner{}
		uow := NewUnitOfWork(beginner)
		sentinel := &pgconn.PgError{Code: "40001", Message: "still conflicting"}
		err := uow.Within(context.Background(), func(context.Context) error { return sentinel })
		if !errors.Is(err, sentinel) {
			t.Fatalf("Within() error = %v, want retry sentinel", err)
		}
		if beginner.beginCalls != maxTransactionAttempts {
			t.Fatalf("BeginTx calls = %d, want bounded %d", beginner.beginCalls, maxTransactionAttempts)
		}
	})
}

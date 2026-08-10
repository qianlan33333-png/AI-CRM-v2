// Package port contains platform contracts that do not import domain packages.
package port

import (
	"context"
	"errors"
)

var (
	ErrTransactionRequired = errors.New("transaction context required")
	ErrNestedTransaction   = errors.New("nested transaction forbidden")
)

// UnitOfWork supplies a transaction-bound context. The callback must not retain
// the context or use it from another goroutine.
type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

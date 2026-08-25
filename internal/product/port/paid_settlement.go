package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrPaidSettlementInvalid     = errors.New("invalid paid settlement")
	ErrPaidSettlementConflict    = errors.New("paid settlement conflict")
	ErrPaidSettlementUnavailable = errors.New("paid settlement unavailable")
)

type PaidSettlementAction string

const (
	PaidSettlementGrant      PaidSettlementAction = "grant"
	PaidSettlementCompensate PaidSettlementAction = "compensate"
)

type PaidSettlementProductKind string

const (
	PaidSettlementOrdinary      PaidSettlementProductKind = "ordinary"
	PaidSettlementServicePeriod PaidSettlementProductKind = "service_period"
)

type PaidSettlementCommand struct {
	Action                  PaidSettlementAction
	ProductKind             PaidSettlementProductKind
	ProductID               ID
	ProductVersion          int64
	OrderID                 int64
	CustomerID              int64
	SettlementReceiptDigest [32]byte
	OccurredAt              time.Time
}

type PaidSettlementResult struct {
	EntitlementID EntitlementID
	MemberRef     string
	State         string
	Version       int64
}

// PaidSettlementWriter runs inside the caller's existing UnitOfWork. It must
// never begin a nested transaction and owns all Product entitlement/member
// writes and their Product events.
type PaidSettlementWriter interface {
	ApplyPaidSettlement(context.Context, PaidSettlementCommand) (PaidSettlementResult, error)
}

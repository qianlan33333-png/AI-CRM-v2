package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidSettlement      = errors.New("invalid order settlement command")
	ErrSettlementConflict     = errors.New("order settlement conflict")
	ErrSettlementNotFound     = errors.New("order settlement not found")
	ErrSettlementUnavailable  = errors.New("order settlement unavailable")
	ErrProviderOutcomeUnknown = errors.New("provider outcome unknown")
)

type ProductKind string

const (
	ProductKindOrdinary      ProductKind = "ordinary"
	ProductKindServicePeriod ProductKind = "service_period"
)

func (kind ProductKind) Valid() bool {
	return kind == ProductKindOrdinary || kind == ProductKindServicePeriod
}

type FinancialState string

const (
	FinancialAwaitingPrepay    FinancialState = "awaiting_prepay"
	FinancialAwaitingPayment   FinancialState = "awaiting_payment"
	FinancialPaid              FinancialState = "paid"
	FinancialPartiallyRefunded FinancialState = "partially_refunded"
	FinancialRefunded          FinancialState = "refunded"
)

type EffectState string

const (
	EffectAccepted       EffectState = "accepted"
	EffectQueued         EffectState = "queued"
	EffectExecuted       EffectState = "executed"
	EffectOutcomeUnknown EffectState = "outcome_unknown"
	EffectReconciled     EffectState = "reconciled"
	EffectFinalFailed    EffectState = "final_failed"
)

type CheckoutCommand struct {
	CustomerID            int64
	ProductID             int64
	ProductKind           ProductKind
	PaymentIdentityDigest [32]byte
	ActorScope            string
	IdempotencyKey        string
}

type Checkout struct {
	OrderID          ID
	MerchantOrderNo  string
	State            FinancialState
	ProductKind      ProductKind
	CustomerID       int64
	ProductID        int64
	AmountMinor      int64
	Currency         string
	PaymentCommandID int64
	CreatedAt        time.Time
}

type PaymentCallbackCommand struct {
	MerchantOrderNo           string
	ProviderEventDigest       [32]byte
	PayloadDigest             [32]byte
	ProviderTransactionDigest [32]byte
	AmountMinor               int64
	Currency                  string
	Succeeded                 bool
	OccurredAt                time.Time
}

type RefundCommandV2 struct {
	OrderID        ID
	AmountMinor    int64
	Reason         string
	Actor          int64
	IdempotencyKey string
}

type RefundCallbackCommand struct {
	OutRefundNo          string
	ProviderEventDigest  [32]byte
	PayloadDigest        [32]byte
	ProviderRefundDigest [32]byte
	AmountMinor          int64
	Currency             string
	Succeeded            bool
	OccurredAt           time.Time
}

type PaymentCommand struct {
	ID                  int64
	OrderID             ID
	SourceRefDigest     [32]byte
	TargetRefDigest     [32]byte
	PayloadDigest       [32]byte
	PolicyVersionDigest [32]byte
	ExternalEffectID    string
	State               EffectState
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type RefundV2 struct {
	ID               int64
	OrderID          ID
	OutRefundNo      string
	AmountMinor      int64
	Currency         string
	ReasonDigest     [32]byte
	SourceRefDigest  [32]byte
	TargetRefDigest  [32]byte
	PayloadDigest    [32]byte
	PolicyDigest     [32]byte
	ExternalEffectID string
	State            EffectState
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SettlementApplication is the complete Order-owned financial boundary. A
// successful callback means authoritative provider settlement, while EER
// executed only means the provider request itself completed.
type SettlementApplication interface {
	Checkout(context.Context, CheckoutCommand) (Checkout, error)
	ApplyPaymentCallback(context.Context, PaymentCallbackCommand) (Checkout, error)
	RequestRefundV2(context.Context, RefundCommandV2) (RefundV2, error)
	ApplyRefundCallback(context.Context, RefundCallbackCommand) (RefundV2, error)
	GetSelfScoped(context.Context, string, [32]byte) (Checkout, error)
}

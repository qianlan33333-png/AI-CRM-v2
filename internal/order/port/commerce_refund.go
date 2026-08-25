package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrCommerceRefundInvalid     = errors.New("invalid commerce refund command")
	ErrCommerceRefundConflict    = errors.New("commerce refund conflict")
	ErrCommerceRefundNotFound    = errors.New("commerce refund not found")
	ErrCommerceRefundUnavailable = errors.New("commerce refund unavailable")
	ErrWeChatShopRefundDisabled  = errors.New("wechat shop refund provider disabled")
)

// WeChatPayRefundCompatibilityCommand is a typed carrier into PE01. Its result
// is RefundV2 and is deliberately not the legacy response contract.
type WeChatPayRefundCompatibilityCommand struct {
	OrderReference            string
	TransactionIDConfirmation string
	AmountMinor               int64
	Reason                    string
	Checked                   bool
	Actor                     int64
	IdempotencyKey            string
}

type WeChatPayRefundCompatibilityApplication interface {
	RequestWeChatPayRefundV2(context.Context, WeChatPayRefundCompatibilityCommand) (RefundV2, error)
}

type WeChatShopRefundState string

const (
	WeChatShopRefundAccepted         WeChatShopRefundState = "accepted"
	WeChatShopRefundExecuting        WeChatShopRefundState = "executing"
	WeChatShopRefundProviderAccepted WeChatShopRefundState = "provider_accepted"
	WeChatShopRefundOutcomeUnknown   WeChatShopRefundState = "outcome_unknown"
	WeChatShopRefundSucceeded        WeChatShopRefundState = "succeeded"
	WeChatShopRefundFinalFailed      WeChatShopRefundState = "final_failed"
)

type WeChatShopRefundCommand struct {
	OrderReference            string
	TransactionIDConfirmation string
	AmountMinor               int64
	Reason                    string
	Checked                   bool
	Actor                     int64
	IdempotencyKey            string
}

type WeChatShopRefund struct {
	ID                       int64
	OrderID                  ID
	MerchantOrderNo          string
	OutRefundNo              string
	AmountMinor              int64
	Currency                 string
	ReasonDigest             [32]byte
	TransactionDigest        [32]byte
	SourceRefDigest          [32]byte
	TargetRefDigest          [32]byte
	PayloadDigest            [32]byte
	PolicyDigest             [32]byte
	ProviderAcceptanceDigest [32]byte
	ProviderRefundDigest     [32]byte
	SettlementDigest         [32]byte
	State                    WeChatShopRefundState
	AttemptCount             int64
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
	SettledAt                time.Time
}

type WeChatShopExecutionJob struct {
	RefundID     int64
	RiverJobID   int64
	RiverAttempt int64
	ArgsDigest   [32]byte
	ScheduledAt  time.Time
}

type WeChatShopRefundRequest struct {
	MerchantOrderNo string
	OutRefundNo     string
	AmountMinor     int64
	Currency        string
	ReasonDigest    [32]byte
}

type WeChatShopProviderCompletion string

const (
	WeChatShopProviderAccepted       WeChatShopProviderCompletion = "provider_accepted"
	WeChatShopProviderOutcomeUnknown WeChatShopProviderCompletion = "outcome_unknown"
	WeChatShopProviderFinalFailed    WeChatShopProviderCompletion = "final_failed"
)

type WeChatShopProviderResult struct {
	Completion     WeChatShopProviderCompletion
	EvidenceDigest [32]byte
}

type WeChatShopRefundQueryResult struct {
	Confirmed            bool
	EvidenceDigest       [32]byte
	ProviderRefundDigest [32]byte
	AmountMinor          int64
	Currency             string
	OccurredAt           time.Time
}

type WeChatShopRefundCallbackCommand struct {
	OutRefundNo          string
	ProviderEventDigest  [32]byte
	PayloadDigest        [32]byte
	ProviderRefundDigest [32]byte
	AmountMinor          int64
	Currency             string
	Succeeded            bool
	OccurredAt           time.Time
}

// WeChatShopRefundProvider is independent from WeChatPayProvider. Enabled is
// false in the production default so accepted commands cannot schedule calls.
type WeChatShopRefundProvider interface {
	Enabled() bool
	RequestRefund(context.Context, WeChatShopRefundRequest) (WeChatShopProviderResult, error)
	QueryRefund(context.Context, string) (WeChatShopRefundQueryResult, error)
}

type WeChatShopRefundCallbackVerifier interface {
	VerifyRefund(context.Context, []byte, map[string]string) (WeChatShopRefundCallbackCommand, error)
}

type WeChatShopRefundApplication interface {
	RequestRefund(context.Context, WeChatShopRefundCommand) (WeChatShopRefund, error)
	ExecuteRefund(context.Context, WeChatShopExecutionJob) (WeChatShopRefund, error)
	ApplyRefundCallback(context.Context, WeChatShopRefundCallbackCommand) (WeChatShopRefund, error)
	ReconcileRefund(context.Context, int64) (WeChatShopRefund, error)
}

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
	ProductID                 string
	SKUID                     string
	Count                     int64
	AmountMinor               int64
	ReasonCode                string
	Reason                    string
	Checked                   bool
	Actor                     int64
	IdempotencyKey            string
}

type WeChatShopRefund struct {
	ID                       int64
	OrderID                  ID
	ContractVersion          string
	MerchantOrderNo          string
	ProviderOrderID          string
	ProductID                string
	SKUID                    string
	RefundCount              int64
	UnitPriceMinor           int64
	ReasonCode               string
	MaterialEvidenceDigest   [32]byte
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
	ProviderAfterSaleID      string
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
	ProviderOrderID string
	ProductID       string
	SKUID           string
	Count           int64
	OutRefundNo     string
	AmountMinor     int64
	Currency        string
	ReasonCode      string
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
	AfterSaleID    string
}

type WeChatShopRefundQueryResult struct {
	EvidenceDigest       [32]byte
	ProviderRefundDigest [32]byte
	AfterSaleID          string
	ProviderOrderID      string
	ProductID            string
	SKUID                string
	Count                int64
	AmountMinor          int64
	Currency             string
	Type                 string
	Status               string
	OccurredAt           time.Time
}

type WeChatShopRefundCallbackCommand struct {
	AfterSaleID         string
	ProviderOrderID     string
	ProviderStatus      string
	ProviderEventDigest [32]byte
	PayloadDigest       [32]byte
	OccurredAt          time.Time
}

// WeChatShopRefundProvider is independent from WeChatPayProvider. Enabled is
// false in the production default so accepted commands cannot schedule calls.
type WeChatShopRefundProvider interface {
	Enabled() bool
	RequestRefund(context.Context, WeChatShopRefundRequest) (WeChatShopProviderResult, error)
	QueryRefund(context.Context, string) (WeChatShopRefundQueryResult, error)
}

type WeChatShopRefundCallbackVerifier interface {
	VerifyURL(context.Context, map[string]string) (string, error)
	VerifyRefund(context.Context, []byte, map[string]string) (WeChatShopRefundCallbackCommand, error)
}

type WeChatShopRefundApplication interface {
	RequestRefund(context.Context, WeChatShopRefundCommand) (WeChatShopRefund, error)
	ExecuteRefund(context.Context, WeChatShopExecutionJob) (WeChatShopRefund, error)
	ApplyRefundCallback(context.Context, WeChatShopRefundCallbackCommand) (WeChatShopRefund, error)
	QueueRefundReconciliation(context.Context, int64) (WeChatShopRefund, error)
	ReconcileRefund(context.Context, int64) (WeChatShopRefund, error)
}

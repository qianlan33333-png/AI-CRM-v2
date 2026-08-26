package port

import (
	"context"
	"errors"
	"time"
)

var ErrWeChatPayDisabled = errors.New("wechat pay provider disabled")

type ExternalEffectKind string

const (
	ExternalEffectPaymentPrepay ExternalEffectKind = "order_payment_prepay"
	ExternalEffectRefund        ExternalEffectKind = "order_refund"
)

type ExternalEffectCommand struct {
	Kind                ExternalEffectKind
	SourceRefDigest     [32]byte
	TargetRefDigest     [32]byte
	PayloadDigest       [32]byte
	PolicyVersionDigest [32]byte
	RiverJobID          int64
	RiverGeneration     int64
	RiverQueue          string
	RiverArgsDigest     [32]byte
	RiverScheduledAt    time.Time
}

type ProviderCompletion string

const (
	ProviderExecuted       ProviderCompletion = "executed"
	ProviderOutcomeUnknown ProviderCompletion = "outcome_unknown"
	ProviderFinalFailed    ProviderCompletion = "final_failed"
)

type ProviderResult struct {
	Completion               ProviderCompletion
	ReceiptDigest            [32]byte
	JSAPIHandoff             *JSAPIHandoff
	BusinessCallDispatched   bool
	RealExternalCallExecuted bool
}

// JSAPIHandoff is the short-lived, client-facing material required by
// WeixinJSBridge.getBrandWCPayRequest. It contains neither payer identity nor
// merchant signing credentials and must only be returned through the
// payment-identity-scoped checkout read.
type JSAPIHandoff struct {
	AppID     string    `json:"appId"`
	TimeStamp string    `json:"timeStamp"`
	NonceStr  string    `json:"nonceStr"`
	Package   string    `json:"package"`
	SignType  string    `json:"signType"`
	PaySign   string    `json:"paySign"`
	ExpiresAt time.Time `json:"-"`
}

type ExternalEffectResult struct {
	EffectID      string
	State         EffectState
	ReceiptDigest [32]byte
}

type ProviderExecution func(context.Context) (ProviderResult, error)

// ExternalEffectRuntime is the narrow Order-facing seam to EER. It owns the
// accepted/queued/attempted fence around ProviderExecution; Order owns the
// resulting financial command and benefit state.
type ExternalEffectRuntime interface {
	Execute(context.Context, ExternalEffectCommand, ProviderExecution) (ExternalEffectResult, error)
	Reconcile(context.Context, string, [32]byte) (ExternalEffectResult, error)
}

type PrepayRequest struct {
	MerchantOrderNo      string
	AmountMinor          int64
	Currency             string
	ProductSnapshot      string
	PayerIdentityDigest  [32]byte
	ProviderNotifyTarget [32]byte
}

type RefundRequest struct {
	MerchantOrderNo string
	OutRefundNo     string
	AmountMinor     int64
	Currency        string
	ReasonDigest    [32]byte
}

type PaymentQueryResult struct {
	Confirmed                 bool
	EvidenceDigest            [32]byte
	ProviderTransactionDigest [32]byte
	AmountMinor               int64
	Currency                  string
	OccurredAt                time.Time
}

type RefundQueryResult struct {
	Confirmed            bool
	EvidenceDigest       [32]byte
	ProviderRefundDigest [32]byte
	AmountMinor          int64
	Currency             string
	OccurredAt           time.Time
}

// WeChatPayProvider is provider-shaped and closed to the two PE01 effects.
// Implementations return receipt digests plus the short-lived JSAPI prepay
// handoff; they never assert that a payment is paid. Paid and refunded
// financial facts enter through verified callbacks or active-query
// reconciliation.
type WeChatPayProvider interface {
	CreatePrepay(context.Context, PrepayRequest) (ProviderResult, error)
	RequestRefund(context.Context, RefundRequest) (ProviderResult, error)
	QueryPayment(context.Context, string) (PaymentQueryResult, error)
	QueryRefund(context.Context, string) (RefundQueryResult, error)
}

type CallbackVerifier interface {
	VerifyPayment(context.Context, []byte, map[string]string) (PaymentCallbackCommand, error)
	VerifyRefund(context.Context, []byte, map[string]string) (RefundCallbackCommand, error)
}

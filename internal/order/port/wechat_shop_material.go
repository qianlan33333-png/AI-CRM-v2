package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrWeChatShopMaterialInvalid     = errors.New("invalid wechat shop order material")
	ErrWeChatShopMaterialConflict    = errors.New("wechat shop order material conflict")
	ErrWeChatShopMaterialNotFound    = errors.New("wechat shop order material not found")
	ErrWeChatShopMaterialUnavailable = errors.New("wechat shop order material unavailable")
)

type WeChatShopMaterialSource string

const (
	WeChatShopMaterialProvider  WeChatShopMaterialSource = "provider"
	WeChatShopMaterialLegacyRaw WeChatShopMaterialSource = "legacy_raw"
)

type WeChatShopMaterialReadiness string

const (
	WeChatShopMaterialReady                 WeChatShopMaterialReadiness = "ready"
	WeChatShopMaterialProviderSyncRequired  WeChatShopMaterialReadiness = "provider_sync_required"
	WeChatShopMaterialOrderNotPaid          WeChatShopMaterialReadiness = "order_not_paid"
	WeChatShopMaterialAfterSaleEvidenceMiss WeChatShopMaterialReadiness = "aftersale_evidence_missing"
	WeChatShopMaterialNoRefundableLine      WeChatShopMaterialReadiness = "no_refundable_line"
)

type WeChatShopLineReadiness string

const (
	WeChatShopLineReady                 WeChatShopLineReadiness = "ready"
	WeChatShopLineAfterSaleEvidenceMiss WeChatShopLineReadiness = "aftersale_evidence_missing"
	WeChatShopLineNoRemainingCount      WeChatShopLineReadiness = "no_remaining_count"
)

// WeChatShopOrderLine contains only the Provider identifiers and counts needed
// to construct a later aftersale request. Titles, buyer details and delivery
// details are deliberately excluded.
type WeChatShopOrderLine struct {
	Position                int
	ProductID               string
	SKUID                   string
	SKUCount                int64
	OnAfterSaleSKUCount     int64
	FinishAfterSaleSKUCount int64
	RealPriceMinor          int64
	RemainingSKUCount       int64
	AfterSaleEvidenceExact  bool
	Readiness               WeChatShopLineReadiness
}

// WeChatShopOrderMaterial is a PII-free, typed projection of order/get. A
// legacy_raw value is never refund-ready until replaced by a Provider sync.
type WeChatShopOrderMaterial struct {
	ProviderOrderID   string
	StatusCode        int64
	DealRecorded      bool
	AmountMinor       int64
	Currency          string
	TransactionDigest [32]byte
	EvidenceDigest    [32]byte
	SourceKeyDigest   [32]byte
	Source            WeChatShopMaterialSource
	Readiness         WeChatShopMaterialReadiness
	ProviderVerified  bool
	CreatedAt         time.Time
	PaidAt            time.Time
	UpdatedAt         time.Time
	SyncedAt          time.Time
	Lines             []WeChatShopOrderLine
}

type WeChatShopOrderMaterialProvider interface {
	GetOrder(context.Context, string) (WeChatShopOrderMaterial, error)
}

type WeChatShopOrderMaterialApplication interface {
	SyncOrder(context.Context, string) (WeChatShopOrderMaterial, error)
	GetOrderMaterial(context.Context, string) (WeChatShopOrderMaterial, error)
}

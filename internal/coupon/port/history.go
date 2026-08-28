package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrHistoryInvalid     = errors.New("invalid coupon history")
	ErrHistoryConflict    = errors.New("coupon history conflict")
	ErrHistoryUnavailable = errors.New("coupon history unavailable")
)

// Historical records are source facts, never currently claimable benefits.
type HistoricalDefinition struct {
	Coupon
	SourceCouponID int64      `json:"source_coupon_id"`
	OriginalStatus string     `json:"original_status"`
	FirstClaimAt   *time.Time `json:"first_claim_at"`
}

type HistoricalClaim struct {
	ID                  int64      `json:"id"`
	SourceClaimID       int64      `json:"source_claim_id"`
	SourceCouponID      int64      `json:"source_coupon_id"`
	CouponID            int64      `json:"coupon_id"`
	CustomerID          *int64     `json:"customer_id"`
	ClaimNo             string     `json:"claim_no"`
	Status              string     `json:"status"`
	DiscountAmountTotal int64      `json:"discount_amount_total"`
	Currency            string     `json:"currency"`
	ValidFrom           time.Time  `json:"valid_from"`
	ValidUntil          time.Time  `json:"valid_until"`
	ClaimedAt           time.Time  `json:"claimed_at"`
	ReservedAt          *time.Time `json:"reserved_at"`
	ConsumedAt          *time.Time `json:"consumed_at"`
	ExpiredAt           *time.Time `json:"expired_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type HistoricalRedemption struct {
	ID                  int64      `json:"id"`
	SourceRedemptionID  int64      `json:"source_redemption_id"`
	SourceClaimID       int64      `json:"source_claim_id"`
	SourceOrderID       int64      `json:"source_order_id"`
	ClaimHistoryID      int64      `json:"claim_history_id"`
	OrderID             *int64     `json:"order_id"`
	OutTradeNo          string     `json:"out_trade_no"`
	Status              string     `json:"status"`
	OriginalAmountTotal int64      `json:"original_amount_total"`
	DiscountAmountTotal int64      `json:"discount_amount_total"`
	PayableAmountTotal  int64      `json:"payable_amount_total"`
	Currency            string     `json:"currency"`
	ReservedUntil       time.Time  `json:"reserved_until"`
	ReleaseReason       string     `json:"release_reason"`
	ReservedAt          time.Time  `json:"reserved_at"`
	ConsumedAt          *time.Time `json:"consumed_at"`
	ReleasedAt          *time.Time `json:"released_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type HistoricalReceipt struct {
	SourceIdentifier string
	PayloadDigest    [32]byte
	TargetID         int64
	TargetDigest     [32]byte
	Replayed         bool
}

// Writes and the scoped journal share the caller's transaction.
type HistoricalStore interface {
	CreateHistoricalDefinition(context.Context, HistoricalDefinition) (HistoricalDefinition, error)
	GetHistoricalDefinition(context.Context, int64) (HistoricalDefinition, error)
	CreateHistoricalClaim(context.Context, HistoricalClaim) (HistoricalClaim, error)
	GetHistoricalClaim(context.Context, int64) (HistoricalClaim, error)
	CreateHistoricalRedemption(context.Context, HistoricalRedemption) (HistoricalRedemption, error)
	GetHistoricalRedemption(context.Context, int64) (HistoricalRedemption, error)
}

// Kind is exactly definitions, claims, or redemptions.
type HistoricalJournal interface {
	LoadHistoricalCoupon(context.Context, string, string) (HistoricalReceipt, bool, error)
	RecordHistoricalCoupon(context.Context, string, HistoricalReceipt) error
}

type HistoricalReader interface {
	ListHistoricalDefinitions(context.Context, int32, int32) ([]HistoricalDefinition, int64, error)
	ListHistoricalClaims(context.Context, int64, int32, int32) ([]HistoricalClaim, int64, error)
	ListHistoricalRedemptions(context.Context, int64, int32, int32) ([]HistoricalRedemption, int64, error)
}

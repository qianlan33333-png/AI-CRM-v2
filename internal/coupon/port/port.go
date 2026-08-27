package port

import "time"

type ID int64

type ValidityMode string

const (
	ValidityFixedRange   ValidityMode = "fixed_range"
	ValidityRelativeDays ValidityMode = "relative_days"
)

type Coupon struct {
	ID                   ID           `json:"id"`
	Name                 string       `json:"name"`
	DiscountAmountTotal  int64        `json:"discount_amount_total"`
	Currency             string       `json:"currency"`
	Status               string       `json:"status"`
	AvailabilityStatus   string       `json:"availability_status"`
	TotalIssueLimit      int64        `json:"total_issue_limit"`
	PerUserIssueLimit    int64        `json:"per_user_issue_limit"`
	IssuedCount          int64        `json:"issued_count"`
	ClaimStartsAt        time.Time    `json:"claim_starts_at"`
	ClaimEndsAt          time.Time    `json:"claim_ends_at"`
	ValidityMode         ValidityMode `json:"validity_mode"`
	UseStartsAt          *time.Time   `json:"use_starts_at"`
	UseEndsAt            *time.Time   `json:"use_ends_at"`
	RelativeValidityDays *int32       `json:"relative_validity_days"`
	Instructions         string       `json:"instructions"`
	TargetRefs           []string     `json:"target_refs"`
	CreatedBy            int64        `json:"created_by"`
	UpdatedBy            int64        `json:"updated_by"`
	Version              int64        `json:"version"`
	HistoryOnly          bool         `json:"history_only,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

type UpsertCommand struct {
	Coupon
	Actor          int64
	IdempotencyKey string
}

type Page struct {
	Items  []Coupon `json:"items"`
	Total  int64    `json:"total"`
	Limit  int32    `json:"limit"`
	Offset int32    `json:"offset"`
}

// Claim is a durable local coupon fact. CustomerID is internal-only; legacy
// transports expose the opaque ClaimRef instead of a customer identifier.
type Claim struct {
	ID, CouponID, CustomerID int64
	ClaimNumber              int32
	ClaimRef, Status         string
	ClaimedAt                time.Time
}

type ClaimPage struct {
	Items  []Claim `json:"items"`
	Total  int64   `json:"total"`
	Limit  int32   `json:"limit"`
	Offset int32   `json:"offset"`
}

type ClaimCommand struct {
	CouponID       ID
	CustomerID     int64
	IdempotencyKey string
}

// SidebarCoupon is an owner-scoped projection. It intentionally carries no
// customer identifier: the sidebar grant has already selected the owner.
type SidebarCoupon struct {
	CouponID     ID        `json:"coupon_id"`
	CouponName   string    `json:"coupon_name"`
	ClaimRef     string    `json:"claim_ref"`
	ClaimedAt    time.Time `json:"claimed_at"`
	CouponStatus string    `json:"coupon_status"`
}

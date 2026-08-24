// Package port exposes the Order compatibility contract. It records local
// exports, refund intents, and external-effect review, but never provider calls.
package port

import (
	"context"
	"errors"
	"time"
)

type ID int64

var (
	ErrPaidOrderReadNotFound    = errors.New("paid order projection not found")
	ErrPaidOrderReadUnavailable = errors.New("paid order projection unavailable")
)

// PaidOrderProjection is the minimum local fact a Product-owned entitlement
// grant may consume. It never exposes payment credentials, payer PII, or a
// provider receipt.
type PaidOrderProjection struct {
	ID         ID
	ProductID  int64
	CustomerID int64
}

type PaidOrderReader interface {
	ReadPaidOrder(context.Context, ID) (PaidOrderProjection, error)
}

type Filter struct {
	Provider, OrderNo, Mobile, ProductCode, Status string
	CustomerID                                     *int64
	CreatedFrom, CreatedTo                         *time.Time
	Limit, Offset                                  int32
}

type Record struct {
	ID                    ID
	Provider              string
	ProviderLabel         string
	MerchantOrderNo       string
	PlatformTransactionNo string
	CustomerID            *int64
	PayerNameSnapshot     string
	MobileSnapshot        string
	IdentityKind          string
	IdentityValue         string
	ProductID             *int64
	ProductCode           string
	ProductNameSnapshot   string
	AmountMinor           int64
	Currency              string
	Status                string
	StatusLabel           string
	DetailURL             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Item struct {
	CreatedAt             time.Time `json:"created_at"`
	MerchantOrderNo       string    `json:"merchant_order_no"`
	OutTradeNo            string    `json:"out_trade_no"`
	OrderNo               string    `json:"order_no"`
	PlatformTransactionNo string    `json:"platform_transaction_no"`
	TransactionID         string    `json:"transaction_id"`
	PayerName             string    `json:"payer_name"`
	Mobile                string    `json:"mobile"`
	UserID                string    `json:"userid,omitempty"`
	ExternalUserID        string    `json:"external_userid,omitempty"`
	UnionID               string    `json:"unionid,omitempty"`
	ProductCode           string    `json:"product_code"`
	ProductName           string    `json:"product_name"`
	AmountYuan            string    `json:"amount_yuan"`
	Currency              string    `json:"currency"`
	Status                string    `json:"status"`
	StatusLabel           string    `json:"status_label"`
	Provider              string    `json:"provider"`
	ProviderLabel         string    `json:"provider_label"`
	DetailURL             string    `json:"detail_url"`
}

type Page struct {
	Items   []Item `json:"items"`
	Total   int64  `json:"total"`
	Limit   int32  `json:"limit"`
	HasMore bool   `json:"has_more"`
}

type Query interface {
	List(context.Context, Filter) (Page, error)
}

// BoardFilter is the frozen compatibility filter shared by the unified order
// and provider transaction routes. All fields are optional unless documented
// by the route that uses the filter.
type BoardFilter struct {
	Provider, Status, ProductCode, Mobile, Identity, TransactionID, OrderNo string
	CreatedFrom, CreatedTo                                                  *time.Time
	Limit, Offset                                                           int32
}

type Detail struct {
	Item
	ID                    ID    `json:"id"`
	RefundableAmountMinor int64 `json:"refundable_amount_total"`
}

type RefundFilter struct {
	Provider, OrderNo, TransactionID, RefundID, OutRefundNo, Status string
	CreatedFrom, CreatedTo                                          *time.Time
	Limit, Offset                                                   int32
}

type Refund struct {
	ID                  int64     `json:"id"`
	OrderID             ID        `json:"order_id"`
	Provider            string    `json:"provider"`
	OrderNo             string    `json:"order_no"`
	TransactionID       string    `json:"transaction_id"`
	RefundID            string    `json:"refund_id"`
	OutRefundNo         string    `json:"out_refund_no"`
	RefundAmountTotal   int64     `json:"refund_amount_total"`
	Currency            string    `json:"currency"`
	Reason              string    `json:"reason"`
	Status              string    `json:"status"`
	ExternalEffectID    int64     `json:"external_effect_id"`
	ExternalEffectState string    `json:"external_effect_state"`
	AutoRetryAllowed    bool      `json:"auto_retry_allowed"`
	CreatedAt           time.Time `json:"created_at"`
}

type RefundPage struct {
	Items   []Refund `json:"items"`
	Total   int64    `json:"total"`
	Limit   int32    `json:"limit"`
	HasMore bool     `json:"has_more"`
}

type ExportJob struct {
	JobID       string    `json:"job_id"`
	Resource    string    `json:"resource"`
	Format      string    `json:"format"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	Operator    int64     `json:"operator"`
	DownloadURL string    `json:"download_url"`
	ContentType string    `json:"content_type,omitempty"`
	FileName    string    `json:"file_name,omitempty"`
	ContentText string    `json:"content_text,omitempty"`
}

// ExportFilter deliberately accepts only locally held, non-identity facts.
// It is shared by the preview and the durable CSV export so their projections
// cannot drift.
type ExportFilter struct {
	Provider    string     `json:"provider,omitempty"`
	Status      string     `json:"status,omitempty"`
	ProductCode string     `json:"product_code,omitempty"`
	LocalID     *int64     `json:"local_id,omitempty"`
	CreatedFrom *time.Time `json:"created_from,omitempty"`
	CreatedTo   *time.Time `json:"created_to,omitempty"`
}

// ExportPreview is read-only. It has no job, receipt, event, provider result,
// or download URL because it never creates an external or durable effect.
type ExportPreview struct {
	Resource    string `json:"resource"`
	Format      string `json:"format"`
	Total       int64  `json:"total"`
	Truncated   bool   `json:"truncated"`
	ContentText string `json:"content_text"`
}

type ExternalEffect struct {
	ID                    int64     `json:"id"`
	OrderID               ID        `json:"order_id"`
	Provider              string    `json:"provider"`
	EffectKind            string    `json:"effect_kind"`
	State                 string    `json:"state"`
	AutoRetryAllowed      bool      `json:"auto_retry_allowed"`
	ProviderReceipt       []byte    `json:"provider_receipt,omitempty"`
	ManualReviewRequested time.Time `json:"manual_review_requested_at,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type ExternalEffectPage struct {
	Items []ExternalEffect `json:"items"`
	Total int64            `json:"total"`
}

type ExportCommand struct {
	Resource, Format, IdempotencyKey string
	Filter                           ExportFilter
	Actor                            int64
}

type RefundCommand struct {
	Provider, OrderReference, TransactionIDConfirmation, Reason, IdempotencyKey string
	RefundAmountTotal                                                           int64
	Checked                                                                     bool
	Actor                                                                       int64
}

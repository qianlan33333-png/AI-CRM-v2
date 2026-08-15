// Package port exposes the read-only Order list contract. It does not expose
// order creation, payment, cancellation, refund, callback, or provider calls.
package port

import (
	"context"
	"time"
)

type ID int64

type Filter struct {
	Provider, OrderNo, Mobile, ProductCode, Status string
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

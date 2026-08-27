package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrHistoricalInput       = errors.New("invalid historical order fact")
	ErrHistoricalConflict    = errors.New("historical order target conflict")
	ErrHistoricalUnavailable = errors.New("historical order import unavailable")
)

type HistoricalFact struct {
	SourceKeyDigest [32]byte
	PayloadDigest   [32]byte
	FieldDigest     [32]byte
}

type HistoricalRefund struct {
	ID               int64     `json:"id"`
	OrderID          ID        `json:"order_id"`
	SourceRefundID   int64     `json:"source_refund_id"`
	RefundNumber     string    `json:"refund_number"`
	ProviderRefundID string    `json:"provider_refund_id"`
	TransactionID    string    `json:"transaction_id"`
	Status           string    `json:"status"`
	AmountMinor      int64     `json:"amount_minor"`
	OrderAmountMinor int64     `json:"order_amount_minor"`
	Currency         string    `json:"currency"`
	Reason           string    `json:"reason"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type HistoricalOrderRecord struct {
	Fact  HistoricalFact
	Order Record
}
type HistoricalRefundRecord struct {
	Fact   HistoricalFact
	Refund HistoricalRefund
}

type HistoricalImportReceipt struct {
	HistoricalFact
	TargetID     int64
	TargetDigest [32]byte
}

// These writes preserve historical facts only. They cannot create payment,
// refund, entitlement, event, or Provider execution records.
type HistoricalImportStore interface {
	CreateHistoricalOrder(context.Context, Record) (Record, error)
	GetHistoricalOrder(context.Context, ID) (Record, error)
	CreateHistoricalRefund(context.Context, HistoricalRefund) (HistoricalRefund, error)
	GetHistoricalRefund(context.Context, int64) (HistoricalRefund, error)
}

type HistoricalImportJournal interface {
	FindHistoricalOrderReceipt(context.Context, string, [32]byte) (HistoricalImportReceipt, bool, error)
	AppendHistoricalOrderReceipt(context.Context, string, HistoricalImportReceipt) error
}

type HistoricalRefundReader interface {
	ListHistoricalRefunds(context.Context, ID) ([]HistoricalRefund, error)
}

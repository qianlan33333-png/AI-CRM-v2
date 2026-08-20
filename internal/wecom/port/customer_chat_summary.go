// Package port freezes narrow cross-domain WeCom read contracts. It contains
// no provider client, message bodies, media URLs, or external identities.
package port

import (
	"context"
	"errors"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var (
	ErrInvalidCustomerChatSummaryQuery = errors.New("invalid customer chat summary query")
	ErrCustomerChatSummaryUnavailable  = errors.New("customer chat summary unavailable")
)

type CustomerChatSummaryQuery struct {
	CustomerID contactport.CustomerID
	ChatType   string
	Limit      int32
	Offset     int32
}

// CustomerChatSummary is deliberately a zero-body local archive projection.
// It excludes message text, senders, recipients, provider IDs, media, and all
// external delivery or receipt claims.
type CustomerChatSummary struct {
	ChatType    string
	MessageType string
	SentAt      time.Time
}

type CustomerChatSummaryPage struct {
	Items  []CustomerChatSummary
	Total  int64
	Limit  int32
	Offset int32
}

type CustomerChatSummaryReader interface {
	ListCustomerChatSummaries(context.Context, CustomerChatSummaryQuery) (CustomerChatSummaryPage, error)
}

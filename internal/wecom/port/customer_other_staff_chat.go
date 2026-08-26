package port

import (
	"context"
	"errors"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var ErrCustomerOtherStaffChatUnavailable = errors.New("customer other staff chat unavailable")

// CustomerOtherStaffChatReader is the deliberately narrow, local-only archive
// projection used by the Sidebar. It never starts a provider sync or exposes
// source/provider identifiers, participants, or receipts.
type CustomerOtherStaffChatReader interface {
	ListCustomerOtherStaffChats(context.Context, CustomerOtherStaffChatQuery) (CustomerOtherStaffChatPage, error)
}

type CustomerOtherStaffChatQuery struct {
	CustomerID   contactport.CustomerID
	OwnerStaffID int64
}

type CustomerOtherStaffChat struct {
	StaffUserID   string
	MessageType   string
	ContentMasked string
	SentAt        time.Time
}

type CustomerOtherStaffChatPage struct {
	Items []CustomerOtherStaffChat
}

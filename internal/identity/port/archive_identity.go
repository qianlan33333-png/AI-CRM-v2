package port

import (
	"context"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

// ArchiveIdentityTarget imports only sealed external identities absent from DM01.
// It is transaction-bound, never creates a Customer, and emits no live event.
// A non-nil CustomerID must already be locked and proved by Contact in that transaction.
type ArchiveIdentityTarget interface {
	ImportArchiveWeComIdentity(context.Context, ArchiveIdentityInput) (ArchiveIdentityFact, error)
	ReadArchiveWeComIdentity(context.Context, int64) (ArchiveIdentityFact, error)
}

type ArchiveIdentityInput struct {
	CustomerID     *contactport.CustomerID `json:"-"`
	Scope          string                  `json:"-"`
	ExternalUserID string                  `json:"-"`
	SourceKeyHMAC  [32]byte                `json:"-"`
	HMACKeyVersion int16                   `json:"-"`
}

// Source credentials and identifiers have no public JSON representation.
type ArchiveIdentityFact struct {
	CustomerID        *contactport.CustomerID `json:"-"`
	Scope             string                  `json:"-"`
	ExternalUserID    string                  `json:"-"`
	HMACKeyVersion    int16                   `json:"-"`
	ID                int64                   `json:"-"`
	Assurance         string                  `json:"-"`
	Source            string                  `json:"-"`
	NormalizerVersion int16                   `json:"-"`
	ReviewFingerprint [16]byte                `json:"-"`
	CreatedAt         time.Time               `json:"-"`
	BoundAt           *time.Time              `json:"-"`
}

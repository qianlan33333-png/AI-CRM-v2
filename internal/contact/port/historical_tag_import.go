package port

import (
	"context"
	"errors"
	"time"
)

var (
	ErrHistoricalTagInput       = errors.New("invalid historical tag import")
	ErrHistoricalTagConflict    = errors.New("historical tag import conflict")
	ErrHistoricalTagBlocked     = errors.New("historical tag import blocked")
	ErrHistoricalTagUnavailable = errors.New("historical tag import unavailable")
)

// HistoricalTagSource is deliberately narrower than the generic migration
// harness. The main-line journal owns durable source receipts and runs it in
// the same UnitOfWork as this Contact-owned writer.
type HistoricalTagSource string

const (
	HistoricalTagGroupSource      HistoricalTagSource = "v1.wecom_corp_tag_groups"
	HistoricalTagCatalogTagSource HistoricalTagSource = "v1.wecom_corp_tags"
	HistoricalCustomerTagSource   HistoricalTagSource = "v1.contact_tags"
)

// HistoricalTagFact contains only main-line supplied digests. Raw V1 rows and
// identities are never persisted by this leaf.
type HistoricalTagFact struct {
	SourceKeyDigest [32]byte
	PayloadDigest   [32]byte
	FieldDigest     [32]byte
}

type HistoricalTagLineage struct {
	TargetID      int64
	TargetDigest  [32]byte
	PayloadDigest [32]byte
	FieldDigest   [32]byte
}

// HistoricalTagJournal is implemented by the main migration receipt journal.
// It provides the missing durable V1 group/source lineage without adding a
// second migration platform or a schema change in this leaf.
type HistoricalTagJournal interface {
	FindHistoricalTagLineage(context.Context, HistoricalTagSource, [32]byte) (HistoricalTagLineage, bool, error)
	AppendHistoricalTagLineage(context.Context, HistoricalTagSource, HistoricalTagFact, HistoricalTagLineage) error
}

type HistoricalTagGroupRecord struct {
	Fact      HistoricalTagFact
	Name      string
	SortOrder int32
}

type HistoricalTagRecord struct {
	Fact                 HistoricalTagFact
	GroupSourceKeyDigest [32]byte
	ProviderTagID        string
	Name                 string
	SortOrder            int32
}

// HistoricalCustomerTagRecord intentionally has no V1 userid field: it is a
// staff/owner value, not an external customer identity. The caller must supply
// a DM01-verified customer target for this union ID.
type HistoricalCustomerTagRecord struct {
	Fact               HistoricalTagFact
	UnionID            string
	VerifiedCustomerID CustomerID
	ProviderTagID      string
	TaggedAt           time.Time
}

type HistoricalTagGroup struct {
	ID        int64
	Name      string
	SortOrder int32
}

type HistoricalTag struct {
	ID            int64
	GroupID       int64
	ProviderTagID string
	Name          string
	SortOrder     int32
}

type HistoricalCustomerTag struct {
	CustomerID CustomerID
	TagID      int64
	TaggedAt   time.Time
	TaggedBy   string
}

// HistoricalTagStore is Contact-owned and transaction-bound. Its methods
// only mutate tag_groups, tags, and customer_tags; no event, Provider, EER, or
// River capability crosses this boundary.
type HistoricalTagStore interface {
	GetHistoricalTagGroup(context.Context, int64) (HistoricalTagGroup, error)
	CreateHistoricalTagGroup(context.Context, HistoricalTagGroup) (HistoricalTagGroup, error)
	GetHistoricalTag(context.Context, int64) (HistoricalTag, error)
	FindHistoricalTagByProviderID(context.Context, string) (HistoricalTag, bool, error)
	CreateHistoricalTag(context.Context, HistoricalTag) (HistoricalTag, bool, error)
	GetHistoricalCustomerTag(context.Context, CustomerID, int64) (HistoricalCustomerTag, bool, error)
	BindHistoricalCustomerTag(context.Context, HistoricalCustomerTag) (HistoricalCustomerTag, bool, error)
}

// HistoricalTagCustomerVerifier is supplied by main-line composition. It must
// validate the supplied target against the existing DM01 exact unionid mapping;
// a caller cannot turn a V1 staff userid into a customer target through this
// interface.
type HistoricalTagCustomerVerifier interface {
	VerifyHistoricalTagCustomer(context.Context, string, CustomerID) error
}

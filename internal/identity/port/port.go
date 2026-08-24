// Package port freezes identity resolution and attribution semantics.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

var ErrHistoricalScopedIdentityConflict = errors.New("historical scoped identity conflict")

type IDKind string
type Assurance string

const (
	KindWeComExternalUserID IDKind = "wecom_external_userid"
	KindUnionID             IDKind = "unionid"
	KindMPOpenID            IDKind = "mp_openid"
	KindOAOpenID            IDKind = "oa_openid"
	KindAlipayUserID        IDKind = "alipay_user_id"
	KindPhone               IDKind = "phone"
	KindExtension           IDKind = "ext"

	AssuranceVerified Assurance = "verified"
	AssuranceDeclared Assurance = "declared"
)

const MergePolicyVerifiedUnionIDUniqueWeCom = "verified_unionid_unique_wecom_v1"

type IDRef struct {
	Kind      IDKind
	Scope     string
	Value     string
	Assurance Assurance
	Source    string
}

// HistoricalScopedIdentityBinder is a transaction-bound DM01-only port. It
// supports exactly one scoped WeCom identity binding and cannot merge, emit an
// event, invoke a Provider, or expose arbitrary identity SQL.
type HistoricalScopedIdentityBinder interface {
	BindHistoricalScopedWeComIdentity(context.Context, HistoricalScopedIdentity) (HistoricalScopedIdentityResult, error)
}

type HistoricalScopedIdentity struct {
	CustomerID     contactport.CustomerID
	Scope          string
	ExternalUserID string
	SourceKeyHMAC  []byte
	HMACKeyVersion int16
}

type HistoricalScopedIdentityResult struct {
	IdentityID int64
	Bound      bool
}

type ResolveStatus string

const (
	ResolveFound    ResolveStatus = "found"
	ResolveNotFound ResolveStatus = "not_found"
	ResolveConflict ResolveStatus = "conflict"
)

type ResolveResult struct {
	Status     ResolveStatus
	CustomerID contactport.CustomerID
}

// TrustedWeComExternalIdentity is the unique verified reverse projection for
// one canonical OneID. Callers must not persist or expose it outside their
// already-authorized local response boundary.
type TrustedWeComExternalIdentity struct {
	CustomerID     contactport.CustomerID
	ExternalUserID string
}

const MaximumTrustedWeComIdentityCustomerIDs = 200

// TrustedWeComIdentityReader is transaction-bound: callers must supply the
// active outer UnitOfWork context. Missing, unverified, or ambiguous values are
// omitted from the result. Each call accepts at most 200 canonical OneIDs.
type TrustedWeComIdentityReader interface {
	ListPrimaryWeComExternalUserIDs(context.Context, []contactport.CustomerID) ([]TrustedWeComExternalIdentity, error)
}

// CustomerMatchRequest keeps raw identity hints inside the local matching
// boundary. Callers receive only a boolean OneID match and must never expose
// the refs or the unscoped legacy unionid in an HTTP projection.
type CustomerMatchRequest struct {
	CustomerID    contactport.CustomerID
	Refs          []IDRef
	LegacyUnionID string
}

type CustomerMatcher interface {
	MatchCustomers(context.Context, []CustomerMatchRequest) ([]bool, error)
}

type BindStatus string

const (
	BindBound        BindStatus = "bound"
	BindAlreadyBound BindStatus = "already_bound"
	BindMerged       BindStatus = "merged"
	BindManualReview BindStatus = "manual_review"
	BindRejected     BindStatus = "rejected"
)

type BindCommand struct {
	CustomerID     contactport.CustomerID
	Ref            IDRef
	Actor          contactport.Actor
	IdempotencyKey string
}
type BindResult struct {
	Status                        BindStatus
	CustomerID, PrimaryCustomerID contactport.CustomerID
	MergeAuditID, ReviewID        int64
}

type IngestStatus string

const (
	IngestAttributed IngestStatus = "attributed"
	IngestPending    IngestStatus = "pending"
	IngestConflict   IngestStatus = "conflict"
)

type IngestCommand struct {
	Refs           []IDRef
	EventType      string
	Payload        json.RawMessage
	Source         string
	OccurredAt     time.Time
	IdempotencyKey string
}
type IngestResult struct {
	Status         IngestStatus
	CustomerID     contactport.CustomerID
	EventID        contactport.EventID
	PendingEventID int64
}

type Service interface {
	Resolve(context.Context, IDRef) (ResolveResult, error)
	Bind(context.Context, BindCommand) (BindResult, error)
	Ingest(context.Context, IngestCommand) (IngestResult, error)
}

type MergeReviewStatus string

const (
	MergeReviewPending  MergeReviewStatus = "pending"
	MergeReviewApproved MergeReviewStatus = "approved"
	MergeReviewRejected MergeReviewStatus = "rejected"
)

func (status MergeReviewStatus) Valid() bool {
	switch status {
	case MergeReviewPending, MergeReviewApproved, MergeReviewRejected:
		return true
	default:
		return false
	}
}

// MergeReview is the closed administrative review fact. It intentionally omits
// normalized identities, raw provider identifiers and payloads. The fingerprint
// is a versioned secret-backed HMAC, never the underlying identity value.
type MergeReview struct {
	ReviewID            int64
	Status              MergeReviewStatus
	Kind                IDKind
	Scope               string
	CustomerIDs         []contactport.CustomerID
	IdentityFingerprint string
	Version             int64
	CreatedAt           time.Time
	ResolvedAt          *time.Time
}

type MergeReviewPage struct {
	Items      []MergeReview
	NextCursor string
}

// CustomerMergeHistory is the redacted, append-only merge lineage exposed to
// local administrators. It intentionally omits identity values, fingerprints,
// operator identifiers and the private audit detail document.
type CustomerMergeHistory struct {
	MergeAuditID      int64
	PrimaryCustomerID contactport.CustomerID
	MergedCustomerID  contactport.CustomerID
	Mode              string
	PolicyVersion     string
	MergedAt          time.Time
}

type CustomerMergeHistoryPage struct {
	CustomerID contactport.CustomerID
	Items      []CustomerMergeHistory
	NextCursor string
}

type CustomerMergeHistoryReader interface {
	ListCustomerMergeHistory(context.Context, contactport.CustomerID, string, int32) (CustomerMergeHistoryPage, error)
}

type ApproveMergeReviewCommand struct {
	ReviewID          int64
	ExpectedVersion   int64
	PrimaryCustomerID contactport.CustomerID
	Reason            string
	Actor             contactport.Actor
	IdempotencyKey    string
}

type RejectMergeReviewCommand struct {
	ReviewID        int64
	ExpectedVersion int64
	Reason          string
	Actor           contactport.Actor
	IdempotencyKey  string
}

type ReviewHistoryService interface {
	ListMergeReviewsByStatus(context.Context, MergeReviewStatus, string, int32) (MergeReviewPage, error)
}

type ReviewService interface {
	ReviewHistoryService
	// ListMergeReviews preserves the original pending-only default for internal callers.
	ListMergeReviews(context.Context, string, int32) (MergeReviewPage, error)
	ApproveMergeReview(context.Context, ApproveMergeReviewCommand) (MergeReview, error)
	RejectMergeReview(context.Context, RejectMergeReviewCommand) (MergeReview, error)
}

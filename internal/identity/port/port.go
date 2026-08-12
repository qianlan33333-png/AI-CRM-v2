// Package port freezes identity resolution and attribution semantics.
package port

import (
	"context"
	"encoding/json"
	"time"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
)

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

type MergeReview struct {
	ReviewID            int64
	Status              MergeReviewStatus
	Kind                IDKind
	Scope               string
	IdentityFingerprint string
	CustomerIDs         []contactport.CustomerID
	Version             int64
	CreatedAt           time.Time
	ResolvedAt          *time.Time
}

type MergeReviewPage struct {
	Items      []MergeReview
	NextCursor string
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

type ReviewService interface {
	ListMergeReviews(context.Context, string, int32) (MergeReviewPage, error)
	ApproveMergeReview(context.Context, ApproveMergeReviewCommand) (MergeReview, error)
	RejectMergeReview(context.Context, RejectMergeReviewCommand) (MergeReview, error)
}

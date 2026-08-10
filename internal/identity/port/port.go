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
	AssuranceVerified Assurance = "verified"
	AssuranceDeclared Assurance = "declared"
)

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
	CustomerID contactport.CustomerID
	Ref        IDRef
	Actor      contactport.Actor
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

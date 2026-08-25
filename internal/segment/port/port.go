// Package port freezes the cross-domain Segment contract. It does not parse
// DSL, compile predicates, access SQL, enqueue jobs, or serve HTTP.
package port

import (
	"context"
	"encoding/json"
	"time"
)

type SegmentID int64
type CustomerID int64
type Actor string
type Definition json.RawMessage

type RefreshMode string
type RefreshStatus string
type LifecycleStatus string

const (
	RefreshModeManual    RefreshMode = "manual"
	RefreshModeScheduled RefreshMode = "scheduled"

	RefreshStatusIdle    RefreshStatus = "idle"
	RefreshStatusRunning RefreshStatus = "running"
	RefreshStatusFailed  RefreshStatus = "failed"

	LifecycleStatusActive   LifecycleStatus = "active"
	LifecycleStatusArchived LifecycleStatus = "archived"
)

type Segment struct {
	ID              SegmentID
	Name            string
	Definition      Definition
	RefreshMode     RefreshMode
	RefreshCron     *string
	MemberCount     int64
	RefreshedAt     *time.Time
	RefreshStatus   RefreshStatus
	LifecycleStatus LifecycleStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Page struct {
	Items      []Segment
	NextCursor string
}

type MemberPage struct {
	CustomerIDs []CustomerID
	NextCursor  string
}

type CreateCommand struct {
	Name           string
	Definition     Definition
	RefreshMode    RefreshMode
	RefreshCron    *string
	Actor          Actor
	IdempotencyKey string
}

type UpdateCommand struct {
	SegmentID      SegmentID
	Name           *string
	Definition     *Definition
	RefreshMode    *RefreshMode
	RefreshCron    *string
	Actor          Actor
	IdempotencyKey string
}

type RefreshCommand struct {
	SegmentID      SegmentID
	Actor          Actor
	IdempotencyKey string
}

// DefinitionEvaluation is a PII-free, reproducible result for one canonical
// definition at one frozen UTC instant. MemberDigest is over sorted OneIDs and
// must never be decoded or exposed as member identities.
type DefinitionEvaluation struct {
	MemberCount  int64
	MemberDigest [32]byte
	EvaluatedAt  time.Time
}

// AudienceDefinitionEngine evaluates or materializes a caller-frozen Segment
// definition inside the caller's transaction. It never invokes a provider.
type AudienceDefinitionEngine interface {
	Preview(context.Context, Definition, time.Time) (DefinitionEvaluation, error)
	Materialize(context.Context, SegmentID, Definition, time.Time) (DefinitionEvaluation, error)
}

type ArchiveCommand struct {
	SegmentID      SegmentID
	Actor          Actor
	IdempotencyKey string
}

// Service is the only Segment dependency available to another domain. Every
// write command is idempotent; implementation and worker activation are later
// slices.
type Service interface {
	List(context.Context, string, int32) (Page, error)
	Get(context.Context, SegmentID) (Segment, error)
	Create(context.Context, CreateCommand) (Segment, error)
	Update(context.Context, UpdateCommand) (Segment, error)
	ListMembers(context.Context, SegmentID, string, int32) (MemberPage, error)
	RequestRefresh(context.Context, RefreshCommand) (Segment, error)
	Archive(context.Context, ArchiveCommand) (Segment, error)
}

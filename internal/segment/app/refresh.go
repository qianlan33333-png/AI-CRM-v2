// Package app coordinates Segment use cases without exposing database or SQL
// details to callers.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/compiler"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/segment/dsl"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

var (
	ErrInvalidSegmentRefresh = errors.New("invalid segment refresh")
	ErrSegmentNotFound       = errors.New("segment not found")
	ErrSegmentRefreshFailed  = errors.New("segment refresh failed")
)

// RefreshResult is returned only after the replacement transaction commits.
type RefreshResult struct {
	SegmentID   segmentport.SegmentID
	MemberCount int64
	RefreshedAt time.Time
}

// RefreshStore is Segment-internal. Every method requires the transaction-bound
// context supplied by UnitOfWork.
type RefreshStore interface {
	LockDefinition(context.Context, segmentport.SegmentID) ([]byte, error)
	QuerySet(context.Context) (compiler.QuerySet, error)
	ReplaceMembers(context.Context, segmentport.SegmentID, []int64, time.Time) error
}

// RefreshService performs one deterministic materialization transaction. Job
// receipts, River scheduling and public commands deliberately remain outside
// this core slice.
type RefreshService struct {
	uow    platformport.UnitOfWork
	store  RefreshStore
	events eventport.Appender
}

func NewRefreshService(
	uow platformport.UnitOfWork,
	store RefreshStore,
	events eventport.Appender,
) *RefreshService {
	return &RefreshService{uow: uow, store: store, events: events}
}

// RefreshOnce locks the Segment definition, reuses the S01-S03 pipeline and
// replaces the materialized OneID set in the same transaction. reference must
// be a stable UTC instant so UnitOfWork retries preserve relative-date meaning.
func (service *RefreshService) RefreshOnce(
	ctx context.Context,
	segmentID segmentport.SegmentID,
	reference time.Time,
) (RefreshResult, error) {
	if segmentID <= 0 || reference.IsZero() || reference.Location() != time.UTC {
		return RefreshResult{}, ErrInvalidSegmentRefresh
	}
	if service == nil || service.uow == nil || service.store == nil || service.events == nil || ctx == nil {
		return RefreshResult{}, ErrSegmentRefreshFailed
	}

	var result RefreshResult
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		definition, err := service.store.LockDefinition(txCtx, segmentID)
		if err != nil {
			return err
		}
		ast, err := dsl.Parse(definition)
		if err != nil {
			return err
		}
		program, err := compiler.Compile(ast, reference)
		if err != nil {
			return err
		}
		queries, err := service.store.QuerySet(txCtx)
		if err != nil {
			return err
		}
		if queries == nil {
			return ErrSegmentRefreshFailed
		}
		customerIDs, err := compiler.Execute(txCtx, program, queries)
		if err != nil {
			return err
		}
		if err := service.store.ReplaceMembers(txCtx, segmentID, customerIDs, reference); err != nil {
			return err
		}
		payload, err := json.Marshal(struct {
			SegmentID   segmentport.SegmentID `json:"segment_id"`
			MemberCount int64                 `json:"member_count"`
		}{SegmentID: segmentID, MemberCount: int64(len(customerIDs))})
		if err != nil {
			return err
		}
		if _, err := service.events.Append(txCtx, eventport.Event{
			Type:           "segment.refreshed",
			Payload:        payload,
			OccurredAt:     reference,
			IdempotencyKey: "segment.refresh:" + strconv.FormatInt(int64(segmentID), 10) + ":" + reference.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
		result = RefreshResult{
			SegmentID: segmentID, MemberCount: int64(len(customerIDs)), RefreshedAt: reference,
		}
		return nil
	})
	if err != nil {
		return RefreshResult{}, errors.Join(ErrSegmentRefreshFailed, err)
	}
	return result, nil
}

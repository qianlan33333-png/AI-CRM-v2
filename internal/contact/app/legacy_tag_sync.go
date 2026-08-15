package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	LegacyTagSyncJobKind       = "contact_tag_catalog_sync"
	legacyTagSyncAcceptedEvent = "contact.tag_catalog_sync_accepted"
)

var (
	ErrInvalidLegacyTagSync  = errors.New("invalid legacy tag sync command")
	ErrLegacyTagSyncConflict = errors.New("legacy tag sync idempotency command conflict")
	ErrLegacyTagSyncFailed   = errors.New("accept legacy tag sync command")
)

// LegacyTagSyncKind distinguishes an explicit admin request from a request
// that is due for refresh. Both are accepted locally; neither is evidence that
// a WeCom read has been attempted or executed.
type LegacyTagSyncKind string

const (
	LegacyTagSyncManual LegacyTagSyncKind = "manual"
	LegacyTagSyncDue    LegacyTagSyncKind = "due"
)

// LegacyTagSyncState is the cross-process execution boundary. This app slice
// commits only Queued. A worker must persist Attempted/Executed, classify an
// interrupted dispatch as OutcomeUnknown, and require reconciliation rather
// than automatically retrying that unknown outcome.
type LegacyTagSyncState string

const (
	LegacyTagSyncQueued         LegacyTagSyncState = "queued"
	LegacyTagSyncAttempted      LegacyTagSyncState = "attempted"
	LegacyTagSyncExecuted       LegacyTagSyncState = "executed"
	LegacyTagSyncOutcomeUnknown LegacyTagSyncState = "outcome_unknown"
	LegacyTagSyncReconciled     LegacyTagSyncState = "reconciled"
)

// LegacyTagSyncReceiptState is the durable acceptance-receipt state exposed
// to the Contact-owned store adapter. It is deliberately narrower than the
// worker lifecycle: this command boundary can only reserve or queue work.
type LegacyTagSyncReceiptState string

const (
	LegacyTagSyncReceiptReserved LegacyTagSyncReceiptState = "reserved"
	LegacyTagSyncReceiptQueued   LegacyTagSyncReceiptState = "queued"
)

// LegacyTagSyncCommand carries only local command facts. CorpID and provider
// credentials are intentionally absent: provider reads belong to the WeCom
// public port at the later shared integration boundary.
type LegacyTagSyncCommand struct {
	Actor          int64
	IdempotencyKey string
	TraceID        string
	Kind           LegacyTagSyncKind
}

type LegacyTagSyncReceipt struct {
	ID         int64
	Command    LegacyTagSyncCommand
	State      LegacyTagSyncReceiptState
	EventID    eventport.EventID
	RiverJobID int64
}

// LegacyTagSyncJob is a durable worker input. Enqueueing it inside the UoW is
// acceptance only; this package has no provider client and never calls WeCom.
type LegacyTagSyncJob struct {
	ReceiptID int64             `json:"receipt_id"`
	SyncKind  LegacyTagSyncKind `json:"kind"`
	TraceID   string            `json:"trace_id,omitempty"`
}

func (LegacyTagSyncJob) Kind() string { return LegacyTagSyncJobKind }

type LegacyTagSyncAcceptance struct {
	ReceiptID  int64              `json:"receipt_id"`
	EventID    eventport.EventID  `json:"event_id"`
	RiverJobID int64              `json:"river_job_id"`
	State      LegacyTagSyncState `json:"state"`
}

// LegacyTagSyncReceiptStore must reserve and accept the idempotency receipt in
// the caller's transaction. Its durable implementation is shared-tail work.
type LegacyTagSyncReceiptStore interface {
	ReserveLegacyTagSync(context.Context, LegacyTagSyncCommand) (LegacyTagSyncReceipt, error)
	AcceptLegacyTagSync(context.Context, int64, eventport.EventID, int64) (LegacyTagSyncReceipt, error)
}

// LegacyTagSyncEnqueuer is an outbound queue acceptance boundary, not a
// provider adapter. The later worker owns any WeCom read through its public
// port and owns Attempted/Executed/OutcomeUnknown/Reconciled transitions.
type LegacyTagSyncEnqueuer interface {
	EnqueueLegacyTagSync(context.Context, LegacyTagSyncJob) (int64, error)
}

type LegacyTagSyncService struct {
	uow      platformport.UnitOfWork
	receipts LegacyTagSyncReceiptStore
	events   eventport.Appender
	enqueuer LegacyTagSyncEnqueuer
	now      func() time.Time
}

func NewLegacyTagSyncService(
	uow platformport.UnitOfWork,
	receipts LegacyTagSyncReceiptStore,
	events eventport.Appender,
	enqueuer LegacyTagSyncEnqueuer,
) *LegacyTagSyncService {
	return &LegacyTagSyncService{uow: uow, receipts: receipts, events: events, enqueuer: enqueuer, now: time.Now}
}

// Request accepts a manual or due sync exactly once. The immutable event,
// River job insertion, and receipt acceptance share one UoW. It returns
// Queued, never Attempted or Executed, so an HTTP success cannot be mistaken
// for a provider result.
func (service *LegacyTagSyncService) Request(ctx context.Context, command LegacyTagSyncCommand) (LegacyTagSyncAcceptance, error) {
	if ctx == nil || !validLegacyTagSyncCommand(command) || !legacyTagSyncReady(service) {
		return LegacyTagSyncAcceptance{}, ErrInvalidLegacyTagSync
	}

	var result LegacyTagSyncAcceptance
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.receipts.ReserveLegacyTagSync(txCtx, command)
		if err != nil {
			return err
		}
		if !sameLegacyTagSyncCommand(receipt.Command, command) {
			return ErrLegacyTagSyncConflict
		}
		switch receipt.State {
		case LegacyTagSyncReceiptQueued:
			acceptance, err := legacyTagSyncAcceptanceFromReceipt(receipt)
			if err != nil {
				return err
			}
			result = acceptance
			return nil
		case LegacyTagSyncReceiptReserved:
			if receipt.ID <= 0 {
				return ErrLegacyTagSyncFailed
			}
		default:
			return ErrLegacyTagSyncFailed
		}

		occurredAt := service.now().UTC()
		if occurredAt.IsZero() {
			return ErrLegacyTagSyncFailed
		}
		payload, err := json.Marshal(struct {
			ReceiptID int64             `json:"receipt_id"`
			Actor     int64             `json:"actor"`
			Kind      LegacyTagSyncKind `json:"kind"`
			TraceID   string            `json:"trace_id,omitempty"`
		}{ReceiptID: receipt.ID, Actor: command.Actor, Kind: command.Kind, TraceID: command.TraceID})
		if err != nil {
			return err
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type:           legacyTagSyncAcceptedEvent,
			Payload:        payload,
			OccurredAt:     occurredAt,
			IdempotencyKey: legacyTagSyncEventKey(command),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrLegacyTagSyncFailed, err)
		}
		jobID, err := service.enqueuer.EnqueueLegacyTagSync(txCtx, LegacyTagSyncJob{ReceiptID: receipt.ID, SyncKind: command.Kind, TraceID: command.TraceID})
		if err != nil || jobID <= 0 {
			return errors.Join(ErrLegacyTagSyncFailed, err)
		}
		accepted, err := service.receipts.AcceptLegacyTagSync(txCtx, receipt.ID, eventID, jobID)
		if err != nil {
			return err
		}
		acceptance, err := legacyTagSyncAcceptanceFromReceipt(accepted)
		if err != nil {
			return err
		}
		result = acceptance
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidLegacyTagSync) || errors.Is(err, ErrLegacyTagSyncConflict) {
			return LegacyTagSyncAcceptance{}, err
		}
		return LegacyTagSyncAcceptance{}, errors.Join(ErrLegacyTagSyncFailed, err)
	}
	return result, nil
}

// LegacyTagSyncCanAutoRetry makes the safety rule explicit for the eventual
// worker: after any dispatch is ambiguous, only reconciliation may proceed.
func LegacyTagSyncCanAutoRetry(state LegacyTagSyncState) bool {
	return state == LegacyTagSyncQueued
}

func validLegacyTagSyncCommand(command LegacyTagSyncCommand) bool {
	return command.Actor > 0 && validLegacyTagSyncText(command.IdempotencyKey, 1, 200) &&
		validLegacyTagSyncText(command.TraceID, 0, 200) &&
		(command.Kind == LegacyTagSyncManual || command.Kind == LegacyTagSyncDue)
}

func validLegacyTagSyncText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value
}

func sameLegacyTagSyncCommand(left, right LegacyTagSyncCommand) bool {
	return left == right
}

func legacyTagSyncAcceptanceFromReceipt(receipt LegacyTagSyncReceipt) (LegacyTagSyncAcceptance, error) {
	if receipt.ID <= 0 || receipt.State != LegacyTagSyncReceiptQueued || !validLegacyTagSyncCommand(receipt.Command) ||
		receipt.EventID <= 0 || receipt.RiverJobID <= 0 {
		return LegacyTagSyncAcceptance{}, ErrLegacyTagSyncFailed
	}
	return LegacyTagSyncAcceptance{ReceiptID: receipt.ID, EventID: receipt.EventID, RiverJobID: receipt.RiverJobID, State: LegacyTagSyncQueued}, nil
}

func legacyTagSyncEventKey(command LegacyTagSyncCommand) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", command.Actor, command.Kind, command.IdempotencyKey)))
	return "tag-catalog:sync-accepted:" + hex.EncodeToString(digest[:])
}

func legacyTagSyncReady(service *LegacyTagSyncService) bool {
	return service != nil && !nilLegacyTagSyncDependency(service.uow) && !nilLegacyTagSyncDependency(service.receipts) &&
		!nilLegacyTagSyncDependency(service.events) && !nilLegacyTagSyncDependency(service.enqueuer) && service.now != nil
}

func nilLegacyTagSyncDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	}
	return false
}

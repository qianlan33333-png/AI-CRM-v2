package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	LegacyTagLiveMutationJobKind       = "contact_tag_live_mutation"
	legacyTagLiveMutationAcceptedEvent = "contact.tag_live_mutation_accepted"
)

var (
	ErrInvalidLegacyTagExecutionStatus = errors.New("invalid legacy tag execution status")
	ErrLegacyTagExecutionUnavailable   = errors.New("legacy tag execution status unavailable")
	ErrInvalidLegacyTagLiveMutation    = errors.New("invalid legacy tag live mutation command")
	ErrLegacyTagLiveMutationConflict   = errors.New("legacy tag live mutation idempotency command conflict")
	ErrLegacyTagLiveMutationFailed     = errors.New("accept legacy tag live mutation command")
)

// LegacyTagExecutionStatus is intentionally opaque. Canonical route evidence
// proves the `tag_execution_status` surface, but not a stable response body.
// Keeping the validated JSON payload opaque avoids inventing user-visible gate
// fields before the shared legacy handler can replay the exact envelope.
type LegacyTagExecutionStatus struct {
	Payload    json.RawMessage
	ObservedAt time.Time
}

// LegacyTagExecutionGate is the only stable, user-visible projection of the
// legacy singleton. The raw payload remains an internal compatibility input:
// callers never receive its mode, revision, or future fields.
type LegacyTagExecutionGate struct {
	ProviderExecutionEligible       bool
	LocalCommandAcceptanceAvailable bool
	LocalQueueAvailable             bool
	SyncExecuted                    bool
	ObservedAt                      time.Time
	RealExternalCallExecuted        bool
}

type LegacyTagExecutionStatusReader interface {
	ReadLegacyTagExecutionStatus(context.Context) (LegacyTagExecutionStatus, error)
}

// LegacyTagExecutionStatusService reads a local gate projection only. It has
// no WeCom client, credential, CorpID, or provider-call capability.
type LegacyTagExecutionStatusService struct {
	uow    platformport.UnitOfWork
	reader LegacyTagExecutionStatusReader
}

func NewLegacyTagExecutionStatusService(uow platformport.UnitOfWork, reader LegacyTagExecutionStatusReader) *LegacyTagExecutionStatusService {
	return &LegacyTagExecutionStatusService{uow: uow, reader: reader}
}

func (service *LegacyTagExecutionStatusService) Get(ctx context.Context) (LegacyTagExecutionGate, error) {
	if ctx == nil || service == nil || nilLegacyTagLiveDependency(service.uow) || nilLegacyTagLiveDependency(service.reader) {
		return LegacyTagExecutionGate{}, ErrLegacyTagExecutionUnavailable
	}
	var gate LegacyTagExecutionGate
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		loaded, err := service.reader.ReadLegacyTagExecutionStatus(txCtx)
		if err != nil {
			return err
		}
		projected, ok := projectLegacyTagExecutionGate(loaded)
		if !ok {
			return ErrInvalidLegacyTagExecutionStatus
		}
		gate = projected
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidLegacyTagExecutionStatus) {
			return LegacyTagExecutionGate{}, err
		}
		return LegacyTagExecutionGate{}, errors.Join(ErrLegacyTagExecutionUnavailable, err)
	}
	return gate, nil
}

func projectLegacyTagExecutionGate(status LegacyTagExecutionStatus) (LegacyTagExecutionGate, bool) {
	if status.ObservedAt.IsZero() {
		return LegacyTagExecutionGate{}, false
	}
	var source map[string]json.RawMessage
	if json.Unmarshal(status.Payload, &source) != nil || source == nil {
		return LegacyTagExecutionGate{}, false
	}
	var mode string
	var accepted, queued, attempted, executed, unknown, reconciled, external, synced bool
	if json.Unmarshal(source["mode"], &mode) != nil || mode != "provider_execution_unavailable" ||
		json.Unmarshal(source["accepted"], &accepted) != nil || !accepted ||
		json.Unmarshal(source["queued"], &queued) != nil || !queued ||
		json.Unmarshal(source["attempted"], &attempted) != nil || attempted ||
		json.Unmarshal(source["executed"], &executed) != nil || executed ||
		json.Unmarshal(source["outcome_unknown"], &unknown) != nil || unknown ||
		json.Unmarshal(source["reconciled"], &reconciled) != nil || reconciled ||
		json.Unmarshal(source["real_external_call_executed"], &external) != nil || external ||
		json.Unmarshal(source["sync_executed"], &synced) != nil || synced {
		return LegacyTagExecutionGate{}, false
	}
	return LegacyTagExecutionGate{
		ProviderExecutionEligible: false, LocalCommandAcceptanceAvailable: true,
		LocalQueueAvailable: true, SyncExecuted: false, ObservedAt: status.ObservedAt.UTC(),
		RealExternalCallExecuted: false,
	}, true
}

type LegacyTagLiveMutationOperation string

const (
	LegacyTagLiveMutationMark   LegacyTagLiveMutationOperation = "mark"
	LegacyTagLiveMutationUnmark LegacyTagLiveMutationOperation = "unmark"
)

// LegacyTagLiveMutationState is owned by the durable worker tail. This slice
// can return only Queued; a post-dispatch interruption is OutcomeUnknown and
// must be reconciled, never automatically retried.
type LegacyTagLiveMutationState string

const (
	LegacyTagLiveMutationQueued         LegacyTagLiveMutationState = "queued"
	LegacyTagLiveMutationAttempted      LegacyTagLiveMutationState = "attempted"
	LegacyTagLiveMutationExecuted       LegacyTagLiveMutationState = "executed"
	LegacyTagLiveMutationOutcomeUnknown LegacyTagLiveMutationState = "outcome_unknown"
	LegacyTagLiveMutationReconciled     LegacyTagLiveMutationState = "reconciled"
)

// LegacyTagLiveMutationReceiptState is the Contact store's technical receipt
// state. Provider execution state remains separately owned by the worker.
type LegacyTagLiveMutationReceiptState string

const (
	LegacyTagLiveMutationReceiptReserved LegacyTagLiveMutationReceiptState = "reserved"
	LegacyTagLiveMutationReceiptQueued   LegacyTagLiveMutationReceiptState = "queued"
)

// LegacyTagLiveMutationCommand deliberately preserves the still-unmapped
// legacy body as opaque JSON. It validates structure and idempotency without
// deriving an external user, tag identifier, CorpID, or provider request.
type LegacyTagLiveMutationCommand struct {
	Actor          int64
	IdempotencyKey string
	TraceID        string
	Operation      LegacyTagLiveMutationOperation
	Payload        json.RawMessage
}

type LegacyTagLiveMutationReceipt struct {
	ID         int64
	Command    LegacyTagLiveMutationCommand
	State      LegacyTagLiveMutationReceiptState
	EventID    eventport.EventID
	RiverJobID int64
}

type LegacyTagLiveMutationJob struct {
	ReceiptID int64                          `json:"receipt_id"`
	Operation LegacyTagLiveMutationOperation `json:"operation"`
	Payload   json.RawMessage                `json:"payload"`
	TraceID   string                         `json:"trace_id,omitempty"`
}

func (LegacyTagLiveMutationJob) Kind() string { return LegacyTagLiveMutationJobKind }

type LegacyTagLiveMutationAcceptance struct {
	ReceiptID  int64                      `json:"receipt_id"`
	EventID    eventport.EventID          `json:"event_id"`
	RiverJobID int64                      `json:"river_job_id"`
	State      LegacyTagLiveMutationState `json:"state"`
}

type LegacyTagLiveMutationReceiptStore interface {
	ReserveLegacyTagLiveMutation(context.Context, LegacyTagLiveMutationCommand) (LegacyTagLiveMutationReceipt, error)
	AcceptLegacyTagLiveMutation(context.Context, int64, eventport.EventID, int64) (LegacyTagLiveMutationReceipt, error)
}

// LegacyTagLiveMutationEnqueuer is a transactional queue acceptance boundary.
// The later worker must reach WeCom only through the frozen public port.
type LegacyTagLiveMutationEnqueuer interface {
	EnqueueLegacyTagLiveMutation(context.Context, LegacyTagLiveMutationJob) (int64, error)
}

type LegacyTagLiveMutationService struct {
	uow      platformport.UnitOfWork
	receipts LegacyTagLiveMutationReceiptStore
	events   eventport.Appender
	enqueuer LegacyTagLiveMutationEnqueuer
	now      func() time.Time
}

func NewLegacyTagLiveMutationService(
	uow platformport.UnitOfWork,
	receipts LegacyTagLiveMutationReceiptStore,
	events eventport.Appender,
	enqueuer LegacyTagLiveMutationEnqueuer,
) *LegacyTagLiveMutationService {
	return &LegacyTagLiveMutationService{uow: uow, receipts: receipts, events: events, enqueuer: enqueuer, now: time.Now}
}

// Request atomically records a mutation acceptance, immutable event, and
// durable worker job. Success proves only Queued; it is never a WeCom write
// receipt and cannot be exposed as an executed live mutation.
func (service *LegacyTagLiveMutationService) Request(ctx context.Context, command LegacyTagLiveMutationCommand) (LegacyTagLiveMutationAcceptance, error) {
	if ctx == nil || !validLegacyTagLiveMutationCommand(command) || !legacyTagLiveMutationReady(service) {
		return LegacyTagLiveMutationAcceptance{}, ErrInvalidLegacyTagLiveMutation
	}

	var result LegacyTagLiveMutationAcceptance
	err := service.uow.Within(ctx, func(txCtx context.Context) error {
		receipt, err := service.receipts.ReserveLegacyTagLiveMutation(txCtx, command)
		if err != nil {
			return err
		}
		if !sameLegacyTagLiveMutationCommand(receipt.Command, command) {
			return ErrLegacyTagLiveMutationConflict
		}
		switch receipt.State {
		case LegacyTagLiveMutationReceiptQueued:
			acceptance, err := legacyTagLiveMutationAcceptanceFromReceipt(receipt)
			if err != nil {
				return err
			}
			result = acceptance
			return nil
		case LegacyTagLiveMutationReceiptReserved:
			if receipt.ID <= 0 {
				return ErrLegacyTagLiveMutationFailed
			}
		default:
			return ErrLegacyTagLiveMutationFailed
		}

		occurredAt := service.now().UTC()
		if occurredAt.IsZero() {
			return ErrLegacyTagLiveMutationFailed
		}
		payload, err := json.Marshal(struct {
			ReceiptID int64                          `json:"receipt_id"`
			Actor     int64                          `json:"actor"`
			Operation LegacyTagLiveMutationOperation `json:"operation"`
			Payload   json.RawMessage                `json:"payload"`
			TraceID   string                         `json:"trace_id,omitempty"`
		}{ReceiptID: receipt.ID, Actor: command.Actor, Operation: command.Operation, Payload: command.Payload, TraceID: command.TraceID})
		if err != nil {
			return err
		}
		eventID, err := service.events.Append(txCtx, eventport.Event{
			Type:           legacyTagLiveMutationAcceptedEvent,
			Payload:        payload,
			OccurredAt:     occurredAt,
			IdempotencyKey: legacyTagLiveMutationEventKey(command),
		})
		if err != nil || eventID <= 0 {
			return errors.Join(ErrLegacyTagLiveMutationFailed, err)
		}
		jobID, err := service.enqueuer.EnqueueLegacyTagLiveMutation(txCtx, LegacyTagLiveMutationJob{
			ReceiptID: receipt.ID, Operation: command.Operation, Payload: append(json.RawMessage(nil), command.Payload...), TraceID: command.TraceID,
		})
		if err != nil || jobID <= 0 {
			return errors.Join(ErrLegacyTagLiveMutationFailed, err)
		}
		accepted, err := service.receipts.AcceptLegacyTagLiveMutation(txCtx, receipt.ID, eventID, jobID)
		if err != nil {
			return err
		}
		acceptance, err := legacyTagLiveMutationAcceptanceFromReceipt(accepted)
		if err != nil {
			return err
		}
		result = acceptance
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidLegacyTagLiveMutation) || errors.Is(err, ErrLegacyTagLiveMutationConflict) {
			return LegacyTagLiveMutationAcceptance{}, err
		}
		return LegacyTagLiveMutationAcceptance{}, errors.Join(ErrLegacyTagLiveMutationFailed, err)
	}
	return result, nil
}

func LegacyTagLiveMutationCanAutoRetry(state LegacyTagLiveMutationState) bool {
	return state == LegacyTagLiveMutationQueued
}

func validLegacyTagLiveMutationCommand(command LegacyTagLiveMutationCommand) bool {
	return command.Actor > 0 && validLegacyTagLiveText(command.IdempotencyKey, 1, 200) &&
		validLegacyTagLiveText(command.TraceID, 0, 200) &&
		(command.Operation == LegacyTagLiveMutationMark || command.Operation == LegacyTagLiveMutationUnmark) &&
		validLegacyTagJSONObject(command.Payload)
}

func validLegacyTagLiveText(value string, minimum, maximum int) bool {
	return len(value) >= minimum && len(value) <= maximum && strings.TrimSpace(value) == value
}

func validLegacyTagJSONObject(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	return decoder.Decode(&object) == nil && object != nil && errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func sameLegacyTagLiveMutationCommand(left, right LegacyTagLiveMutationCommand) bool {
	return left.Actor == right.Actor && left.IdempotencyKey == right.IdempotencyKey && left.TraceID == right.TraceID &&
		left.Operation == right.Operation && legacyTagLiveJSONEqual(left.Payload, right.Payload)
}

func legacyTagLiveMutationAcceptanceFromReceipt(receipt LegacyTagLiveMutationReceipt) (LegacyTagLiveMutationAcceptance, error) {
	if receipt.ID <= 0 || receipt.State != LegacyTagLiveMutationReceiptQueued || !validLegacyTagLiveMutationCommand(receipt.Command) ||
		receipt.EventID <= 0 || receipt.RiverJobID <= 0 {
		return LegacyTagLiveMutationAcceptance{}, ErrLegacyTagLiveMutationFailed
	}
	return LegacyTagLiveMutationAcceptance{ReceiptID: receipt.ID, EventID: receipt.EventID, RiverJobID: receipt.RiverJobID, State: LegacyTagLiveMutationQueued}, nil
}

func legacyTagLiveMutationEventKey(command LegacyTagLiveMutationCommand) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", command.Actor, command.Operation, command.IdempotencyKey)))
	return "tag-live-mutation:accepted:" + hex.EncodeToString(digest[:])
}

func legacyTagLiveMutationReady(service *LegacyTagLiveMutationService) bool {
	return service != nil && !nilLegacyTagLiveDependency(service.uow) && !nilLegacyTagLiveDependency(service.receipts) &&
		!nilLegacyTagLiveDependency(service.events) && !nilLegacyTagLiveDependency(service.enqueuer) && service.now != nil
}

func nilLegacyTagLiveDependency(value any) bool {
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

func legacyTagLiveJSONEqual(left, right json.RawMessage) bool {
	leftValue, leftOK := decodeLegacyTagLiveJSON(left)
	rightValue, rightOK := decodeLegacyTagLiveJSON(right)
	return leftOK && rightOK && semanticLegacyTagLiveJSONEqual(leftValue, rightValue)
}

func decodeLegacyTagLiveJSON(raw json.RawMessage) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	return value, decoder.Decode(&value) == nil && errors.Is(decoder.Decode(&struct{}{}), io.EOF)
}

func semanticLegacyTagLiveJSONEqual(left, right any) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	rightNumber, rightIsNumber := right.(json.Number)
	if leftIsNumber || rightIsNumber {
		if !leftIsNumber || !rightIsNumber {
			return false
		}
		leftRat, leftOK := new(big.Rat).SetString(leftNumber.String())
		rightRat, rightOK := new(big.Rat).SetString(rightNumber.String())
		return leftOK && rightOK && leftRat.Cmp(rightRat) == 0
	}
	leftObject, leftIsObject := left.(map[string]any)
	rightObject, rightIsObject := right.(map[string]any)
	if leftIsObject || rightIsObject {
		if !leftIsObject || !rightIsObject || len(leftObject) != len(rightObject) {
			return false
		}
		for key, leftValue := range leftObject {
			rightValue, ok := rightObject[key]
			if !ok || !semanticLegacyTagLiveJSONEqual(leftValue, rightValue) {
				return false
			}
		}
		return true
	}
	leftArray, leftIsArray := left.([]any)
	rightArray, rightIsArray := right.([]any)
	if leftIsArray || rightIsArray {
		if !leftIsArray || !rightIsArray || len(leftArray) != len(rightArray) {
			return false
		}
		for index := range leftArray {
			if !semanticLegacyTagLiveJSONEqual(leftArray[index], rightArray[index]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(left, right)
}

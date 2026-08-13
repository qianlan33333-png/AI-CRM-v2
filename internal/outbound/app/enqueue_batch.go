package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const (
	OutboundEnqueueBatchJobKind = "outbound_enqueue_batch_task"
	outboundBatchAcceptedEvent  = "outbound.batch.accepted"
	maxBatchRecipients          = 200000
)

var (
	ErrInvalidEnqueueBatchCommand = errors.New("invalid outbound enqueue batch command")
	ErrEnqueueBatchConflict       = errors.New("outbound enqueue batch idempotency command conflict")
	ErrEnqueueBatchFailed         = errors.New("enqueue outbound batch command")
)

type BatchTier string

const (
	BatchTierS BatchTier = "S"
	BatchTierM BatchTier = "M"
	BatchTierL BatchTier = "L"
)

func (tier BatchTier) ChunkSize() int {
	switch tier {
	case BatchTierS:
		return 1000
	case BatchTierM:
		return 5000
	case BatchTierL:
		return 10000
	default:
		return 0
	}
}

type EnqueueBatchCommand struct {
	IdempotencyScope string
	IdempotencyKey   string
	Tier             BatchTier
	CustomerIDs      []int64
	TemplateKey      string
	Payload          json.RawMessage
}

type BatchDefinition struct {
	IdempotencyScope string
	IdempotencyKey   string
	Tier             BatchTier
	RecipientDigest  [sha256.Size]byte
	RecipientCount   int
	TemplateKey      string
	Payload          json.RawMessage
}

type BatchReceipt struct {
	ID              int64
	Definition      BatchDefinition
	AcceptedEventID eventport.EventID
}

type BatchChunkState string

const (
	BatchChunkReserved BatchChunkState = "reserved"
	BatchChunkExpanded BatchChunkState = "expanded"
)

type BatchChunk struct {
	ID             int64
	BatchID        int64
	Index          int
	RecipientStart int
	RecipientCount int
	State          BatchChunkState
}

type BatchTaskCommand struct {
	BatchID    int64
	ChunkIndex int
	OneCommand
}

type EnqueueBatchTaskArgs struct {
	BatchID    int64  `json:"batch_id"`
	ChunkIndex int    `json:"chunk_index"`
	TaskID     TaskID `json:"task_id"`
}

func (EnqueueBatchTaskArgs) Kind() string { return OutboundEnqueueBatchJobKind }

type EnqueuedBatch struct {
	BatchID         int64
	AcceptedEventID eventport.EventID
	TaskCount       int
	ChunkCount      int
}

type EnqueueBatchRepository interface {
	ReserveBatch(context.Context, BatchDefinition) (BatchReceipt, error)
	AcceptBatch(context.Context, int64, eventport.EventID) (BatchReceipt, error)
	ReserveBatchChunk(context.Context, int64, int, int, int) (BatchChunk, error)
	CreateBatchTask(context.Context, BatchTaskCommand) (TaskID, error)
	EnqueueBatchTask(context.Context, EnqueueBatchTaskArgs) (int64, error)
	AcceptBatchChunk(context.Context, int64) (BatchChunk, error)
}

type EnqueueBatchService struct {
	uow        platformport.UnitOfWork
	events     eventport.Appender
	repository EnqueueBatchRepository
	clock      func() time.Time
}

func NewEnqueueBatchService(uow platformport.UnitOfWork, events eventport.Appender, repository EnqueueBatchRepository) *EnqueueBatchService {
	return &EnqueueBatchService{uow: uow, events: events, repository: repository, clock: time.Now}
}

// Enqueue durably accepts one batch, then expands it in bounded transactions.
// A committed chunk is its receipt: retries skip it instead of duplicating its
// task, event, or River job facts.
func (service *EnqueueBatchService) Enqueue(ctx context.Context, command EnqueueBatchCommand) (EnqueuedBatch, error) {
	definition, customerIDs, err := normalizeBatchCommand(command)
	if err != nil || ctx == nil || service == nil || service.uow == nil || service.events == nil || service.repository == nil || service.clock == nil {
		return EnqueuedBatch{}, ErrInvalidEnqueueBatchCommand
	}

	var receipt BatchReceipt
	err = service.uow.Within(ctx, func(txCtx context.Context) error {
		reserved, reserveErr := service.repository.ReserveBatch(txCtx, definition)
		if reserveErr != nil {
			return reserveErr
		}
		if !sameBatchDefinition(reserved.Definition, definition) {
			return ErrEnqueueBatchConflict
		}
		if reserved.AcceptedEventID > 0 {
			receipt = reserved
			return nil
		}
		if reserved.ID <= 0 {
			return ErrEnqueueBatchFailed
		}
		payload, marshalErr := json.Marshal(struct {
			BatchID        int64     `json:"batch_id"`
			RecipientCount int       `json:"recipient_count"`
			Tier           BatchTier `json:"tier"`
		}{reserved.ID, definition.RecipientCount, definition.Tier})
		if marshalErr != nil {
			return marshalErr
		}
		eventID, appendErr := service.events.Append(txCtx, eventport.Event{
			Type: outboundBatchAcceptedEvent, Payload: payload, OccurredAt: service.clock().UTC(),
			IdempotencyKey: fmt.Sprintf("outbound.batch.accepted:%d", reserved.ID),
		})
		if appendErr != nil || eventID <= 0 {
			return errors.Join(ErrEnqueueBatchFailed, appendErr)
		}
		accepted, acceptErr := service.repository.AcceptBatch(txCtx, reserved.ID, eventID)
		if acceptErr != nil || accepted.ID != reserved.ID || accepted.AcceptedEventID != eventID || !sameBatchDefinition(accepted.Definition, definition) {
			return errors.Join(ErrEnqueueBatchFailed, acceptErr)
		}
		receipt = accepted
		return nil
	})
	if err != nil {
		return EnqueuedBatch{}, batchError(err)
	}

	chunkSize := definition.Tier.ChunkSize()
	chunkCount := (len(customerIDs) + chunkSize - 1) / chunkSize
	for chunkIndex, start := 0, 0; start < len(customerIDs); chunkIndex, start = chunkIndex+1, start+chunkSize {
		end := min(start+chunkSize, len(customerIDs))
		err = service.uow.Within(ctx, func(txCtx context.Context) error {
			chunk, reserveErr := service.repository.ReserveBatchChunk(txCtx, receipt.ID, chunkIndex, start, end-start)
			if reserveErr != nil {
				return reserveErr
			}
			if chunk.BatchID != receipt.ID || chunk.Index != chunkIndex || chunk.RecipientStart != start || chunk.RecipientCount != end-start {
				return ErrEnqueueBatchConflict
			}
			if chunk.State == BatchChunkExpanded {
				return nil
			}
			if chunk.ID <= 0 || chunk.State != BatchChunkReserved {
				return ErrEnqueueBatchFailed
			}
			for _, customerID := range customerIDs[start:end] {
				taskID, createErr := service.repository.CreateBatchTask(txCtx, BatchTaskCommand{
					BatchID: receipt.ID, ChunkIndex: chunkIndex,
					OneCommand: OneCommand{CustomerID: customerID, TemplateKey: definition.TemplateKey, Payload: definition.Payload},
				})
				if createErr != nil || taskID <= 0 {
					return errors.Join(ErrEnqueueBatchFailed, createErr)
				}
				eventID, appendErr := service.events.Append(txCtx, eventport.Event{
					Type: eventport.EvOutboundAccepted, CustomerID: eventport.CustomerID(customerID),
					Payload:    json.RawMessage(fmt.Sprintf(`{"batch_id":%d,"task_id":%d}`, receipt.ID, taskID)),
					OccurredAt: service.clock().UTC(), IdempotencyKey: fmt.Sprintf("outbound.accepted:%d", taskID),
				})
				if appendErr != nil || eventID <= 0 {
					return errors.Join(ErrEnqueueBatchFailed, appendErr)
				}
				jobID, enqueueErr := service.repository.EnqueueBatchTask(txCtx, EnqueueBatchTaskArgs{BatchID: receipt.ID, ChunkIndex: chunkIndex, TaskID: taskID})
				if enqueueErr != nil || jobID <= 0 {
					return errors.Join(ErrEnqueueBatchFailed, enqueueErr)
				}
			}
			expanded, acceptErr := service.repository.AcceptBatchChunk(txCtx, chunk.ID)
			if acceptErr != nil || expanded.ID != chunk.ID || expanded.State != BatchChunkExpanded {
				return errors.Join(ErrEnqueueBatchFailed, acceptErr)
			}
			return nil
		})
		if err != nil {
			return EnqueuedBatch{}, batchError(err)
		}
	}
	return EnqueuedBatch{BatchID: receipt.ID, AcceptedEventID: receipt.AcceptedEventID, TaskCount: len(customerIDs), ChunkCount: chunkCount}, nil
}

func normalizeBatchCommand(command EnqueueBatchCommand) (BatchDefinition, []int64, error) {
	if !validEnqueueText(command.IdempotencyScope, 1, 200) || !validEnqueueText(command.IdempotencyKey, 16, 128) ||
		command.Tier.ChunkSize() == 0 || len(command.CustomerIDs) == 0 || len(command.CustomerIDs) > maxBatchRecipients ||
		!validOneCommand(OneCommand{CustomerID: 1, TemplateKey: command.TemplateKey, Payload: command.Payload}) {
		return BatchDefinition{}, nil, ErrInvalidEnqueueBatchCommand
	}
	ids := slices.Clone(command.CustomerIDs)
	slices.Sort(ids)
	for index, id := range ids {
		if id <= 0 || index > 0 && ids[index-1] == id {
			return BatchDefinition{}, nil, ErrInvalidEnqueueBatchCommand
		}
	}
	var canonicalPayload any
	if json.Unmarshal(command.Payload, &canonicalPayload) != nil {
		return BatchDefinition{}, nil, ErrInvalidEnqueueBatchCommand
	}
	canonical, err := json.Marshal(struct {
		Tier        BatchTier `json:"tier"`
		CustomerIDs []int64   `json:"customer_ids"`
		TemplateKey string    `json:"template_key"`
		Payload     any       `json:"payload"`
	}{command.Tier, ids, command.TemplateKey, canonicalPayload})
	if err != nil {
		return BatchDefinition{}, nil, ErrInvalidEnqueueBatchCommand
	}
	digest := sha256.Sum256(canonical)
	return BatchDefinition{command.IdempotencyScope, command.IdempotencyKey, command.Tier, digest, len(ids), command.TemplateKey, command.Payload}, ids, nil
}

func sameBatchDefinition(left, right BatchDefinition) bool {
	return left.IdempotencyScope == right.IdempotencyScope && left.IdempotencyKey == right.IdempotencyKey && left.Tier == right.Tier &&
		left.RecipientDigest == right.RecipientDigest && left.RecipientCount == right.RecipientCount && left.TemplateKey == right.TemplateKey && equalEnqueueJSON(left.Payload, right.Payload)
}

func batchError(err error) error {
	if errors.Is(err, ErrEnqueueBatchConflict) || errors.Is(err, ErrInvalidEnqueueBatchCommand) {
		return err
	}
	return errors.Join(ErrEnqueueBatchFailed, err)
}

package app

import (
	"context"
	"encoding/json"
	"testing"

	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
)

func TestEnqueueBatchUsesOneUoWPerBoundedChunkAtMaximumCardinality(t *testing.T) {
	for _, test := range []struct {
		tier       BatchTier
		chunkCount int
	}{{BatchTierS, 200}, {BatchTierM, 40}, {BatchTierL, 20}} {
		t.Run(string(test.tier), func(t *testing.T) {
			ids := make([]int64, maxBatchRecipients)
			for index := range ids {
				ids[index] = int64(index + 1)
			}
			uow := &countingBatchUoW{}
			repository := &expandedBatchRepository{}
			service := NewEnqueueBatchService(uow, unusedBatchEvents{}, repository)
			got, err := service.Enqueue(context.Background(), EnqueueBatchCommand{
				IdempotencyScope: "operator:7", IdempotencyKey: "outbound-max-cardinality-command",
				Tier: test.tier, CustomerIDs: ids, TemplateKey: TemplateTextNoticeV1, Payload: json.RawMessage(`{"text":"batch"}`),
			})
			if err != nil || got.TaskCount != maxBatchRecipients || got.ChunkCount != test.chunkCount {
				t.Fatalf("Enqueue()=%+v err=%v", got, err)
			}
			if uow.calls != test.chunkCount+1 || repository.chunkCalls != test.chunkCount {
				t.Fatalf("UoW/chunk calls=%d/%d, want %d/%d", uow.calls, repository.chunkCalls, test.chunkCount+1, test.chunkCount)
			}
		})
	}
}

type countingBatchUoW struct{ calls int }

func (uow *countingBatchUoW) Within(ctx context.Context, callback func(context.Context) error) error {
	uow.calls++
	return callback(ctx)
}

type unusedBatchEvents struct{}

func (unusedBatchEvents) Append(context.Context, eventport.Event) (eventport.EventID, error) {
	return 0, ErrEnqueueBatchFailed
}

type expandedBatchRepository struct {
	definition BatchDefinition
	chunkCalls int
}

func (repository *expandedBatchRepository) ReserveBatch(_ context.Context, definition BatchDefinition) (BatchReceipt, error) {
	repository.definition = definition
	return BatchReceipt{ID: 1, Definition: definition, AcceptedEventID: 2}, nil
}

func (*expandedBatchRepository) AcceptBatch(context.Context, int64, eventport.EventID) (BatchReceipt, error) {
	return BatchReceipt{}, ErrEnqueueBatchFailed
}

func (repository *expandedBatchRepository) ReserveBatchChunk(_ context.Context, batchID int64, index, start, count int) (BatchChunk, error) {
	repository.chunkCalls++
	return BatchChunk{ID: int64(index + 1), BatchID: batchID, Index: index, RecipientStart: start, RecipientCount: count, State: BatchChunkExpanded}, nil
}

func (*expandedBatchRepository) CreateBatchTask(context.Context, BatchTaskCommand) (TaskID, error) {
	return 0, ErrEnqueueBatchFailed
}

func (*expandedBatchRepository) EnqueueBatchTask(context.Context, EnqueueBatchTaskArgs) (int64, error) {
	return 0, ErrEnqueueBatchFailed
}

func (*expandedBatchRepository) AcceptBatchChunk(context.Context, int64) (BatchChunk, error) {
	return BatchChunk{}, ErrEnqueueBatchFailed
}

package app

import (
	"context"
	"errors"
	"strconv"
	"time"

	outboundport "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/port"
	platformport "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/port"
)

const deliveryLineageMaximumLimit int32 = 1_000_101

var ErrDeliveryLineageUnavailable = errors.New("outbound delivery lineage unavailable")

// DeliveryLineageStoreRecord remains inside Outbound so no task read model
// with customer, provider, or failure fields can accidentally cross the port.
type DeliveryLineageStoreRecord struct {
	TaskID             TaskID
	State              TaskStatus
	AttemptCount       int32
	UpdatedAt          time.Time
	SameTimestampCount int64
}

type deliveryLineageStore interface {
	ListDeliveryLineage(context.Context, int32) ([]DeliveryLineageStoreRecord, error)
}

// DeliveryLineageReader owns the outbound half of the read-only projection.
type DeliveryLineageReader struct {
	uow   platformport.UnitOfWork
	store deliveryLineageStore
}

var _ outboundport.DeliveryLineageReader = (*DeliveryLineageReader)(nil)

func NewDeliveryLineageReader(uow platformport.UnitOfWork, store deliveryLineageStore) *DeliveryLineageReader {
	return &DeliveryLineageReader{uow: uow, store: store}
}

func (reader *DeliveryLineageReader) ListDeliveryLineage(ctx context.Context, limit int32) (outboundport.DeliveryLineagePage, error) {
	if ctx == nil || ctx.Err() != nil || limit < 1 || limit > deliveryLineageMaximumLimit || reader == nil || reader.uow == nil || reader.store == nil {
		return outboundport.DeliveryLineagePage{}, ErrDeliveryLineageUnavailable
	}
	var records []DeliveryLineageStoreRecord
	if err := reader.uow.Within(ctx, func(txCtx context.Context) error {
		var err error
		records, err = reader.store.ListDeliveryLineage(txCtx, limit)
		return err
	}); err != nil {
		return outboundport.DeliveryLineagePage{}, errors.Join(ErrDeliveryLineageUnavailable, err)
	}
	if len(records) > int(limit) || !validDeliveryLineageRecords(records) {
		return outboundport.DeliveryLineagePage{}, ErrDeliveryLineageUnavailable
	}
	page := outboundport.DeliveryLineagePage{Items: make([]outboundport.DeliveryLineageItem, 0, len(records)), Complete: true}
	for _, record := range records {
		page.Items = append(page.Items, outboundport.DeliveryLineageItem{
			LineageID:     outboundDeliveryLineageID(record.TaskID),
			InternalState: string(record.State), AttemptCount: record.AttemptCount, UpdatedAt: record.UpdatedAt.UTC(),
		})
	}
	return page, nil
}

func validDeliveryLineageRecords(records []DeliveryLineageStoreRecord) bool {
	for index, record := range records {
		if record.TaskID <= 0 || !validTaskStatus(record.State) || record.AttemptCount < 0 || record.UpdatedAt.IsZero() || record.SameTimestampCount < 1 {
			return false
		}
		if index > 0 {
			previous := records[index-1]
			if record.UpdatedAt.After(previous.UpdatedAt) {
				return false
			}
			if record.UpdatedAt.Equal(previous.UpdatedAt) && outboundDeliveryLineageID(record.TaskID) < outboundDeliveryLineageID(previous.TaskID) {
				return false
			}
		}
	}
	for start := 0; start < len(records); {
		end := start + 1
		for end < len(records) && records[end].UpdatedAt.Equal(records[start].UpdatedAt) {
			end++
		}
		groupSize := int64(end - start)
		for _, record := range records[start:end] {
			if record.SameTimestampCount != groupSize {
				return false
			}
		}
		start = end
	}
	return true
}

func outboundDeliveryLineageID(taskID TaskID) string {
	return "outbound-task:" + strconv.FormatInt(int64(taskID), 10)
}

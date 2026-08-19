package store

import (
	"context"

	outboundapp "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/app"
	outbounddb "github.com/qianlan33333-png/AI-CRM-v2/internal/outbound/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// DeliveryLineageRepository reads only the four outbound_tasks processing
// columns declared by the delivery-lineage contract.
type DeliveryLineageRepository struct{}

func NewDeliveryLineageRepository() *DeliveryLineageRepository { return &DeliveryLineageRepository{} }

func (*DeliveryLineageRepository) ListDeliveryLineage(ctx context.Context, limit int32) ([]outboundapp.DeliveryLineageStoreRecord, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := outbounddb.New(tx).ListOutboundDeliveryLineage(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]outboundapp.DeliveryLineageStoreRecord, len(rows))
	for index, row := range rows {
		result[index] = outboundapp.DeliveryLineageStoreRecord{
			TaskID: outboundapp.TaskID(row.ID), State: outboundapp.TaskStatus(row.Status), AttemptCount: row.AttemptCount,
			UpdatedAt: outboundTime(row.StatusUpdatedAt), SameTimestampCount: row.SameTimestampCount,
		}
	}
	return result, nil
}

package store

import (
	"context"

	eventapp "github.com/qianlan33333-png/AI-CRM-v2/internal/events/app"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventdb "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store/generated"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// DeliveryLineageRepository reads only event_deliveries processing columns.
// In particular it never joins event_log, whose payload is outside this API.
type DeliveryLineageRepository struct{}

func NewDeliveryLineageRepository() *DeliveryLineageRepository { return &DeliveryLineageRepository{} }

func (*DeliveryLineageRepository) ListDeliveryLineage(ctx context.Context, limit int32) ([]eventapp.DeliveryLineageStoreRecord, error) {
	tx, err := platformstore.TxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := eventdb.New(tx).ListEventDeliveryLineage(ctx, limit)
	if err != nil {
		return nil, err
	}
	result := make([]eventapp.DeliveryLineageStoreRecord, len(rows))
	for index, row := range rows {
		result[index] = eventapp.DeliveryLineageStoreRecord{
			EventID: eventport.EventID(row.EventID), Consumer: row.Consumer, State: eventport.DeliveryStatus(row.Status),
			AttemptCount: row.AttemptCount, UpdatedAt: row.UpdatedAt.Time, SameTimestampCount: row.SameTimestampCount,
		}
	}
	return result, nil
}

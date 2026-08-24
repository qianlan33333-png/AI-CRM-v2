package acceptancefixture

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	eventstore "github.com/qianlan33333-png/AI-CRM-v2/internal/events/store"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

// AppendCustomerSafeExportCreated records the Events-owned fact used by the
// isolated Contact completion-invariant acceptance fixture.
func AppendCustomerSafeExportCreated(ctx context.Context, pool *pgxpool.Pool, exportID string, receiptID int64, recordCount int, filterDigest []byte) error {
	if pool == nil || exportID == "" || receiptID < 1 || recordCount < 0 || len(filterDigest) != 32 {
		return fmt.Errorf("valid customer safe export event fixture required")
	}
	payload, err := json.Marshal(struct {
		ExportID     string `json:"export_id"`
		RecordCount  int    `json:"record_count"`
		FilterDigest string `json:"filter_digest"`
	}{exportID, recordCount, fmt.Sprintf("%x", filterDigest)})
	if err != nil {
		return fmt.Errorf("encode customer safe export acceptance fact: %w", err)
	}
	return platformstore.NewUnitOfWork(pool).Within(ctx, func(txCtx context.Context) error {
		_, appendErr := eventstore.NewAppender().Append(txCtx, eventport.Event{Type: eventport.EvCustomerSafeExportCreated, Payload: payload, OccurredAt: time.Now().UTC(), IdempotencyKey: fmt.Sprintf("customer-safe-export:%d", receiptID)})
		return appendErr
	})
}

package port

import (
	"context"
	"time"
)

// DeliveryLineageItem is the Events domain's redacted processing projection.
// It intentionally excludes the event payload, event type, and consumer.
type DeliveryLineageItem struct {
	LineageID     string
	InternalState string
	AttemptCount  int32
	UpdatedAt     time.Time
}

// DeliveryLineagePage is a bounded page that can only be consumed when it is
// complete with respect to its final equal-timestamp group.
type DeliveryLineagePage struct {
	Items    []DeliveryLineageItem
	Complete bool
}

// DeliveryLineageReader is a read-only public Events port. It never exposes
// event IDs or consumer names to callers.
type DeliveryLineageReader interface {
	ListDeliveryLineage(context.Context, int32) (DeliveryLineagePage, error)
}

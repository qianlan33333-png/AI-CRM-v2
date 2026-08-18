// Package port exposes outbound's deliberately redacted delivery-lineage read
// boundary. It never returns recipient, customer, owner, payload, provider, or
// failure-detail facts.
package port

import (
	"context"
	"time"
)

// DeliveryLineageItem is a stable internal outbound processing observation.
// Its lineage ID is intentionally the only task identifier exposed outside
// the Outbound domain.
type DeliveryLineageItem struct {
	LineageID     string
	InternalState string
	AttemptCount  int32
	UpdatedAt     time.Time
}

// DeliveryLineagePage is a bounded, newest-first local page. Complete is
// false when the bounded page cuts through an equal-timestamp group and the
// caller could not form a deterministic global ordering safely.
type DeliveryLineagePage struct {
	Items    []DeliveryLineageItem
	Complete bool
}

// DeliveryLineageReader is a read-only cross-domain projection. limit is the
// complete bounded source window required by the composition layer.
type DeliveryLineageReader interface {
	ListDeliveryLineage(context.Context, int32) (DeliveryLineagePage, error)
}

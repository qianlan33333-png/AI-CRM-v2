package port

import "context"

// AdminDetailSnapshot is the bounded, read-only source projection for one
// internal event. Found is false only when event_log has no matching row.
type AdminDetailSnapshot struct {
	Found      bool
	Event      AdminReadEvent
	Deliveries []AdminReadDelivery
}

// AdminDetailRepository reads one event and all of its local delivery rows in
// one read-only transaction. It deliberately exposes no payload or runtime
// delivery fields.
type AdminDetailRepository interface {
	Read(context.Context, EventID) (AdminDetailSnapshot, error)
}

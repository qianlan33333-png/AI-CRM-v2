package port

import (
	"context"
	"time"
)

type CloudAuditFilter struct {
	TraceID   string `json:"trace_id"`
	SessionID string `json:"session_id"`
	Limit     int32  `json:"limit"`
}

type CloudAuditFact struct {
	EventID        EventID   `json:"event_id"`
	EventType      string    `json:"event_type"`
	OccurredAt     time.Time `json:"occurred_at"`
	Dispatched     bool      `json:"dispatched"`
	Pending        int64     `json:"pending"`
	Processing     int64     `json:"processing"`
	Completed      int64     `json:"completed"`
	FinalFailed    int64     `json:"final_failed"`
	OutcomeUnknown int64     `json:"outcome_unknown"`
}

// CloudAuditRepository returns only local event and delivery facts selected by
// exact trace/session identifiers. It never returns the event payload itself.
type CloudAuditRepository interface {
	ListCloudAudit(context.Context, CloudAuditFilter) ([]CloudAuditFact, error)
}

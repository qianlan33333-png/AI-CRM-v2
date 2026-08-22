package campaign

import (
	"context"
	"encoding/json"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	"strings"
)

type EventLogAdapter struct{ appender eventport.Appender }

func NewEventLogAdapter(appender eventport.Appender) (*EventLogAdapter, error) {
	if appender == nil {
		return nil, ErrUnavailable
	}
	return &EventLogAdapter{appender}, nil
}
func (a *EventLogAdapter) Append(ctx context.Context, event AuditEvent) error {
	if a == nil || a.appender == nil {
		return ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		CampaignCode string `json:"campaign_code"`
		ActorID      int64  `json:"actor_id"`
	}{event.CampaignCode, event.ActorID})
	if err != nil {
		return ErrUnavailable
	}
	// event_log has a globally unique idempotency key. A batch command writes
	// one audit event per campaign, so preserve its per-campaign identity rather
	// than making the second item collide with the first.
	key := strings.Join([]string{event.IdempotencyKey, event.Type, event.CampaignCode}, ":")
	_, err = a.appender.Append(ctx, eventport.Event{Type: eventport.EvCloudCampaignFact, Payload: payload, OccurredAt: event.OccurredAt, IdempotencyKey: key})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

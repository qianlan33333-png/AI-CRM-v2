package campaign

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	"strconv"
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
		AuditType    string `json:"audit_type"`
		CampaignCode string `json:"campaign_code"`
		ActorID      int64  `json:"actor_id"`
	}{event.Type, event.CampaignCode, event.ActorID})
	if err != nil {
		return ErrUnavailable
	}
	// event_log has a globally unique idempotency key. A batch command writes
	// one audit event per campaign, so preserve its per-campaign identity rather
	// than making the second item collide with the first.
	keyMaterial := strings.Join([]string{strconv.FormatInt(event.ActorID, 10), event.IdempotencyKey, event.Type, event.CampaignCode}, ":")
	digest := sha256.Sum256([]byte(keyMaterial))
	key := hex.EncodeToString(digest[:])
	_, err = a.appender.Append(ctx, eventport.Event{Type: eventport.EvCloudCampaignFact, Payload: payload, OccurredAt: event.OccurredAt, IdempotencyKey: key})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

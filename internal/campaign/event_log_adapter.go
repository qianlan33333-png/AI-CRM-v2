package campaign

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	eventport "github.com/qianlan33333-png/AI-CRM-v2/internal/events/port"
	"strconv"
	"strings"
	"time"
)

type EventLogAdapter struct{ appender eventport.Appender }

// TouchPlanCreatedAuditEvent is the immutable Campaign snapshot audit payload.
// Its EventLog fact is deliberately bound to the existing Campaign delivery
// consumer, which only completes the local Events delivery receipt. It never
// creates an Outbound task or invokes a provider.
type TouchPlanCreatedAuditEvent struct {
	PlanID         string
	CampaignCode   string
	OwnerActorID   int64
	TargetDigest   string
	TargetCount    int32
	ContentDigest  string
	OccurredAt     time.Time
	IdempotencyKey string
}

// TouchPlanReviewAuditEvent keeps review decisions as Campaign-owned local
// facts. The dispatcher uses the existing EvCloudCampaignFact binding, whose
// only consumer completes an internal Events delivery receipt.
type TouchPlanReviewAuditEvent struct {
	AuditType      string
	PlanID         string
	CampaignCode   string
	ReviewVersion  int64
	ActorID        int64
	OccurredAt     time.Time
	IdempotencyKey string
}

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
	_, err = a.appendCampaignFact(ctx, payload, event.OccurredAt, strings.Join([]string{strconv.FormatInt(event.ActorID, 10), event.IdempotencyKey, event.Type, event.CampaignCode}, ":"))
	return err
}

// AppendTouchPlanCreated appends the event record within the caller's UoW.
// Dispatch happens only after commit through the existing Campaign Fact
// delivery binding, whose consumer performs local receipt completion only.
func (a *EventLogAdapter) AppendTouchPlanCreated(ctx context.Context, event TouchPlanCreatedAuditEvent) (eventport.EventID, error) {
	if a == nil || a.appender == nil || !ValidDraftTouchPlanID(event.PlanID) || !validCode(event.CampaignCode) || event.OwnerActorID < 1 ||
		event.TargetCount < 1 || !validAuditDigest(event.TargetDigest) || !validAuditDigest(event.ContentDigest) ||
		event.OccurredAt.IsZero() || strings.TrimSpace(event.IdempotencyKey) == "" {
		return 0, ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		AuditType     string `json:"audit_type"`
		PlanID        string `json:"plan_id"`
		CampaignCode  string `json:"campaign_code"`
		OwnerActorID  int64  `json:"owner_actor_id"`
		TargetDigest  string `json:"target_digest"`
		TargetCount   int32  `json:"target_count"`
		ContentDigest string `json:"content_digest"`
	}{"touch_plan_created", event.PlanID, event.CampaignCode, event.OwnerActorID, event.TargetDigest, event.TargetCount, event.ContentDigest})
	if err != nil {
		return 0, ErrUnavailable
	}
	return a.appendCampaignFact(ctx, payload, event.OccurredAt, strings.Join([]string{"campaign.touch_plan.event.v1", event.IdempotencyKey}, "\x00"))
}

func (a *EventLogAdapter) AppendTouchPlanReview(ctx context.Context, event TouchPlanReviewAuditEvent) (eventport.EventID, error) {
	if a == nil || a.appender == nil || !ValidReviewAuditType(event.AuditType) || !ValidTouchPlanReviewID(event.PlanID) || !validCode(event.CampaignCode) || event.ReviewVersion < 1 || event.ActorID < 1 ||
		event.OccurredAt.IsZero() || !event.OccurredAt.Equal(event.OccurredAt.UTC().Truncate(time.Microsecond)) || strings.TrimSpace(event.IdempotencyKey) == "" {
		return 0, ErrUnavailable
	}
	payload, err := json.Marshal(struct {
		AuditType     string `json:"audit_type"`
		PlanID        string `json:"plan_id"`
		CampaignCode  string `json:"campaign_code"`
		ReviewVersion int64  `json:"review_version"`
		ActorID       int64  `json:"actor_id"`
	}{event.AuditType, event.PlanID, event.CampaignCode, event.ReviewVersion, event.ActorID})
	if err != nil {
		return 0, ErrUnavailable
	}
	return a.appendCampaignFact(ctx, payload, event.OccurredAt, strings.Join([]string{"campaign.touch_plan.review.v1", event.IdempotencyKey}, "\x00"))
}

func (a *EventLogAdapter) appendCampaignFact(ctx context.Context, payload []byte, occurredAt time.Time, keyMaterial string) (eventport.EventID, error) {
	digest := sha256.Sum256([]byte(keyMaterial))
	eventID, err := a.appender.Append(ctx, eventport.Event{
		Type: eventport.EvCloudCampaignFact, Payload: payload, OccurredAt: occurredAt.UTC(), IdempotencyKey: hex.EncodeToString(digest[:]),
	})
	if err != nil || eventID < 1 {
		return 0, ErrUnavailable
	}
	return eventID, nil
}

func validAuditDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

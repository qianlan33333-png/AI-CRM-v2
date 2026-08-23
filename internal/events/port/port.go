// Package port freezes the cross-domain transactional event append contract.
package port

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type CustomerID int64
type EventID int64

const (
	EvCustomerAdded   = "customer.added"
	EvCustomerDeleted = "customer.deleted"
	// EvCustomerUpdated is required by the frozen updateCustomer operation and
	// the same-transaction event rule. It is additive to the original v1 list.
	EvCustomerUpdated           = "customer.updated"
	EvCustomerMerged            = "customer.merged"
	EvTagApplied                = "customer.tag_applied"
	EvTagRemoved                = "customer.tag_removed"
	EvStageChanged              = "customer.stage_changed"
	EvSurveySubmitted           = "survey.submitted"
	EvOutboundAccepted          = "outbound.accepted"
	EvOutboundSent              = "outbound.sent"
	EvOutboundFailed            = "outbound.failed"
	EvAutomationTriggered       = "automation.triggered"
	EvProductCreated            = "product.created"
	EvProductUpdated            = "product.updated"
	EvProductEntitlementGranted = "product.entitlement_granted"
	EvProductEntitlementRevoked = "product.entitlement_revoked"
	EvMediaImageCreated         = "media.image_created"
	EvMediaGroupInviteCreated   = "media.group_invite_created"
	EvMediaGroupInviteUpdated   = "media.group_invite_updated"
	EvMediaGroupInviteArchived  = "media.group_invite_archived"
	EvSurveyCreated             = "survey.created"
	EvSurveyUpdated             = "survey.updated"
	EvSurveyDeleted             = "survey.deleted"
	EvChannelCreated            = "channel.created"
	EvChannelUpdated            = "channel.updated"
	EvCouponCreated             = "coupon.created"
	EvCouponUpdated             = "coupon.updated"
	EvCouponPublished           = "coupon.published"
	EvCouponStopped             = "coupon.stopped"
	EvCouponArchived            = "coupon.archived"
	EvCouponDeleted             = "coupon.deleted"
	EvCouponCopied              = "coupon.copied"
	EvCouponClaimed             = "coupon.claimed"
	EvOrderExportCreated        = "order.export_created"
	EvOrderRefundRequested      = "order.refund_requested"
	EvOrderEffectRetryRequested = "order.external_effect_retry_requested"
	// EvOperationCycleFact is a local lifecycle fact. Its delivery consumer
	// only completes the durable receipt; it never invokes a provider.
	EvOperationCycleFact = "operation_cycle.fact_recorded"
	EvCloudCampaignFact  = "cloud_campaign.fact_recorded"
	// EvGroupOpsPlanUpdated records only local Group Ops configuration. No
	// dispatcher binding is registered for it, so it cannot send a group
	// message, invoke a webhook, or call a provider.
	EvGroupOpsPlanUpdated = "group_ops.plan_updated"
	// EvCustomerContactPolicyChanged records a Contact-owned local policy
	// mutation. It has no delivery binding and cannot invoke a provider.
	EvCustomerContactPolicyChanged = "customer.contact_policy_changed"

	ConsumerAutomationTagTrigger = "automation.tag-trigger.v1"
	ConsumerStatsTagApplied      = "stats.tag-applied.v1"
	ConsumerOperationCycleFact   = "operation-cycle.fact.v1"
	ConsumerCloudCampaignFact    = "cloud-campaign.fact.v1"
	DeliveryJobKind              = "events_deliver"
)

// EvServicePeriodMemberChanged records only a local service-period member
// fact. It has no delivery binding and cannot invoke payment or a provider.
const EvServicePeriodMemberChanged = "service_period_member.changed"

var (
	ErrInvalidEvent           = errors.New("invalid event")
	ErrIdempotencyConflict    = errors.New("event idempotency conflict")
	ErrInvalidDelivery        = errors.New("invalid event delivery")
	ErrDeliveryLeaseActive    = errors.New("event delivery lease is active")
	ErrDeliveryPoison         = errors.New("poison event delivery")
	ErrDeliveryOutcomeUnknown = errors.New("event delivery outcome is unknown")
)

type Event struct {
	Type           string
	CustomerID     CustomerID
	Payload        json.RawMessage
	OccurredAt     time.Time
	IdempotencyKey string
}

type CustomerMergeMode string

const (
	CustomerMergeAuto   CustomerMergeMode = "auto"
	CustomerMergeManual CustomerMergeMode = "manual"
)

type CustomerMergedPayload struct {
	PrimaryCustomerID CustomerID        `json:"primary_customer_id"`
	MergedCustomerID  CustomerID        `json:"merged_customer_id"`
	MergeAuditID      int64             `json:"merge_audit_id"`
	Mode              CustomerMergeMode `json:"mode"`
	PolicyVersion     string            `json:"policy_version"`
}

// Record is the immutable event fact loaded by a River delivery job.
type Record struct {
	ID EventID
	Event
}

// Subscriber consumes committed event facts. Implementations must persistently
// deduplicate by Record.ID (or a domain-specific stable key) because delivery is
// at least once. Multiple subscribers may handle the same event type.
type Subscriber interface {
	EventTypes() []string
	Consume(context.Context, Record) error
}

type DeliveryStatus string

const (
	DeliveryPending        DeliveryStatus = "pending"
	DeliveryProcessing     DeliveryStatus = "processing"
	DeliveryCompleted      DeliveryStatus = "completed"
	DeliveryFinalFailed    DeliveryStatus = "final_failed"
	DeliveryOutcomeUnknown DeliveryStatus = "outcome_unknown"
)

type DeliveryJobArgs struct {
	EventID  int64  `json:"event_id"`
	Consumer string `json:"consumer,omitempty"`
}

func (DeliveryJobArgs) Kind() string { return DeliveryJobKind }

type DeliveryBinding struct {
	EventType string
	Consumer  string
}

type DeliveryClaim struct {
	Record   Record
	Consumer string
	Owner    string
	Status   DeliveryStatus
	Attempt  int32
}

type DeliverySubscriber interface {
	Consumer() string
	EventTypes() []string
	ConsumeDelivery(context.Context, DeliveryClaim) error
}

type DeliveryAcceptor interface {
	Accept(context.Context, EventID, string) error
}

type DeliveryCompleter interface {
	Complete(context.Context, EventID, string, string) error
}

type DeliveryRuntime interface {
	Load(context.Context, EventID) (Record, error)
	Claim(context.Context, EventID, string, string, time.Duration) (DeliveryClaim, error)
	Retry(context.Context, EventID, string, string, string) error
	FinalFail(context.Context, EventID, string, string, string) error
	OutcomeUnknown(context.Context, EventID, string, string, string) error
}

func PoisonDelivery(err error) error {
	return errors.Join(ErrDeliveryPoison, err)
}

func UnknownDeliveryOutcome(err error) error {
	return errors.Join(ErrDeliveryOutcomeUnknown, err)
}

// Appender only persists the event in the transaction supplied by UnitOfWork.
// It does not dispatch work or call an external service.
type Appender interface {
	Append(context.Context, Event) (EventID, error)
}

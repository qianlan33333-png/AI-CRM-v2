package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RecipientReviewAuditMessageOverridden = "recipient_message_overridden"
	RecipientReviewAuditApproved          = "recipient_approved"
	RecipientReviewAuditRejected          = "recipient_rejected"
	MaximumCampaignMemberPage             = 100
)

type TouchPlanRecipientReviewStatus string

const (
	TouchPlanRecipientReviewPending  TouchPlanRecipientReviewStatus = "pending_review"
	TouchPlanRecipientReviewApproved TouchPlanRecipientReviewStatus = "approved"
	TouchPlanRecipientReviewRejected TouchPlanRecipientReviewStatus = "rejected"
)

func (value TouchPlanRecipientReviewStatus) Valid() bool {
	return value == TouchPlanRecipientReviewPending || value == TouchPlanRecipientReviewApproved || value == TouchPlanRecipientReviewRejected
}

// TouchPlanRecipientReview is a local, plan-scoped reviewer sidecar. Its
// safety facts deliberately rule out handoff creation, runtime execution, and
// Provider delivery evidence.
type TouchPlanRecipientReview struct {
	PlanID           string
	CampaignCode     string
	CustomerID       int64
	MessageOverride  string
	Status           TouchPlanRecipientReviewStatus
	Version          int64
	UpdatedByActorID int64
	UpdatedAt        time.Time
	Safety           InitiationSafety
}

// CampaignMemberStatus is a Campaign-owned projection over the newest
// immutable touch plan. CustomerID remains a canonical local OneID snapshot;
// it is not a Customer, Identity, send, or delivery fact.
type CampaignMemberStatus struct {
	PlanID     string                         `json:"plan_id"`
	CustomerID int64                          `json:"customer_id"`
	Status     TouchPlanRecipientReviewStatus `json:"status"`
}

type CampaignMemberStatusSnapshot struct {
	PlanID string
	Items  []CampaignMemberStatus
	Total  int64
}

type CampaignMemberStatusPage struct {
	PlanID string                 `json:"plan_id,omitempty"`
	Items  []CampaignMemberStatus `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int32                  `json:"limit"`
	Offset int32                  `json:"offset"`
	Safety InitiationSafety       `json:"safety"`
}

type SaveTouchPlanRecipientMessageOverrideCommand struct {
	PlanID                   string
	CampaignCode             string
	CustomerID               int64
	ExpectedPlanVersion      int64
	ExpectedRecipientVersion int64
	MessageOverride          string
	Actor                    Actor
	IdempotencyKey           string
}

type DecideTouchPlanRecipientCommand struct {
	PlanID                   string
	CampaignCode             string
	CustomerID               int64
	ExpectedPlanVersion      int64
	ExpectedRecipientVersion int64
	Actor                    Actor
	IdempotencyKey           string
}

type TouchPlanRecipientReviewReceiptState string

const (
	TouchPlanRecipientReviewReceiptReserved  TouchPlanRecipientReviewReceiptState = "reserved"
	TouchPlanRecipientReviewReceiptCompleted TouchPlanRecipientReviewReceiptState = "completed"
)

type TouchPlanRecipientReviewResult struct {
	Review  TouchPlanRecipientReview
	EventID int64
}

type TouchPlanRecipientReviewReceipt struct {
	ID            int64
	ActorID       int64
	Operation     string
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
	CampaignCode  string
	CustomerID    int64
	State         TouchPlanRecipientReviewReceiptState
	Result        *TouchPlanRecipientReviewResult
}

type TouchPlanRecipientReviewEvent struct {
	AuditType        string
	PlanID           string
	CampaignCode     string
	CustomerID       int64
	RecipientVersion int64
	ActorID          int64
	OccurredAt       time.Time
	IdempotencyKey   string
}

func ValidTouchPlanRecipientMessageOverride(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.RuneCountInString(value) <= MaximumStepContentRunes
}

func ValidTouchPlanRecipientReview(value TouchPlanRecipientReview) bool {
	return ValidTouchPlanReviewID(value.PlanID) && validCode(value.CampaignCode) && value.CustomerID > 0 && value.Status.Valid() && value.Version > 0 && (value.MessageOverride == "" || ValidTouchPlanRecipientMessageOverride(value.MessageOverride)) && value.UpdatedByActorID > 0 && validReviewTime(value.UpdatedAt) && value.Safety == LocalInitiationSafety()
}

func TouchPlanRecipientReviewPayloadDigest(operation, campaignCode, planID string, customerID, expectedPlanVersion, expectedRecipientVersion int64, messageOverride string) [sha256.Size]byte {
	messageDigest := sha256.Sum256([]byte(messageOverride))
	return sha256.Sum256([]byte(operation + "\x00" + campaignCode + "\x00" + planID + "\x00" + strconv.FormatInt(customerID, 10) + "\x00" + strconv.FormatInt(expectedPlanVersion, 10) + "\x00" + strconv.FormatInt(expectedRecipientVersion, 10) + "\x00" + hex.EncodeToString(messageDigest[:])))
}

func ValidTouchPlanRecipientReviewAuditType(value string) bool {
	return value == RecipientReviewAuditMessageOverridden || value == RecipientReviewAuditApproved || value == RecipientReviewAuditRejected
}

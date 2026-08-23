package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

const (
	MaximumReviewRecipientPage   = 100
	HandoffPendingOutboundAccept = "pending_outbound_acceptance"
	ReviewAuditSubmitted         = "touch_plan_submitted"
	ReviewAuditApproved          = "approved"
	ReviewAuditRejected          = "rejected"
	ReviewAuditHandoffCreated    = "handoff_created"
)

type TouchPlanReviewStatus string

const (
	TouchPlanReviewDraft    TouchPlanReviewStatus = "draft"
	TouchPlanReviewPending  TouchPlanReviewStatus = "pending_review"
	TouchPlanReviewApproved TouchPlanReviewStatus = "approved"
	TouchPlanReviewRejected TouchPlanReviewStatus = "rejected"
)

type TouchPlanReview struct {
	PlanID             string
	CampaignCode       string
	Status             TouchPlanReviewStatus
	Version            int64
	SubmittedByActorID int64
	SubmittedAt        time.Time
	ReviewedByActorID  int64
	ReviewedAt         time.Time
	ConfirmationDigest [sha256.Size]byte
}

type TouchPlanHandoff struct {
	PlanID                    string
	CampaignCode              string
	ReviewVersion             int64
	Status                    string
	CreatedAt                 time.Time
	LocalOnly                 bool
	ProviderExecutionEligible bool
	RealExternalCallExecuted  bool
	DeliveryProven            bool
}

type SubmitTouchPlanReviewCommand struct {
	PlanID          string
	CampaignCode    string
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
}

type DecideTouchPlanReviewCommand struct {
	PlanID          string
	CampaignCode    string
	ExpectedVersion int64
	Actor           Actor
	IdempotencyKey  string
	Confirmation    string
}

type TouchPlanRecipient struct {
	PlanID     string
	CustomerID int64
}
type TouchPlanRecipientKeyset struct {
	PlanID     string
	CustomerID int64
}
type TouchPlanRecipientPage struct {
	Items []TouchPlanRecipient
	Next  *TouchPlanRecipientKeyset
}

type TouchPlanReviewReceiptState string

const (
	TouchPlanReviewReceiptReserved  TouchPlanReviewReceiptState = "reserved"
	TouchPlanReviewReceiptCompleted TouchPlanReviewReceiptState = "completed"
)

type TouchPlanReviewResult struct {
	Review   TouchPlanReview
	Handoff  *TouchPlanHandoff
	EventIDs []int64
}
type TouchPlanReviewReceipt struct {
	ID            int64
	ActorID       int64
	Operation     string
	KeyDigest     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	PlanID        string
	CampaignCode  string
	State         TouchPlanReviewReceiptState
	Result        *TouchPlanReviewResult
}
type TouchPlanReviewEvent struct {
	AuditType      string
	PlanID         string
	CampaignCode   string
	ReviewVersion  int64
	ActorID        int64
	OccurredAt     time.Time
	IdempotencyKey string
}

func ValidTouchPlanReviewID(value string) bool { return ValidDraftTouchPlanID(value) }
func (status TouchPlanReviewStatus) Valid() bool {
	return status == TouchPlanReviewDraft || status == TouchPlanReviewPending || status == TouchPlanReviewApproved || status == TouchPlanReviewRejected
}
func validReviewTime(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Equal(value.UTC().Truncate(time.Microsecond))
}
func ValidTouchPlanReview(value TouchPlanReview) bool {
	if !ValidTouchPlanReviewID(value.PlanID) || !validCode(value.CampaignCode) || !value.Status.Valid() || value.Version < 1 {
		return false
	}
	switch value.Status {
	case TouchPlanReviewDraft:
		return value.Version == 1 && value.SubmittedByActorID == 0 && value.SubmittedAt.IsZero() && value.ReviewedByActorID == 0 && value.ReviewedAt.IsZero() && value.ConfirmationDigest == [sha256.Size]byte{}
	case TouchPlanReviewPending:
		return value.SubmittedByActorID > 0 && validReviewTime(value.SubmittedAt) && value.ReviewedByActorID == 0 && value.ReviewedAt.IsZero() && value.ConfirmationDigest == [sha256.Size]byte{}
	case TouchPlanReviewApproved, TouchPlanReviewRejected:
		return value.SubmittedByActorID > 0 && validReviewTime(value.SubmittedAt) && value.ReviewedByActorID > 0 && validReviewTime(value.ReviewedAt) && value.ConfirmationDigest != [sha256.Size]byte{}
	}
	return false
}
func ValidTouchPlanHandoff(value TouchPlanHandoff) bool {
	return ValidTouchPlanReviewID(value.PlanID) && validCode(value.CampaignCode) && value.ReviewVersion > 1 && value.Status == HandoffPendingOutboundAccept && validReviewTime(value.CreatedAt) && value.LocalOnly && !value.ProviderExecutionEligible && !value.RealExternalCallExecuted && !value.DeliveryProven
}
func ReviewConfirmation(operation, planID string) string {
	return strings.ToUpper(operation) + " " + planID
}
func ReviewConfirmationDigest(value string) [sha256.Size]byte { return sha256.Sum256([]byte(value)) }
func ReviewPayloadDigest(operation, campaignCode, planID string, expectedVersion int64, confirmation string) [sha256.Size]byte {
	confirmationDigest := ReviewConfirmationDigest(confirmation)
	return sha256.Sum256([]byte(operation + "\x00" + campaignCode + "\x00" + planID + "\x00" + strconv.FormatInt(expectedVersion, 10) + "\x00" + hex.EncodeToString(confirmationDigest[:])))
}
func ValidReviewAuditType(value string) bool {
	return value == ReviewAuditSubmitted || value == ReviewAuditApproved || value == ReviewAuditRejected || value == ReviewAuditHandoffCreated
}

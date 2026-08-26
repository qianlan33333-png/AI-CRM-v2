package outbound

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrCampaignDispatchInvalid     = errors.New("invalid outbound campaign dispatch")
	ErrCampaignDispatchConflict    = errors.New("outbound campaign dispatch conflict")
	ErrCampaignDispatchUnavailable = errors.New("outbound campaign dispatch unavailable")
)

type CampaignDispatchState string

const (
	CampaignDispatchAccepted        CampaignDispatchState = "accepted"
	CampaignDispatchQueued          CampaignDispatchState = "queued"
	CampaignDispatchAttempted       CampaignDispatchState = "attempted"
	CampaignDispatchExecuted        CampaignDispatchState = "executed"
	CampaignDispatchOutcomeUnknown  CampaignDispatchState = "outcome_unknown"
	CampaignDispatchReconciled      CampaignDispatchState = "reconciled"
	CampaignDispatchRetryableFailed CampaignDispatchState = "retryable_failed"
	CampaignDispatchFinalFailed     CampaignDispatchState = "final_failed"
	CampaignDispatchBlocked         CampaignDispatchState = "blocked"
)

type CampaignDispatchSummary struct {
	HandoffID                                                                                                int64
	Blocked, Accepted, Queued, Attempted, Executed, OutcomeUnknown, Reconciled, RetryableFailed, FinalFailed int64
	ProviderExecutionEligible                                                                                bool
	BusinessCallDispatched                                                                                   bool
	RealExternalCallExecuted                                                                                 bool
	DeliveryProven                                                                                           bool
	UpdatedAt                                                                                                time.Time
}

func CampaignDispatchPayloadDigest(handoffID, customerID int64, stepIndex int32, content string) string {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.v1\x00" + strconv.FormatInt(handoffID, 10) + "\x00" + strconv.FormatInt(customerID, 10) + "\x00" + strconv.FormatInt(int64(stepIndex), 10) + "\x00" + content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AudienceCampaignDispatchPayloadDigest binds the exact sender and verified
// external identity selected before the effect is accepted. It is deliberately
// distinct from the historical v1 digest so a delayed Audience worker can
// never re-resolve a changed relationship owner.
func AudienceCampaignDispatchPayloadDigest(handoffID, customerID int64, stepIndex int32, content, senderUserID, externalUserID string) string {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.audience.v2\x00" + strconv.FormatInt(handoffID, 10) + "\x00" + strconv.FormatInt(customerID, 10) + "\x00" + strconv.FormatInt(int64(stepIndex), 10) + "\x00" + content + "\x00" + senderUserID + "\x00" + externalUserID))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func CampaignDispatchRecipientDigest(customerID int64) string {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.recipient.v1\x00" + strconv.FormatInt(customerID, 10)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func AudienceCampaignDispatchRecipientDigest(customerID int64, senderUserID, externalUserID string) string {
	sum := sha256.Sum256([]byte("outbound.campaign_dispatch.audience.recipient.v2\x00" + strconv.FormatInt(customerID, 10) + "\x00" + senderUserID + "\x00" + externalUserID))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidCampaignDispatchDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func ValidCampaignDispatchSummary(value CampaignDispatchSummary) bool {
	if value.HandoffID < 1 || value.Blocked < 0 || value.Accepted < 0 || value.Queued < 0 || value.Attempted < 0 || value.Executed < 0 || value.OutcomeUnknown < 0 || value.Reconciled < 0 || value.RetryableFailed < 0 || value.FinalFailed < 0 || value.UpdatedAt.IsZero() {
		return false
	}
	// A provider call fact is not delivery proof. Delivery may become true only
	// after an owner-specific protocol verifier records independent evidence.
	return (!value.RealExternalCallExecuted || value.BusinessCallDispatched) && (!value.DeliveryProven || (value.BusinessCallDispatched && value.RealExternalCallExecuted))
}

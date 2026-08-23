package outbound

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const (
	CampaignHandoffHeld              = "held"
	CampaignHandoffBlocked           = "blocked"
	CampaignHandoffPending           = "pending"
	CampaignEligibilityNotEvaluated  = "not_evaluated"
	CampaignEligibilityEligible      = "eligible"
	CampaignEligibilityInactive      = "inactive"
	CampaignEligibilityContactPolicy = "contact_policy"
	MaximumCampaignHandoffTargets    = 1000
	MaximumCampaignHandoffSteps      = 100
)

var (
	ErrCampaignHandoffInvalid             = errors.New("invalid outbound campaign handoff")
	ErrCampaignHandoffNotFound            = errors.New("outbound campaign handoff not found")
	ErrCampaignHandoffConflict            = errors.New("outbound campaign handoff conflict")
	ErrCampaignHandoffIdempotencyConflict = errors.New("outbound campaign handoff idempotency conflict")
	ErrCampaignHandoffUnavailable         = errors.New("outbound campaign handoff unavailable")
	ErrCampaignDispatchBlockedRedline     = errors.New("outbound campaign dispatch blocked redline")
)

var campaignCodePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)

type CampaignHandoffSafety struct {
	LocalOnly                 bool
	ProviderExecutionEligible bool
	RealExternalCallExecuted  bool
	DeliveryProven            bool
}

func LocalCampaignHandoffSafety() CampaignHandoffSafety {
	return CampaignHandoffSafety{LocalOnly: true}
}

type CampaignHandoffStep struct {
	Index        int32
	DelayMinutes int32
	Content      string
}

type CampaignHandoffLink struct {
	CustomerID     int64
	State          string
	Eligibility    string
	OutboundTaskID *int64
}

type AcceptedCampaignHandoff struct {
	ID            int64
	CampaignCode  string
	PlanID        string
	ReviewVersion int64
	SourceDigest  string
	TargetDigest  string
	ContentDigest string
	TargetCount   int32
	StepCount     int32
	Status        string
	AcceptedBy    int64
	AcceptedAt    time.Time
	Safety        CampaignHandoffSafety
	Steps         []CampaignHandoffStep
	Links         []CampaignHandoffLink
}

type CampaignHandoffSummary struct {
	ID                 int64
	CampaignCode       string
	PlanID             string
	ReviewVersion      int64
	Status             string
	TargetCount        int32
	StepCount          int32
	HeldCount          int32
	BlockedCount       int32
	PendingCount       int32
	NotEvaluatedCount  int32
	EligibleCount      int32
	InactiveCount      int32
	ContactPolicyCount int32
	AcceptedAt         time.Time
	Safety             CampaignHandoffSafety
}

func ValidCampaignHandoffIdentity(campaignCode, planID string) bool {
	if !campaignCodePattern.MatchString(campaignCode) || len(planID) != 68 || planID[:4] != "ctp_" {
		return false
	}
	_, err := hex.DecodeString(planID[4:])
	return err == nil
}

func ValidAcceptedCampaignHandoff(value AcceptedCampaignHandoff) bool {
	if value.ID < 1 || !ValidCampaignHandoffIdentity(value.CampaignCode, value.PlanID) || value.ReviewVersion < 3 ||
		!validDigest(value.SourceDigest) || !validDigest(value.TargetDigest) || !validDigest(value.ContentDigest) ||
		value.TargetCount < 1 || value.TargetCount > MaximumCampaignHandoffTargets || value.StepCount < 1 || value.StepCount > MaximumCampaignHandoffSteps ||
		value.TargetCount != int32(len(value.Links)) || value.StepCount != int32(len(value.Steps)) || value.Status != CampaignHandoffHeld ||
		value.AcceptedBy < 1 || !validCampaignHandoffTime(value.AcceptedAt) || value.Safety != LocalCampaignHandoffSafety() {
		return false
	}
	for index, step := range value.Steps {
		if step.Index != int32(index+1) || step.DelayMinutes < 0 || step.Content == "" {
			return false
		}
	}
	for index, link := range value.Links {
		if link.CustomerID < 1 || index > 0 && value.Links[index-1].CustomerID >= link.CustomerID ||
			link.State != CampaignHandoffHeld || link.Eligibility != CampaignEligibilityNotEvaluated || link.OutboundTaskID != nil {
			return false
		}
	}
	return true
}

func SummaryOf(value AcceptedCampaignHandoff) CampaignHandoffSummary {
	return CampaignHandoffSummary{
		ID: value.ID, CampaignCode: value.CampaignCode, PlanID: value.PlanID, ReviewVersion: value.ReviewVersion,
		Status: value.Status, TargetCount: value.TargetCount, StepCount: value.StepCount,
		HeldCount: value.TargetCount, NotEvaluatedCount: value.TargetCount,
		AcceptedAt: value.AcceptedAt, Safety: value.Safety,
	}
}

func ValidCampaignHandoffSummary(value CampaignHandoffSummary) bool {
	if value.ID < 1 || !ValidCampaignHandoffIdentity(value.CampaignCode, value.PlanID) || value.ReviewVersion < 3 ||
		value.Status != CampaignHandoffHeld || value.TargetCount < 1 || value.TargetCount > MaximumCampaignHandoffTargets ||
		value.StepCount < 1 || value.StepCount > MaximumCampaignHandoffSteps || !validCampaignHandoffTime(value.AcceptedAt) ||
		value.Safety != LocalCampaignHandoffSafety() {
		return false
	}
	return value.HeldCount >= 0 && value.BlockedCount >= 0 && value.PendingCount >= 0 &&
		value.NotEvaluatedCount >= 0 && value.EligibleCount >= 0 && value.InactiveCount >= 0 && value.ContactPolicyCount >= 0 &&
		value.HeldCount+value.BlockedCount+value.PendingCount == value.TargetCount &&
		value.NotEvaluatedCount+value.EligibleCount+value.InactiveCount+value.ContactPolicyCount == value.TargetCount
}

func CampaignHandoffPayloadDigest(campaignCode, planID string, expectedReviewVersion int64) [sha256.Size]byte {
	return sha256.Sum256([]byte("outbound.campaign_handoff.accept.v1\x00" + campaignCode + "\x00" + planID + "\x00" + strconv.FormatInt(expectedReviewVersion, 10)))
}

func CanonicalCampaignHandoffLinks(customerIDs []int64) ([]CampaignHandoffLink, bool) {
	if len(customerIDs) < 1 || len(customerIDs) > MaximumCampaignHandoffTargets {
		return nil, false
	}
	canonical := append([]int64(nil), customerIDs...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
	links := make([]CampaignHandoffLink, len(canonical))
	for index, customerID := range canonical {
		if customerID < 1 || index > 0 && canonical[index-1] == customerID {
			return nil, false
		}
		links[index] = CampaignHandoffLink{CustomerID: customerID, State: CampaignHandoffHeld, Eligibility: CampaignEligibilityNotEvaluated}
	}
	return links, true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validCampaignHandoffTime(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && value.Equal(value.Truncate(time.Microsecond))
}

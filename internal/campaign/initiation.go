package campaign

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaximumDraftTouchTargets       = 1_000
	MaximumDraftTouchPlanPageLimit = 100
)

var ErrBlockedRedline = errors.New("campaign initiation blocked redline")

type InitiationSourceKind string

const (
	InitiationSourceCustomerFilter         InitiationSourceKind = "customer_filter"
	InitiationSourceCustomerSelection      InitiationSourceKind = "customer_selection"
	InitiationSourceSegmentMembers         InitiationSourceKind = "segment_members"
	InitiationSourceAudiencePackageMembers InitiationSourceKind = "ai_audience_package_members"
)

func (kind InitiationSourceKind) Valid() bool {
	return kind == InitiationSourceCustomerFilter || kind == InitiationSourceCustomerSelection || kind == InitiationSourceSegmentMembers || kind == InitiationSourceAudiencePackageMembers
}

// InitiationSourceRequest contains only the local selector a caller may
// provide. The resolver, not the caller, supplies the persisted version and
// digest in InitiationSourceRef.
type InitiationSourceRequest struct {
	Kind              InitiationSourceKind `json:"kind"`
	CustomerIDs       []int64              `json:"customer_ids,omitempty"`
	SegmentID         int64                `json:"segment_id,omitempty"`
	AudiencePackageID int64                `json:"audience_package_id,omitempty"`
}

func (request InitiationSourceRequest) Valid() bool {
	switch request.Kind {
	case InitiationSourceCustomerFilter:
		// Current main has no saved, versioned Customer-filter fact. The app
		// returns ErrBlockedRedline for this otherwise closed selector.
		return len(request.CustomerIDs) == 0 && request.SegmentID == 0 && request.AudiencePackageID == 0
	case InitiationSourceCustomerSelection:
		// Customer List can create a plan directly, but must first provide a
		// unique local OneID selection. Campaign sorts it before reservation,
		// while duplicate IDs remain invalid rather than being deduplicated.
		return request.SegmentID == 0 && request.AudiencePackageID == 0 && ValidCustomerSelection(request.CustomerIDs)
	case InitiationSourceSegmentMembers:
		return len(request.CustomerIDs) == 0 && request.SegmentID > 0 && request.AudiencePackageID == 0
	case InitiationSourceAudiencePackageMembers:
		return len(request.CustomerIDs) == 0 && request.SegmentID == 0 && request.AudiencePackageID > 0
	default:
		return false
	}
}

func (request InitiationSourceRequest) Matches(ref InitiationSourceRef) bool {
	if !request.Valid() || !ref.Valid() || request.Kind != ref.Kind {
		return false
	}
	switch request.Kind {
	case InitiationSourceCustomerSelection:
		expected, valid := NewCustomerSelectionSourceRef(request.CustomerIDs)
		return valid && ref.CustomerSelection != nil && expected.CustomerSelection != nil && *ref.CustomerSelection == *expected.CustomerSelection
	case InitiationSourceSegmentMembers:
		return ref.Segment != nil && ref.Segment.SegmentID == request.SegmentID
	case InitiationSourceAudiencePackageMembers:
		return ref.AudiencePackage != nil && ref.AudiencePackage.PackageID == request.AudiencePackageID
	default:
		return false
	}
}

// ValidCustomerSelection accepts 1..MaximumDraftTouchTargets unique local
// OneIDs. Their order is normalized by CanonicalCustomerSelection.
func ValidCustomerSelection(customerIDs []int64) bool {
	if len(customerIDs) < 1 || len(customerIDs) > MaximumDraftTouchTargets {
		return false
	}
	seen := make(map[int64]struct{}, len(customerIDs))
	for _, customerID := range customerIDs {
		if customerID < 1 {
			return false
		}
		if _, duplicate := seen[customerID]; duplicate {
			return false
		}
		seen[customerID] = struct{}{}
	}
	return true
}

func CanonicalCustomerSelection(customerIDs []int64) ([]int64, bool) {
	if !ValidCustomerSelection(customerIDs) {
		return nil, false
	}
	result := append([]int64(nil), customerIDs...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result, true
}

type CustomerSelectionSourceFact struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (fact CustomerSelectionSourceFact) Valid() bool {
	return fact.ID == "local_selection" && fact.Version == "v1" && validInitiationDigest(fact.Digest)
}

// NewCustomerSelectionSourceRef authors the Campaign-local source fact for a
// direct Customer List selection. It never stores filter text or any external
// identity; its digest is over the canonical candidate OneID set.
func NewCustomerSelectionSourceRef(customerIDs []int64) (InitiationSourceRef, bool) {
	canonicalCustomerIDs, valid := CanonicalCustomerSelection(customerIDs)
	if !valid {
		return InitiationSourceRef{}, false
	}
	return InitiationSourceRef{
		Kind: InitiationSourceCustomerSelection,
		CustomerSelection: &CustomerSelectionSourceFact{
			ID: "local_selection", Version: "v1", Digest: CanonicalCustomerSelectionDigest(canonicalCustomerIDs),
		},
	}, true
}

func CanonicalCustomerSelectionDigest(customerIDs []int64) string {
	return canonicalMemberDigest("campaign.customer_selection.v1", customerIDs)
}

// SegmentMemberSourceFact is the authoritative local Segment member snapshot.
// Its watermark and digest are resolver-authored from the locked snapshot.
type SegmentMemberSourceFact struct {
	SegmentID               int64     `json:"segment_id"`
	MemberSnapshotWatermark time.Time `json:"member_snapshot_watermark"`
	Digest                  string    `json:"digest"`
}

func (fact SegmentMemberSourceFact) Valid() bool {
	return fact.SegmentID > 0 && validSourceWatermark(fact.MemberSnapshotWatermark) && validInitiationDigest(fact.Digest)
}

// NewSegmentMemberSourceRefFromSnapshot accepts only the digest authored by
// the Segment-owned transaction-bound snapshot seam. Campaign persists that
// closed source fact but never reads Segment tables itself.
func NewSegmentMemberSourceRefFromSnapshot(segmentID int64, watermark time.Time, digest string) (InitiationSourceRef, bool) {
	if segmentID < 1 || !validSourceWatermark(watermark) || !validInitiationDigest(digest) {
		return InitiationSourceRef{}, false
	}
	return InitiationSourceRef{
		Kind: InitiationSourceSegmentMembers,
		Segment: &SegmentMemberSourceFact{
			SegmentID: segmentID, MemberSnapshotWatermark: watermark.UTC(),
			Digest: digest,
		},
	}, true
}

// AudiencePackageMemberSourceFact binds the local AI Audience package version
// and the underlying member snapshot watermark/digest in one frozen source.
type AudiencePackageMemberSourceFact struct {
	PackageID               int64     `json:"package_id"`
	PackageVersion          int64     `json:"package_version"`
	MemberSnapshotWatermark time.Time `json:"member_snapshot_watermark"`
	Digest                  string    `json:"digest"`
}

func (fact AudiencePackageMemberSourceFact) Valid() bool {
	return fact.PackageID > 0 && fact.PackageVersion > 0 && validSourceWatermark(fact.MemberSnapshotWatermark) && validInitiationDigest(fact.Digest)
}

// NewAudiencePackageMemberSourceRefFromSnapshot preserves the package version
// and digest authored by the Segment-owned locked Audience package snapshot.
func NewAudiencePackageMemberSourceRefFromSnapshot(packageID, packageVersion int64, watermark time.Time, digest string) (InitiationSourceRef, bool) {
	if packageID < 1 || packageVersion < 1 || !validSourceWatermark(watermark) || !validInitiationDigest(digest) {
		return InitiationSourceRef{}, false
	}
	return InitiationSourceRef{
		Kind: InitiationSourceAudiencePackageMembers,
		AudiencePackage: &AudiencePackageMemberSourceFact{
			PackageID: packageID, PackageVersion: packageVersion, MemberSnapshotWatermark: watermark.UTC(),
			Digest: digest,
		},
	}, true
}

// InitiationSourceRef is a resolver-authored local source fact. It has a
// separate shape per source rather than forcing Customer, Segment, and
// Audience into a caller-supplied generic id/version tuple.
type InitiationSourceRef struct {
	Kind              InitiationSourceKind             `json:"kind"`
	CustomerSelection *CustomerSelectionSourceFact     `json:"customer_selection,omitempty"`
	Segment           *SegmentMemberSourceFact         `json:"segment,omitempty"`
	AudiencePackage   *AudiencePackageMemberSourceFact `json:"audience_package,omitempty"`
}

func (ref InitiationSourceRef) Valid() bool {
	switch ref.Kind {
	case InitiationSourceCustomerSelection:
		return ref.CustomerSelection != nil && ref.CustomerSelection.Valid() && ref.Segment == nil && ref.AudiencePackage == nil
	case InitiationSourceSegmentMembers:
		return ref.CustomerSelection == nil && ref.Segment != nil && ref.Segment.Valid() && ref.AudiencePackage == nil
	case InitiationSourceAudiencePackageMembers:
		return ref.CustomerSelection == nil && ref.Segment == nil && ref.AudiencePackage != nil && ref.AudiencePackage.Valid()
	default:
		return false
	}
}

func CanonicalInitiationSourceRef(ref InitiationSourceRef) InitiationSourceRef {
	result := InitiationSourceRef{Kind: ref.Kind}
	if ref.CustomerSelection != nil {
		fact := *ref.CustomerSelection
		result.CustomerSelection = &fact
	}
	if ref.Segment != nil {
		fact := *ref.Segment
		fact.MemberSnapshotWatermark = fact.MemberSnapshotWatermark.UTC()
		result.Segment = &fact
	}
	if ref.AudiencePackage != nil {
		fact := *ref.AudiencePackage
		fact.MemberSnapshotWatermark = fact.MemberSnapshotWatermark.UTC()
		result.AudiencePackage = &fact
	}
	return result
}

type ContentSnapshot struct {
	Steps  []Step `json:"steps"`
	Digest string `json:"content_digest"`
}

type CustomerTargetSnapshot struct {
	CustomerIDs []int64 `json:"customer_ids"`
	Digest      string  `json:"digest"`
}

type PreviewExclusionSummary struct {
	CandidateCount        int32 `json:"candidate_count"`
	ActiveCustomerCount   int32 `json:"active_customer_count"`
	InactiveExcludedCount int32 `json:"inactive_excluded_count"`
	PolicyExcludedCount   int32 `json:"policy_excluded_count"`
}

type InitiationSafety struct {
	LocalOnly                 bool `json:"local_only"`
	ProviderExecutionEligible bool `json:"provider_execution_eligible"`
	RuntimeExecuted           bool `json:"runtime_executed"`
	RealExternalCallExecuted  bool `json:"real_external_call_executed"`
	DeliveryProven            bool `json:"delivery_proven"`
}

func LocalInitiationSafety() InitiationSafety {
	return InitiationSafety{LocalOnly: true}
}

// DraftTouchPlan is Campaign-owned and stops at a draft review snapshot. It
// has no outbound, runtime, provider, recipient-delivery, or queue state.
type DraftTouchPlan struct {
	ID              string                  `json:"id"`
	CampaignCode    string                  `json:"campaign_code"`
	CampaignVersion int64                   `json:"campaign_version"`
	Source          InitiationSourceRef     `json:"source"`
	Targets         CustomerTargetSnapshot  `json:"targets"`
	Content         ContentSnapshot         `json:"content"`
	OwnerActorID    int64                   `json:"owner_actor_id"`
	Exclusions      PreviewExclusionSummary `json:"preview_exclusion_summary"`
	CreatedAt       time.Time               `json:"created_at"`
	Safety          InitiationSafety        `json:"safety"`
}

// DraftTouchPlanSummary is deliberately recipient-safe: it exposes counts and
// immutable digests, not the individual canonical customer IDs held by the
// Campaign-owned target snapshot. Per-recipient reads remain a later contract.
type DraftTouchPlanSummary struct {
	ID               string                  `json:"id"`
	CampaignCode     string                  `json:"campaign_code"`
	CampaignVersion  int64                   `json:"campaign_version"`
	Source           InitiationSourceRef     `json:"source"`
	TargetCount      int32                   `json:"target_count"`
	TargetDigest     string                  `json:"target_digest"`
	ContentStepCount int32                   `json:"content_step_count"`
	ContentDigest    string                  `json:"content_digest"`
	OwnerActorID     int64                   `json:"owner_actor_id"`
	Exclusions       PreviewExclusionSummary `json:"preview_exclusion_summary"`
	CreatedAt        time.Time               `json:"created_at"`
	Safety           InitiationSafety        `json:"safety"`
}

// DraftTouchPlanPage uses an opaque application-authored keyset cursor. It
// contains summaries only, never the individual target snapshot IDs.
type DraftTouchPlanPage struct {
	Items      []DraftTouchPlanSummary `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

func DraftTouchPlanSummaryOf(plan DraftTouchPlan) DraftTouchPlanSummary {
	return DraftTouchPlanSummary{
		ID:               plan.ID,
		CampaignCode:     plan.CampaignCode,
		CampaignVersion:  plan.CampaignVersion,
		Source:           CanonicalInitiationSourceRef(plan.Source),
		TargetCount:      int32(len(plan.Targets.CustomerIDs)),
		TargetDigest:     plan.Targets.Digest,
		ContentStepCount: int32(len(plan.Content.Steps)),
		ContentDigest:    plan.Content.Digest,
		OwnerActorID:     plan.OwnerActorID,
		Exclusions:       plan.Exclusions,
		CreatedAt:        plan.CreatedAt,
		Safety:           plan.Safety,
	}
}

func ValidDraftTouchPlanSummary(plan DraftTouchPlanSummary) bool {
	if !validPlanID(plan.ID) || !validCode(plan.CampaignCode) || !plan.Source.Valid() ||
		plan.CampaignVersion < 1 || plan.TargetCount < 1 || plan.TargetCount > MaximumDraftTouchTargets ||
		plan.ContentStepCount < 1 || plan.ContentStepCount > MaximumSteps ||
		!validInitiationDigest(plan.TargetDigest) || !validInitiationDigest(plan.ContentDigest) ||
		plan.OwnerActorID < 1 || plan.CreatedAt.IsZero() ||
		plan.Safety != LocalInitiationSafety() {
		return false
	}
	return plan.Exclusions.CandidateCount >= plan.TargetCount &&
		plan.Exclusions.ActiveCustomerCount >= plan.TargetCount &&
		plan.Exclusions.InactiveExcludedCount >= 0 && plan.Exclusions.PolicyExcludedCount >= 0 &&
		plan.Exclusions.CandidateCount == plan.TargetCount+plan.Exclusions.InactiveExcludedCount+plan.Exclusions.PolicyExcludedCount &&
		plan.Exclusions.ActiveCustomerCount == plan.TargetCount+plan.Exclusions.PolicyExcludedCount
}

type CreateDraftTouchPlanCommand struct {
	CampaignCode            string
	ExpectedCampaignVersion int64
	Source                  InitiationSourceRequest
	Owner                   Actor
	IdempotencyKey          string
}

func ValidateCreateDraftTouchPlanCommand(command CreateDraftTouchPlanCommand) bool {
	return validCode(command.CampaignCode) && command.ExpectedCampaignVersion > 0 && command.Source.Valid() && command.Owner.ID > 0 && validKey(command.IdempotencyKey)
}

func ValidContentSnapshot(content ContentSnapshot) bool {
	return len(content.Steps) >= 1 && validSteps(content.Steps) &&
		validInitiationDigest(content.Digest) && content.Digest == CanonicalContentDigest(content.Steps)
}

func CanonicalContentSnapshot(steps []Step) ContentSnapshot {
	canonical := append([]Step(nil), steps...)
	return ContentSnapshot{Steps: canonical, Digest: CanonicalContentDigest(canonical)}
}

// CanonicalContentDigest freezes the Campaign step payload using the same
// ordered fields persisted in cloud_campaign_touch_plan_steps. It deliberately
// excludes any future material-reference shape until Campaign owns one.
func CanonicalContentDigest(steps []Step) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("campaign.touch_plan_content.v1"))
	for _, step := range steps {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(int64(step.Index), 10)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(int64(step.DelayMinutes), 10)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(step.Content))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func CanonicalTargetDigest(source InitiationSourceRef, customerIDs []int64) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(string(source.Kind)))
	switch source.Kind {
	case InitiationSourceCustomerSelection:
		if source.CustomerSelection != nil {
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.CustomerSelection.ID))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.CustomerSelection.Version))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.CustomerSelection.Digest))
		}
	case InitiationSourceSegmentMembers:
		if source.Segment != nil {
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(strconv.FormatInt(source.Segment.SegmentID, 10)))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.Segment.MemberSnapshotWatermark.UTC().Format(time.RFC3339Nano)))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.Segment.Digest))
		}
	case InitiationSourceAudiencePackageMembers:
		if source.AudiencePackage != nil {
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(strconv.FormatInt(source.AudiencePackage.PackageID, 10)))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(strconv.FormatInt(source.AudiencePackage.PackageVersion, 10)))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.AudiencePackage.MemberSnapshotWatermark.UTC().Format(time.RFC3339Nano)))
			_, _ = hash.Write([]byte{0})
			_, _ = hash.Write([]byte(source.AudiencePackage.Digest))
		}
	}
	for _, id := range customerIDs {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(id, 10)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func DraftTouchPlanID(actorID int64, campaignCode, idempotencyKey string) string {
	digest := sha256.Sum256([]byte("campaign.draft_touch_plan.v1\x00" + strconv.FormatInt(actorID, 10) + "\x00" + campaignCode + "\x00" + idempotencyKey))
	return "ctp_" + hex.EncodeToString(digest[:])
}

func ValidDraftTouchPlan(plan DraftTouchPlan) bool {
	if !validPlanID(plan.ID) || !validCode(plan.CampaignCode) || !plan.Source.Valid() ||
		plan.CampaignVersion < 1 || plan.OwnerActorID < 1 || plan.CreatedAt.IsZero() ||
		plan.Safety != LocalInitiationSafety() || !ValidContentSnapshot(plan.Content) ||
		len(plan.Targets.CustomerIDs) < 1 || len(plan.Targets.CustomerIDs) > MaximumDraftTouchTargets || plan.Targets.Digest != CanonicalTargetDigest(plan.Source, plan.Targets.CustomerIDs) {
		return false
	}
	for index, id := range plan.Targets.CustomerIDs {
		if id < 1 || index > 0 && plan.Targets.CustomerIDs[index-1] >= id {
			return false
		}
	}
	return plan.Exclusions.CandidateCount >= 0 && plan.Exclusions.ActiveCustomerCount >= 0 &&
		plan.Exclusions.InactiveExcludedCount >= 0 && plan.Exclusions.PolicyExcludedCount >= 0 &&
		int(plan.Exclusions.CandidateCount) == len(plan.Targets.CustomerIDs)+int(plan.Exclusions.InactiveExcludedCount)+int(plan.Exclusions.PolicyExcludedCount) &&
		int(plan.Exclusions.ActiveCustomerCount) == len(plan.Targets.CustomerIDs)+int(plan.Exclusions.PolicyExcludedCount)
}

func CloneDraftTouchPlan(plan DraftTouchPlan) DraftTouchPlan {
	plan.Source = CanonicalInitiationSourceRef(plan.Source)
	plan.Targets.CustomerIDs = append([]int64(nil), plan.Targets.CustomerIDs...)
	plan.Content.Steps = append([]Step(nil), plan.Content.Steps...)
	return plan
}

func CloneDraftTouchPlanSummaries(plans []DraftTouchPlanSummary) []DraftTouchPlanSummary {
	result := make([]DraftTouchPlanSummary, len(plans))
	for index, plan := range plans {
		plan.Source = CanonicalInitiationSourceRef(plan.Source)
		result[index] = plan
	}
	return result
}

func validSourceWatermark(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC
}

func validInitiationDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validPlanID(value string) bool {
	if !strings.HasPrefix(value, "ctp_") || len(value) != len("ctp_")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value[len("ctp_"):])
	return err == nil
}

func ValidDraftTouchPlanID(value string) bool { return validPlanID(value) }

func canonicalMemberDigest(namespace string, customerIDs []int64) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(namespace))
	for _, customerID := range customerIDs {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strconv.FormatInt(customerID, 10)))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

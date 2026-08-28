// Package v1audiencehistory classifies V1 Audience and Segment rows as
// non-executable historical facts. It has no target store, queue, Provider,
// SQL execution, or V2 Segment DSL dependency.
package v1audiencehistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	PackageGroupsTableID   = "public/ai_audience_package_group"
	PackagesTableID        = "public/ai_audience_package"
	PackageVersionsTableID = "public/ai_audience_package_version"
	PackageSendersTableID  = "public/ai_audience_package_sender"
	RulesTableID           = "public/audience_rule"
	RuleVersionsTableID    = "public/audience_rule_version"
	SegmentsTableID        = "public/segments"
	AudienceMembersTableID = "public/ai_audience_member_current"
)

type Disposition string

const (
	DispositionCandidate  Disposition = "historical_candidate"
	DispositionQuarantine Disposition = "quarantine"
)

// OpaqueDigest points to source material retained by the encrypted archive.
// It is deliberately not an executable definition or a recoverable payload.
type OpaqueDigest [sha256.Size]byte

type PackageGroupFact struct {
	SourceID  int64
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PackageFact struct {
	SourceID                  int64
	GroupSourceID             *int64
	CurrentVersionSourceID    *int64
	PackageKey                string
	Name                      string
	NaturalLanguageDefinition string
	OriginalStatus            string
	QueryMode                 string
	IdentityPolicy            string
	IncrementalEnabled        bool
	DailyEnabled              bool
	IncrementalIntervalSecs   int64
	DailyRefreshTime          string
	Timezone                  string
	LookbackSecs              int64
	LastIncrementalAt         *time.Time
	LastDailyRefreshedAt      *time.Time
	NextIncrementalAt         *time.Time
	NextDailyAt               *time.Time
	PausedReason              string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
	RuntimeDigest             OpaqueDigest
}

type PackageVersionFact struct {
	SourceID                   int64
	PackageSourceID            int64
	VersionNumber              int64
	OriginalStatus             string
	AIPrompt                   string
	AIRationale                string
	NaturalLanguageExplanation string
	CreatedAt                  time.Time
	PublishedAt                *time.Time
	TemplateKey                string
	TemplateVersion            *int64
	TemplateFingerprint        string
	DefinitionDigest           OpaqueDigest
}

type PackageSenderFact struct {
	SourceID        int64
	PackageSourceID int64
	SenderUserID    string `json:"-"`
	DisplayName     string `json:"-"`
	Priority        int64
	OriginalStatus  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type RuleFact struct {
	SourceID       int64
	RuleKey        string
	DisplayName    string
	Description    string
	RuleType       string
	SourceOwner    string `json:"-"`
	OriginalStatus string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RuleVersionFact struct {
	SourceID         int64
	RuleSourceID     int64
	Version          int64
	ExecutorType     string
	OriginalStatus   string
	PublishedAt      *time.Time
	CreatedAt        time.Time
	DefinitionDigest OpaqueDigest
}

type SegmentFact struct {
	SourceID         int64
	SegmentCode      string
	DisplayName      string
	Description      string
	SourceType       string
	SQLDialect       string
	OriginalStatus   string
	Version          int64
	CreatedByAgent   string `json:"-"`
	CreatedBySession string `json:"-"`
	CachedHeadcount  int64
	LastRefreshedAt  *time.Time
	LastRefreshError string `json:"-"`
	UsageCount       int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DefinitionDigest OpaqueDigest
}

// AudienceMemberFact retains only a V1 membership snapshot. Its identity
// fields are private inputs for a future verified crosswalk, never OneID or a
// V2 segment_members refresh result.
type AudienceMemberFact struct {
	SourceID        int64
	PackageSourceID int64
	IdentityType    string `json:"-"`
	IdentityValue   string `json:"-"`
	MobileHash      string `json:"-"`
	OwnerUserID     string `json:"-"`
	EventSourceKey  string `json:"-"`
	PayloadHash     string `json:"-"`
	UnionID         string `json:"-"`
	OriginalStatus  string
	FirstEnteredAt  time.Time
	LastSeenAt      time.Time
	LastUpdatedAt   time.Time
	ExitedAt        *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PayloadDigest   OpaqueDigest
}

type PackageGroupResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PackageGroupFact
}
type PackageResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PackageFact
}
type PackageVersionResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PackageVersionFact
}
type PackageSenderResult struct {
	Disposition Disposition
	Reason      string
	Fact        *PackageSenderFact
}
type RuleResult struct {
	Disposition Disposition
	Reason      string
	Fact        *RuleFact
}
type RuleVersionResult struct {
	Disposition Disposition
	Reason      string
	Fact        *RuleVersionFact
}
type SegmentResult struct {
	Disposition Disposition
	Reason      string
	Fact        *SegmentFact
}
type AudienceMemberResult struct {
	Disposition Disposition
	Reason      string
	Fact        *AudienceMemberFact
}

// History keeps source-table rows separate. Source IDs are only historical
// references and never V2 IDs.
type History struct {
	PackageGroups   []PackageGroupResult
	Packages        []PackageResult
	PackageVersions []PackageVersionResult
	PackageSenders  []PackageSenderResult
	Rules           []RuleResult
	RuleVersions    []RuleVersionResult
	Segments        []SegmentResult
	AudienceMembers []AudienceMemberResult
}

// AdaptHistory parses the frozen static definition shape without translating a
// query into a V2 DSL or recovering encrypted archive material.
func AdaptHistory(groups, packages, versions, senders, rules, ruleVersions, segments []json.RawMessage) History {
	return AdaptHistoryWithMembers(groups, packages, versions, senders, rules, ruleVersions, segments, nil)
}

// AdaptHistoryWithMembers extends the frozen static definition shape with V1
// membership snapshots. It still performs no V2 member refresh or identity
// resolution.
func AdaptHistoryWithMembers(groups, packages, versions, senders, rules, ruleVersions, segments, members []json.RawMessage) History {
	history := History{
		PackageGroups: make([]PackageGroupResult, len(groups)), Packages: make([]PackageResult, len(packages)),
		PackageVersions: make([]PackageVersionResult, len(versions)), PackageSenders: make([]PackageSenderResult, len(senders)),
		Rules: make([]RuleResult, len(rules)), RuleVersions: make([]RuleVersionResult, len(ruleVersions)), Segments: make([]SegmentResult, len(segments)), AudienceMembers: make([]AudienceMemberResult, len(members)),
	}
	for index, value := range groups {
		history.PackageGroups[index] = adaptPackageGroup(value)
	}
	groupIDs := uniqueIDs(history.PackageGroups, packageGroupID, quarantinePackageGroup)
	for index, value := range packages {
		history.Packages[index] = adaptPackage(value)
	}
	for index := range history.Packages {
		if fact := history.Packages[index].Fact; fact != nil && fact.GroupSourceID != nil {
			if _, found := groupIDs[*fact.GroupSourceID]; !found {
				quarantinePackage(&history.Packages[index], "audience_package_group_unresolved")
			}
		}
	}
	packageIDs := uniqueIDs(history.Packages, packageID, quarantinePackage)
	for index, value := range versions {
		history.PackageVersions[index] = adaptPackageVersion(value)
	}
	for index := range history.PackageVersions {
		if fact := history.PackageVersions[index].Fact; fact != nil {
			if _, found := packageIDs[fact.PackageSourceID]; !found {
				quarantinePackageVersion(&history.PackageVersions[index], "audience_package_version_package_unresolved")
			}
		}
	}
	uniqueIDs(history.PackageVersions, packageVersionID, quarantinePackageVersion)
	for index, value := range senders {
		history.PackageSenders[index] = adaptPackageSender(value)
	}
	for index := range history.PackageSenders {
		if fact := history.PackageSenders[index].Fact; fact != nil {
			if _, found := packageIDs[fact.PackageSourceID]; !found {
				quarantinePackageSender(&history.PackageSenders[index], "audience_package_sender_package_unresolved")
			}
		}
	}
	uniqueIDs(history.PackageSenders, packageSenderID, quarantinePackageSender)

	versionsByID := packageVersionFacts(history.PackageVersions)
	for index := range history.Packages {
		fact := history.Packages[index].Fact
		if fact == nil || fact.CurrentVersionSourceID == nil {
			continue
		}
		version, found := versionsByID[*fact.CurrentVersionSourceID]
		if !found || version.PackageSourceID != fact.SourceID {
			quarantinePackage(&history.Packages[index], "audience_package_current_version_unresolved")
		}
	}
	packageIDs = uniqueIDs(history.Packages, packageID, quarantinePackage)
	for index := range history.PackageVersions {
		if fact := history.PackageVersions[index].Fact; fact != nil {
			if _, found := packageIDs[fact.PackageSourceID]; !found {
				quarantinePackageVersion(&history.PackageVersions[index], "audience_package_version_package_unresolved")
			}
		}
	}
	for index := range history.PackageSenders {
		if fact := history.PackageSenders[index].Fact; fact != nil {
			if _, found := packageIDs[fact.PackageSourceID]; !found {
				quarantinePackageSender(&history.PackageSenders[index], "audience_package_sender_package_unresolved")
			}
		}
	}

	for index, value := range rules {
		history.Rules[index] = adaptRule(value)
	}
	ruleIDs := uniqueIDs(history.Rules, ruleID, quarantineRule)
	for index, value := range ruleVersions {
		history.RuleVersions[index] = adaptRuleVersion(value)
	}
	for index := range history.RuleVersions {
		if fact := history.RuleVersions[index].Fact; fact != nil {
			if _, found := ruleIDs[fact.RuleSourceID]; !found {
				quarantineRuleVersion(&history.RuleVersions[index], "audience_rule_version_rule_unresolved")
			}
		}
	}
	uniqueIDs(history.RuleVersions, ruleVersionID, quarantineRuleVersion)
	for index, value := range segments {
		history.Segments[index] = adaptSegment(value)
	}
	uniqueIDs(history.Segments, segmentID, quarantineSegment)
	for index, value := range members {
		history.AudienceMembers[index] = adaptAudienceMember(value)
	}
	for index := range history.AudienceMembers {
		if fact := history.AudienceMembers[index].Fact; fact != nil {
			if _, found := packageIDs[fact.PackageSourceID]; !found {
				quarantineAudienceMember(&history.AudienceMembers[index], "audience_member_package_unresolved")
			}
		}
	}
	uniqueIDs(history.AudienceMembers, audienceMemberID, quarantineAudienceMember)
	return history
}

func adaptPackageGroup(value json.RawMessage) PackageGroupResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	name, nameOK := required[string](fields, "name")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	if !ok || !idOK || id < 1 || !nameOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() {
		return PackageGroupResult{Disposition: DispositionQuarantine, Reason: "audience_package_group_shape_invalid"}
	}
	return PackageGroupResult{Disposition: DispositionCandidate, Fact: &PackageGroupFact{SourceID: id, Name: name, CreatedAt: createdAt, UpdatedAt: updatedAt}}
}

func adaptPackage(value json.RawMessage) PackageResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	packageKey, keyOK := required[string](fields, "package_key")
	name, nameOK := required[string](fields, "name")
	natural, naturalOK := required[string](fields, "natural_language_definition")
	status, statusOK := required[string](fields, "status")
	queryMode, queryModeOK := required[string](fields, "query_mode")
	identityPolicy, policyOK := required[string](fields, "identity_policy")
	incrementalEnabled, incrementalOK := required[bool](fields, "incremental_enabled")
	dailyEnabled, dailyOK := required[bool](fields, "daily_enabled")
	interval, intervalOK := required[int64](fields, "incremental_interval_seconds")
	dailyTime, dailyTimeOK := required[string](fields, "daily_refresh_time")
	timezone, timezoneOK := required[string](fields, "timezone")
	lookback, lookbackOK := required[int64](fields, "lookback_seconds")
	pausedReason, pausedReasonOK := required[string](fields, "paused_reason")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	groupID, groupOK := optional[int64](fields, "group_id")
	currentVersionID, versionOK := optional[int64](fields, "current_version_id")
	lastIncremental, lastIncrementalOK := optional[time.Time](fields, "last_incremental_watermark_at")
	lastDaily, lastDailyOK := optional[time.Time](fields, "last_daily_refreshed_at")
	nextIncremental, nextIncrementalOK := optional[time.Time](fields, "next_incremental_refresh_at")
	nextDaily, nextDailyOK := optional[time.Time](fields, "next_daily_refresh_at")
	leaseExpires, leaseExpiresOK := optional[time.Time](fields, "lease_expires_at")
	runtimeDigest, digestOK := opaque(fields, "lease_token", "lease_expires_at")
	if !ok || !idOK || id < 1 || !keyOK || !nameOK || !naturalOK || !statusOK || !queryModeOK || !policyOK || !incrementalOK || !dailyOK || !intervalOK || !dailyTimeOK || !timezoneOK || !lookbackOK || !pausedReasonOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() || !groupOK || !versionOK || !lastIncrementalOK || !lastDailyOK || !nextIncrementalOK || !nextDailyOK || !leaseExpiresOK || !digestOK {
		return PackageResult{Disposition: DispositionQuarantine, Reason: "audience_package_shape_invalid"}
	}
	_ = leaseExpires // preserved by RuntimeDigest without exposing the source lease state.
	return PackageResult{Disposition: DispositionCandidate, Fact: &PackageFact{SourceID: id, GroupSourceID: groupID, CurrentVersionSourceID: currentVersionID, PackageKey: packageKey, Name: name, NaturalLanguageDefinition: natural, OriginalStatus: status, QueryMode: queryMode, IdentityPolicy: identityPolicy, IncrementalEnabled: incrementalEnabled, DailyEnabled: dailyEnabled, IncrementalIntervalSecs: interval, DailyRefreshTime: dailyTime, Timezone: timezone, LookbackSecs: lookback, LastIncrementalAt: lastIncremental, LastDailyRefreshedAt: lastDaily, NextIncrementalAt: nextIncremental, NextDailyAt: nextDaily, PausedReason: pausedReason, CreatedAt: createdAt, UpdatedAt: updatedAt, RuntimeDigest: runtimeDigest}}
}

func adaptPackageVersion(value json.RawMessage) PackageVersionResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	packageID, packageOK := required[int64](fields, "package_id")
	version, versionOK := required[int64](fields, "version_number")
	status, statusOK := required[string](fields, "status")
	prompt, promptOK := required[string](fields, "ai_prompt")
	rationale, rationaleOK := required[string](fields, "ai_rationale")
	explanation, explanationOK := required[string](fields, "natural_language_explanation")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	publishedAt, publishedOK := optional[time.Time](fields, "published_at")
	templateKey, keyOK := required[string](fields, "template_key")
	templateVersion, templateVersionOK := optional[int64](fields, "template_version")
	fingerprint, fingerprintOK := required[string](fields, "template_fingerprint")
	digest, digestOK := opaque(fields, "incremental_sql_text", "snapshot_sql_text", "dependencies_json", "explain_json", "sample_rows_json", "validation_errors_json", "parameters_json", "simple_sql_text", "simple_compiled_sql_text", "template_params_json")
	if !ok || !idOK || id < 1 || !packageOK || packageID < 1 || !versionOK || !statusOK || !promptOK || !rationaleOK || !explanationOK || !createdOK || createdAt.IsZero() || !publishedOK || !keyOK || !templateVersionOK || !fingerprintOK || !digestOK {
		return PackageVersionResult{Disposition: DispositionQuarantine, Reason: "audience_package_version_shape_invalid"}
	}
	return PackageVersionResult{Disposition: DispositionCandidate, Fact: &PackageVersionFact{SourceID: id, PackageSourceID: packageID, VersionNumber: version, OriginalStatus: status, AIPrompt: prompt, AIRationale: rationale, NaturalLanguageExplanation: explanation, CreatedAt: createdAt, PublishedAt: publishedAt, TemplateKey: templateKey, TemplateVersion: templateVersion, TemplateFingerprint: fingerprint, DefinitionDigest: digest}}
}

func adaptPackageSender(value json.RawMessage) PackageSenderResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	packageID, packageOK := required[int64](fields, "package_id")
	userID, userOK := required[string](fields, "sender_userid")
	displayName, displayOK := required[string](fields, "display_name")
	priority, priorityOK := required[int64](fields, "priority")
	status, statusOK := required[string](fields, "status")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	if !ok || !idOK || id < 1 || !packageOK || packageID < 1 || !userOK || !displayOK || !priorityOK || !statusOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() {
		return PackageSenderResult{Disposition: DispositionQuarantine, Reason: "audience_package_sender_shape_invalid"}
	}
	return PackageSenderResult{Disposition: DispositionCandidate, Fact: &PackageSenderFact{SourceID: id, PackageSourceID: packageID, SenderUserID: userID, DisplayName: displayName, Priority: priority, OriginalStatus: status, CreatedAt: createdAt, UpdatedAt: updatedAt}}
}

func adaptRule(value json.RawMessage) RuleResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	key, keyOK := required[string](fields, "rule_key")
	displayName, displayOK := required[string](fields, "display_name")
	description, descriptionOK := required[string](fields, "description")
	ruleType, typeOK := required[string](fields, "rule_type")
	owner, ownerOK := required[string](fields, "owner")
	status, statusOK := required[string](fields, "status")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	if !ok || !idOK || id < 1 || !keyOK || !displayOK || !descriptionOK || !typeOK || !ownerOK || !statusOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() {
		return RuleResult{Disposition: DispositionQuarantine, Reason: "audience_rule_shape_invalid"}
	}
	return RuleResult{Disposition: DispositionCandidate, Fact: &RuleFact{SourceID: id, RuleKey: key, DisplayName: displayName, Description: description, RuleType: ruleType, SourceOwner: owner, OriginalStatus: status, CreatedAt: createdAt, UpdatedAt: updatedAt}}
}

func adaptRuleVersion(value json.RawMessage) RuleVersionResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	ruleID, ruleOK := required[int64](fields, "rule_id")
	version, versionOK := required[int64](fields, "version")
	executorType, executorOK := required[string](fields, "executor_type")
	status, statusOK := required[string](fields, "status")
	publishedAt, publishedOK := optional[time.Time](fields, "published_at")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	digest, digestOK := opaque(fields, "code_or_sql", "params_schema", "output_schema", "refresh_policy")
	if !ok || !idOK || id < 1 || !ruleOK || ruleID < 1 || !versionOK || !executorOK || !statusOK || !publishedOK || !createdOK || createdAt.IsZero() || !digestOK {
		return RuleVersionResult{Disposition: DispositionQuarantine, Reason: "audience_rule_version_shape_invalid"}
	}
	return RuleVersionResult{Disposition: DispositionCandidate, Fact: &RuleVersionFact{SourceID: id, RuleSourceID: ruleID, Version: version, ExecutorType: executorType, OriginalStatus: status, PublishedAt: publishedAt, CreatedAt: createdAt, DefinitionDigest: digest}}
}

func adaptSegment(value json.RawMessage) SegmentResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	code, codeOK := required[string](fields, "segment_code")
	displayName, displayOK := required[string](fields, "display_name")
	description, descriptionOK := required[string](fields, "description")
	sourceType, sourceTypeOK := required[string](fields, "source_type")
	dialect, dialectOK := required[string](fields, "sql_dialect")
	status, statusOK := required[string](fields, "status")
	version, versionOK := required[int64](fields, "version")
	createdByAgent, agentOK := required[string](fields, "created_by_agent")
	createdBySession, sessionOK := required[string](fields, "created_by_session")
	cachedHeadcount, headcountOK := required[int64](fields, "cached_headcount")
	lastRefreshedAt, refreshedOK := optional[time.Time](fields, "last_refreshed_at")
	lastRefreshError, errorOK := required[string](fields, "last_refresh_error")
	usageCount, usageOK := required[int64](fields, "usage_count")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	digest, digestOK := opaque(fields, "sql_query", "sql_params_json", "cached_sample_json", "tags_json")
	if !ok || !idOK || id < 1 || !codeOK || !displayOK || !descriptionOK || !sourceTypeOK || !dialectOK || !statusOK || !versionOK || !agentOK || !sessionOK || !headcountOK || !refreshedOK || !errorOK || !usageOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() || !digestOK {
		return SegmentResult{Disposition: DispositionQuarantine, Reason: "audience_segment_shape_invalid"}
	}
	return SegmentResult{Disposition: DispositionCandidate, Fact: &SegmentFact{SourceID: id, SegmentCode: code, DisplayName: displayName, Description: description, SourceType: sourceType, SQLDialect: dialect, OriginalStatus: status, Version: version, CreatedByAgent: createdByAgent, CreatedBySession: createdBySession, CachedHeadcount: cachedHeadcount, LastRefreshedAt: lastRefreshedAt, LastRefreshError: lastRefreshError, UsageCount: usageCount, CreatedAt: createdAt, UpdatedAt: updatedAt, DefinitionDigest: digest}}
}

func adaptAudienceMember(value json.RawMessage) AudienceMemberResult {
	fields, ok := object(value)
	id, idOK := required[int64](fields, "id")
	packageID, packageOK := required[int64](fields, "package_id")
	identityType, typeOK := required[string](fields, "identity_type")
	identityValue, valueOK := required[string](fields, "identity_value")
	status, statusOK := required[string](fields, "status")
	mobileHash, mobileOK := required[string](fields, "mobile_hash")
	ownerUserID, ownerOK := required[string](fields, "owner_userid")
	eventSourceKey, sourceOK := required[string](fields, "event_source_key")
	payloadHash, hashOK := required[string](fields, "payload_hash")
	firstEnteredAt, enteredOK := required[time.Time](fields, "first_entered_at")
	lastSeenAt, seenOK := required[time.Time](fields, "last_seen_at")
	lastUpdatedAt, updatedMemberOK := required[time.Time](fields, "last_updated_at")
	exitedAt, exitedOK := optional[time.Time](fields, "exited_at")
	createdAt, createdOK := required[time.Time](fields, "created_at")
	updatedAt, updatedOK := required[time.Time](fields, "updated_at")
	unionID, unionOK := required[string](fields, "unionid")
	payloadDigest, digestOK := opaque(fields, "payload_json")
	if !ok || !idOK || id < 1 || !packageOK || packageID < 1 || !typeOK || !valueOK || !statusOK || !mobileOK || !ownerOK || !sourceOK || !hashOK || !enteredOK || firstEnteredAt.IsZero() || !seenOK || lastSeenAt.IsZero() || !updatedMemberOK || lastUpdatedAt.IsZero() || !exitedOK || !createdOK || createdAt.IsZero() || !updatedOK || updatedAt.IsZero() || !unionOK || !digestOK {
		return AudienceMemberResult{Disposition: DispositionQuarantine, Reason: "audience_member_shape_invalid"}
	}
	return AudienceMemberResult{Disposition: DispositionCandidate, Fact: &AudienceMemberFact{SourceID: id, PackageSourceID: packageID, IdentityType: identityType, IdentityValue: identityValue, MobileHash: mobileHash, OwnerUserID: ownerUserID, EventSourceKey: eventSourceKey, PayloadHash: payloadHash, UnionID: unionID, OriginalStatus: status, FirstEnteredAt: firstEnteredAt, LastSeenAt: lastSeenAt, LastUpdatedAt: lastUpdatedAt, ExitedAt: exitedAt, CreatedAt: createdAt, UpdatedAt: updatedAt, PayloadDigest: payloadDigest}}
}

func packageGroupID(value PackageGroupResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func packageID(value PackageResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func packageVersionID(value PackageVersionResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func packageSenderID(value PackageSenderResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func ruleID(value RuleResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func ruleVersionID(value RuleVersionResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func segmentID(value SegmentResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}
func audienceMemberID(value AudienceMemberResult) (int64, bool) {
	if value.Disposition == DispositionCandidate && value.Fact != nil {
		return value.Fact.SourceID, true
	}
	return 0, false
}

func quarantinePackageGroup(value *PackageGroupResult, reason string) {
	*value = PackageGroupResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantinePackage(value *PackageResult, reason string) {
	*value = PackageResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantinePackageVersion(value *PackageVersionResult, reason string) {
	*value = PackageVersionResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantinePackageSender(value *PackageSenderResult, reason string) {
	*value = PackageSenderResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantineRule(value *RuleResult, reason string) {
	*value = RuleResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantineRuleVersion(value *RuleVersionResult, reason string) {
	*value = RuleVersionResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantineSegment(value *SegmentResult, reason string) {
	*value = SegmentResult{Disposition: DispositionQuarantine, Reason: reason}
}
func quarantineAudienceMember(value *AudienceMemberResult, reason string) {
	*value = AudienceMemberResult{Disposition: DispositionQuarantine, Reason: reason}
}

func uniqueIDs[T any](values []T, identify func(T) (int64, bool), quarantine func(*T, string)) map[int64]struct{} {
	counts := make(map[int64]int)
	for _, value := range values {
		if id, ok := identify(value); ok {
			counts[id]++
		}
	}
	for index := range values {
		if id, ok := identify(values[index]); ok && counts[id] != 1 {
			quarantine(&values[index], "audience_source_id_ambiguous")
		}
	}
	ids := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if id, ok := identify(value); ok {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func packageVersionFacts(values []PackageVersionResult) map[int64]PackageVersionFact {
	result := make(map[int64]PackageVersionFact, len(values))
	for _, value := range values {
		if value.Disposition == DispositionCandidate && value.Fact != nil {
			result[value.Fact.SourceID] = *value.Fact
		}
	}
	return result
}

type fields map[string]json.RawMessage

func object(value json.RawMessage) (fields, bool) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	result := make(fields)
	if decoder.Decode(&result) != nil || result == nil {
		return nil, false
	}
	var extra any
	return result, errors.Is(decoder.Decode(&extra), io.EOF)
}

func required[T any](source fields, name string) (T, bool) {
	var zero T
	raw, found := source[name]
	if !found || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &zero) != nil {
		return zero, false
	}
	return zero, true
}

func optional[T any](source fields, name string) (*T, bool) {
	raw, found := source[name]
	if !found {
		return nil, false
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, true
	}
	var value T
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return &value, true
}

func opaque(source fields, names ...string) (OpaqueDigest, bool) {
	values := make(map[string]json.RawMessage, len(names))
	for _, name := range names {
		raw, found := source[name]
		if !found || !json.Valid(raw) {
			return OpaqueDigest{}, false
		}
		values[name] = append(json.RawMessage(nil), raw...)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return OpaqueDigest{}, false
	}
	sum := sha256.Sum256(append([]byte("v1-audience-history-opaque-v1\x00"), encoded...))
	return OpaqueDigest(sum), true
}

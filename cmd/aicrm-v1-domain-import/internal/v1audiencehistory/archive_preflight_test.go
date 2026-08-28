package v1audiencehistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var audienceArchiveRun = flag.String("audience-history-archive-run", "", "optional reconciled V2 archive run for read-only Audience history preflight")

var (
	errAudienceHistoryArchiveRead   = errors.New("audience_history_archive_read_failed")
	errAudienceHistoryResultInvalid = errors.New("audience_history_preflight_result_invalid")
)

func TestAudienceHistoryRequiredFieldRedaction(t *testing.T) {
	if !audienceRequiredFieldRedacted(PackagesTableID, v1archive.ArchivedRow{RedactedFields: []string{"name"}}) {
		t.Fatal("redacted package name was accepted")
	}
	if audienceRequiredFieldRedacted(PackagesTableID, v1archive.ArchivedRow{RedactedFields: []string{"lease_token"}}) {
		t.Fatal("opaque lease token blocked non-executable history candidate")
	}
	if audienceRequiredFieldRedacted(PackageVersionsTableID, v1archive.ArchivedRow{RedactedFields: []string{"incremental_sql_text", "template_params_json"}}) {
		t.Fatal("opaque SQL or parameters blocked non-executable history candidate")
	}
	if audienceRequiredFieldRedacted(RuleVersionsTableID, v1archive.ArchivedRow{RedactedFields: []string{"code_or_sql"}}) {
		t.Fatal("opaque rule code blocked non-executable history candidate")
	}
	if !audienceRequiredFieldRedacted(AudienceMembersTableID, v1archive.ArchivedRow{RedactedFields: []string{"unionid"}}) {
		t.Fatal("private member identity missing its migration input was accepted")
	}
	if audienceRequiredFieldRedacted(AudienceMembersTableID, v1archive.ArchivedRow{RedactedFields: []string{"payload_json"}}) {
		t.Fatal("opaque member payload blocked non-executable history candidate")
	}
}

func TestAudienceDispositionCountsRejectsInconsistentOrUnknownResult(t *testing.T) {
	for _, values := range [][]AudienceMemberResult{
		{{Disposition: DispositionCandidate}},
		{{Disposition: DispositionQuarantine, Reason: "source_payload_here"}},
		{{Disposition: "unexpected", Fact: &AudienceMemberFact{}}},
	} {
		if _, err := audienceDispositionCounts(values); !errors.Is(err, errAudienceHistoryResultInvalid) {
			t.Fatalf("inconsistent preflight result was accepted: %+v", values)
		}
	}
	counts, err := audienceDispositionCounts([]AudienceMemberResult{{Disposition: DispositionQuarantine, Reason: "audience_member_package_unresolved"}})
	if err != nil || counts.quarantine != 1 || counts.reasons["audience_member_package_unresolved"] != 1 {
		t.Fatalf("fixed quarantine reason was not counted: counts=%+v err=%v", counts, err)
	}
}

// TestReconciledAudienceHistoryArchivePreflight is opt-in and read-only. It
// validates archive identity, static-definition and member conservation, and pure candidate
// classification without opening a target write transaction or logging rows.
func TestReconciledAudienceHistoryArchivePreflight(t *testing.T) {
	if *audienceArchiveRun == "" {
		t.Skip("supply -audience-history-archive-run and V2 archive environment for read-only Audience history preflight")
	}
	ctx := context.Background()
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(ctx, environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("cannot open V2 archive reader")
	}
	defer archive.Close()
	groups, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, PackageGroupsTableID)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, PackagesTableID)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, PackageVersionsTableID)
	if err != nil {
		t.Fatal(err)
	}
	senders, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, PackageSendersTableID)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, RulesTableID)
	if err != nil {
		t.Fatal(err)
	}
	ruleVersions, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, RuleVersionsTableID)
	if err != nil {
		t.Fatal(err)
	}
	segments, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, SegmentsTableID)
	if err != nil {
		t.Fatal(err)
	}
	members, err := readAudienceHistoryTable(ctx, archive, *audienceArchiveRun, AudienceMembersTableID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || len(packages) != 38 || len(versions) != 83 || len(senders) != 33 || len(rules) != 1 || len(ruleVersions) != 1 || len(segments) != 7680 || len(members) != 29211 {
		t.Fatal("unexpected Audience history archive table counts")
	}
	history := AdaptHistoryWithMembers(groups, packages, versions, senders, rules, ruleVersions, segments, members)
	if len(history.PackageGroups) != len(groups) || len(history.Packages) != len(packages) || len(history.PackageVersions) != len(versions) || len(history.PackageSenders) != len(senders) || len(history.Rules) != len(rules) || len(history.RuleVersions) != len(ruleVersions) || len(history.Segments) != len(segments) || len(history.AudienceMembers) != len(members) {
		t.Fatal("Audience history archive row conservation failed")
	}
	if err := logAudienceHistoryPreflight(t, history); err != nil {
		t.Fatal(err)
	}
}

func readAudienceHistoryTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID, table string) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, 0)
	err := archive.EachTableRow(ctx, runID, table, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal < 1 || row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) || !json.Valid(row.Payload) {
			return errors.New("invalid Audience history archive row identity")
		}
		if audienceRequiredFieldRedacted(table, row) {
			result = append(result, json.RawMessage(`{}`))
			return nil
		}
		result = append(result, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		return nil, errAudienceHistoryArchiveRead
	}
	return result, nil
}

func audienceRequiredFieldRedacted(table string, row v1archive.ArchivedRow) bool {
	required := map[string][]string{
		PackageGroupsTableID:   {"id", "name", "created_at", "updated_at"},
		PackagesTableID:        {"id", "package_key", "name", "natural_language_definition", "status", "query_mode", "identity_policy", "current_version_id", "incremental_enabled", "daily_enabled", "incremental_interval_seconds", "daily_refresh_time", "timezone", "lookback_seconds", "last_incremental_watermark_at", "last_daily_refreshed_at", "next_incremental_refresh_at", "next_daily_refresh_at", "paused_reason", "created_at", "updated_at", "group_id"},
		PackageVersionsTableID: {"id", "package_id", "version_number", "status", "ai_prompt", "ai_rationale", "natural_language_explanation", "created_at", "published_at", "template_key", "template_version", "template_fingerprint"},
		PackageSendersTableID:  {"id", "package_id", "sender_userid", "display_name", "priority", "status", "created_at", "updated_at"},
		RulesTableID:           {"id", "rule_key", "display_name", "description", "rule_type", "owner", "status", "created_at", "updated_at"},
		RuleVersionsTableID:    {"id", "rule_id", "version", "executor_type", "status", "published_at", "created_at"},
		SegmentsTableID:        {"id", "segment_code", "display_name", "description", "source_type", "sql_dialect", "status", "version", "created_by_agent", "created_by_session", "cached_headcount", "last_refreshed_at", "last_refresh_error", "usage_count", "created_at", "updated_at"},
		AudienceMembersTableID: {"id", "package_id", "identity_type", "identity_value", "status", "mobile_hash", "owner_userid", "event_source_key", "payload_hash", "first_entered_at", "last_seen_at", "last_updated_at", "exited_at", "created_at", "updated_at", "unionid"},
	}
	for _, field := range required[table] {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func logAudienceHistoryPreflight(t *testing.T, history History) error {
	groups, err := audienceDispositionCounts(history.PackageGroups)
	if err != nil {
		return err
	}
	packages, err := audienceDispositionCounts(history.Packages)
	if err != nil {
		return err
	}
	versions, err := audienceDispositionCounts(history.PackageVersions)
	if err != nil {
		return err
	}
	senders, err := audienceDispositionCounts(history.PackageSenders)
	if err != nil {
		return err
	}
	rules, err := audienceDispositionCounts(history.Rules)
	if err != nil {
		return err
	}
	ruleVersions, err := audienceDispositionCounts(history.RuleVersions)
	if err != nil {
		return err
	}
	segments, err := audienceDispositionCounts(history.Segments)
	if err != nil {
		return err
	}
	members, err := audienceDispositionCounts(history.AudienceMembers)
	if err != nil {
		return err
	}
	t.Logf("read-only Audience history preflight: groups=%s packages=%s versions=%s senders=%s rules=%s rule_versions=%s segments=%s members=%s", groups, packages, versions, senders, rules, ruleVersions, segments, members)
	return nil
}

type audiencePreflightResult interface {
	audiencePreflightState() (Disposition, bool, string)
}

func (value PackageGroupResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value PackageResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value PackageVersionResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value PackageSenderResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value RuleResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value RuleVersionResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value SegmentResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value AudienceMemberResult) audiencePreflightState() (Disposition, bool, string) {
	return audienceResultState(value.Disposition, value.Fact != nil, value.Reason)
}

func audienceResultState(disposition Disposition, hasFact bool, reason string) (Disposition, bool, string) {
	switch disposition {
	case DispositionCandidate:
		return disposition, hasFact && reason == "", ""
	case DispositionQuarantine:
		return disposition, !hasFact && validAudienceQuarantineReason(reason), reason
	default:
		return disposition, false, ""
	}
}

func validAudienceQuarantineReason(reason string) bool {
	switch reason {
	case "audience_source_id_ambiguous", "audience_package_group_shape_invalid", "audience_package_shape_invalid", "audience_package_group_unresolved", "audience_package_current_version_unresolved", "audience_package_version_shape_invalid", "audience_package_version_package_unresolved", "audience_package_sender_shape_invalid", "audience_package_sender_package_unresolved", "audience_rule_shape_invalid", "audience_rule_version_shape_invalid", "audience_rule_version_rule_unresolved", "audience_segment_shape_invalid", "audience_member_shape_invalid", "audience_member_package_unresolved":
		return true
	default:
		return false
	}
}

type dispositionCounts struct {
	candidate  int
	quarantine int
	reasons    map[string]int
}

func (counts dispositionCounts) String() string {
	reasons := make([]string, 0, len(counts.reasons))
	for reason, count := range counts.reasons {
		reasons = append(reasons, fmt.Sprintf("%s:%d", reason, count))
	}
	sort.Strings(reasons)
	return fmt.Sprintf("candidate:%d quarantine:%d reasons:[%s]", counts.candidate, counts.quarantine, strings.Join(reasons, ","))
}

func audienceDispositionCounts[T audiencePreflightResult](values []T) (dispositionCounts, error) {
	result := dispositionCounts{reasons: make(map[string]int)}
	for _, value := range values {
		disposition, consistent, reason := value.audiencePreflightState()
		if !consistent {
			return dispositionCounts{}, errAudienceHistoryResultInvalid
		}
		if disposition == DispositionCandidate {
			result.candidate++
		} else {
			result.quarantine++
			result.reasons[reason]++
		}
	}
	return result, nil
}

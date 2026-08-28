package v1profilecatalog

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var profileCatalogArchiveRun = flag.String("profile-catalog-archive-run", "", "optional reconciled V2 archive run for read-only profile catalogue candidate validation")

const (
	profileCatalogTemplatesTable  = "public/automation_profile_segment_template"
	profileCatalogCategoriesTable = "public/automation_profile_segment_category"
	profileCatalogMappingsTable   = "public/automation_profile_segment_option_mapping"
	profileCatalogTagRulesTable   = "public/signup_tag_rules"
)

type profileCatalogTable struct {
	name   string
	count  int
	fields []string
}

var profileCatalogTables = []profileCatalogTable{
	{name: profileCatalogTemplatesTable, count: 4, fields: []string{"id", "template_code", "template_name", "questionnaire_id", "segmentation_question_id", "description", "enabled", "version", "created_by", "updated_by", "created_at", "updated_at", "program_id"}},
	{name: profileCatalogCategoriesTable, count: 10, fields: []string{"id", "template_id", "category_key", "category_name", "description", "sort_order", "enabled", "created_at", "updated_at"}},
	{name: profileCatalogMappingsTable, count: 6, fields: []string{"id", "template_id", "category_id", "question_id", "option_id", "created_at"}},
	{name: profileCatalogTagRulesTable, count: 10, fields: []string{"tag_id", "tag_name", "signup_status", "active", "updated_at"}},
}

// TestReconciledProfileCatalogArchivePreflight is opt-in and read-only. It
// checks archive identity, frozen row counts, row conservation, and candidate
// quarantine without opening a target write transaction or logging source data.
func TestReconciledProfileCatalogArchivePreflight(t *testing.T) {
	if *profileCatalogArchiveRun == "" {
		t.Skip("supply -profile-catalog-archive-run and V2 archive environment for read-only profile catalogue preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("profile_catalog_archive_open_failed")
	}
	defer archive.Close()

	payloads := make([][]json.RawMessage, len(profileCatalogTables))
	redactions := make(map[string]int)
	for index, table := range profileCatalogTables {
		payloads[index], err = readProfileCatalogArchiveTable(context.Background(), archive, *profileCatalogArchiveRun, table, redactions)
		if err != nil {
			t.Fatal("profile_catalog_archive_read_failed")
		}
		if len(payloads[index]) != table.count {
			t.Fatalf("profile_catalog_archive_count_mismatch table_index=%d count=%d", index, len(payloads[index]))
		}
	}

	history := AdaptHistory(payloads[0], payloads[1], payloads[2], payloads[3])
	if history.SourceCount() != 30 || history.TerminalCount() != 30 {
		t.Fatal("profile_catalog_candidate_row_conservation_failed")
	}
	counts, err := profileCatalogCountResults(history)
	if err != nil {
		t.Fatal("profile_catalog_candidate_disposition_invalid")
	}
	logProfileCatalogPreflight(t, counts, redactions)
}

func readProfileCatalogArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, table profileCatalogTable, redactions map[string]int) ([]json.RawMessage, error) {
	payloads := make([]json.RawMessage, 0, table.count)
	seen := make(map[[sha256.Size]byte]bool)
	verificationReason := ""
	err := archive.EachTableRow(ctx, runID, table.name, func(row v1archive.ArchivedRow) error {
		verificationReason = profileCatalogArchiveRowReason(row, table.name, int64(len(payloads)+1))
		if verificationReason == "" && seen[row.SourceKeyHMAC] {
			verificationReason = "archive_duplicate_source_key"
		}
		if verificationReason != "" {
			return errors.New(verificationReason)
		}
		seen[row.SourceKeyHMAC] = true
		for _, field := range row.RedactedFields {
			redactions[profileCatalogRedactedFieldName(table, field)]++
		}
		if profileCatalogManifestFieldRedacted(table, row) {
			payloads = append(payloads, json.RawMessage(`{}`))
			return nil
		}
		if !json.Valid(row.Payload) {
			verificationReason = "archive_payload_invalid"
			return errors.New(verificationReason)
		}
		payloads = append(payloads, append(json.RawMessage(nil), row.Payload...))
		return nil
	})
	if err != nil {
		return nil, errors.New("archive_read_failed")
	}
	return payloads, nil
}

func profileCatalogArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal {
		return "archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
		return "archive_hmac_missing"
	}
	return ""
}

// A redacted optional reference cannot safely be treated as a source NULL, so
// every frozen field blocks a typed candidate until the sealed archive is read.
func profileCatalogManifestFieldRedacted(table profileCatalogTable, row v1archive.ArchivedRow) bool {
	for _, field := range table.fields {
		if v1archive.IsRedacted(row, field) {
			return true
		}
	}
	return false
}

func profileCatalogRedactedFieldName(table profileCatalogTable, path string) string {
	for _, field := range table.fields {
		if path == field || len(path) > len(field) && path[:len(field)] == field && path[len(field)] == '.' {
			return table.name + "." + field
		}
	}
	return table.name + ".unknown"
}

type profileCatalogCounts struct {
	templates, categories, mappings, rules int
	quarantine                             int
	reasons                                map[string]int
}

func profileCatalogCountResults(history History) (profileCatalogCounts, error) {
	result := profileCatalogCounts{reasons: make(map[string]int)}
	countTemplate := func(value TemplateResult) error {
		if value.Disposition == DispositionCandidate && value.Fact != nil && value.Reason == "" {
			result.templates++
			return nil
		}
		return profileCatalogQuarantine(&result, value.Disposition, value.Fact == nil, value.Reason)
	}
	countCategory := func(value CategoryResult) error {
		if value.Disposition == DispositionCandidate && value.Fact != nil && value.Reason == "" {
			result.categories++
			return nil
		}
		return profileCatalogQuarantine(&result, value.Disposition, value.Fact == nil, value.Reason)
	}
	countMapping := func(value OptionMappingResult) error {
		if value.Disposition == DispositionCandidate && value.Fact != nil && value.Reason == "" {
			result.mappings++
			return nil
		}
		return profileCatalogQuarantine(&result, value.Disposition, value.Fact == nil, value.Reason)
	}
	countRule := func(value SignupTagRuleResult) error {
		if value.Disposition == DispositionCandidate && value.Fact != nil && value.Reason == "" {
			result.rules++
			return nil
		}
		return profileCatalogQuarantine(&result, value.Disposition, value.Fact == nil, value.Reason)
	}
	for _, value := range history.Templates {
		if err := countTemplate(value); err != nil {
			return profileCatalogCounts{}, err
		}
	}
	for _, value := range history.Categories {
		if err := countCategory(value); err != nil {
			return profileCatalogCounts{}, err
		}
	}
	for _, value := range history.OptionMappings {
		if err := countMapping(value); err != nil {
			return profileCatalogCounts{}, err
		}
	}
	for _, value := range history.SignupTagRules {
		if err := countRule(value); err != nil {
			return profileCatalogCounts{}, err
		}
	}
	return result, nil
}

func profileCatalogQuarantine(counts *profileCatalogCounts, disposition Disposition, noFact bool, reason string) error {
	if disposition != DispositionQuarantine || !noFact || !validProfileCatalogQuarantineReason(reason) {
		return errors.New("candidate_disposition_invalid")
	}
	counts.quarantine++
	counts.reasons[reason]++
	return nil
}

func validProfileCatalogQuarantineReason(reason string) bool {
	switch reason {
	case "profile_segment_template_shape_invalid", "profile_segment_template_source_id_ambiguous", "profile_segment_category_shape_invalid", "profile_segment_category_template_unresolved", "profile_segment_category_source_id_ambiguous", "profile_segment_option_mapping_shape_invalid", "profile_segment_option_mapping_template_unresolved", "profile_segment_option_mapping_category_unresolved", "profile_segment_option_mapping_template_category_mismatch", "profile_segment_option_mapping_source_id_ambiguous", "signup_tag_rule_shape_invalid", "signup_tag_rule_source_id_ambiguous":
		return true
	default:
		return false
	}
}

func logProfileCatalogPreflight(t *testing.T, counts profileCatalogCounts, redactions map[string]int) {
	for _, name := range sortedProfileCatalogKeys(counts.reasons) {
		t.Logf("reason=%s count=%d", name, counts.reasons[name])
	}
	for _, name := range sortedProfileCatalogKeys(redactions) {
		t.Logf("redacted_field=%s count=%d", name, redactions[name])
	}
	t.Logf("source_templates=4 source_categories=10 source_mappings=6 source_signup_tag_rules=10 candidate_templates=%d candidate_categories=%d candidate_mappings=%d candidate_signup_tag_rules=%d quarantine_rows=%d source_relations_only=1 target_writer_checked=0 target_fk_checked=0 target_writes=0", counts.templates, counts.categories, counts.mappings, counts.rules, counts.quarantine)
}

func sortedProfileCatalogKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestProfileCatalogArchiveRowChecksScopeAndRedaction(t *testing.T) {
	table := profileCatalogTables[0]
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table.name, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, RedactedFields: []string{"created_by", "unknown.path"}}
	if reason := profileCatalogArchiveRowReason(row, table.name, 1); reason != "" {
		t.Fatal(reason)
	}
	if !profileCatalogManifestFieldRedacted(table, row) {
		t.Fatal("redacted actor was treated as a source value")
	}
	if profileCatalogRedactedFieldName(table, "created_by") != table.name+".created_by" || profileCatalogRedactedFieldName(table, "unknown.path") != table.name+".unknown" {
		t.Fatal("redaction field name was not safely classified")
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = profileCatalogCategoriesTable },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
	} {
		changed := row
		mutate(&changed)
		if profileCatalogArchiveRowReason(changed, table.name, 1) == "" {
			t.Fatal("invalid archive identity accepted")
		}
	}
}

func TestProfileCatalogCountsRejectInvalidResults(t *testing.T) {
	if _, err := profileCatalogCountResults(History{Templates: []TemplateResult{{Disposition: DispositionCandidate}}}); err == nil {
		t.Fatal("candidate without fact accepted")
	}
	counts, err := profileCatalogCountResults(History{SignupTagRules: []SignupTagRuleResult{{Disposition: DispositionQuarantine, Reason: "signup_tag_rule_shape_invalid"}}})
	if err != nil || counts.quarantine != 1 || counts.reasons["signup_tag_rule_shape_invalid"] != 1 {
		t.Fatalf("quarantine count rejected: counts=%+v err=%v", counts, err)
	}
}

package v1statictail

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"sort"
	"strings"
	"testing"

	appconfig "github.com/qianlan33333-png/AI-CRM-v2/internal/config"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var staticTailArchiveRun = flag.String("static-tail-archive-run", "", "optional reconciled V2 archive run for read-only static-tail candidate validation")

const (
	groupInviteLibraryTable                     = "public/group_invite_library"
	wechatPayProductPageSlicesTable             = "public/wechat_pay_product_page_slices"
	operationCycleStrategiesTable               = "public/operation_cycle_strategies"
	operationCycleStrategyVersionsTable         = "public/operation_cycle_strategy_versions"
	operationCycleStrategyVersionDocumentsTable = "public/operation_cycle_strategy_version_documents"
)

type staticTailTable struct {
	name   string
	count  int
	fields []string
}

var staticTailTables = []staticTailTable{
	{name: groupInviteLibraryTable, count: 4, fields: []string{"id", "name", "title", "description", "pic_url", "join_url", "config_id", "state", "chat_id_list", "auto_create_room", "room_base_name", "room_base_id", "enabled", "created_at", "updated_at", "chat_id", "binding_status"}},
	{name: wechatPayProductPageSlicesTable, count: 46, fields: []string{"id", "product_id", "image_library_id", "sort_order", "enabled", "created_at", "updated_at"}},
	{name: operationCycleStrategiesTable, count: 1, fields: []string{"id", "tenant_id", "strategy_key", "title", "description", "cadence", "timezone", "status", "current_version", "created_at", "updated_at"}},
	{name: operationCycleStrategyVersionsTable, count: 2, fields: []string{"id", "strategy_id", "version", "label", "objective", "definition_json", "version_hash", "effective_from", "created_at", "governance_status", "confirmed_by", "confirmed_at", "confirmation_note", "operation_skill_json", "operation_skill_hash"}},
	{name: operationCycleStrategyVersionDocumentsTable, count: 1, fields: []string{"id", "strategy_version_id", "schema_version", "execution_guide_markdown", "execution_guide_sha256", "execution_guide_generated_at", "execution_guide_source", "copy_guide_markdown", "copy_guide_sha256", "copy_guide_generated_at", "copy_guide_source", "measurement_guide_markdown", "measurement_guide_sha256", "measurement_guide_generated_at", "measurement_guide_source", "execution_contract_json", "document_pack_hash", "created_at"}},
}

// TestReconciledStaticTailArchivePreflight is opt-in and read-only. It never
// opens a target write transaction and logs only aggregate counts/redaction
// roots, not source payloads, identities, or archive contents.
func TestReconciledStaticTailArchivePreflight(t *testing.T) {
	if *staticTailArchiveRun == "" {
		t.Skip("supply -static-tail-archive-run and V2 archive environment for read-only static-tail preflight")
	}
	environment := appconfig.LoadV1ArchiveRuntimeEnvironment()
	archive, err := v1archive.OpenPostgresArchiveReader(context.Background(), environment.TargetDatabaseURL, []byte(environment.ArchiveKey))
	if err != nil {
		t.Fatal("static_tail_archive_open_failed")
	}
	defer archive.Close()

	records := make([][]SourceRecord, len(staticTailTables))
	redactions := make(map[string]int)
	for index, table := range staticTailTables {
		records[index], err = readStaticTailArchiveTable(context.Background(), archive, *staticTailArchiveRun, table, redactions)
		if err != nil {
			t.Fatal("static_tail_archive_read_failed")
		}
		if len(records[index]) != table.count {
			t.Fatal("static_tail_archive_count_mismatch")
		}
	}

	history := AdaptHistory(records[0], records[1], records[2], records[3], records[4])
	if history.SourceCount() != 54 || history.TerminalCount() != 54 {
		t.Fatal("static_tail_candidate_row_conservation_failed")
	}
	counts, err := staticTailCountResults(history)
	if err != nil {
		t.Fatal("static_tail_candidate_disposition_invalid")
	}
	logStaticTailPreflight(t, counts, redactions)
}

func readStaticTailArchiveTable(ctx context.Context, archive *v1archive.PostgresArchiveReader, runID string, table staticTailTable, redactions map[string]int) ([]SourceRecord, error) {
	records := make([]SourceRecord, 0, table.count)
	seen := make(map[[sha256.Size]byte]bool)
	err := archive.EachTableRow(ctx, runID, table.name, func(row v1archive.ArchivedRow) error {
		if reason := staticTailArchiveRowReason(row, table.name, int64(len(records)+1)); reason != "" {
			return errors.New(reason)
		}
		if seen[row.SourceKeyHMAC] {
			return errors.New("archive_duplicate_source_key")
		}
		if !json.Valid(row.Payload) {
			return errors.New("archive_payload_invalid")
		}
		seen[row.SourceKeyHMAC] = true
		for _, path := range row.RedactedFields {
			redactions[staticTailRedactionRoot(table, path)]++
		}
		records = append(records, staticTailCandidateRecord(table, row))
		return nil
	})
	if err != nil {
		return nil, errors.New("archive_read_failed")
	}
	return records, nil
}

func staticTailArchiveRowReason(row v1archive.ArchivedRow, table string, ordinal int64) string {
	if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != table || row.SourceOrdinal != ordinal {
		return "archive_scope_or_ordinal_mismatch"
	}
	if row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) || row.FieldHMAC == ([sha256.Size]byte{}) {
		return "archive_hmac_missing"
	}
	return ""
}

// staticTailCandidatePayload prevents a redacted manifest field from being
// mistaken for either a source NULL or its former value. The adapter receives
// an invalid shape and emits its fixed table-specific quarantine reason.
func staticTailCandidatePayload(table staticTailTable, row v1archive.ArchivedRow) json.RawMessage {
	if staticTailManifestFieldRedacted(table, row) {
		return json.RawMessage(`{}`)
	}
	return append(json.RawMessage(nil), row.Payload...)
}

func staticTailCandidateRecord(table staticTailTable, row v1archive.ArchivedRow) SourceRecord {
	return SourceRecord{Payload: staticTailCandidatePayload(table, row), PayloadHMAC: OpaqueDigest(row.PayloadHMAC)}
}

func staticTailManifestFieldRedacted(table staticTailTable, row v1archive.ArchivedRow) bool {
	for _, path := range row.RedactedFields {
		root := staticTailRedactionRoot(table, path)
		if root != table.name+".unknown" {
			return true
		}
	}
	return false
}

// staticTailRedactionRoot collapses a known nested path to its frozen
// manifest field. Unknown paths intentionally remain unknown rather than
// being interpreted as a current schema field.
func staticTailRedactionRoot(table staticTailTable, path string) string {
	for _, field := range table.fields {
		if path == field || strings.HasPrefix(path, field+".") || strings.HasPrefix(path, field+"[") {
			return table.name + "." + field
		}
	}
	return table.name + ".unknown"
}

type staticTailResult interface {
	staticTailState() (Disposition, bool, string)
}

func (value GroupInviteResult) staticTailState() (Disposition, bool, string) {
	return staticTailResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value ProductPageSliceResult) staticTailState() (Disposition, bool, string) {
	return staticTailResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value OperationCycleStrategyResult) staticTailState() (Disposition, bool, string) {
	return staticTailResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value OperationCycleVersionResult) staticTailState() (Disposition, bool, string) {
	return staticTailResultState(value.Disposition, value.Fact != nil, value.Reason)
}
func (value OperationCycleDocumentResult) staticTailState() (Disposition, bool, string) {
	return staticTailResultState(value.Disposition, value.Fact != nil, value.Reason)
}

func staticTailResultState(disposition Disposition, hasFact bool, reason string) (Disposition, bool, string) {
	switch disposition {
	case DispositionCandidate:
		return disposition, hasFact && reason == "", ""
	case DispositionQuarantine:
		return disposition, !hasFact && validStaticTailQuarantineReason(reason), reason
	default:
		return disposition, false, ""
	}
}

func validStaticTailQuarantineReason(reason string) bool {
	switch reason {
	case "group_invite_library_shape_invalid", "group_invite_library_source_ambiguous",
		"wechat_pay_product_page_slice_shape_invalid", "wechat_pay_product_page_slice_source_ambiguous",
		"operation_cycle_strategy_shape_invalid", "operation_cycle_strategy_source_ambiguous", "operation_cycle_strategy_current_version_unresolved",
		"operation_cycle_strategy_version_shape_invalid", "operation_cycle_strategy_version_source_ambiguous", "operation_cycle_strategy_version_strategy_unresolved",
		"operation_cycle_strategy_version_document_shape_invalid", "operation_cycle_strategy_version_document_source_ambiguous", "operation_cycle_strategy_version_document_version_unresolved":
		return true
	default:
		return false
	}
}

type staticTailCounts struct {
	groupInvites, pageSlices, strategies, versions, documents, quarantine int
	reasons                                                               map[string]int
}

func staticTailCountResults(history History) (staticTailCounts, error) {
	counts := staticTailCounts{reasons: make(map[string]int)}
	if err := countStaticTailResults(history.GroupInvites, &counts, func() { counts.groupInvites++ }); err != nil {
		return staticTailCounts{}, err
	}
	if err := countStaticTailResults(history.PageSlices, &counts, func() { counts.pageSlices++ }); err != nil {
		return staticTailCounts{}, err
	}
	if err := countStaticTailResults(history.Strategies, &counts, func() { counts.strategies++ }); err != nil {
		return staticTailCounts{}, err
	}
	if err := countStaticTailResults(history.Versions, &counts, func() { counts.versions++ }); err != nil {
		return staticTailCounts{}, err
	}
	if err := countStaticTailResults(history.Documents, &counts, func() { counts.documents++ }); err != nil {
		return staticTailCounts{}, err
	}
	return counts, nil
}

func countStaticTailResults[T staticTailResult](values []T, counts *staticTailCounts, candidate func()) error {
	for _, value := range values {
		disposition, consistent, reason := value.staticTailState()
		if !consistent {
			return errors.New("candidate_disposition_invalid")
		}
		if disposition == DispositionCandidate {
			candidate()
			continue
		}
		counts.quarantine++
		counts.reasons[reason]++
	}
	return nil
}

func logStaticTailPreflight(t *testing.T, counts staticTailCounts, redactions map[string]int) {
	for _, root := range sortedStaticTailKeys(redactions) {
		t.Logf("redaction_root=%s count=%d", root, redactions[root])
	}
	t.Logf("read-only static-tail preflight: source_group_invites=4 source_page_slices=46 source_strategies=1 source_versions=2 source_documents=1 candidate_group_invites=%d candidate_page_slices=%d candidate_strategies=%d candidate_versions=%d candidate_documents=%d quarantine_rows=%d target_writes=0", counts.groupInvites, counts.pageSlices, counts.strategies, counts.versions, counts.documents, counts.quarantine)
}

func sortedStaticTailKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestStaticTailArchiveIdentityAndRedactionRoots(t *testing.T) {
	table := staticTailTables[3]
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table.name, SourceOrdinal: 1, SourceKeyHMAC: [sha256.Size]byte{1}, PayloadHMAC: [sha256.Size]byte{2}, FieldHMAC: [sha256.Size]byte{3}, Payload: json.RawMessage(`{"definition_json":{"private":"no-leak"}}`), RedactedFields: []string{"definition_json.nested", "unknown.path"}}
	if reason := staticTailArchiveRowReason(row, table.name, 1); reason != "" {
		t.Fatal(reason)
	}
	if !staticTailManifestFieldRedacted(table, row) {
		t.Fatal("redacted manifest field accepted")
	}
	if got := staticTailRedactionRoot(table, "definition_json.nested"); got != table.name+".definition_json" {
		t.Fatalf("nested manifest field root=%q", got)
	}
	if got := staticTailRedactionRoot(table, "definition_json[0].nested"); got != table.name+".definition_json" {
		t.Fatalf("array manifest field root=%q", got)
	}
	if got := staticTailRedactionRoot(table, "unknown.path"); got != table.name+".unknown" {
		t.Fatalf("unknown nested field root=%q", got)
	}
	if got := staticTailCandidatePayload(table, row); string(got) != "{}" {
		t.Fatalf("redacted field reused archive payload: %q", got)
	}
	for _, mutate := range []func(*v1archive.ArchivedRow){
		func(value *v1archive.ArchivedRow) { value.AdapterID = "wrong" },
		func(value *v1archive.ArchivedRow) { value.TableID = groupInviteLibraryTable },
		func(value *v1archive.ArchivedRow) { value.SourceOrdinal = 2 },
		func(value *v1archive.ArchivedRow) { value.SourceKeyHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.PayloadHMAC = [sha256.Size]byte{} },
		func(value *v1archive.ArchivedRow) { value.FieldHMAC = [sha256.Size]byte{} },
	} {
		changed := row
		mutate(&changed)
		if staticTailArchiveRowReason(changed, table.name, 1) == "" {
			t.Fatal("invalid archive identity accepted")
		}
	}
}

func TestStaticTailRequiredRedactionCannotBecomeNullOrOriginalValue(t *testing.T) {
	table := staticTailTables[0]
	row := v1archive.ArchivedRow{Payload: json.RawMessage(`{"room_base_id":123}`), RedactedFields: []string{"room_base_id[0].nested"}}
	if !staticTailManifestFieldRedacted(table, row) {
		t.Fatal("redacted nullable source field was treated as NULL")
	}
	if got := staticTailCandidatePayload(table, row); string(got) != "{}" {
		t.Fatalf("redacted nullable source field reused original: %q", got)
	}
	unknownOnly := v1archive.ArchivedRow{Payload: row.Payload, RedactedFields: []string{"unrelated.nested"}}
	if staticTailManifestFieldRedacted(table, unknownOnly) {
		t.Fatal("unknown source path was guessed as a manifest field")
	}
}

func TestStaticTailCountsRequireFixedQuarantineReasons(t *testing.T) {
	if _, err := staticTailCountResults(History{GroupInvites: []GroupInviteResult{{Disposition: DispositionCandidate}}}); err == nil {
		t.Fatal("candidate without fact accepted")
	}
	counts, err := staticTailCountResults(History{Documents: []OperationCycleDocumentResult{{Disposition: DispositionQuarantine, Reason: "operation_cycle_strategy_version_document_version_unresolved"}}})
	if err != nil || counts.quarantine != 1 || counts.reasons["operation_cycle_strategy_version_document_version_unresolved"] != 1 {
		t.Fatalf("fixed quarantine reason rejected: counts=%+v err=%v", counts, err)
	}
	if _, err := staticTailCountResults(History{Versions: []OperationCycleVersionResult{{Disposition: DispositionQuarantine, Reason: "free_form_reason"}}}); err == nil {
		t.Fatal("unknown quarantine reason accepted")
	}
}

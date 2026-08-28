package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1contacthistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type contactHistoryArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (fake *contactHistoryArchiveFake) EachTableRow(_ context.Context, runID, tableID string, callback func(v1archive.ArchivedRow) error) error {
	if runID != "archive-run" || callback == nil {
		return ErrInvalidScope
	}
	for _, row := range fake.rows[tableID] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type contactHistoryTxKey struct{}

type contactHistoryRuntimeFake struct {
	terminals map[string]TerminalReceipt
	sidebars  map[int64]contactport.HistoricalSidebarProfile
	results   map[int64]contactport.HistoricalOwnerMigrationResult
	customer  *int64

	writeErr, resolverErr          error
	retryOnce                      bool
	writes, resolverCalls, replays int
	records, commits, rollbacks    int
}

func newContactHistoryRuntimeFake() *contactHistoryRuntimeFake {
	return &contactHistoryRuntimeFake{terminals: map[string]TerminalReceipt{}, sidebars: map[int64]contactport.HistoricalSidebarProfile{}, results: map[int64]contactport.HistoricalOwnerMigrationResult{}}
}

func (fake *contactHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, sidebars, results := copyContactHistoryTerminals(fake.terminals), copyContactHistorySidebars(fake.sidebars), copyContactHistoryResults(fake.results)
		err := callback(context.WithValue(ctx, contactHistoryTxKey{}, true))
		if err != nil {
			fake.terminals, fake.sidebars, fake.results = terminals, sidebars, results
			fake.rollbacks++
			return err
		}
		if fake.retryOnce && attempt == 0 {
			fake.terminals, fake.sidebars, fake.results = terminals, sidebars, results
			continue
		}
		fake.commits++
		return nil
	}
}

func (fake *contactHistoryRuntimeFake) ValidateContactHistoryImportScope(run string) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (fake *contactHistoryRuntimeFake) LoadTerminal(ctx context.Context, tableID, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(contactHistoryTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	value, found := fake.terminals[contactHistoryTerminalKey(tableID, source)]
	return value, found, nil
}

func (fake *contactHistoryRuntimeFake) RecordTerminal(ctx context.Context, tableID string, receipt TerminalReceipt) error {
	if ctx.Value(contactHistoryTxKey{}) != true {
		return errors.New("missing transaction")
	}
	key := contactHistoryTerminalKey(tableID, SourceIdentifier(receipt.SourceKeyDigest))
	if previous, found := fake.terminals[key]; found && !reflect.DeepEqual(previous, receipt) {
		return ErrConflict
	}
	fake.records++
	fake.terminals[key] = receipt
	return nil
}

func (fake *contactHistoryRuntimeFake) LoadContactHistory(ctx context.Context, kind, source string) (contactport.ContactHistoryReceipt, bool, error) {
	tableID, err := contactHistoryTableForKind(kind)
	if err != nil {
		return contactport.ContactHistoryReceipt{}, false, err
	}
	terminal, found, err := fake.LoadTerminal(ctx, tableID, source)
	if err != nil || !found {
		return contactport.ContactHistoryReceipt{}, found, err
	}
	receipt, err := contactHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (fake *contactHistoryRuntimeFake) RecordContactHistory(ctx context.Context, receipt contactport.ContactHistoryReceipt) error {
	tableID, err := contactHistoryTableForKind(receipt.Kind)
	if err != nil {
		return err
	}
	terminal, err := contactHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return fake.RecordTerminal(ctx, tableID, terminal)
}

func (fake *contactHistoryRuntimeFake) ResolveHistoricalContactCustomer(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(contactHistoryTxKey{}) != true {
		return nil, errors.New("resolver outside transaction")
	}
	fake.resolverCalls++
	if fake.resolverErr != nil {
		return nil, fake.resolverErr
	}
	if fake.customer == nil {
		return nil, nil
	}
	value := *fake.customer
	return &value, nil
}

func (fake *contactHistoryRuntimeFake) WriteSidebarProfile(ctx context.Context, source string, payload [sha256.Size]byte, value contactport.HistoricalSidebarProfile) (contactport.ContactHistoryReceipt, error) {
	if ctx.Value(contactHistoryTxKey{}) != true {
		return contactport.ContactHistoryReceipt{}, errors.New("writer outside transaction")
	}
	fake.writes++
	if fake.writeErr != nil {
		return contactport.ContactHistoryReceipt{}, fake.writeErr
	}
	if receipt, found, err := fake.LoadContactHistory(ctx, contactport.ContactHistorySidebar, source); err != nil || found {
		if err != nil || receipt.PayloadDigest != payload {
			return contactport.ContactHistoryReceipt{}, contactport.ErrContactHistoryConflict
		}
		actual, exists := fake.sidebars[receipt.TargetID]
		value.ID = receipt.TargetID
		if !exists || !reflect.DeepEqual(actual, value) {
			return contactport.ContactHistoryReceipt{}, contactport.ErrContactHistoryConflict
		}
		fake.replays++
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(100 + len(fake.sidebars))
	fake.sidebars[value.ID] = value
	receipt := contactport.ContactHistoryReceipt{Kind: contactport.ContactHistorySidebar, SourceIdentifier: source, PayloadDigest: payload,
		TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("sidebar/" + source))}
	if err := fake.RecordContactHistory(ctx, receipt); err != nil {
		return contactport.ContactHistoryReceipt{}, err
	}
	return receipt, nil
}

func (fake *contactHistoryRuntimeFake) WriteOwnerMigrationResult(ctx context.Context, source string, payload [sha256.Size]byte, value contactport.HistoricalOwnerMigrationResult) (contactport.ContactHistoryReceipt, error) {
	if ctx.Value(contactHistoryTxKey{}) != true {
		return contactport.ContactHistoryReceipt{}, errors.New("writer outside transaction")
	}
	fake.writes++
	if fake.writeErr != nil {
		return contactport.ContactHistoryReceipt{}, fake.writeErr
	}
	if receipt, found, err := fake.LoadContactHistory(ctx, contactport.ContactHistoryOwnerResult, source); err != nil || found {
		if err != nil || receipt.PayloadDigest != payload {
			return contactport.ContactHistoryReceipt{}, contactport.ErrContactHistoryConflict
		}
		actual, exists := fake.results[receipt.TargetID]
		value.ID = receipt.TargetID
		if !exists || !reflect.DeepEqual(actual, value) {
			return contactport.ContactHistoryReceipt{}, contactport.ErrContactHistoryConflict
		}
		fake.replays++
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(200 + len(fake.results))
	fake.results[value.ID] = value
	receipt := contactport.ContactHistoryReceipt{Kind: contactport.ContactHistoryOwnerResult, SourceIdentifier: source, PayloadDigest: payload,
		TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("owner-result/" + source))}
	if err := fake.RecordContactHistory(ctx, receipt); err != nil {
		return contactport.ContactHistoryReceipt{}, err
	}
	return receipt, nil
}

func contactHistoryTableForKind(kind string) (string, error) {
	switch kind {
	case contactport.ContactHistorySidebar:
		return v1contacthistory.SidebarProfileFieldsTableID, nil
	case contactport.ContactHistoryOwnerResult:
		return v1contacthistory.OwnerMigrationResultsTableID, nil
	default:
		return "", ErrInvalidScope
	}
}

func contactHistoryTerminalKey(tableID, source string) string { return tableID + "\x00" + source }

func copyContactHistoryTerminals(values map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyContactHistorySidebars(values map[int64]contactport.HistoricalSidebarProfile) map[int64]contactport.HistoricalSidebarProfile {
	copy := make(map[int64]contactport.HistoricalSidebarProfile, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyContactHistoryResults(values map[int64]contactport.HistoricalOwnerMigrationResult) map[int64]contactport.HistoricalOwnerMigrationResult {
	copy := make(map[int64]contactport.HistoricalOwnerMigrationResult, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func contactHistoryRow(t *testing.T, tableID string, ordinal int64, value map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(tableID + "/key/" + strconv.FormatInt(ordinal, 10))),
		PayloadHMAC:   sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(tableID + "/fields/" + strconv.FormatInt(ordinal, 10))),
		Payload: payload, RedactedFields: redacted}
}

func contactHistorySidebarRow(t *testing.T, ordinal int64, mutate func(map[string]any)) v1archive.ArchivedRow {
	t.Helper()
	value := map[string]any{"source": "sidebar", "industry": "education", "industry_description": "history", "needs_blockers_followup": "follow up",
		"updated_by": "never-copy", "updated_at": "2026-08-01T09:02:03.123456+08:00", "unionid": "verified-union"}
	if mutate != nil {
		mutate(value)
	}
	return contactHistoryRow(t, v1contacthistory.SidebarProfileFieldsTableID, ordinal, value)
}

func contactHistorySessionRow(t *testing.T, ordinal int64, sessionID string) v1archive.ArchivedRow {
	t.Helper()
	return contactHistoryRow(t, v1contacthistory.OwnerMigrationSessionsTableID, ordinal, map[string]any{"session_id": sessionID, "file_name": "private.csv", "file_hash": "hash",
		"source_owner_userid": "old", "target_owner_userid": "new", "include_wecom_transfer": false, "transfer_welcome_msg": "", "rows_json": []any{},
		"row_stats_json": map[string]any{}, "operator": "never-copy", "created_at": "2026-08-01T01:02:03Z"})
}

func contactHistoryPreviewRow(t *testing.T, ordinal int64, sessionID, resultID string) v1archive.ArchivedRow {
	t.Helper()
	row := contactHistoryRow(t, v1contacthistory.OwnerMigrationPreviewsTableID, ordinal, map[string]any{"preview_token": "[REDACTED]", "preview_hash": "hash", "scope_type": "all",
		"session_id": sessionID, "file_hash": "hash", "source_owner_userid": "old", "target_owner_userid": "new", "source_owner_display_name": "old display",
		"target_owner_display_name": "new display", "include_wecom_transfer": false, "transfer_welcome_msg": "", "eligible_external_userids_json": []any{},
		"rows_json": []any{}, "row_stats_json": map[string]any{}, "surface_counts_json": map[string]any{}, "pending_review_json": map[string]any{},
		"confirm_phrase": "never-copy", "operator": "never-copy", "created_at": "2026-08-01T01:02:03Z", "expires_at": "2026-08-02T01:02:03Z", "executed_result_id": resultID}, "preview_token")
	return row
}

func contactHistoryResultRow(t *testing.T, ordinal int64, resultID, sessionID string) v1archive.ArchivedRow {
	t.Helper()
	return contactHistoryRow(t, v1contacthistory.OwnerMigrationResultsTableID, ordinal, map[string]any{"result_id": resultID, "job_id": "never-copy", "preview_token": "[REDACTED]",
		"scope_type": "all", "session_id": sessionID, "file_hash": "file-hash", "source_owner_userid": "old", "target_owner_userid": "new",
		"source_owner_display_name": "old display", "target_owner_display_name": "new display", "operator": "never-copy", "preview_hash": "preview-hash",
		"total_rows": 2, "eligible_count": 1, "wecom_success": 1, "wecom_failed": 1, "crm_updated": 1, "include_wecom_transfer": true,
		"transfer_welcome_msg": "historical only", "rows_json": []any{}, "stats_json": map[string]any{"preview_token": "[REDACTED]"},
		"created_at": "2026-08-01T01:02:03Z", "executed_at": "2026-08-01T01:03:03Z"}, "preview_token", "stats_json.preview_token")
}

func contactHistoryImporterFixture(t *testing.T, rows map[string][]v1archive.ArchivedRow) (*ContactHistoryImporter, *contactHistoryRuntimeFake) {
	t.Helper()
	runtime := newContactHistoryRuntimeFake()
	importer, err := NewContactHistoryImporter(&contactHistoryArchiveFake{rows: rows}, runtime, runtime, runtime, runtime)
	if err != nil {
		t.Fatal("create_importer_failed")
	}
	return importer, runtime
}

func TestContactHistoryImporterPreservesBusinessHistoryAndArchivesContext(t *testing.T) {
	rows := map[string][]v1archive.ArchivedRow{
		v1contacthistory.SidebarProfileFieldsTableID:   {contactHistorySidebarRow(t, 1, nil)},
		v1contacthistory.OwnerMigrationSessionsTableID: {contactHistorySessionRow(t, 1, "session-1")},
		v1contacthistory.OwnerMigrationPreviewsTableID: {contactHistoryPreviewRow(t, 1, "session-1", "result-1")},
		v1contacthistory.OwnerMigrationResultsTableID:  {contactHistoryResultRow(t, 1, "result-1", "session-1")},
	}
	importer, runtime := contactHistoryImporterFixture(t, rows)
	customerID := int64(41)
	runtime.customer = &customerID
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ContactHistoryImportResult{Imported: 2, Archived: 2}) || runtime.resolverCalls != 1 || runtime.writes != 2 {
		t.Fatalf("history_not_imported result=%#v err=%v", result, err)
	}
	sidebar := runtime.sidebars[100]
	if sidebar.CustomerID == nil || *sidebar.CustomerID != customerID || sidebar.UpdatedAt != time.Date(2026, 8, 1, 1, 2, 3, 123456000, time.UTC) {
		t.Fatal("sidebar_history_not_preserved")
	}
	owner := runtime.results[200]
	if owner.SessionRelation != v1contacthistory.OwnerSessionRelationResolved || owner.PreviewRelation != v1contacthistory.OwnerPreviewRelationResolved ||
		owner.WeComSuccess != 1 || owner.CreatedAt != time.Date(2026, 8, 1, 1, 2, 3, 0, time.UTC) {
		t.Fatal("owner_history_or_relation_not_preserved")
	}
	if encoded, marshalErr := json.Marshal(owner); marshalErr != nil || strings.Contains(string(encoded), "session-1") || strings.Contains(string(encoded), "never-copy") {
		t.Fatal("context_or_command_material_reached_target")
	}
	for _, tableID := range []string{v1contacthistory.OwnerMigrationSessionsTableID, v1contacthistory.OwnerMigrationPreviewsTableID} {
		row := rows[tableID][0]
		terminal := runtime.terminals[contactHistoryTerminalKey(tableID, SourceIdentifier(row.SourceKeyHMAC))]
		if terminal.Disposition != "archive" || terminal.Reason != contactHistoryContextArchiveReason || terminal.TargetID != "" {
			t.Fatal("context_not_archived")
		}
	}
}

func TestContactHistoryImporterKeepsEmptyOwnerSessionUnresolved(t *testing.T) {
	rows := map[string][]v1archive.ArchivedRow{
		v1contacthistory.SidebarProfileFieldsTableID:   {},
		v1contacthistory.OwnerMigrationSessionsTableID: {},
		v1contacthistory.OwnerMigrationPreviewsTableID: {contactHistoryPreviewRow(t, 1, "", "result-1")},
		v1contacthistory.OwnerMigrationResultsTableID:  {contactHistoryResultRow(t, 1, "result-1", "")},
	}
	importer, runtime := contactHistoryImporterFixture(t, rows)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ContactHistoryImportResult{Imported: 1, Archived: 1}) {
		t.Fatalf("empty_session_not_preserved result=%#v err=%v", result, err)
	}
	value := runtime.results[200]
	if value.SessionRelation != v1contacthistory.OwnerSessionRelationUnresolved || value.PreviewRelation != v1contacthistory.OwnerPreviewRelationUnresolved {
		t.Fatal("empty_session_relation_guessed")
	}
}

func TestContactHistoryImporterQuarantinesOnlyInvalidBusinessRows(t *testing.T) {
	sidebar := contactHistorySidebarRow(t, 1, nil)
	sidebar.RedactedFields = []string{"industry"}
	rows := map[string][]v1archive.ArchivedRow{
		v1contacthistory.SidebarProfileFieldsTableID:   {sidebar},
		v1contacthistory.OwnerMigrationSessionsTableID: {contactHistorySessionRow(t, 1, "session-1"), contactHistorySessionRow(t, 2, "session-2")},
		v1contacthistory.OwnerMigrationPreviewsTableID: {contactHistoryPreviewRow(t, 1, "session-2", "result-1")},
		v1contacthistory.OwnerMigrationResultsTableID:  {contactHistoryResultRow(t, 1, "result-1", "session-1")},
	}
	importer, runtime := contactHistoryImporterFixture(t, rows)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ContactHistoryImportResult{Archived: 3, Quarantined: 2}) || runtime.writes != 0 {
		t.Fatalf("invalid_rows_not_isolated result=%#v err=%v", result, err)
	}
	if terminal := runtime.terminals[contactHistoryTerminalKey(v1contacthistory.SidebarProfileFieldsTableID, SourceIdentifier(sidebar.SourceKeyHMAC))]; terminal.Reason != v1contacthistory.ReasonRetainedFieldRedacted {
		t.Fatal("sidebar_quarantine_reason_changed")
	}
}

func TestContactHistoryImporterRejectsArchiveIntegrityBeforeWrites(t *testing.T) {
	bad := contactHistorySidebarRow(t, 2, nil)
	importer, runtime := contactHistoryImporterFixture(t, map[string][]v1archive.ArchivedRow{
		v1contacthistory.SidebarProfileFieldsTableID: {bad},
	})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.writes != 0 || runtime.records != 0 {
		t.Fatal("invalid_archive_was_processed")
	}
}

func TestContactHistoryImporterReplaysAndResetsRetryOutcome(t *testing.T) {
	rows := map[string][]v1archive.ArchivedRow{v1contacthistory.SidebarProfileFieldsTableID: {contactHistorySidebarRow(t, 1, nil)}}
	importer, runtime := contactHistoryImporterFixture(t, rows)
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (ContactHistoryImportResult{Imported: 1}) {
		t.Fatal("first_import_failed")
	}
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (ContactHistoryImportResult{Imported: 1, Replayed: 1}) || runtime.replays != 1 {
		t.Fatal("actual_replay_not_verified")
	}
	retryImporter, retryRuntime := contactHistoryImporterFixture(t, rows)
	retryRuntime.retryOnce = true
	if result, err := retryImporter.Import(context.Background(), "archive-run"); err != nil || result != (ContactHistoryImportResult{Imported: 1}) || len(retryRuntime.sidebars) != 1 || retryRuntime.writes != 2 {
		t.Fatal("retry_result_not_reset")
	}
}

func TestContactHistoryImporterQuarantinesWriterInputAndRejectsResolverFailure(t *testing.T) {
	rows := map[string][]v1archive.ArchivedRow{v1contacthistory.SidebarProfileFieldsTableID: {contactHistorySidebarRow(t, 1, nil)}}
	importer, runtime := contactHistoryImporterFixture(t, rows)
	runtime.writeErr = contactport.ErrContactHistoryInvalid
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (ContactHistoryImportResult{Quarantined: 1}) || len(runtime.sidebars) != 0 {
		t.Fatal("invalid_target_not_quarantined")
	}
	importer, runtime = contactHistoryImporterFixture(t, rows)
	runtime.resolverErr = errors.New("db unavailable")
	if _, err := importer.Import(context.Background(), "archive-run"); err == nil || len(runtime.terminals) != 0 {
		t.Fatal("resolver_failure_was_hidden_or_recorded")
	}
}

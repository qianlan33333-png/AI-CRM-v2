package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1audiencehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	segmentport "github.com/qianlan33333-png/AI-CRM-v2/internal/segment/port"
)

type audienceImportTxKey struct{}

type audienceImportUOW struct{ commits, rollbacks int }

func (uow *audienceImportUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if err := callback(context.WithValue(ctx, audienceImportTxKey{}, uow)); err != nil {
		uow.rollbacks++
		return err
	}
	uow.commits++
	return nil
}

type audienceArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *audienceArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type audienceResolverFake struct {
	customers, staff  int
	customer, staffID *int64
	err               error
	uow               *audienceImportUOW
}

func (fake *audienceResolverFake) ResolveAudienceHistoryCustomer(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(audienceImportTxKey{}) != fake.uow {
		return nil, errors.New("customer resolver without caller transaction")
	}
	fake.customers++
	return fake.customer, fake.err
}
func (fake *audienceResolverFake) ResolveAudienceHistoryStaff(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(audienceImportTxKey{}) != fake.uow {
		return nil, errors.New("staff resolver without caller transaction")
	}
	fake.staff++
	return fake.staffID, fake.err
}

type audienceWriterFake struct {
	uow             *audienceImportUOW
	calls           map[string]int
	lastGroup       segmentport.HistoricalAudienceGroup
	lastPackage     segmentport.HistoricalAudiencePackage
	lastVersion     segmentport.HistoricalAudienceVersion
	lastSender      segmentport.HistoricalAudienceSender
	lastRule        segmentport.HistoricalAudienceRule
	lastRuleVersion segmentport.HistoricalAudienceRuleVersion
	lastDefinition  segmentport.HistoricalAudienceDefinition
	lastMember      segmentport.HistoricalAudienceMember
}

func (writer *audienceWriterFake) receipt(ctx context.Context, kind, source string, payload [sha256.Size]byte) (segmentport.AudienceHistoryReceipt, error) {
	if ctx.Value(audienceImportTxKey{}) != writer.uow {
		return segmentport.AudienceHistoryReceipt{}, errors.New("writer without caller transaction")
	}
	writer.calls[kind]++
	return segmentport.AudienceHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: int64(100 + writer.calls[kind]), TargetDigest: sha256.Sum256([]byte(kind + "/" + source)), Replayed: writer.calls[kind] > 1}, nil
}
func (writer *audienceWriterFake) WriteGroup(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceGroup) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastGroup = value
	return writer.receipt(ctx, "groups", source, payload)
}
func (writer *audienceWriterFake) WritePackage(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudiencePackage) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastPackage = value
	return writer.receipt(ctx, "packages", source, payload)
}
func (writer *audienceWriterFake) WriteVersion(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceVersion) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastVersion = value
	return writer.receipt(ctx, "versions", source, payload)
}
func (writer *audienceWriterFake) WriteSender(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceSender) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastSender = value
	return writer.receipt(ctx, "senders", source, payload)
}
func (writer *audienceWriterFake) WriteRule(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceRule) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastRule = value
	return writer.receipt(ctx, "rules", source, payload)
}
func (writer *audienceWriterFake) WriteRuleVersion(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceRuleVersion) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastRuleVersion = value
	return writer.receipt(ctx, "rule_versions", source, payload)
}
func (writer *audienceWriterFake) WriteDefinition(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceDefinition) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastDefinition = value
	return writer.receipt(ctx, "definitions", source, payload)
}
func (writer *audienceWriterFake) WriteMember(ctx context.Context, source string, payload [sha256.Size]byte, value segmentport.HistoricalAudienceMember) (segmentport.AudienceHistoryReceipt, error) {
	writer.lastMember = value
	return writer.receipt(ctx, "members", source, payload)
}

func audienceImporterFixture(t *testing.T, rows map[string][]v1archive.ArchivedRow) (*AudienceHistoryImporter, *audienceImportUOW, map[string]*journalTestTx, *audienceWriterFake) {
	t.Helper()
	uow, txs, journals := &audienceImportUOW{}, map[string]*journalTestTx{}, map[string]*Journal{}
	for _, scope := range audienceHistoryScopes {
		tx := &journalTestTx{}
		txs[scope.source] = tx
		journals[scope.source] = &Journal{scope: Scope{ImportVersion: "v1-audience-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID, TableID: scope.source, TargetDomain: "segment", TargetTable: scope.target}, tx: func(ctx context.Context) (pgx.Tx, error) {
			if ctx.Value(audienceImportTxKey{}) != uow {
				return nil, errors.New("journal without caller transaction")
			}
			return tx, nil
		}}
	}
	writer := &audienceWriterFake{uow: uow, calls: map[string]int{}}
	resolver := &audienceResolverFake{uow: uow}
	importer, err := NewAudienceHistoryImporter(&audienceArchiveFake{rows: rows}, uow, writer, resolver, journals, 19)
	if err != nil {
		t.Fatal(err)
	}
	return importer, uow, txs, writer
}

func audienceGroupRow(t *testing.T, ordinal int64) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{"id": int64(10), "name": "历史人群", "created_at": stamp, "updated_at": stamp})
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1audiencehistory.PackageGroupsTableID, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte("group/10")), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte("group fields")), Payload: payload}
}

func audienceFixtureRow(t *testing.T, table string, id int64, payloadValue map[string]any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: 1,
		SourceKeyHMAC: sha256.Sum256([]byte(table + "/" + strconv.FormatInt(id, 10))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte("fields/" + table)), Payload: payload}
}

func audienceFullFixtureRows(t *testing.T) map[string][]v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 9, 30, 0, 0, time.UTC)
	return map[string][]v1archive.ArchivedRow{
		v1audiencehistory.PackageGroupsTableID:   {audienceFixtureRow(t, v1audiencehistory.PackageGroupsTableID, 10, map[string]any{"id": int64(10), "name": "历史人群", "created_at": stamp, "updated_at": stamp})},
		v1audiencehistory.PackagesTableID:        {audienceFixtureRow(t, v1audiencehistory.PackagesTableID, 20, map[string]any{"id": int64(20), "package_key": "legacy_package_20", "name": "历史包", "natural_language_definition": "过去七天", "status": "active", "query_mode": "incremental", "identity_policy": "unionid", "current_version_id": int64(30), "incremental_enabled": true, "daily_enabled": false, "incremental_interval_seconds": int64(180), "daily_refresh_time": "08:00", "timezone": "Asia/Shanghai", "lookback_seconds": int64(86400), "last_incremental_watermark_at": nil, "last_daily_refreshed_at": nil, "next_incremental_refresh_at": nil, "next_daily_refresh_at": nil, "lease_token": "opaque", "lease_expires_at": nil, "paused_reason": "", "created_at": stamp, "updated_at": stamp, "group_id": int64(10)})},
		v1audiencehistory.PackageVersionsTableID: {audienceFixtureRow(t, v1audiencehistory.PackageVersionsTableID, 30, map[string]any{"id": int64(30), "package_id": int64(20), "version_number": int64(7), "status": "published", "incremental_sql_text": "opaque", "snapshot_sql_text": "opaque", "ai_prompt": "提示", "ai_rationale": "理由", "natural_language_explanation": "解释", "dependencies_json": map[string]any{}, "explain_json": map[string]any{}, "sample_rows_json": []any{}, "validation_errors_json": []any{}, "created_at": stamp, "published_at": stamp, "parameters_json": map[string]any{}, "simple_sql_text": "opaque", "simple_compiled_sql_text": "opaque", "template_key": "template", "template_version": int64(3), "template_params_json": map[string]any{}, "template_fingerprint": "fp"})},
		v1audiencehistory.PackageSendersTableID:  {audienceFixtureRow(t, v1audiencehistory.PackageSendersTableID, 40, map[string]any{"id": int64(40), "package_id": int64(20), "sender_userid": "sender-private", "display_name": "历史发送人", "priority": int64(4), "status": "enabled", "created_at": stamp, "updated_at": stamp})},
		v1audiencehistory.RulesTableID:           {audienceFixtureRow(t, v1audiencehistory.RulesTableID, 50, map[string]any{"id": int64(50), "rule_key": "v1_rule", "display_name": "历史规则", "description": "说明", "rule_type": "sql", "owner": "owner-private", "status": "published", "created_at": stamp, "updated_at": stamp})},
		v1audiencehistory.RuleVersionsTableID:    {audienceFixtureRow(t, v1audiencehistory.RuleVersionsTableID, 60, map[string]any{"id": int64(60), "rule_id": int64(50), "version": int64(2), "executor_type": "sql", "code_or_sql": "opaque", "params_schema": map[string]any{}, "output_schema": map[string]any{}, "refresh_policy": map[string]any{}, "status": "published", "published_at": stamp, "created_at": stamp})},
		v1audiencehistory.SegmentsTableID:        {audienceFixtureRow(t, v1audiencehistory.SegmentsTableID, 70, map[string]any{"id": int64(70), "segment_code": "legacy_70", "display_name": "历史细分", "description": "细分说明", "source_type": "sql", "sql_query": "opaque", "sql_params_json": map[string]any{}, "sql_dialect": "postgres", "status": "active", "version": int64(9), "created_by_agent": "agent-private", "created_by_session": "session-private", "cached_headcount": int64(8), "cached_sample_json": []any{}, "last_refreshed_at": stamp, "last_refresh_error": "", "usage_count": int64(2), "tags_json": []any{}, "created_at": stamp, "updated_at": stamp})},
		v1audiencehistory.AudienceMembersTableID: {audienceFixtureRow(t, v1audiencehistory.AudienceMembersTableID, 80, map[string]any{"id": int64(80), "package_id": int64(20), "identity_type": "unionid", "identity_value": "not-a-unionid", "status": "active", "mobile_hash": "private", "owner_userid": "owner-private", "event_source_key": "event-private", "payload_hash": "payload-private", "payload_json": map[string]any{}, "first_entered_at": stamp, "last_seen_at": stamp, "last_updated_at": stamp, "exited_at": nil, "created_at": stamp, "updated_at": stamp, "unionid": "unionid-private"})},
	}
}

func foundAudienceTerminal(row v1archive.ArchivedRow, target string, digest [sha256.Size]byte, disposition, reason string) journalTestRow {
	return func(values ...any) error {
		*values[0].(*[]byte), *values[1].(*string), *values[2].(*string) = row.PayloadHMAC[:], disposition, reason
		if len(values) == 9 && disposition == "import" {
			domain, table := "segment", audienceJournalTarget(row.TableID)
			*values[3].(**string), *values[4].(**string), *values[5].(**string), *values[6].(*[]byte) = &domain, &table, &target, digest[:]
		}
		if len(values) == 9 {
			*values[7].(*[]byte), *values[8].(*bool) = []byte(`{}`), true
			return nil
		}
		if disposition == "import" {
			*values[3].(**string), *values[4].(*[]byte) = &target, digest[:]
		}
		*values[5].(*bool) = true
		return nil
	}
}

func audienceJournalTarget(table string) string {
	for _, scope := range audienceHistoryScopes {
		if scope.source == table {
			return scope.target
		}
	}
	return ""
}

func missingAudienceTerminal() journalTestRow { return func(...any) error { return pgx.ErrNoRows } }

func TestAudienceHistoryImporterImportsAndReplaysGroup(t *testing.T) {
	row := audienceGroupRow(t, 1)
	importer, uow, txs, writer := audienceImporterFixture(t, map[string][]v1archive.ArchivedRow{row.TableID: {row}})
	digest := sha256.Sum256([]byte("groups/" + SourceIdentifier(row.SourceKeyHMAC)))
	txs[row.TableID].rows = append(txs[row.TableID].rows, foundAudienceTerminal(row, "101", digest, "import", ""))
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Imported: 1}) || writer.calls["groups"] != 1 || uow.commits != 1 || writer.lastGroup.SourceID != 10 {
		t.Fatalf("result=%+v err=%v writer=%+v uow=%+v", result, err, writer, uow)
	}
	txs[row.TableID].rows = append(txs[row.TableID].rows, foundAudienceTerminal(row, "102", digest, "import", ""))
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Imported: 1, Replayed: 1}) || writer.calls["groups"] != 2 {
		t.Fatalf("replay result=%+v err=%v calls=%d", result, err, writer.calls["groups"])
	}
}

func TestAudienceHistoryImporterMapsAllEightFrozenTablesAndReplays(t *testing.T) {
	rows := audienceFullFixtureRows(t)
	importer, _, txs, writer := audienceImporterFixture(t, rows)
	resolver := importer.resolver.(*audienceResolverFake)
	staffID, customerID := int64(501), int64(601)
	resolver.staffID, resolver.customer = &staffID, &customerID
	for _, scope := range audienceHistoryScopes {
		row := rows[scope.source][0]
		digest := sha256.Sum256([]byte(scope.kind + "/" + SourceIdentifier(row.SourceKeyHMAC)))
		txs[scope.source].rows = append(txs[scope.source].rows, foundAudienceTerminal(row, "101", digest, "import", ""))
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Imported: 8}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if resolver.staff != 2 || resolver.customers != 1 || writer.lastGroup.SourceID != 10 || writer.lastPackage.GroupHistoryID == nil || *writer.lastPackage.GroupHistoryID != 101 || writer.lastPackage.CurrentVersionSourceID == nil || *writer.lastPackage.CurrentVersionSourceID != 30 || writer.lastPackage.RuntimeDigest == [sha256.Size]byte{} {
		t.Fatalf("group/package mapping lost: writer=%+v resolver=%+v", writer, resolver)
	}
	if writer.lastVersion.PackageHistoryID != 101 || writer.lastVersion.DefinitionDigest == [sha256.Size]byte{} || writer.lastSender.PackageHistoryID != 101 || writer.lastSender.StaffID == nil || *writer.lastSender.StaffID != staffID || writer.lastRule.OwnerStaffID == nil || *writer.lastRule.OwnerStaffID != staffID || writer.lastRuleVersion.RuleHistoryID != 101 || writer.lastDefinition.Code != "legacy_70" || writer.lastDefinition.DefinitionDigest == [sha256.Size]byte{} || writer.lastMember.PackageHistoryID != 101 || writer.lastMember.CustomerID == nil || *writer.lastMember.CustomerID != customerID || writer.lastMember.IdentityKind != "unionid" || writer.lastMember.PayloadDigest == [sha256.Size]byte{} {
		t.Fatalf("typed history mapping lost: writer=%+v", writer)
	}
	for _, scope := range audienceHistoryScopes {
		row := rows[scope.source][0]
		digest := sha256.Sum256([]byte(scope.kind + "/" + SourceIdentifier(row.SourceKeyHMAC)))
		txs[scope.source].rows = append(txs[scope.source].rows, foundAudienceTerminal(row, "102", digest, "import", ""))
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Imported: 8, Replayed: 8}) {
		t.Fatalf("replay result=%+v err=%v", result, err)
	}
	for _, scope := range audienceHistoryScopes {
		if writer.calls[scope.kind] != 2 {
			t.Fatalf("writer %s calls=%d, want replay verification", scope.kind, writer.calls[scope.kind])
		}
	}
}

func TestAudienceHistoryImporterQuarantinesCurrentVersionRedactionBeforeParentWrites(t *testing.T) {
	rows := audienceFullFixtureRows(t)
	version := rows[v1audiencehistory.PackageVersionsTableID][0]
	version.RedactedFields = []string{"template_key"}
	rows[v1audiencehistory.PackageVersionsTableID][0] = version
	importer, _, txs, writer := audienceImporterFixture(t, rows)
	for _, scope := range audienceHistoryScopes {
		row := rows[scope.source][0]
		if scope.source == v1audiencehistory.PackagesTableID || scope.source == v1audiencehistory.PackageVersionsTableID || scope.source == v1audiencehistory.PackageSendersTableID || scope.source == v1audiencehistory.AudienceMembersTableID {
			txs[scope.source].rows = append(txs[scope.source].rows, missingAudienceTerminal(), missingAudienceTerminal(), foundAudienceTerminal(row, "", [sha256.Size]byte{}, "quarantine", map[string]string{v1audiencehistory.PackagesTableID: "audience_package_current_version_unresolved", v1audiencehistory.PackageVersionsTableID: "audience_required_field_redacted", v1audiencehistory.PackageSendersTableID: "audience_package_sender_package_unresolved", v1audiencehistory.AudienceMembersTableID: "audience_member_package_unresolved"}[scope.source]))
			continue
		}
		digest := sha256.Sum256([]byte(scope.kind + "/" + SourceIdentifier(row.SourceKeyHMAC)))
		txs[scope.source].rows = append(txs[scope.source].rows, foundAudienceTerminal(row, "101", digest, "import", ""))
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Imported: 4, Quarantined: 4}) || writer.calls["packages"] != 0 || writer.calls["versions"] != 0 || writer.calls["senders"] != 0 || writer.calls["members"] != 0 {
		t.Fatalf("redacted current version result=%+v err=%v calls=%+v", result, err, writer.calls)
	}
}

func TestAudienceHistoryImporterRollsBackResolverFailure(t *testing.T) {
	rows := audienceFullFixtureRows(t)
	importer, uow, txs, _ := audienceImporterFixture(t, rows)
	resolver := importer.resolver.(*audienceResolverFake)
	resolver.err = errors.New("dm01 unavailable")
	for _, scope := range audienceHistoryScopes[:3] {
		row := rows[scope.source][0]
		digest := sha256.Sum256([]byte(scope.kind + "/" + SourceIdentifier(row.SourceKeyHMAC)))
		txs[scope.source].rows = append(txs[scope.source].rows, foundAudienceTerminal(row, "101", digest, "import", ""))
	}
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, resolver.err) || uow.commits != 3 || uow.rollbacks != 1 || resolver.staff != 1 {
		t.Fatalf("resolver error=%v uow=%+v resolver=%+v", err, uow, resolver)
	}
}

func TestAudienceHistoryImporterQuarantinesRedactionAndRejectsBadArchiveIdentity(t *testing.T) {
	row := audienceGroupRow(t, 1)
	row.RedactedFields = []string{"name"}
	importer, _, txs, writer := audienceImporterFixture(t, map[string][]v1archive.ArchivedRow{row.TableID: {row}})
	loaded, loadErr := importer.loadRows(context.Background(), "archive-run", row.TableID)
	if loadErr != nil || len(loaded.redacted) != 1 || !loaded.redacted[0] {
		t.Fatalf("redaction precheck=%+v err=%v", loaded.redacted, loadErr)
	}
	tx := txs[row.TableID]
	tx.rows = append(tx.rows, missingAudienceTerminal(), missingAudienceTerminal(), foundAudienceTerminal(row, "", [sha256.Size]byte{}, "quarantine", "audience_required_field_redacted"))
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (AudienceHistoryImportResult{Quarantined: 1}) || writer.calls["groups"] != 0 {
		t.Fatalf("redaction result=%+v err=%v", result, err)
	}
	bad := audienceGroupRow(t, 0)
	importer, badUOW, _, _ := audienceImporterFixture(t, map[string][]v1archive.ArchivedRow{bad.TableID: {bad}})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || badUOW.commits != 0 {
		t.Fatalf("bad identity err=%v uow=%+v", err, badUOW)
	}
}

func TestNewAudienceHistoryImporterRejectsBadScope(t *testing.T) {
	importer, _, _, _ := audienceImporterFixture(t, nil)
	journals := make(map[string]*Journal, len(importer.journals))
	for table, journal := range importer.journals {
		copy := *journal
		journals[table] = &copy
	}
	journals[v1audiencehistory.SegmentsTableID].scope.TargetTable = "segments"
	if _, err := NewAudienceHistoryImporter(importer.archive, importer.uow, importer.writer, importer.resolver, journals, 19); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("scope err=%v", err)
	}
}

func TestNewAudienceHistoryImporterCopiesCallerJournalMap(t *testing.T) {
	base, _, _, _ := audienceImporterFixture(t, nil)
	caller := make(map[string]*Journal, len(base.journals))
	for table, journal := range base.journals {
		caller[table] = journal
	}
	importer, err := NewAudienceHistoryImporter(base.archive, base.uow, base.writer, base.resolver, caller, 19)
	if err != nil {
		t.Fatal(err)
	}
	delete(caller, v1audiencehistory.PackageGroupsTableID)
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (AudienceHistoryImportResult{}) {
		t.Fatalf("caller map mutation result=%+v err=%v", result, err)
	}
}

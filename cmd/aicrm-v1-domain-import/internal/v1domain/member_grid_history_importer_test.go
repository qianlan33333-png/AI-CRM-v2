package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1membergridhistory"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	productport "github.com/qianlan33333-png/AI-CRM-v2/internal/product/port"
)

type memberGridHistoryArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (fake *memberGridHistoryArchiveFake) EachTableRow(_ context.Context, runID, tableID string, callback func(v1archive.ArchivedRow) error) error {
	if runID != v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID || callback == nil {
		return ErrInvalidScope
	}
	for _, row := range fake.rows[tableID] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type memberGridHistoryTxKey struct{}

type memberGridHistoryRuntimeFake struct {
	terminals map[string]TerminalReceipt
	views     map[int64]productport.HistoricalMemberView
	usage     map[int64]productport.HistoricalMemberUsage

	product, customer *int64
	writeErr          error
	retryOnce         bool
	writes, resolves  int
}

func newMemberGridHistoryRuntimeFake() *memberGridHistoryRuntimeFake {
	return &memberGridHistoryRuntimeFake{terminals: map[string]TerminalReceipt{}, views: map[int64]productport.HistoricalMemberView{}, usage: map[int64]productport.HistoricalMemberUsage{}}
}

func (fake *memberGridHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, views, usage := copyMemberGridHistoryTerminals(fake.terminals), copyMemberGridHistoryViews(fake.views), copyMemberGridHistoryUsage(fake.usage)
		err := callback(context.WithValue(ctx, memberGridHistoryTxKey{}, true))
		if err != nil {
			fake.terminals, fake.views, fake.usage = terminals, views, usage
			return err
		}
		if fake.retryOnce && attempt == 0 {
			fake.terminals, fake.views, fake.usage = terminals, views, usage
			continue
		}
		return nil
	}
}

func (fake *memberGridHistoryRuntimeFake) ValidateMemberGridHistoryImportScope(run string) error {
	if run != v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID {
		return ErrInvalidScope
	}
	return nil
}

func (fake *memberGridHistoryRuntimeFake) LoadTerminal(ctx context.Context, tableID, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	value, found := fake.terminals[memberGridHistoryTerminalKey(tableID, source)]
	return value, found, nil
}

func (fake *memberGridHistoryRuntimeFake) RecordTerminal(ctx context.Context, tableID string, receipt TerminalReceipt) error {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return errors.New("missing transaction")
	}
	key := memberGridHistoryTerminalKey(tableID, SourceIdentifier(receipt.SourceKeyDigest))
	if current, found := fake.terminals[key]; found && !reflect.DeepEqual(current, receipt) {
		return ErrConflict
	}
	fake.terminals[key] = receipt
	return nil
}

func (fake *memberGridHistoryRuntimeFake) LoadMemberGridHistory(ctx context.Context, kind, source string) (productport.MemberGridHistoryReceipt, bool, error) {
	tableID, err := memberGridHistoryTableForKind(kind)
	if err != nil {
		return productport.MemberGridHistoryReceipt{}, false, err
	}
	terminal, found, err := fake.LoadTerminal(ctx, tableID, source)
	if err != nil || !found {
		return productport.MemberGridHistoryReceipt{}, found, err
	}
	receipt, receiptErr := memberGridHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, receiptErr == nil, receiptErr
}

func (fake *memberGridHistoryRuntimeFake) RecordMemberGridHistory(ctx context.Context, receipt productport.MemberGridHistoryReceipt) error {
	tableID, err := memberGridHistoryTableForKind(receipt.Kind)
	if err != nil {
		return err
	}
	terminal, err := memberGridHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return fake.RecordTerminal(ctx, tableID, terminal)
}

func (fake *memberGridHistoryRuntimeFake) ResolveHistoricalMemberGridCustomer(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return nil, errors.New("customer resolver outside transaction")
	}
	fake.resolves++
	return copyMemberGridHistoryID(fake.customer), nil
}

func (fake *memberGridHistoryRuntimeFake) ResolveHistoricalMemberGridProduct(ctx context.Context, _ int64) (*int64, error) {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return nil, errors.New("product resolver outside transaction")
	}
	fake.resolves++
	return copyMemberGridHistoryID(fake.product), nil
}

func (fake *memberGridHistoryRuntimeFake) WriteMemberView(ctx context.Context, source string, payload [sha256.Size]byte, value productport.HistoricalMemberView) (productport.MemberGridHistoryReceipt, error) {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return productport.MemberGridHistoryReceipt{}, errors.New("view writer outside transaction")
	}
	fake.writes++
	if fake.writeErr != nil {
		return productport.MemberGridHistoryReceipt{}, fake.writeErr
	}
	if receipt, found, err := fake.LoadMemberGridHistory(ctx, productport.MemberGridHistoryView, source); err != nil || found {
		if err != nil || receipt.PayloadDigest != payload {
			return productport.MemberGridHistoryReceipt{}, productport.ErrMemberGridHistoryConflict
		}
		value.ID = receipt.TargetID
		if !reflect.DeepEqual(fake.views[receipt.TargetID], value) {
			return productport.MemberGridHistoryReceipt{}, productport.ErrMemberGridHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(100 + len(fake.views))
	fake.views[value.ID] = value
	receipt := productport.MemberGridHistoryReceipt{Kind: productport.MemberGridHistoryView, SourceIdentifier: source, PayloadDigest: payload,
		TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("view/" + source))}
	if err := fake.RecordMemberGridHistory(ctx, receipt); err != nil {
		return productport.MemberGridHistoryReceipt{}, err
	}
	return receipt, nil
}

func (fake *memberGridHistoryRuntimeFake) WriteMemberUsage(ctx context.Context, source string, payload [sha256.Size]byte, value productport.HistoricalMemberUsage) (productport.MemberGridHistoryReceipt, error) {
	if ctx.Value(memberGridHistoryTxKey{}) != true {
		return productport.MemberGridHistoryReceipt{}, errors.New("usage writer outside transaction")
	}
	fake.writes++
	if fake.writeErr != nil {
		return productport.MemberGridHistoryReceipt{}, fake.writeErr
	}
	if receipt, found, err := fake.LoadMemberGridHistory(ctx, productport.MemberGridHistoryUsage, source); err != nil || found {
		if err != nil || receipt.PayloadDigest != payload {
			return productport.MemberGridHistoryReceipt{}, productport.ErrMemberGridHistoryConflict
		}
		value.ID = receipt.TargetID
		if !reflect.DeepEqual(fake.usage[receipt.TargetID], value) {
			return productport.MemberGridHistoryReceipt{}, productport.ErrMemberGridHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(200 + len(fake.usage))
	fake.usage[value.ID] = value
	receipt := productport.MemberGridHistoryReceipt{Kind: productport.MemberGridHistoryUsage, SourceIdentifier: source, PayloadDigest: payload,
		TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("usage/" + source))}
	if err := fake.RecordMemberGridHistory(ctx, receipt); err != nil {
		return productport.MemberGridHistoryReceipt{}, err
	}
	return receipt, nil
}

func memberGridHistoryTableForKind(kind string) (string, error) {
	switch kind {
	case productport.MemberGridHistoryView:
		return v1membergridhistory.MemberViewsTableID, nil
	case productport.MemberGridHistoryUsage:
		return v1membergridhistory.UsageSnapshotsTableID, nil
	default:
		return "", ErrInvalidScope
	}
}

func memberGridHistoryTerminalKey(tableID, source string) string { return tableID + "\x00" + source }

func copyMemberGridHistoryTerminals(values map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyMemberGridHistoryViews(values map[int64]productport.HistoricalMemberView) map[int64]productport.HistoricalMemberView {
	copy := make(map[int64]productport.HistoricalMemberView, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func copyMemberGridHistoryUsage(values map[int64]productport.HistoricalMemberUsage) map[int64]productport.HistoricalMemberUsage {
	copy := make(map[int64]productport.HistoricalMemberUsage, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func memberGridHistoryRow(t *testing.T, tableID string, ordinal int64, payload map[string]any, redacted ...string) v1archive.ArchivedRow {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: tableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(tableID + "/key/" + strconv.FormatInt(ordinal, 10))),
		PayloadHMAC:   sha256.Sum256(encoded), FieldHMAC: sha256.Sum256([]byte(tableID + "/fields/" + strconv.FormatInt(ordinal, 10))), Payload: encoded, RedactedFields: redacted}
}

func memberGridHistoryViewRow(t *testing.T, ordinal int64) v1archive.ArchivedRow {
	t.Helper()
	return memberGridHistoryRow(t, v1membergridhistory.MemberViewsTableID, ordinal, map[string]any{
		"id": 7, "tenant_id": "archived", "service_product_id": 11, "name": "old view", "position": -1, "is_default": true,
		"schema_version": -2, "config_json": map[string]any{"old": true}, "version": -3, "created_by": "archived", "updated_by": "archived",
		"created_at": "2026-08-01T01:02:03.123456+08:00", "updated_at": "2026-08-01T01:02:03.123456+08:00",
	})
}

func memberGridHistoryUsageRecovery(t *testing.T, hasTokenUsage bool) (v1archive.ArchivedRow, v1membergridhistory.UsageSnapshotRecoveryEntry, []byte) {
	t.Helper()
	key := bytes.Repeat([]byte{9}, sha256.Size)
	fullPayload, err := json.Marshal(map[string]any{
		"huangyoucan_user_id": "archived-user", "unionid": "archived-union", "mobile_md5": "archived-md5", "formally_logged_in": true,
		"has_token_usage": hasTokenUsage, "learning_plan_id": "plan", "learning_plan_current": nil, "learning_plan_total": nil,
		"open_count_7d": 2, "last_open_at": nil, "refreshed_at": "2026-08-01T01:02:03.123456+08:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	redacted, fields, err := v1archive.RedactPayload(fullPayload)
	if err != nil {
		t.Fatal(err)
	}
	sourceKeyJSON := []byte(`["archived-user"]`)
	source, err := v1archive.SourceKeyHMAC(key, "service_period_huangyoucan_usage_snapshot", sourceKeyJSON)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := v1archive.PayloadHMAC(key, "service_period_huangyoucan_usage_snapshot", redacted)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(key, "service_period_huangyoucan_usage_snapshot", fields)
	if err != nil {
		t.Fatal(err)
	}
	row := v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: v1membergridhistory.UsageSnapshotsTableID, SourceOrdinal: 1,
		SourceKeyHMAC: source, PayloadHMAC: payload, FieldHMAC: field, Payload: redacted, RedactedFields: fields}
	entry, err := v1membergridhistory.BuildUsageSnapshotRecoveryEntry(row, sourceKeyJSON, fullPayload, key, v1membergridhistory.FixedUsageSnapshotRecoveryScope())
	if err != nil {
		t.Fatal(err)
	}
	return row, entry, key
}

func memberGridHistoryContextRow(t *testing.T, tableID string) v1archive.ArchivedRow {
	t.Helper()
	now := "2026-08-01T01:02:03Z"
	switch tableID {
	case v1membergridhistory.UsageSyncRunsTableID:
		return memberGridHistoryRow(t, tableID, 1, map[string]any{"id": 1, "trigger_source": "old", "status": "done", "source_row_count": 1, "snapshot_row_count": 1, "started_at": now, "finished_at": now, "error_summary": ""})
	case v1membergridhistory.MemberCollaboratorsTableID:
		return memberGridHistoryRow(t, tableID, 1, map[string]any{"id": 1, "tenant_id": "old", "service_product_id": 1, "admin_user_id": 1, "wecom_userid": "old", "display_name": "old", "avatar_url": "", "permission": "admin", "version": 1, "created_by": "old", "updated_by": "old", "created_at": now, "updated_at": now})
	case v1membergridhistory.MemberSharesTableID:
		return memberGridHistoryRow(t, tableID, 1, map[string]any{"id": 1, "tenant_id": "old", "service_product_id": 1, "enabled": true, "public_id": "old", "generation": 1, "version": 1, "created_by": "old", "updated_by": "old", "created_at": now, "updated_at": now})
	default:
		t.Fatal("unknown context table")
		return v1archive.ArchivedRow{}
	}
}

func memberGridHistoryImporterFixture(t *testing.T, hasTokenUsage bool) (*MemberGridHistoryImporter, *memberGridHistoryRuntimeFake, v1membergridhistory.UsageSnapshotRecoveryEntry) {
	t.Helper()
	usage, recovery, key := memberGridHistoryUsageRecovery(t, hasTokenUsage)
	archive := &memberGridHistoryArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		v1membergridhistory.MemberViewsTableID:         {memberGridHistoryViewRow(t, 1)},
		v1membergridhistory.UsageSnapshotsTableID:      {usage},
		v1membergridhistory.UsageSyncRunsTableID:       {memberGridHistoryContextRow(t, v1membergridhistory.UsageSyncRunsTableID)},
		v1membergridhistory.MemberCollaboratorsTableID: {memberGridHistoryContextRow(t, v1membergridhistory.MemberCollaboratorsTableID)},
		v1membergridhistory.MemberSharesTableID:        {memberGridHistoryContextRow(t, v1membergridhistory.MemberSharesTableID)},
	}}
	runtime := newMemberGridHistoryRuntimeFake()
	product, customer := int64(9), int64(19)
	runtime.product, runtime.customer = &product, &customer
	importer, err := NewMemberGridHistoryImporter(archive, runtime, runtime, runtime, []v1membergridhistory.UsageSnapshotRecoveryEntry{recovery}, key, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return importer, runtime, recovery
}

func TestMemberGridHistoryImporterPreservesTwoBusinessFactsAndArchivesContext(t *testing.T) {
	for _, want := range []bool{false, true} {
		t.Run(strconv.FormatBool(want), func(t *testing.T) {
			importer, runtime, _ := memberGridHistoryImporterFixture(t, want)
			result, err := importer.Import(context.Background(), v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID)
			if err != nil || result != (MemberGridHistoryImportResult{ImportedViews: 1, ImportedUsage: 1, Archived: 3}) || runtime.writes != 2 || runtime.resolves != 2 {
				t.Fatalf("result=%#v writes=%d resolves=%d err=%v", result, runtime.writes, runtime.resolves, err)
			}
			view := runtime.views[100]
			usage := runtime.usage[200]
			if view.ProductID == nil || *view.ProductID != 9 || view.Position != -1 || view.SchemaVersion != -2 || view.Version != -3 ||
				usage.CustomerID == nil || *usage.CustomerID != 19 || usage.HasTokenUsage != want || usage.LastOpenAt != nil || usage.RefreshedAt.Location() != time.UTC {
				t.Fatal("historical_fact_changed_or_mapping_guessed")
			}
			if len(runtime.terminals) != 5 {
				t.Fatal("source_rows_not_conserved")
			}
		})
	}
}

func TestMemberGridHistoryImporterReplaysOnlyWithSameTarget(t *testing.T) {
	importer, runtime, _ := memberGridHistoryImporterFixture(t, true)
	run := v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID
	if _, err := importer.Import(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), run)
	if err != nil || result != (MemberGridHistoryImportResult{ImportedViews: 1, ImportedUsage: 1, Archived: 3, Replayed: 5}) || len(runtime.views) != 1 || len(runtime.usage) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMemberGridHistoryImporterFailsBeforeWritesForMissingOrTamperedRecovery(t *testing.T) {
	for _, mutate := range []func(*MemberGridHistoryImporter, *v1membergridhistory.UsageSnapshotRecoveryEntry){
		func(importer *MemberGridHistoryImporter, _ *v1membergridhistory.UsageSnapshotRecoveryEntry) {
			importer.recoveryEntries = nil
		},
		func(_ *MemberGridHistoryImporter, entry *v1membergridhistory.UsageSnapshotRecoveryEntry) {
			entry.HasTokenUsage = !entry.HasTokenUsage
		},
	} {
		importer, runtime, recovery := memberGridHistoryImporterFixture(t, true)
		mutate(importer, &importer.recoveryEntries[0])
		if importer.recoveryEntries == nil {
			// Constructor protects an empty input; this simulates a bad composition after construction.
			_ = recovery
		}
		if _, err := importer.Import(context.Background(), v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID); err == nil || runtime.writes != 0 || len(runtime.terminals) != 0 {
			t.Fatal("unauthenticated_recovery_reached_a_write")
		}
	}
}

func TestMemberGridHistoryImporterRollsBackWriterFailureAndDoesNotCreateRuntimeFacts(t *testing.T) {
	importer, runtime, _ := memberGridHistoryImporterFixture(t, true)
	runtime.writeErr = errors.New("write failed")
	if _, err := importer.Import(context.Background(), v1membergridhistory.FixedUsageSnapshotRecoveryScope().ArchiveRunID); err == nil {
		t.Fatal("writer_failure_accepted")
	}
	if len(runtime.views) != 0 || len(runtime.usage) != 0 || len(runtime.terminals) != 0 {
		t.Fatal("failed_write_left_history_or_runtime_facts")
	}
}

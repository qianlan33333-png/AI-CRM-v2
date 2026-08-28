package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type hxcImporterContextKey struct{}

type hxcImporterArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive hxcImporterArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
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

type hxcImporterUOW struct{ err error }

func (uow hxcImporterUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	if uow.err != nil {
		return uow.err
	}
	return callback(context.WithValue(ctx, hxcImporterContextKey{}, "transaction"))
}

type hxcImporterJournal struct {
	run       string
	terminals map[string]map[string]TerminalReceipt
}

func newHXCImporterJournal() *hxcImporterJournal {
	return &hxcImporterJournal{run: "archive-run", terminals: make(map[string]map[string]TerminalReceipt)}
}

func (journal *hxcImporterJournal) ValidateHXCHistoryImportScope(run string) error {
	if journal == nil || run != journal.run {
		return ErrInvalidScope
	}
	return nil
}

func (journal *hxcImporterJournal) LoadHXCHistoryTerminal(_ context.Context, kind, source string) (TerminalReceipt, bool, error) {
	if journal == nil || !validHXCHistoryKind(kind) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	value, found := journal.terminals[kind][source]
	return value, found, nil
}

func (journal *hxcImporterJournal) RecordHXCHistoryTerminal(_ context.Context, kind string, receipt TerminalReceipt) error {
	if journal == nil || !validHXCHistoryKind(kind) {
		return ErrInvalidScope
	}
	if journal.terminals[kind] == nil {
		journal.terminals[kind] = map[string]TerminalReceipt{}
	}
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if current, found := journal.terminals[kind][source]; found && !reflect.DeepEqual(current, receipt) {
		return ErrConflict
	}
	journal.terminals[kind][source] = receipt
	return nil
}

func (journal *hxcImporterJournal) LoadHXCHistory(ctx context.Context, kind, source string) (hxcport.HXCHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadHXCHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return hxcport.HXCHistoryReceipt{}, found, err
	}
	receipt, err := hxcHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *hxcImporterJournal) RecordHXCHistory(ctx context.Context, receipt hxcport.HXCHistoryReceipt) error {
	terminal, err := hxcHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordHXCHistoryTerminal(ctx, receipt.Kind, terminal)
}

type hxcImporterResolver struct {
	customer *int64
	err      error
	unionID  string
	ctx      context.Context
}

func (resolver *hxcImporterResolver) ResolveHXCHistoryCustomer(ctx context.Context, unionID string) (*int64, error) {
	resolver.ctx, resolver.unionID = ctx, unionID
	return copyInt64(resolver.customer), resolver.err
}

type hxcImporterWriter struct {
	journal *hxcImporterJournal
	nextID  int64
	invalid string
	calls   []string
	meta    hxcport.HistoricalHXCMeta
	snap    hxcport.HistoricalHXCSnapshot
	act     []hxcport.HistoricalHXCActivation
	lead    hxcport.HistoricalHXCLead
	batch   hxcport.HistoricalHXCBatch
}

func (writer *hxcImporterWriter) ImportMeta(ctx context.Context, source string, value hxcport.HistoricalHXCMeta) (hxcport.HXCHistoryReceipt, error) {
	writer.meta = value
	return writer.write(ctx, hxcport.HXCHistoryMeta, source, value.SourcePayloadDigest)
}

func (writer *hxcImporterWriter) ImportSnapshot(ctx context.Context, source string, value hxcport.HistoricalHXCSnapshot) (hxcport.HXCHistoryReceipt, error) {
	writer.snap = value
	return writer.write(ctx, hxcport.HXCHistorySnapshot, source, value.SourcePayloadDigest)
}

func (writer *hxcImporterWriter) ImportActivation(ctx context.Context, source string, value hxcport.HistoricalHXCActivation) (hxcport.HXCHistoryReceipt, error) {
	writer.act = append(writer.act, value)
	kind := hxcport.HXCHistoryActivationStatus
	if value.SourceTable == v1hxchistory.HuangxiaocanActivationID {
		kind = hxcport.HXCHistoryHuangxiaocanActivation
	}
	return writer.write(ctx, kind, source, value.SourcePayloadDigest)
}

func (writer *hxcImporterWriter) ImportLead(ctx context.Context, source string, value hxcport.HistoricalHXCLead) (hxcport.HXCHistoryReceipt, error) {
	writer.lead = value
	return writer.write(ctx, hxcport.HXCHistoryLead, source, value.SourcePayloadDigest)
}

func (writer *hxcImporterWriter) ImportBatch(ctx context.Context, source string, value hxcport.HistoricalHXCBatch) (hxcport.HXCHistoryReceipt, error) {
	writer.batch = value
	return writer.write(ctx, hxcport.HXCHistoryBatch, source, value.SourcePayloadDigest)
}

func (writer *hxcImporterWriter) write(ctx context.Context, kind, source string, payload [sha256.Size]byte) (hxcport.HXCHistoryReceipt, error) {
	if ctx.Value(hxcImporterContextKey{}) != "transaction" {
		return hxcport.HXCHistoryReceipt{}, errors.New("writer was outside caller transaction")
	}
	if writer.invalid == kind {
		return hxcport.HXCHistoryReceipt{}, hxcport.ErrHXCHistoryInvalid
	}
	if receipt, found, err := writer.journal.LoadHXCHistory(ctx, kind, source); err != nil {
		return hxcport.HXCHistoryReceipt{}, err
	} else if found {
		if receipt.PayloadDigest != payload {
			return hxcport.HXCHistoryReceipt{}, hxcport.ErrHXCHistoryConflict
		}
		receipt.Replayed = true
		return receipt, nil
	}
	writer.nextID++
	digest := sha256.Sum256([]byte(kind + "\x00" + source))
	receipt := hxcport.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: payload, TargetID: writer.nextID, TargetDigest: digest}
	if err := writer.journal.RecordHXCHistory(ctx, receipt); err != nil {
		return hxcport.HXCHistoryReceipt{}, err
	}
	writer.calls = append(writer.calls, kind)
	return receipt, nil
}

func TestHXCHistoryImporterConservesEightSourcesAndReplays(t *testing.T) {
	archive := hxcImporterFixture(t)
	journal := newHXCImporterJournal()
	customer := int64(71)
	resolver := &hxcImporterResolver{customer: &customer}
	writer := &hxcImporterWriter{journal: journal}
	importer, err := NewHXCHistoryImporter(archive, hxcImporterUOW{}, writer, resolver, journal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := importer.Import(context.Background(), "archive-run")
	if err != nil || first.SourceCount() != 8 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	for _, table := range []string{v1hxchistory.DashboardMetaTableID, v1hxchistory.DashboardSnapshotTableID, v1hxchistory.ActivationStatusTableID, v1hxchistory.HuangxiaocanActivationID, v1hxchistory.ExperienceLeadsTableID, v1hxchistory.ImportBatchesTableID} {
		if got := first.Tables[table]; got.Imported != 1 || got.Archived != 0 || got.Quarantined != 0 || got.Replayed != 0 {
			t.Fatalf("%s=%+v", table, got)
		}
	}
	for _, table := range []string{v1hxchistory.SendRecordsTableID, v1hxchistory.SendConfigTableID} {
		if got := first.Tables[table]; got.Archived != 1 || got.Imported != 0 || got.Quarantined != 0 {
			t.Fatalf("%s=%+v", table, got)
		}
	}
	if resolver.ctx == nil || resolver.ctx.Value(hxcImporterContextKey{}) != "transaction" || resolver.unionID != "union-a" || writer.snap.CustomerID == nil || *writer.snap.CustomerID != customer {
		t.Fatalf("snapshot resolver/value lost caller Tx or verified customer: resolver=%#v snapshot=%#v", resolver, writer.snap)
	}
	if writer.meta.SourceID != -1 || writer.snap.SourceID != 0 || len(writer.act) != 2 || writer.act[0].SourceTable != v1hxchistory.ActivationStatusTableID || writer.act[1].SourceTable != v1hxchistory.HuangxiaocanActivationID {
		t.Fatalf("signed/source-table facts changed: meta=%#v snapshot=%#v activation=%#v", writer.meta, writer.snap, writer.act)
	}
	if got, want := writer.calls, []string{hxcport.HXCHistoryMeta, hxcport.HXCHistorySnapshot, hxcport.HXCHistoryActivationStatus, hxcport.HXCHistoryHuangxiaocanActivation, hxcport.HXCHistoryLead, hxcport.HXCHistoryBatch}; !reflect.DeepEqual(got, want) {
		t.Fatalf("writer kinds=%v want=%v", got, want)
	}

	second, err := importer.Import(context.Background(), "archive-run")
	if err != nil || second.SourceCount() != 8 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	for _, table := range hxcHistoryTables {
		if got := second.Tables[table]; got.Replayed != 1 {
			t.Fatalf("replay table=%s result=%+v", table, got)
		}
	}
}

func TestHXCHistoryImporterQuarantinesRedactionAndInvalidOwnerWithoutWritingRuntime(t *testing.T) {
	archive := hxcImporterFixture(t)
	row := archive.rows[v1hxchistory.DashboardSnapshotTableID][0]
	row.RedactedFields = []string{"unionid"}
	archive.rows[v1hxchistory.DashboardSnapshotTableID][0] = row
	journal := newHXCImporterJournal()
	writer := &hxcImporterWriter{journal: journal, invalid: hxcport.HXCHistoryLead}
	importer, err := NewHXCHistoryImporter(archive, hxcImporterUOW{}, writer, &hxcImporterResolver{}, journal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result.SourceCount() != 8 || result.Tables[v1hxchistory.DashboardSnapshotTableID].Quarantined != 1 || result.Tables[v1hxchistory.ExperienceLeadsTableID].Quarantined != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(writer.calls) != 4 { // meta, both activations, batch; snapshot and lead never reach an owner write.
		t.Fatalf("runtime writer calls=%v", writer.calls)
	}
	for _, check := range []struct{ kind, reason string }{{hxcport.HXCHistorySnapshot, "hxc_history_business_field_redacted"}, {hxcport.HXCHistoryLead, "hxc_history_target_invalid"}} {
		row := archive.rows[hxcTableForKind(check.kind)][0]
		terminal, found, terminalErr := journal.LoadHXCHistoryTerminal(context.Background(), check.kind, SourceIdentifier(row.SourceKeyHMAC))
		if terminalErr != nil || !found || terminal.Disposition != "quarantine" || terminal.Reason != check.reason {
			t.Fatalf("kind=%s terminal=%+v found=%t err=%v", check.kind, terminal, found, terminalErr)
		}
	}
}

func TestHXCHistoryImporterRejectsBadEnvelopeAndResolverFailure(t *testing.T) {
	archive := hxcImporterFixture(t)
	archive.rows[v1hxchistory.ImportBatchesTableID][0].SourceOrdinal = 2
	journal := newHXCImporterJournal()
	if importer, err := NewHXCHistoryImporter(archive, hxcImporterUOW{}, &hxcImporterWriter{journal: journal}, &hxcImporterResolver{}, journal); err != nil {
		t.Fatal(err)
	} else if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("bad envelope err=%v", err)
	}

	archive = hxcImporterFixture(t)
	journal = newHXCImporterJournal()
	resolverErr := errors.New("resolver transient")
	writer := &hxcImporterWriter{journal: journal}
	importer, err := NewHXCHistoryImporter(archive, hxcImporterUOW{}, writer, &hxcImporterResolver{err: resolverErr}, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, resolverErr) {
		t.Fatalf("resolver error became a guessed customer: %v", err)
	}
}

func TestHXCHistoryImporterRejectsNonPositiveResolvedCustomer(t *testing.T) {
	archive := hxcImporterFixture(t)
	journal := newHXCImporterJournal()
	invalid := int64(0)
	importer, err := NewHXCHistoryImporter(archive, hxcImporterUOW{}, &hxcImporterWriter{journal: journal}, &hxcImporterResolver{customer: &invalid}, journal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result.Tables[v1hxchistory.DashboardSnapshotTableID].Quarantined != 1 {
		t.Fatalf("invalid resolver result=%+v err=%v", result, err)
	}
}

func hxcImporterFixture(t *testing.T) hxcImporterArchive {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 9, 8, 7, 654321000, time.UTC)
	snapshot := map[string]any{
		"id": 0, "unionid": "union-a", "refreshed_at": stamp, "in_lead_pool": true, "in_people": false, "in_questionnaire": true,
		"class_term_no": nil, "class_term_label": "term", "crm_hxc_state": "active", "hxc_member_hit": true, "hxc_user_hit": false,
		"funnel_state": "warm", "hxc_member_status": "active", "hxc_registered_at": stamp, "hxc_last_login_at": nil,
		"membership_type": "annual", "membership_status": "active", "membership_end_at": stamp, "membership_days_left": int64(-1),
		"consultation_used": int64(2), "consultation_limit": int64(10), "conv_chat": int64(1), "conv_consult": int64(2), "conv_lesson": int64(3),
		"msg_user": int64(4), "msg_ai": int64(5), "consult_completed": int64(6), "last_msg_at": nil, "subscription_tier": "annual",
		"subscription_expires_at": stamp, "subscription_quota": int64(20), "subscription_used": int64(3), "crm_created_at": nil,
		"last_questionnaire_at": "2026-08-01", "subscription_period_start": "2026-08-01",
	}
	return hxcImporterArchive{rows: map[string][]v1archive.ArchivedRow{
		v1hxchistory.DashboardMetaTableID:     {hxcTestRow(t, v1hxchistory.DashboardMetaTableID, 1, 1, map[string]any{"id": int64(-1), "started_at": stamp, "finished_at": nil, "status": "done", "row_count": int64(-2), "member_hit": int64(1), "user_hit": int64(2), "only_member": int64(3), "trigger_source": "timer"})},
		v1hxchistory.DashboardSnapshotTableID: {hxcTestRow(t, v1hxchistory.DashboardSnapshotTableID, 1, 2, snapshot)},
		v1hxchistory.ActivationStatusTableID:  {hxcTestRow(t, v1hxchistory.ActivationStatusTableID, 1, 3, map[string]any{"id": int64(3), "mobile": "private", "activation_status": "activated", "import_batch_id": int64(8), "is_active": true, "created_at": stamp, "updated_at": stamp})},
		v1hxchistory.HuangxiaocanActivationID: {hxcTestRow(t, v1hxchistory.HuangxiaocanActivationID, 1, 4, map[string]any{"id": int64(4), "mobile": "private", "activation_state": "not_activated", "import_batch_id": "legacy", "is_active": false, "created_at": stamp, "updated_at": stamp})},
		v1hxchistory.ExperienceLeadsTableID:   {hxcTestRow(t, v1hxchistory.ExperienceLeadsTableID, 1, 5, map[string]any{"id": int64(5), "mobile": "private", "source_type": "manual", "import_batch_id": nil, "is_active": true, "created_at": stamp, "updated_at": stamp})},
		v1hxchistory.ImportBatchesTableID:     {hxcTestRow(t, v1hxchistory.ImportBatchesTableID, 1, 6, map[string]any{"id": int64(6), "import_type": "lead", "total_rows": int64(-3), "success_rows": int64(2), "failed_rows": int64(1), "created_at": stamp})},
		v1hxchistory.SendRecordsTableID:       {hxcTestRow(t, v1hxchistory.SendRecordsTableID, 1, 7, map[string]any{})},
		v1hxchistory.SendConfigTableID:        {hxcTestRow(t, v1hxchistory.SendConfigTableID, 1, 8, map[string]any{})},
	}}
}

func hxcTestRow(t *testing.T, table string, ordinal int64, seed byte, payload any) v1archive.ArchivedRow {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: hxcTestDigest(seed), PayloadHMAC: hxcTestDigest(seed + 20), FieldHMAC: hxcTestDigest(seed + 40), Payload: raw}
}

func hxcTestDigest(seed byte) [sha256.Size]byte {
	var result [sha256.Size]byte
	result[0] = seed
	return result
}

func hxcTableForKind(kind string) string {
	for _, scope := range hxcHistoryScopes {
		if scope.kind == kind {
			return scope.table
		}
	}
	return ""
}

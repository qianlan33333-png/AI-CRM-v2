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

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	wecomport "github.com/qianlan33333-png/AI-CRM-v2/internal/wecom/port"
)

type messageHistoryArchiveFake struct {
	rows  []v1archive.ArchivedRow
	calls int
}

func (fake *messageHistoryArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	fake.calls++
	if run != "archive-run" || table != messageHistoryTableID {
		return ErrInvalidScope
	}
	for _, row := range fake.rows {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type messageHistoryTxKey struct{}

type messageHistoryRuntimeFake struct {
	terminals map[string]TerminalReceipt
	values    map[int64]wecomport.HistoricalMessage
	customer  *int64

	writeErr                      error
	resolverErr                   error
	writes, resolverCalls, checks int
	records, commits, rollbacks   int
	retryOnce                     bool
}

func newMessageHistoryRuntimeFake() *messageHistoryRuntimeFake {
	return &messageHistoryRuntimeFake{terminals: map[string]TerminalReceipt{}, values: map[int64]wecomport.HistoricalMessage{}}
}

func (fake *messageHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, values := copyMessageHistoryTerminals(fake.terminals), copyMessageHistoryValues(fake.values)
		err := callback(context.WithValue(ctx, messageHistoryTxKey{}, true))
		if err != nil {
			fake.terminals, fake.values = terminals, values
			fake.rollbacks++
			return err
		}
		if fake.retryOnce && attempt == 0 {
			fake.terminals, fake.values = terminals, values
			continue
		}
		fake.commits++
		return nil
	}
}

func (fake *messageHistoryRuntimeFake) ValidateMessageHistoryImportScope(run string) error {
	if run != "archive-run" {
		return ErrInvalidScope
	}
	return nil
}

func (fake *messageHistoryRuntimeFake) LoadTerminal(ctx context.Context, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(messageHistoryTxKey{}) != true {
		return TerminalReceipt{}, false, errors.New("missing transaction")
	}
	value, found := fake.terminals[source]
	return value, found, nil
}

func (fake *messageHistoryRuntimeFake) Record(ctx context.Context, receipt TerminalReceipt) error {
	if ctx.Value(messageHistoryTxKey{}) != true {
		return errors.New("missing transaction")
	}
	fake.records++
	key := SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := fake.terminals[key]; found && !reflect.DeepEqual(old, receipt) {
		return ErrConflict
	}
	fake.terminals[key] = receipt
	return nil
}

func (fake *messageHistoryRuntimeFake) LoadMessageHistory(ctx context.Context, source string) (wecomport.MessageHistoryReceipt, bool, error) {
	terminal, found, err := fake.LoadTerminal(ctx, source)
	if err != nil || !found {
		return wecomport.MessageHistoryReceipt{}, found, err
	}
	receipt, err := messageHistoryReceiptFromTerminal(source, terminal)
	return receipt, err == nil, err
}

func (fake *messageHistoryRuntimeFake) RecordMessageHistory(ctx context.Context, receipt wecomport.MessageHistoryReceipt) error {
	terminal, err := messageHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return fake.Record(ctx, terminal)
}

func (fake *messageHistoryRuntimeFake) ResolveHistoricalMessageCustomer(ctx context.Context, _ string) (*int64, error) {
	if ctx.Value(messageHistoryTxKey{}) != true {
		return nil, errors.New("resolver outside transaction")
	}
	fake.resolverCalls++
	if fake.resolverErr != nil {
		return nil, fake.resolverErr
	}
	if fake.customer == nil {
		return nil, nil
	}
	copy := *fake.customer
	return &copy, nil
}

func (fake *messageHistoryRuntimeFake) Write(ctx context.Context, source string, payload [sha256.Size]byte, value wecomport.HistoricalMessage) (wecomport.MessageHistoryReceipt, error) {
	if ctx.Value(messageHistoryTxKey{}) != true {
		return wecomport.MessageHistoryReceipt{}, errors.New("writer outside transaction")
	}
	fake.writes++
	if fake.writeErr != nil {
		return wecomport.MessageHistoryReceipt{}, fake.writeErr
	}
	if terminal, found, err := fake.LoadTerminal(ctx, source); err != nil {
		return wecomport.MessageHistoryReceipt{}, err
	} else if found {
		receipt, err := messageHistoryReceiptFromTerminal(source, terminal)
		if err != nil || receipt.PayloadDigest != payload {
			return wecomport.MessageHistoryReceipt{}, wecomport.ErrMessageHistoryConflict
		}
		actual, found := fake.values[receipt.TargetID]
		expected := value
		expected.ID = receipt.TargetID
		if !found || !reflect.DeepEqual(actual, expected) {
			return wecomport.MessageHistoryReceipt{}, wecomport.ErrMessageHistoryConflict
		}
		fake.checks++
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(100 + len(fake.values))
	fake.values[value.ID] = value
	receipt := wecomport.MessageHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: value.ID,
		TargetDigest: sha256.Sum256([]byte("message-history-target/" + source))}
	if err := fake.RecordMessageHistory(ctx, receipt); err != nil {
		return wecomport.MessageHistoryReceipt{}, err
	}
	return receipt, nil
}

func copyMessageHistoryTerminals(source map[string]TerminalReceipt) map[string]TerminalReceipt {
	copy := make(map[string]TerminalReceipt, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func copyMessageHistoryValues(source map[int64]wecomport.HistoricalMessage) map[int64]wecomport.HistoricalMessage {
	copy := make(map[int64]wecomport.HistoricalMessage, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func messageHistoryRow(t *testing.T, sourceID, ordinal int64, mutate func(map[string]any)) v1archive.ArchivedRow {
	t.Helper()
	value := map[string]any{
		"id": sourceID, "seq": int64(-2), "msgid": "source-message", "chat_type": "private", "owner_userid": "owner",
		"sender": "sender", "receiver": "receiver", "msgtype": "text", "content": " message 13800138000\n",
		"send_time": "2026-08-27 13:36:01", "raw_payload": `{"secret":"never-target"}`,
		"created_at": "2026-08-27T13:36:02.123456Z", "unionid": "verified-union",
	}
	if mutate != nil {
		mutate(value)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: messageHistoryTableID, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte("message-key/" + strconv.FormatInt(ordinal, 10))), PayloadHMAC: sha256.Sum256(payload),
		FieldHMAC: sha256.Sum256([]byte("message-fields/" + strconv.FormatInt(ordinal, 10))), Payload: payload}
}

func messageHistoryImporterFixture(t *testing.T, rows ...v1archive.ArchivedRow) (*MessageHistoryImporter, *messageHistoryRuntimeFake) {
	t.Helper()
	runtime := newMessageHistoryRuntimeFake()
	importer, err := NewMessageHistoryImporter(&messageHistoryArchiveFake{rows: rows}, runtime, runtime, runtime, runtime)
	if err != nil {
		t.Fatal("create_importer_failed")
	}
	return importer, runtime
}

func TestMessageHistoryImporterStreamsMaskedCivilHistory(t *testing.T) {
	row := messageHistoryRow(t, 9, 1, func(value map[string]any) { value["content"] = " \n13800138000 +8613912345678\n " })
	importer, runtime := messageHistoryImporterFixture(t, row)
	customerID := int64(41)
	runtime.customer = &customerID
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Imported: 1}) || runtime.writes != 1 || runtime.resolverCalls != 1 || runtime.commits != 1 {
		t.Fatal("historical_message_not_imported")
	}
	value, found := runtime.values[100]
	if !found || value.CustomerID == nil || *value.CustomerID != customerID || value.SentAt != nil || value.SendTimeBasis != "civil_unzoned" ||
		value.ContentMasked == nil || *value.ContentMasked != " \n[masked-phone] [masked-phone]\n " || value.CreatedAt != time.Date(2026, 8, 27, 13, 36, 2, 123456000, time.UTC) ||
		value.SourcePayloadDigest != row.PayloadHMAC {
		t.Fatal("history_value_not_preserved")
	}
	encoded, err := json.Marshal(value)
	if err != nil || strings.Contains(string(encoded), "never-target") || strings.Contains(string(encoded), "verified-union") || strings.Contains(string(encoded), "sender") {
		t.Fatal("source_material_reached_target")
	}
}

func TestMessageHistoryImporterLeavesEmptyUnionIDUnresolved(t *testing.T) {
	row := messageHistoryRow(t, 9, 1, func(value map[string]any) { value["unionid"] = "" })
	importer, runtime := messageHistoryImporterFixture(t, row)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Imported: 1}) || runtime.resolverCalls != 0 || runtime.values[100].CustomerID != nil {
		t.Fatal("empty_unionid_was_resolved_or_rejected")
	}
}

func TestMessageHistoryImporterQuarantinesOnlyUnsafeRows(t *testing.T) {
	for name, test := range map[string]struct {
		mutate   func(*v1archive.ArchivedRow)
		expected string
	}{
		"required-redaction": {func(row *v1archive.ArchivedRow) { row.RedactedFields = []string{"unionid"} }, "redacted_message_history_field"},
		"pending-time": {func(row *v1archive.ArchivedRow) {
			var value map[string]any
			_ = json.Unmarshal(row.Payload, &value)
			value["created_at"] = "2026-08-27 13:36:02"
			row.Payload, _ = json.Marshal(value)
			row.PayloadHMAC = sha256.Sum256(row.Payload)
		}, "message_created_time_unmapped"},
		"nul-content": {func(row *v1archive.ArchivedRow) {
			var value map[string]any
			_ = json.Unmarshal(row.Payload, &value)
			value["content"] = "not-storable\x00body"
			row.Payload, _ = json.Marshal(value)
			row.PayloadHMAC = sha256.Sum256(row.Payload)
		}, "message_content_nul"},
	} {
		t.Run(name, func(t *testing.T) {
			row := messageHistoryRow(t, 9, 1, nil)
			test.mutate(&row)
			importer, runtime := messageHistoryImporterFixture(t, row)
			result, err := importer.Import(context.Background(), "archive-run")
			if err != nil || result != (MessageHistoryImportResult{Quarantined: 1}) || runtime.writes != 0 || runtime.resolverCalls != 0 {
				t.Fatal("unsafe_message_not_quarantined")
			}
			terminal := runtime.terminals[SourceIdentifier(row.SourceKeyHMAC)]
			if terminal.Disposition != "quarantine" || terminal.Reason != test.expected || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) {
				t.Fatal("quarantine_receipt_not_exact")
			}
		})
	}
}

func TestMessageHistoryImporterAllowsOptionalRedactionAsUnavailable(t *testing.T) {
	row := messageHistoryRow(t, 9, 1, nil)
	row.RedactedFields = []string{"content", "owner_userid"}
	importer, runtime := messageHistoryImporterFixture(t, row)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Imported: 1}) {
		t.Fatal("optional_redaction_rejected")
	}
	if value := runtime.values[100]; value.ContentMasked != nil {
		t.Fatal("redacted_body_not_dropped")
	}
}

func TestMessageHistoryImporterRejectsArchiveIntegrityAndDuplicateSource(t *testing.T) {
	row := messageHistoryRow(t, 9, 1, nil)
	duplicate := messageHistoryRow(t, 9, 2, nil)
	importer, runtime := messageHistoryImporterFixture(t, row, duplicate)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.writes != 1 {
		t.Fatal("duplicate_source_not_rejected")
	}
	badOrdinal := messageHistoryRow(t, 10, 2, nil)
	importer, runtime = messageHistoryImporterFixture(t, badOrdinal)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.writes != 0 {
		t.Fatal("bad_ordinal_not_rejected")
	}
	missingDigest := messageHistoryRow(t, 10, 1, nil)
	missingDigest.FieldHMAC = [sha256.Size]byte{}
	importer, runtime = messageHistoryImporterFixture(t, missingDigest)
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || runtime.writes != 0 {
		t.Fatal("unsigned_row_not_rejected")
	}
}

func TestMessageHistoryImporterReplaysViaWriterInsideSameTransaction(t *testing.T) {
	importer, runtime := messageHistoryImporterFixture(t, messageHistoryRow(t, 9, 1, nil))
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (MessageHistoryImportResult{Imported: 1}) {
		t.Fatal("first_import_failed")
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Imported: 1, Replayed: 1}) || runtime.checks != 1 || runtime.writes != 2 || runtime.records != 1 {
		t.Fatal("writer_replay_not_verified")
	}
	if _, err = importer.Import(context.Background(), "other-run"); !errors.Is(err, ErrInvalidScope) {
		t.Fatal("wrong_run_accepted")
	}
}

func TestMessageHistoryImporterReportsOnlyCommittedRetryOutcome(t *testing.T) {
	importer, runtime := messageHistoryImporterFixture(t, messageHistoryRow(t, 9, 1, nil))
	runtime.retryOnce = true
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Imported: 1}) || len(runtime.values) != 1 || runtime.commits != 1 || runtime.writes != 2 {
		t.Fatal("retry_outcome_not_reset")
	}
}

func TestMessageHistoryImporterQuarantinesWriterInputWithoutEffects(t *testing.T) {
	importer, runtime := messageHistoryImporterFixture(t, messageHistoryRow(t, 9, 1, nil))
	runtime.writeErr = wecomport.ErrMessageHistoryInvalid
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (MessageHistoryImportResult{Quarantined: 1}) || len(runtime.values) != 0 || runtime.records != 1 {
		t.Fatal("invalid_target_not_quarantined")
	}
	if runtime.commits != 1 || runtime.rollbacks != 0 {
		t.Fatal("unexpected_external_or_transaction_effect")
	}
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1wecomcontacthistory"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type weComContactHistoryTxKey struct{}

type weComContactHistoryArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (fake *weComContactHistoryArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
	if run != "archive-run" || callback == nil {
		return v1domain.ErrInvalidScope
	}
	for _, row := range fake.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type weComContactHistoryRuntimeFake struct {
	terminals map[string]v1domain.TerminalReceipt
	events    map[int64]contactport.HistoricalWeComExternalContactEventLog
	follows   map[int64]contactport.HistoricalWeComExternalContactFollowUser

	invalidEvent, invalidFollow bool
	writes, checks, commits     int
	retryOnce                   bool
}

func newWeComContactHistoryRuntimeFake() *weComContactHistoryRuntimeFake {
	return &weComContactHistoryRuntimeFake{terminals: map[string]v1domain.TerminalReceipt{}, events: map[int64]contactport.HistoricalWeComExternalContactEventLog{}, follows: map[int64]contactport.HistoricalWeComExternalContactFollowUser{}}
}

func (fake *weComContactHistoryRuntimeFake) Within(ctx context.Context, callback func(context.Context) error) error {
	for attempt := 0; ; attempt++ {
		terminals, events, follows := copyWeComContactTerminals(fake.terminals), copyWeComContactEvents(fake.events), copyWeComContactFollows(fake.follows)
		if err := callback(context.WithValue(ctx, weComContactHistoryTxKey{}, true)); err != nil {
			fake.terminals, fake.events, fake.follows = terminals, events, follows
			return err
		}
		if fake.retryOnce && attempt == 0 {
			fake.terminals, fake.events, fake.follows = terminals, events, follows
			continue
		}
		fake.commits++
		return nil
	}
}

func (fake *weComContactHistoryRuntimeFake) LoadWeComContactHistory(ctx context.Context, kind, source string) (contactport.WeComContactHistoryReceipt, bool, error) {
	terminal, found, err := fake.loadTerminal(ctx, kind, source)
	if err != nil || !found {
		return contactport.WeComContactHistoryReceipt{}, found, err
	}
	receipt, err := weComContactHistoryReceipt(kind, source, terminal)
	return receipt, err == nil, err
}

func (fake *weComContactHistoryRuntimeFake) RecordWeComContactHistory(ctx context.Context, receipt contactport.WeComContactHistoryReceipt) error {
	terminal, err := weComContactHistoryTerminal(receipt)
	if err != nil {
		return err
	}
	return fake.recordTerminal(ctx, receipt.Kind, terminal)
}

func (fake *weComContactHistoryRuntimeFake) loadTerminal(ctx context.Context, kind, source string) (v1domain.TerminalReceipt, bool, error) {
	if ctx.Value(weComContactHistoryTxKey{}) != true || (kind != weComContactEventLogKind && kind != weComContactFollowUserKind) {
		return v1domain.TerminalReceipt{}, false, v1domain.ErrInvalidScope
	}
	value, found := fake.terminals[kind+"\x00"+source]
	return value, found, nil
}

func (fake *weComContactHistoryRuntimeFake) recordTerminal(ctx context.Context, kind string, receipt v1domain.TerminalReceipt) error {
	if ctx.Value(weComContactHistoryTxKey{}) != true || (kind != weComContactEventLogKind && kind != weComContactFollowUserKind) {
		return v1domain.ErrInvalidScope
	}
	key := kind + "\x00" + v1domain.SourceIdentifier(receipt.SourceKeyDigest)
	if old, found := fake.terminals[key]; found && !reflect.DeepEqual(old, receipt) {
		return v1domain.ErrConflict
	}
	fake.terminals[key] = receipt
	return nil
}

func (fake *weComContactHistoryRuntimeFake) ImportHistoricalWeComExternalContactEventLog(ctx context.Context, source string, value contactport.HistoricalWeComExternalContactEventLog) (contactport.WeComContactHistoryReceipt, error) {
	if ctx.Value(weComContactHistoryTxKey{}) != true {
		return contactport.WeComContactHistoryReceipt{}, v1domain.ErrInvalidScope
	}
	fake.writes++
	if fake.invalidEvent {
		return contactport.WeComContactHistoryReceipt{}, contactport.ErrWeComContactHistoryInvalid
	}
	return fake.writeEvent(ctx, source, value)
}

func (fake *weComContactHistoryRuntimeFake) ImportHistoricalWeComExternalContactFollowUser(ctx context.Context, source string, value contactport.HistoricalWeComExternalContactFollowUser) (contactport.WeComContactHistoryReceipt, error) {
	if ctx.Value(weComContactHistoryTxKey{}) != true {
		return contactport.WeComContactHistoryReceipt{}, v1domain.ErrInvalidScope
	}
	fake.writes++
	if fake.invalidFollow {
		return contactport.WeComContactHistoryReceipt{}, contactport.ErrWeComContactHistoryInvalid
	}
	return fake.writeFollow(ctx, source, value)
}

func (fake *weComContactHistoryRuntimeFake) writeEvent(ctx context.Context, source string, value contactport.HistoricalWeComExternalContactEventLog) (contactport.WeComContactHistoryReceipt, error) {
	if terminal, found, err := fake.loadTerminal(ctx, weComContactEventLogKind, source); err != nil {
		return contactport.WeComContactHistoryReceipt{}, err
	} else if found {
		receipt, err := weComContactHistoryReceipt(weComContactEventLogKind, source, terminal)
		if err != nil || receipt.PayloadDigest != value.SourcePayloadDigest || !reflect.DeepEqual(fake.events[receipt.TargetID], eventWithID(value, receipt.TargetID)) {
			return contactport.WeComContactHistoryReceipt{}, contactport.ErrWeComContactHistoryConflict
		}
		fake.checks++
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(100 + len(fake.events))
	fake.events[value.ID] = value
	receipt := contactport.WeComContactHistoryReceipt{Kind: weComContactEventLogKind, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("event/" + source))}
	return receipt, fake.RecordWeComContactHistory(ctx, receipt)
}

func (fake *weComContactHistoryRuntimeFake) writeFollow(ctx context.Context, source string, value contactport.HistoricalWeComExternalContactFollowUser) (contactport.WeComContactHistoryReceipt, error) {
	if terminal, found, err := fake.loadTerminal(ctx, weComContactFollowUserKind, source); err != nil {
		return contactport.WeComContactHistoryReceipt{}, err
	} else if found {
		receipt, err := weComContactHistoryReceipt(weComContactFollowUserKind, source, terminal)
		if err != nil || receipt.PayloadDigest != value.SourcePayloadDigest || !reflect.DeepEqual(fake.follows[receipt.TargetID], followWithID(value, receipt.TargetID)) {
			return contactport.WeComContactHistoryReceipt{}, contactport.ErrWeComContactHistoryConflict
		}
		fake.checks++
		receipt.Replayed = true
		return receipt, nil
	}
	value.ID = int64(200 + len(fake.follows))
	fake.follows[value.ID] = value
	receipt := contactport.WeComContactHistoryReceipt{Kind: weComContactFollowUserKind, SourceIdentifier: source, PayloadDigest: value.SourcePayloadDigest, TargetID: value.ID, TargetDigest: sha256.Sum256([]byte("follow/" + source))}
	return receipt, fake.RecordWeComContactHistory(ctx, receipt)
}

func TestWeComContactHistoryImporterMapsBothFactsInsideCallerTransaction(t *testing.T) {
	event, follow := weComContactHistoryRows(t)
	importer, runtime := weComContactHistoryImporterFixture(t, event, follow)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (WeComContactHistoryImportResult{ImportedEventLogs: 1, ImportedFollowUsers: 1}) || runtime.commits != 2 || runtime.writes != 2 {
		t.Fatal("wecom_contact_history_import_failed")
	}
	eventValue := runtime.events[100]
	if eventValue.SourceID != -7 || eventValue.EventTime == nil || *eventValue.EventTime != -9 || eventValue.CreatedAt != time.Date(2026, 8, 28, 1, 30, 0, 123456000, time.UTC) || eventValue.SourceKeyDigest != event.SourceKeyHMAC || eventValue.SourcePayloadDigest != event.PayloadHMAC || eventValue.SourceFieldDigest != event.FieldHMAC {
		t.Fatal("event_source_fact_changed")
	}
	followValue := runtime.follows[200]
	if followValue.SourceID != 0 || followValue.AddWay != nil || followValue.CreateTime == nil || *followValue.CreateTime != -8 || followValue.State != "source-scene" || followValue.SourceFieldDigest != follow.FieldHMAC {
		t.Fatal("follow_source_fact_changed")
	}
	encoded, err := json.Marshal(followValue)
	if err != nil || string(encoded) == "" || containsAny(string(encoded), "source-scene", "private-corp", "private-user") {
		t.Fatal("private_wecom_history_field_serialized")
	}
}

func TestWeComContactHistoryImporterQuarantinesOnlyInvalidCandidate(t *testing.T) {
	event, follow := weComContactHistoryRows(t)
	event.RedactedFields = []string{"payload_json.token"}
	importer, runtime := weComContactHistoryImporterFixture(t, event, follow)
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (WeComContactHistoryImportResult{ImportedFollowUsers: 1, QuarantinedEventLogs: 1}) || runtime.writes != 1 {
		t.Fatal("unsafe_row_was_not_quarantined")
	}
	terminal := runtime.terminals[weComContactEventLogKind+"\x00"+v1domain.SourceIdentifier(event.SourceKeyHMAC)]
	if terminal.Disposition != "quarantine" || terminal.Reason != "public/wecom_external_contact_event_logs_source_redacted" || terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) {
		t.Fatal("quarantine_receipt_changed")
	}
}

func TestWeComContactHistoryImporterReplaysAndResetsRetryOutcome(t *testing.T) {
	event, follow := weComContactHistoryRows(t)
	importer, runtime := weComContactHistoryImporterFixture(t, event, follow)
	runtime.retryOnce = true
	if result, err := importer.Import(context.Background(), "archive-run"); err != nil || result != (WeComContactHistoryImportResult{ImportedEventLogs: 1, ImportedFollowUsers: 1}) || runtime.writes != 4 {
		t.Fatal("retry_outcome_was_not_reset")
	}
	runtime.retryOnce = false
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (WeComContactHistoryImportResult{ImportedEventLogs: 1, ImportedFollowUsers: 1, Replayed: 2}) || runtime.checks != 2 {
		t.Fatal("replay_did_not_verify_target")
	}
}

func TestWeComContactHistoryJournalReceiptRejectsDrift(t *testing.T) {
	receipt := contactport.WeComContactHistoryReceipt{Kind: weComContactEventLogKind, SourceIdentifier: v1domain.SourceIdentifier(sha256.Sum256([]byte("source"))), PayloadDigest: sha256.Sum256([]byte("payload")), TargetID: 19, TargetDigest: sha256.Sum256([]byte("target"))}
	terminal, err := weComContactHistoryTerminal(receipt)
	if err != nil {
		t.Fatal("receipt_encode_failed")
	}
	if got, err := weComContactHistoryReceipt(receipt.Kind, receipt.SourceIdentifier, terminal); err != nil || got != receipt {
		t.Fatal("receipt_round_trip_failed")
	}
	terminal.TargetID = "019"
	if _, err := weComContactHistoryReceipt(receipt.Kind, receipt.SourceIdentifier, terminal); !errors.Is(err, v1domain.ErrConflict) {
		t.Fatal("drifted_terminal_accepted")
	}
	if journal, err := newWeComContactHistoryJournal("archive-run"); err != nil || journal == nil || weComContactHistoryImportVersion == domainImportVersion {
		t.Fatal("history_scope_not_isolated")
	}
}

func TestWeComContactHistoryReaderWrapperFailsClosedWithoutCallerTransaction(t *testing.T) {
	reader := newWeComContactHistoryReader(nil)
	if reader == nil {
		t.Fatal("reader_wrapper_missing")
	}
	if _, err := reader.GetHistoricalWeComExternalContactEventLog(context.Background(), 1); !errors.Is(err, contactport.ErrWeComContactHistoryUnavailable) {
		t.Fatal("reader_wrapper_used_non_transaction_dependency")
	}
}

func weComContactHistoryImporterFixture(t *testing.T, event, follow v1archive.ArchivedRow) (*WeComContactHistoryImporter, *weComContactHistoryRuntimeFake) {
	t.Helper()
	runtime := newWeComContactHistoryRuntimeFake()
	archive := &weComContactHistoryArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		v1wecomcontacthistory.ExternalContactEventLogsTableID:   {event},
		v1wecomcontacthistory.ExternalContactFollowUsersTableID: {follow},
	}}
	importer, err := newWeComContactHistoryImporterWithExpectedRows(archive, runtime, runtime, runtime, 1, 1)
	if err != nil {
		t.Fatal("create_importer_failed")
	}
	return importer, runtime
}

func weComContactHistoryRows(t *testing.T) (v1archive.ArchivedRow, v1archive.ArchivedRow) {
	t.Helper()
	stamp := "2026-08-28T09:30:00.123456+08:00"
	event := map[string]any{
		"id": int64(-7), "corp_id": "private-corp", "event_type": "change_external_contact", "change_type": "delete", "external_userid": "private-external", "user_id": "private-user", "event_time": int64(-9), "event_key": "private-event-key", "payload_xml": "private-xml", "payload_json": nil, "process_status": "failed", "retry_count": int32(-2), "error_message": "private-error", "created_at": stamp, "updated_at": stamp, "identity_sync_status": "skipped", "identity_sync_error_code": "private-code", "identity_sync_error_message": "private-error", "identity_sync_response_json": nil,
	}
	follow := map[string]any{
		"id": int64(0), "corp_id": "private-corp", "external_userid": "private-external", "user_id": "private-user", "relation_status": "active", "is_primary": true, "remark": "private-remark", "description": "private-description", "add_way": nil, "state": "source-scene", "oper_userid": "private-oper", "createtime": int64(-8), "raw_follow_user": nil, "first_seen_at": stamp, "last_seen_at": stamp, "created_at": stamp, "updated_at": stamp,
	}
	return weComContactHistoryRow(t, v1wecomcontacthistory.ExternalContactEventLogsTableID, 1, event), weComContactHistoryRow(t, v1wecomcontacthistory.ExternalContactFollowUsersTableID, 1, follow)
}

func weComContactHistoryRow(t *testing.T, table string, ordinal int64, value map[string]any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture_encode_failed")
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sha256.Sum256([]byte(table + "/key/" + strconv.FormatInt(ordinal, 10))), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte(table + "/fields/" + strconv.FormatInt(ordinal, 10))), Payload: payload}
}

func eventWithID(value contactport.HistoricalWeComExternalContactEventLog, id int64) contactport.HistoricalWeComExternalContactEventLog {
	value.ID = id
	return value
}

func followWithID(value contactport.HistoricalWeComExternalContactFollowUser, id int64) contactport.HistoricalWeComExternalContactFollowUser {
	value.ID = id
	return value
}

func copyWeComContactTerminals(source map[string]v1domain.TerminalReceipt) map[string]v1domain.TerminalReceipt {
	copy := make(map[string]v1domain.TerminalReceipt, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func copyWeComContactEvents(source map[int64]contactport.HistoricalWeComExternalContactEventLog) map[int64]contactport.HistoricalWeComExternalContactEventLog {
	copy := make(map[int64]contactport.HistoricalWeComExternalContactEventLog, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func copyWeComContactFollows(source map[int64]contactport.HistoricalWeComExternalContactFollowUser) map[int64]contactport.HistoricalWeComExternalContactFollowUser {
	copy := make(map[int64]contactport.HistoricalWeComExternalContactFollowUser, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func containsAny(value string, targets ...string) bool {
	for _, target := range targets {
		if len(target) > 0 && len(value) >= len(target) {
			for index := 0; index+len(target) <= len(value); index++ {
				if value[index:index+len(target)] == target {
					return true
				}
			}
		}
	}
	return false
}

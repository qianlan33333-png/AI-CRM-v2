package v1hxcruntimehistory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestSelectClosesCompleteArchiveAndTerminalSets(t *testing.T) {
	key := testHMACKey
	archive, terminals := runtimeFixture(t, key)
	selected, err := Select(context.Background(), archive, terminals, SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: key})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.Summary() != (Summary{SenderConfigs: 1, SendRecords: 2}) {
		t.Fatalf("summary=%+v", selected.Summary())
	}
	if selected.SenderConfigs[0].Fact.SourceID != -7 || selected.SendRecords[0].Fact.SourceID != 0 || selected.SendRecords[1].Fact.SourceID != 9 {
		t.Fatalf("signed source IDs changed: %#v %#v", selected.SenderConfigs, selected.SendRecords)
	}
	for _, source := range []OpaqueDigest{selected.SenderConfigs[0].SourceKeyDigest, selected.SenderConfigs[0].SourcePayloadDigest, selected.SenderConfigs[0].SourceFieldDigest, selected.SendRecords[0].SourceKeyDigest, selected.SendRecords[0].SourcePayloadDigest, selected.SendRecords[0].SourceFieldDigest} {
		if source == (OpaqueDigest{}) {
			t.Fatal("source envelope digest was lost")
		}
	}
	if len(terminals.scopes) != 2 || terminals.scopes[0] != oldRuntimeScope("archive-run", SenderConfigTableID) || terminals.scopes[1] != oldRuntimeScope("archive-run", SendRecordsTableID) {
		t.Fatalf("terminal scopes=%#v", terminals.scopes)
	}
}

func TestSelectRejectsMissingExtraDuplicateAndDrift(t *testing.T) {
	key := testHMACKey
	for _, mutate := range []struct {
		name  string
		apply func(*runtimeArchive, *runtimeTerminals)
	}{
		{"missing_row", func(archive *runtimeArchive, terminals *runtimeTerminals) {
			archive.rows[SenderConfigTableID] = nil
			terminals.values[scopeKey(oldRuntimeScope("archive-run", SenderConfigTableID))] = nil
		}},
		{"extra_terminal", func(_ *runtimeArchive, terminals *runtimeTerminals) {
			scope := oldRuntimeScope("archive-run", SenderConfigTableID)
			values := terminals.values[scopeKey(scope)]
			values = append(values, TerminalReceipt{Verified: true, SourceKeyDigest: sha256.Sum256([]byte("extra")), PayloadDigest: sha256.Sum256([]byte("extra-payload")), Disposition: "archive", Reason: SenderConfigArchiveReason, Metadata: map[string]any{}})
			terminals.values[scopeKey(scope)] = values
		}},
		{"duplicate_source_id", func(archive *runtimeArchive, _ *runtimeTerminals) {
			duplicate := archive.rows[SendRecordsTableID][0]
			duplicate.SourceOrdinal = 2
			archive.rows[SendRecordsTableID][1] = duplicate
		}},
		{"payload_hmac_drift", func(archive *runtimeArchive, _ *runtimeTerminals) {
			archive.rows[SendRecordsTableID][0].PayloadHMAC = sha256.Sum256([]byte("drift"))
		}},
		{"source_key_hmac_drift", func(archive *runtimeArchive, _ *runtimeTerminals) {
			archive.rows[SendRecordsTableID][0].SourceKeyHMAC = sha256.Sum256([]byte("drift"))
		}},
		{"field_hmac_drift", func(archive *runtimeArchive, _ *runtimeTerminals) {
			archive.rows[SendRecordsTableID][0].FieldHMAC = sha256.Sum256([]byte("drift"))
		}},
		{"terminal_reason_drift", func(_ *runtimeArchive, terminals *runtimeTerminals) {
			scope := oldRuntimeScope("archive-run", SendRecordsTableID)
			terminals.values[scopeKey(scope)][0].Reason = "wrong_reason"
		}},
		{"redaction_drift", func(archive *runtimeArchive, _ *runtimeTerminals) {
			archive.rows[SendRecordsTableID][0].RedactedFields = []string{"unexpected_secret"}
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			archive, terminals := runtimeFixture(t, key)
			mutate.apply(archive, terminals)
			if _, err := Select(context.Background(), archive, terminals, SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: key}); !errors.Is(err, ErrSealedDrift) {
				t.Fatalf("drift accepted or unsafe error: %v", err)
			}
		})
	}
}

func TestSelectRejectsReaderAndOptionErrorsWithoutForwardingThem(t *testing.T) {
	archive, terminals := runtimeFixture(t, testHMACKey)
	archive.err = errors.New("private reader failure")
	if _, err := Select(context.Background(), archive, terminals, SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: testHMACKey}); !errors.Is(err, ErrSealedDrift) || err.Error() != ErrSealedDrift.Error() {
		t.Fatalf("archive error was not safely normalized: %v", err)
	}
	if _, err := Select(context.Background(), archive, terminals, SelectionOptions{ArchiveRunID: "archive-run", SourceHMACKey: testHMACKey[:sha256.Size-1]}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("invalid options accepted: %v", err)
	}
}

type runtimeArchive struct {
	rows map[string][]v1archive.ArchivedRow
	err  error
}

func (archive *runtimeArchive) EachTableRow(_ context.Context, run, table string, visit func(v1archive.ArchivedRow) error) error {
	if archive.err != nil || run != "archive-run" || visit == nil {
		if archive.err != nil {
			return archive.err
		}
		return ErrInvalidSelection
	}
	for _, row := range archive.rows[table] {
		if err := visit(row); err != nil {
			return err
		}
	}
	return nil
}

type runtimeTerminals struct {
	values map[string][]TerminalReceipt
	scopes []TerminalScope
	err    error
}

func (terminals *runtimeTerminals) EachTerminal(_ context.Context, scope TerminalScope, visit func(TerminalReceipt) error) error {
	terminals.scopes = append(terminals.scopes, scope)
	if terminals.err != nil || visit == nil {
		if terminals.err != nil {
			return terminals.err
		}
		return ErrInvalidSelection
	}
	for _, value := range terminals.values[scopeKey(scope)] {
		if err := visit(value); err != nil {
			return err
		}
	}
	return nil
}

func runtimeFixture(t *testing.T, key []byte) (*runtimeArchive, *runtimeTerminals) {
	t.Helper()
	archive := &runtimeArchive{rows: map[string][]v1archive.ArchivedRow{}}
	terminals := &runtimeTerminals{values: map[string][]TerminalReceipt{}}
	stamp := time.Date(2026, 8, 29, 3, 4, 5, 123456789, time.FixedZone("source", 8*60*60))
	config := runtimeRow(t, key, SenderConfigTableID, 1, senderConfigPayload(t, map[string]any{"id": int64(-7), "priority": int64(-8), "created_at": stamp, "updated_at": stamp}))
	recordZero := runtimeRow(t, key, SendRecordsTableID, 1, sendRecordPayload(t, map[string]any{"id": int64(0), "idempotency_key": "private-token", "created_at": stamp}))
	recordNine := runtimeRow(t, key, SendRecordsTableID, 2, sendRecordPayload(t, map[string]any{"id": int64(9), "idempotency_key": nil, "created_at": stamp}))
	archive.rows[SenderConfigTableID] = []v1archive.ArchivedRow{config}
	archive.rows[SendRecordsTableID] = []v1archive.ArchivedRow{recordZero, recordNine}
	for _, value := range []struct {
		table  string
		reason string
		rows   []v1archive.ArchivedRow
	}{
		{SenderConfigTableID, SenderConfigArchiveReason, archive.rows[SenderConfigTableID]},
		{SendRecordsTableID, SendRecordArchiveReason, archive.rows[SendRecordsTableID]},
	} {
		scope := oldRuntimeScope("archive-run", value.table)
		for _, row := range value.rows {
			terminals.values[scopeKey(scope)] = append(terminals.values[scopeKey(scope)], TerminalReceipt{Verified: true, SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: "archive", Reason: value.reason, Metadata: map[string]any{}})
		}
	}
	return archive, terminals
}

func runtimeRow(t *testing.T, key []byte, table string, ordinal int64, payload []byte) v1archive.ArchivedRow {
	t.Helper()
	canonical, roots, err := v1archive.RedactPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(canonical, &source); err != nil {
		t.Fatal(err)
	}
	sourceKey, err := sourceKeyDigest(key, table, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(key, archiveTableName(table), canonical)
	if err != nil {
		t.Fatal(err)
	}
	fieldDigest, err := v1archive.FieldHMAC(key, archiveTableName(table), roots)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: sourceKey, PayloadHMAC: payloadDigest, FieldHMAC: fieldDigest, Payload: canonical, RedactedFields: roots}
}

func oldRuntimeScope(run, table string) TerminalScope {
	return TerminalScope{ImportVersion: OldImportVersion, ArchiveRunID: run, AdapterID: v1archive.DefaultAdapterID, TableID: table, TargetDomain: OldTargetDomain, TargetTable: OldRuntimeArchiveTarget}
}

func scopeKey(scope TerminalScope) string {
	return scope.ImportVersion + "\x00" + scope.ArchiveRunID + "\x00" + scope.AdapterID + "\x00" + scope.TableID + "\x00" + scope.TargetDomain + "\x00" + scope.TargetTable
}

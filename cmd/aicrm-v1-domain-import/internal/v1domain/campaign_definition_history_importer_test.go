package v1domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

var campaignDefinitionHistoryTestKey = bytes.Repeat([]byte{0x73}, sha256.Size)

type campaignDefinitionHistoryContextKey struct{}

func TestCampaignDefinitionHistoryImporterPreservesSelectedHistoryParents(t *testing.T) {
	definition := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryDefinitionTable, 1, 8, campaignDefinitionHistoryDefinitionPayload(t, 8))
	stepHistory := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryStepTable, 1, 11, campaignDefinitionHistoryStepPayload(t, 11, 8))
	stepCurrent := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryStepTable, 2, 12, campaignDefinitionHistoryStepPayload(t, 12, 99))
	stepUnresolved := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryStepTable, 3, 13, campaignDefinitionHistoryStepPayload(t, 13, 100))
	archive := campaignDefinitionHistoryArchive{rows: map[string][]v1archive.ArchivedRow{
		campaignDefinitionHistoryDefinitionTable: {definition},
		campaignDefinitionHistoryStepTable:       {stepHistory, stepCurrent, stepUnresolved},
	}}
	prior := campaignDefinitionHistoryPriorReceipts{rows: map[string][]CampaignDefinitionPriorReceipt{
		campaignDefinitionHistoryDefinitionTable: {campaignDefinitionHistoryPriorReceipt(definition, "archive", "legacy_definition")},
		campaignDefinitionHistoryStepTable: {
			campaignDefinitionHistoryPriorReceipt(stepHistory, "quarantine", "legacy_step"),
			campaignDefinitionHistoryPriorReceipt(stepCurrent, "archive", "legacy_step"),
			campaignDefinitionHistoryPriorReceipt(stepUnresolved, "quarantine", "legacy_step"),
		},
	}}
	selector, err := NewCampaignDefinitionSelector(archive, prior)
	if err != nil {
		t.Fatal(err)
	}
	journal := &campaignDefinitionHistoryJournalFake{run: "run", terminal: map[string]TerminalReceipt{}}
	writer := &campaignDefinitionHistoryWriterFake{journal: journal, nextID: 101}
	resolver := &campaignDefinitionHistoryParentResolverFake{values: map[int64]campaignDefinitionHistoryParentResult{
		99:  {code: "legacy-current-code", found: true},
		100: {found: false},
	}}
	importer, err := NewCampaignDefinitionHistoryImporter(selector, campaignDefinitionHistoryUOW{}, writer, resolver, journal, campaignDefinitionHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}

	result, err := importer.Import(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	if result != (CampaignDefinitionHistoryImportResult{ImportedDefinitions: 1, ImportedSteps: 3}) {
		t.Fatalf("first result = %#v", result)
	}
	if len(writer.definitions) != 1 || len(writer.steps) != 3 {
		t.Fatalf("writer calls definitions=%d steps=%d", len(writer.definitions), len(writer.steps))
	}
	if value := writer.steps[0]; value.SourceParentState != "history_definition" || value.HistoryDefinitionID == nil || *value.HistoryDefinitionID != 101 || value.CurrentCampaignCode != nil {
		t.Fatalf("history parent = %#v", value)
	}
	if value := writer.steps[1]; value.SourceParentState != "current_definition" || value.HistoryDefinitionID != nil || value.CurrentCampaignCode == nil || *value.CurrentCampaignCode != "legacy-current-code" {
		t.Fatalf("current parent = %#v", value)
	}
	if value := writer.steps[2]; value.SourceParentState != "unresolved_definition" || value.HistoryDefinitionID != nil || value.CurrentCampaignCode != nil {
		t.Fatalf("unresolved parent = %#v", value)
	}
	if got := resolver.calls; len(got) != 2 || got[0] != 99 || got[1] != 100 {
		t.Fatalf("resolver calls = %#v", got)
	}

	replay, err := importer.Import(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	if replay != (CampaignDefinitionHistoryImportResult{ReplayedDefinitions: 1, ReplayedSteps: 3}) {
		t.Fatalf("replay result = %#v", replay)
	}
	if len(writer.definitions) != 1 || len(writer.steps) != 3 {
		t.Fatalf("replay created target: definitions=%d steps=%d", len(writer.definitions), len(writer.steps))
	}
}

func TestCampaignDefinitionHistoryImporterPrevalidatesBeforeWrites(t *testing.T) {
	definition := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryDefinitionTable, 1, 8, campaignDefinitionHistoryDefinitionPayload(t, 8))
	step := campaignDefinitionHistoryArchiveRow(t, campaignDefinitionHistoryStepTable, 1, 11, campaignDefinitionHistoryStepPayload(t, 11, 8))
	step.Payload = bytes.Replace(step.Payload, []byte("content"), []byte("changed"), 1)
	selector, err := NewCampaignDefinitionSelector(
		campaignDefinitionHistoryArchive{rows: map[string][]v1archive.ArchivedRow{campaignDefinitionHistoryDefinitionTable: {definition}, campaignDefinitionHistoryStepTable: {step}}},
		campaignDefinitionHistoryPriorReceipts{rows: map[string][]CampaignDefinitionPriorReceipt{
			campaignDefinitionHistoryDefinitionTable: {campaignDefinitionHistoryPriorReceipt(definition, "archive", "legacy")},
			campaignDefinitionHistoryStepTable:       {campaignDefinitionHistoryPriorReceipt(step, "archive", "legacy")},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := &campaignDefinitionHistoryJournalFake{run: "run", terminal: map[string]TerminalReceipt{}}
	writer := &campaignDefinitionHistoryWriterFake{journal: journal, nextID: 1}
	importer, err := NewCampaignDefinitionHistoryImporter(selector, campaignDefinitionHistoryUOW{}, writer, &campaignDefinitionHistoryParentResolverFake{}, journal, campaignDefinitionHistoryTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "run"); err == nil {
		t.Fatal("tampered selected archive accepted")
	}
	if len(writer.definitions) != 0 || len(writer.steps) != 0 {
		t.Fatalf("writer called before full validation: definitions=%d steps=%d", len(writer.definitions), len(writer.steps))
	}
}

type campaignDefinitionHistoryArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive campaignDefinitionHistoryArchive) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type campaignDefinitionHistoryPriorReceipts struct {
	rows map[string][]CampaignDefinitionPriorReceipt
}

func (receipts campaignDefinitionHistoryPriorReceipts) EachCampaignDefinitionPriorReceipt(_ context.Context, _ string, table string, emit func(CampaignDefinitionPriorReceipt) error) error {
	for _, receipt := range receipts.rows[table] {
		if err := emit(receipt); err != nil {
			return err
		}
	}
	return nil
}

type campaignDefinitionHistoryUOW struct{}

func (campaignDefinitionHistoryUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, campaignDefinitionHistoryContextKey{}, "tx"))
}

type campaignDefinitionHistoryJournalFake struct {
	run      string
	terminal map[string]TerminalReceipt
}

func (journal *campaignDefinitionHistoryJournalFake) ValidateCampaignDefinitionHistoryImportScope(run string) error {
	if journal == nil || journal.run != run {
		return ErrInvalidScope
	}
	return nil
}

func (journal *campaignDefinitionHistoryJournalFake) LoadCampaignDefinitionHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	if ctx.Value(campaignDefinitionHistoryContextKey{}) != "tx" || !validCampaignDefinitionHistoryKind(kind) {
		return TerminalReceipt{}, false, ErrConflict
	}
	value, found := journal.terminal[kind+"/"+source]
	return value, found, nil
}

func (journal *campaignDefinitionHistoryJournalFake) RecordCampaignDefinitionHistoryTerminal(ctx context.Context, kind string, value TerminalReceipt) error {
	if ctx.Value(campaignDefinitionHistoryContextKey{}) != "tx" || !validCampaignDefinitionHistoryKind(kind) {
		return ErrConflict
	}
	journal.terminal[kind+"/"+SourceIdentifier(value.SourceKeyDigest)] = value
	return nil
}

func (journal *campaignDefinitionHistoryJournalFake) LoadCampaignDefinitionHistory(ctx context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadCampaignDefinitionHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return campaignport.CampaignHistoryReceipt{}, found, err
	}
	value, err := campaignDefinitionHistoryReceiptFromTerminal(kind, source, terminal)
	return value, err == nil, err
}

func (journal *campaignDefinitionHistoryJournalFake) RecordCampaignDefinitionHistory(ctx context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	terminal, err := campaignDefinitionHistoryTerminalFromReceipt(kind, receipt)
	if err != nil {
		return err
	}
	return journal.RecordCampaignDefinitionHistoryTerminal(ctx, kind, terminal)
}

type campaignDefinitionHistoryWriterFake struct {
	journal     *campaignDefinitionHistoryJournalFake
	nextID      int64
	definitions []campaignport.HistoricalCampaignDefinition
	steps       []campaignport.HistoricalCampaignDefinitionStep
}

func (writer *campaignDefinitionHistoryWriterFake) WriteDefinition(ctx context.Context, source string, value campaignport.HistoricalCampaignDefinition) (campaignport.CampaignHistoryReceipt, error) {
	if ctx.Value(campaignDefinitionHistoryContextKey{}) != "tx" || source != SourceIdentifier(value.SourceKeyDigest) {
		return campaignport.CampaignHistoryReceipt{}, ErrConflict
	}
	return writer.write(campaignDefinitionHistoryDefinitionKind, source, value.SourcePayloadDigest, func() { writer.definitions = append(writer.definitions, value) })
}

func (writer *campaignDefinitionHistoryWriterFake) WriteStep(ctx context.Context, source string, value campaignport.HistoricalCampaignDefinitionStep) (campaignport.CampaignHistoryReceipt, error) {
	if ctx.Value(campaignDefinitionHistoryContextKey{}) != "tx" || source != SourceIdentifier(value.SourceKeyDigest) {
		return campaignport.CampaignHistoryReceipt{}, ErrConflict
	}
	return writer.write(campaignDefinitionHistoryStepKind, source, value.SourcePayloadDigest, func() { writer.steps = append(writer.steps, value) })
}

func (writer *campaignDefinitionHistoryWriterFake) write(kind, source string, payload [sha256.Size]byte, add func()) (campaignport.CampaignHistoryReceipt, error) {
	if terminal, found := writer.journal.terminal[kind+"/"+source]; found {
		receipt, err := campaignDefinitionHistoryReceiptFromTerminal(kind, source, terminal)
		if err != nil {
			return campaignport.CampaignHistoryReceipt{}, err
		}
		receipt.Replayed = true
		return receipt, nil
	}
	targetID := writer.nextID
	writer.nextID++
	digest := sha256.Sum256([]byte(kind + ":" + source))
	key, err := ParseSourceIdentifier(source)
	if err != nil {
		return campaignport.CampaignHistoryReceipt{}, err
	}
	writer.journal.terminal[kind+"/"+source] = TerminalReceipt{SourceKeyDigest: key, PayloadDigest: payload, Disposition: "import", TargetID: strconv.FormatInt(targetID, 10), TargetDigest: digest, Metadata: map[string]any{}}
	add()
	return campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: payload, TargetID: targetID, TargetDigest: digest}, nil
}

type campaignDefinitionHistoryParentResult struct {
	code  string
	found bool
	err   error
}

type campaignDefinitionHistoryParentResolverFake struct {
	values map[int64]campaignDefinitionHistoryParentResult
	calls  []int64
}

func (resolver *campaignDefinitionHistoryParentResolverFake) ResolveVerifiedCurrentCampaignDefinition(ctx context.Context, sourceID int64, sourceKey [sha256.Size]byte) (string, bool, error) {
	if ctx.Value(campaignDefinitionHistoryContextKey{}) != "tx" {
		return "", false, ErrConflict
	}
	expected, err := v1archive.SourceKeyHMAC(campaignDefinitionHistoryTestKey, "campaigns", []byte("["+strconv.FormatInt(sourceID, 10)+"]"))
	if err != nil || expected != sourceKey {
		return "", false, ErrConflict
	}
	resolver.calls = append(resolver.calls, sourceID)
	value := resolver.values[sourceID]
	return value.code, value.found, value.err
}

func campaignDefinitionHistoryArchiveRow(t *testing.T, table string, ordinal, id int64, payload []byte) v1archive.ArchivedRow {
	t.Helper()
	canonical, fields, err := v1archive.RedactPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	source, err := v1archive.SourceKeyHMAC(campaignDefinitionHistoryTestKey, strings.TrimPrefix(table, "public/"), []byte("["+strconv.FormatInt(id, 10)+"]"))
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest, err := v1archive.PayloadHMAC(campaignDefinitionHistoryTestKey, strings.TrimPrefix(table, "public/"), canonical)
	if err != nil {
		t.Fatal(err)
	}
	field, err := v1archive.FieldHMAC(campaignDefinitionHistoryTestKey, strings.TrimPrefix(table, "public/"), fields)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: payloadDigest, FieldHMAC: field, Payload: canonical, RedactedFields: fields}
}

func campaignDefinitionHistoryPriorReceipt(row v1archive.ArchivedRow, disposition, reason string) CampaignDefinitionPriorReceipt {
	target := "cloud_campaigns"
	if row.TableID == campaignDefinitionHistoryStepTable {
		target = "cloud_campaign_steps"
	}
	receipt := CampaignDefinitionPriorReceipt{ImportVersion: campaignDefinitionSelectionImportVersion, ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID, TableID: row.TableID, SourceKey: row.SourceKeyHMAC, PayloadDigest: row.PayloadHMAC, Disposition: disposition, Reason: reason}
	if disposition == "import" {
		receipt.TargetDomain, receipt.TargetTable = "campaign", target
	}
	return receipt
}

func campaignDefinitionHistoryDefinitionPayload(t *testing.T, id int64) []byte {
	t.Helper()
	return campaignDefinitionHistoryJSON(t, map[string]any{
		"id": id, "campaign_code": "legacy", "display_name": "legacy name", "intent": "history", "anchor_mode": "legacy", "anchor_date": "2024-01-01",
		"review_status": "legacy", "run_status": "stopped", "created_by_agent": "agent", "created_by_session": "session", "trace_id": "trace", "owner_userid": "owner",
		"approval_token_hash": "token", "approved_by": "owner", "approved_at": nil, "started_at": nil, "finished_at": nil, "paused_at": nil, "paused_reason": "",
		"metadata_json": map[string]any{}, "stats_json": map[string]any{}, "created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:04:06Z",
	})
}

func campaignDefinitionHistoryStepPayload(t *testing.T, id, campaignID int64) []byte {
	t.Helper()
	return campaignDefinitionHistoryJSON(t, map[string]any{
		"id": id, "campaign_id": campaignID, "campaign_segment_id": int64(2), "step_index": 1, "day_offset": 0, "send_time": "09:00", "timezone": "Asia/Shanghai",
		"content_text": "content", "content_payload_json": map[string]any{}, "stop_on_reply": false, "skip_if_recently_touched_days": 0, "agent_run_id": "agent",
		"created_at": "2024-01-02T03:04:05Z", "updated_at": "2024-01-02T03:04:06Z",
	})
}

func campaignDefinitionHistoryJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

var _ CampaignDefinitionHistoryWriter = (*campaignDefinitionHistoryWriterFake)(nil)
var _ CampaignDefinitionCurrentParentResolver = (*campaignDefinitionHistoryParentResolverFake)(nil)
var _ CampaignDefinitionHistoryImportJournal = (*campaignDefinitionHistoryJournalFake)(nil)

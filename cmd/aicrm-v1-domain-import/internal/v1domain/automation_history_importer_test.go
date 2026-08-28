package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	automationport "github.com/qianlan33333-png/AI-CRM-v2/internal/automation/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type automationImporterContextKey struct{}

type automationImporterArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive automationImporterArchive) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
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

type automationImporterUOW struct{}

func (automationImporterUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(context.WithValue(ctx, automationImporterContextKey{}, "transaction"))
}

type automationImporterJournal struct {
	run       string
	terminals map[string]map[string]TerminalReceipt
}

func newAutomationImporterJournal() *automationImporterJournal {
	return &automationImporterJournal{run: "archive-run", terminals: make(map[string]map[string]TerminalReceipt)}
}

func (journal *automationImporterJournal) ValidateAutomationHistoryImportScope(run string) error {
	if journal == nil || run != journal.run {
		return ErrInvalidScope
	}
	return nil
}

func (journal *automationImporterJournal) LoadAutomationHistoryTerminal(_ context.Context, kind, source string) (TerminalReceipt, bool, error) {
	if journal == nil || !validAutomationHistoryKind(kind) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	value, found := journal.terminals[kind][source]
	return value, found, nil
}

func (journal *automationImporterJournal) RecordAutomationHistoryTerminal(_ context.Context, kind string, receipt TerminalReceipt) error {
	if journal == nil || !validAutomationHistoryKind(kind) {
		return ErrInvalidScope
	}
	source := SourceIdentifier(receipt.SourceKeyDigest)
	if journal.terminals[kind] == nil {
		journal.terminals[kind] = make(map[string]TerminalReceipt)
	}
	if current, found := journal.terminals[kind][source]; found {
		if !reflect.DeepEqual(current, receipt) {
			return ErrConflict
		}
		return nil
	}
	journal.terminals[kind][source] = receipt
	return nil
}

func (journal *automationImporterJournal) LoadAutomationHistory(ctx context.Context, kind, source string) (automationport.AutomationHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadAutomationHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return automationport.AutomationHistoryReceipt{}, found, err
	}
	receipt, err := automationHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *automationImporterJournal) RecordAutomationHistory(ctx context.Context, receipt automationport.AutomationHistoryReceipt) error {
	terminal, err := automationHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordAutomationHistoryTerminal(ctx, receipt.Kind, terminal)
}

type automationImporterWriter struct {
	journal   *automationImporterJournal
	nextID    int64
	invalid   string
	lastKinds []string
}

func (writer *automationImporterWriter) ImportSOP(ctx context.Context, source string, value automationport.HistoricalAutomationSOP) (automationport.AutomationHistoryReceipt, error) {
	return writer.write(ctx, automationport.AutomationHistorySOP, source, value.HistoricalAutomationIdentity)
}

func (writer *automationImporterWriter) ImportConfig(ctx context.Context, source string, value automationport.HistoricalAutomationConfig) (automationport.AutomationHistoryReceipt, error) {
	return writer.write(ctx, automationport.AutomationHistoryConfig, source, value.HistoricalAutomationIdentity)
}

func (writer *automationImporterWriter) ImportPrompt(ctx context.Context, source string, value automationport.HistoricalAutomationPrompt) (automationport.AutomationHistoryReceipt, error) {
	return writer.write(ctx, automationport.AutomationHistoryPrompt, source, value.HistoricalAutomationIdentity)
}

func (writer *automationImporterWriter) ImportAgent(ctx context.Context, source string, value automationport.HistoricalAutomationAgent) (automationport.AutomationHistoryReceipt, error) {
	return writer.write(ctx, automationport.AutomationHistoryAgent, source, value.HistoricalAutomationIdentity)
}

func (writer *automationImporterWriter) write(ctx context.Context, kind, source string, identity automationport.HistoricalAutomationIdentity) (automationport.AutomationHistoryReceipt, error) {
	if ctx.Value(automationImporterContextKey{}) != "transaction" {
		return automationport.AutomationHistoryReceipt{}, errors.New("writer was outside caller transaction")
	}
	if writer.invalid == kind {
		return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryInvalid
	}
	if existing, found, err := writer.journal.LoadAutomationHistory(ctx, kind, source); err != nil {
		return automationport.AutomationHistoryReceipt{}, err
	} else if found {
		if existing.PayloadDigest != identity.SourcePayloadDigest {
			return automationport.AutomationHistoryReceipt{}, automationport.ErrAutomationHistoryConflict
		}
		existing.Replayed = true
		return existing, nil
	}
	writer.nextID++
	digest := sha256.Sum256([]byte(kind + source))
	receipt := automationport.AutomationHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: identity.SourcePayloadDigest, TargetID: writer.nextID, TargetDigest: digest}
	if err := writer.journal.RecordAutomationHistory(ctx, receipt); err != nil {
		return automationport.AutomationHistoryReceipt{}, err
	}
	writer.lastKinds = append(writer.lastKinds, kind)
	return receipt, nil
}

func TestAutomationHistoryImporterImportsAndReplaysFourTypedFacts(t *testing.T) {
	archive := automationHistoryTestArchive(false)
	journal := newAutomationImporterJournal()
	writer := &automationImporterWriter{journal: journal}
	importer, err := NewAutomationHistoryImporter(archive, automationImporterUOW{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	first, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if want := (AutomationHistoryImportResult{ImportedSOPs: 1, ImportedConfigs: 1, ImportedPrompts: 1, ImportedAgents: 1}); first != want {
		t.Fatalf("first result=%+v want=%+v", first, want)
	}
	if got, want := writer.lastKinds, []string{automationport.AutomationHistorySOP, automationport.AutomationHistoryConfig, automationport.AutomationHistoryPrompt, automationport.AutomationHistoryAgent}; !reflect.DeepEqual(got, want) {
		t.Fatalf("writer kinds=%v want=%v", got, want)
	}
	second, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if want := (AutomationHistoryImportResult{ImportedSOPs: 1, ImportedConfigs: 1, ImportedPrompts: 1, ImportedAgents: 1, Replayed: 4}); second != want {
		t.Fatalf("second result=%+v want=%+v", second, want)
	}
}

func TestAutomationHistoryImporterQuarantinesRedactedAndOwnerInvalidRows(t *testing.T) {
	archive := automationHistoryTestArchive(true)
	journal := newAutomationImporterJournal()
	writer := &automationImporterWriter{journal: journal, invalid: automationport.AutomationHistoryAgent}
	importer, err := NewAutomationHistoryImporter(archive, automationImporterUOW{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if want := (AutomationHistoryImportResult{ImportedSOPs: 1, ImportedPrompts: 1, Quarantined: 2}); result != want {
		t.Fatalf("result=%+v want=%+v", result, want)
	}
	config := automationHistoryTestRow(automationHistoryConfigTable, 1, 2, automationHistoryConfigPayload())
	config.RedactedFields = []string{"draft_task_prompt"}
	configTerminal, found, err := journal.LoadAutomationHistoryTerminal(context.Background(), automationport.AutomationHistoryConfig, SourceIdentifier(config.SourceKeyHMAC))
	if err != nil || !found || configTerminal.Disposition != "quarantine" || configTerminal.Reason != "automation_history_business_field_redacted" {
		t.Fatalf("config terminal=%+v found=%t err=%v", configTerminal, found, err)
	}
	agent := automationHistoryTestRow(automationHistoryAgentTable, 1, 4, automationHistoryAgentPayload())
	agentTerminal, found, err := journal.LoadAutomationHistoryTerminal(context.Background(), automationport.AutomationHistoryAgent, SourceIdentifier(agent.SourceKeyHMAC))
	if err != nil || !found || agentTerminal.Disposition != "quarantine" || agentTerminal.Reason != "automation_history_target_invalid" {
		t.Fatalf("agent terminal=%+v found=%t err=%v", agentTerminal, found, err)
	}
}

func TestAutomationHistoryImporterRejectsArchiveScopeAndReceiptDrift(t *testing.T) {
	archive := automationHistoryTestArchive(false)
	archive.rows[automationHistoryPromptTable][0].SourceOrdinal = 2
	journal := newAutomationImporterJournal()
	writer := &automationImporterWriter{journal: journal}
	importer, err := NewAutomationHistoryImporter(archive, automationImporterUOW{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("scope error=%v want ErrConflict", err)
	}

	archive = automationHistoryTestArchive(false)
	journal = newAutomationImporterJournal()
	writer = &automationImporterWriter{journal: journal}
	importer, err = NewAutomationHistoryImporter(archive, automationImporterUOW{}, writer, journal)
	if err != nil {
		t.Fatal(err)
	}
	row := archive.rows[automationHistorySOPTable][0]
	journal.terminals[automationport.AutomationHistorySOP] = map[string]TerminalReceipt{
		SourceIdentifier(row.SourceKeyHMAC): {SourceKeyDigest: row.SourceKeyHMAC, PayloadDigest: digestByte(99), Disposition: "import", TargetID: "1", TargetDigest: digestByte(98)},
	}
	if _, err = importer.Import(context.Background(), "archive-run"); !errors.Is(err, automationport.ErrAutomationHistoryConflict) {
		t.Fatalf("drift error=%v want owner conflict", err)
	}
}

func TestAutomationHistoryActorsDigestIsDomainSeparated(t *testing.T) {
	left := automationHistoryActorsDigest("config", [2]string{"published_by", "alice"})
	right := automationHistoryActorsDigest("agent", [2]string{"published_by", "alice"})
	if left == ([sha256.Size]byte{}) || left == right {
		t.Fatalf("digest must be nonzero and domain separated")
	}
	if again := automationHistoryActorsDigest("config", [2]string{"published_by", "alice"}); again != left {
		t.Fatal("digest must be stable")
	}
}

func automationHistoryTestArchive(redactConfig bool) automationImporterArchive {
	config := automationHistoryTestRow(automationHistoryConfigTable, 1, 2, automationHistoryConfigPayload())
	if redactConfig {
		config.RedactedFields = []string{"draft_task_prompt"}
	}
	return automationImporterArchive{rows: map[string][]v1archive.ArchivedRow{
		automationHistorySOPTable:    {automationHistoryTestRow(automationHistorySOPTable, 1, 1, automationHistorySOPPayload())},
		automationHistoryConfigTable: {config},
		automationHistoryPromptTable: {automationHistoryTestRow(automationHistoryPromptTable, 1, 3, automationHistoryPromptPayload())},
		automationHistoryAgentTable:  {automationHistoryTestRow(automationHistoryAgentTable, 1, 4, automationHistoryAgentPayload())},
	}}
}

func automationHistoryTestRow(table string, ordinal, seed int64, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal, SourceKeyHMAC: digestByte(byte(seed)), PayloadHMAC: digestByte(byte(seed + 10)), FieldHMAC: digestByte(byte(seed + 20)), Payload: payload}
}

func digestByte(value byte) [sha256.Size]byte {
	var digest [sha256.Size]byte
	digest[0] = value
	return digest
}

func automationHistorySOPPayload() []byte {
	return automationHistoryJSON(map[string]any{"id": 1, "pool_key": "pool", "day_index": 1, "content": "您好13800138000", "images_json": []any{}, "enabled": true, "created_at": automationHistoryTestTime(), "updated_at": automationHistoryTestTime()})
}

func automationHistoryConfigPayload() []byte {
	return automationHistoryJSON(map[string]any{"id": 2, "agent_code": "agent", "display_name": "name", "pool_keys_json": []any{}, "enabled": true, "draft_role_prompt": "role", "draft_task_prompt": "task", "draft_variables_json": map[string]any{}, "draft_output_schema_json": map[string]any{}, "published_role_prompt": "prole", "published_task_prompt": "ptask", "published_variables_json": map[string]any{}, "published_output_schema_json": map[string]any{}, "draft_version": 1, "published_version": 1, "published_at": "", "published_by": "actor", "last_modified_at": "", "last_modified_by": "actor", "last_modified_source": "v1", "last_change_summary": "summary", "created_at": automationHistoryTestTime(), "updated_at": automationHistoryTestTime(), "submitted_for_publish": false, "submitted_at": "", "submitted_by": "", "scenario_code": "scenario"})
}

func automationHistoryPromptPayload() []byte {
	return automationHistoryJSON(map[string]any{"id": 3, "agent_code": "agent", "display_name": "prompt", "prompt_text": "sensitive", "enabled": false, "version": 2, "created_at": automationHistoryTestTime(), "updated_at": automationHistoryTestTime()})
}

func automationHistoryAgentPayload() []byte {
	return automationHistoryJSON(map[string]any{"id": 4, "program_id": 1, "workflow_id": 2, "node_id": 3, "task_id": 4, "agent_code": "agent", "agent_name": "agent name", "agent_type": "type", "status": "disabled", "sort_order": 0, "metadata_json": map[string]any{}, "config_json": map[string]any{}, "enabled": false, "created_by": "created", "updated_by": "updated", "created_at": automationHistoryTestTime(), "updated_at": automationHistoryTestTime(), "archived_at": ""})
}

func automationHistoryJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func automationHistoryTestTime() time.Time {
	return time.Date(2026, 8, 28, 9, 0, 0, 123456000, time.UTC)
}

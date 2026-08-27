package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

type channelImportTxKey struct{}

type channelImportFake struct {
	commits, rollbacks, writerCalls int
}

func (fake *channelImportFake) Within(ctx context.Context, callback func(context.Context) error) error {
	if err := callback(context.WithValue(ctx, channelImportTxKey{}, fake)); err != nil {
		fake.rollbacks++
		return err
	}
	fake.commits++
	return nil
}

type channelArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *channelArchiveFake) EachTableRow(_ context.Context, run, table string, callback func(v1archive.ArchivedRow) error) error {
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

type channelWriterFake struct {
	fake           *channelImportFake
	journal        *Journal
	lastDefinition contactport.HistoricalChannelDefinition
}

func (writer *channelWriterFake) Import(ctx context.Context, definition contactport.HistoricalChannelDefinition) (contactport.HistoricalChannelReceipt, error) {
	if ctx.Value(channelImportTxKey{}) != writer.fake {
		return contactport.HistoricalChannelReceipt{}, errors.New("missing transaction")
	}
	writer.fake.writerCalls++
	writer.lastDefinition = definition
	targetDigest := sha256.Sum256([]byte("contact.channels\x00" + definition.Code))
	receipt := contactport.HistoricalChannelReceipt{SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest,
		TargetID: int64(900 + writer.fake.writerCalls), TargetDigest: targetDigest}
	if err := writer.journal.Record(ctx, TerminalReceipt{SourceKeyDigest: mustChannelSourceDigest(definition.SourceIdentifier), PayloadDigest: definition.PayloadDigest,
		Disposition: "import", TargetID: fmt.Sprintf("%d", receipt.TargetID), TargetDigest: targetDigest}); err != nil {
		return contactport.HistoricalChannelReceipt{}, err
	}
	return receipt, nil
}

func mustChannelSourceDigest(value string) [sha256.Size]byte {
	digest, err := ParseSourceIdentifier(value)
	if err != nil {
		panic(err)
	}
	return digest
}

func channelImporterFixture(t *testing.T, rows map[string][]v1archive.ArchivedRow) (*ChannelImporter, *channelImportFake, map[string]*journalTestTx, *channelWriterFake) {
	t.Helper()
	fake := &channelImportFake{}
	txs, journals := make(map[string]*journalTestTx, len(channelTableIDs)), make(map[string]*Journal, len(channelTableIDs))
	for _, tableID := range channelTableIDs {
		tx := &journalTestTx{}
		txs[tableID] = tx
		journals[tableID] = &Journal{scope: Scope{ImportVersion: "v1-channel-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
			TableID: tableID, TargetDomain: "contact", TargetTable: "channels"}, tx: func(ctx context.Context) (pgx.Tx, error) {
			if ctx.Value(channelImportTxKey{}) != fake {
				return nil, errors.New("missing transaction")
			}
			return tx, nil
		}}
	}
	writer := &channelWriterFake{fake: fake, journal: journals[channelDefinitionTableID]}
	importer, err := NewChannelImporter(&channelArchiveFake{rows: rows}, fake, writer, journals, 17)
	if err != nil {
		t.Fatal(err)
	}
	return importer, fake, txs, writer
}

func channelArchiveRow(t *testing.T, table string, id, ordinal int64) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	payloadValue := map[string]any{"id": id, "channel_code": "v1-course", "channel_name": "课程渠道", "channel_type": "qrcode", "carrier_type": "qrcode",
		"created_at": stamp, "updated_at": stamp.Add(time.Hour), "scene_value": "old", "qr_url": "old", "welcome_message": "old"}
	if table != channelDefinitionTableID {
		payloadValue = map[string]any{"id": id, "legacy_provider_id": "old", "created_at": stamp}
	}
	payload, err := json.Marshal(payloadValue)
	if err != nil {
		t.Fatal(err)
	}
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte(fmt.Sprintf("%s/%d", table, id))), PayloadHMAC: sha256.Sum256(payload),
		FieldHMAC: sha256.Sum256([]byte("field-hmac/" + table)), Payload: payload}
}

func missingChannelTerminal() journalTestRow { return func(...any) error { return pgx.ErrNoRows } }

func foundChannelTerminal(row v1archive.ArchivedRow, disposition, reason, targetID string, targetDigest [sha256.Size]byte) journalTestRow {
	return func(values ...any) error {
		*values[0].(*[]byte), *values[1].(*string), *values[2].(*string) = row.PayloadHMAC[:], disposition, reason
		if len(values) == 9 {
			if disposition == "import" {
				domain, table, id := "contact", "channels", targetID
				*values[3].(**string), *values[4].(**string), *values[5].(**string) = &domain, &table, &id
				*values[6].(*[]byte) = targetDigest[:]
			}
			*values[7].(*[]byte), *values[8].(*bool) = []byte(`{}`), true
			return nil
		}
		if disposition == "import" {
			id := targetID
			*values[3].(**string), *values[4].(*[]byte) = &id, targetDigest[:]
		}
		*values[5].(*bool) = true
		return nil
	}
}

func prepareFirstTerminal(tx *journalTestTx, row v1archive.ArchivedRow, disposition, reason, targetID string, targetDigest [sha256.Size]byte) {
	tx.rows = append(tx.rows, missingChannelTerminal(), missingChannelTerminal(), foundChannelTerminal(row, disposition, reason, targetID, targetDigest))
}

func TestChannelImporterImportsOnlyDefinitionAndTerminatesAllNineTables(t *testing.T) {
	rows := make(map[string][]v1archive.ArchivedRow, len(channelTableIDs))
	for index, tableID := range channelTableIDs {
		rows[tableID] = []v1archive.ArchivedRow{channelArchiveRow(t, tableID, int64(index+1), 1)}
	}
	importer, fake, txs, writer := channelImporterFixture(t, rows)
	definition := rows[channelDefinitionTableID][0]
	targetDigest := sha256.Sum256([]byte("contact.channels\x00v1-course"))
	prepareFirstTerminal(txs[channelDefinitionTableID], definition, "import", "", "901", targetDigest)
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(definition, "import", "", "901", targetDigest))
	for _, tableID := range channelTableIDs[1:] {
		row := rows[tableID][0]
		decision := "archive"
		if tableID == "public/automation_channel_assignee" || tableID == "public/automation_channel_contact" {
			decision = "quarantine"
		}
		prepareFirstTerminal(txs[tableID], row, decision, channelTerminalReason(tableID), "", [sha256.Size]byte{})
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Imported: 1, Archived: 6, Quarantined: 2}) || fake.writerCalls != 1 || fake.commits != 9 {
		t.Fatalf("result=%+v err=%v fake=%+v", result, err, fake)
	}
	if writer.lastDefinition.SourceIdentifier != SourceIdentifier(definition.SourceKeyHMAC) || writer.lastDefinition.PayloadDigest != definition.PayloadHMAC ||
		writer.lastDefinition.LegacyConfigDigest != fmt.Sprintf("sha256:%x", sha256.Sum256(definition.Payload)) || writer.lastDefinition.Code != "v1-course" || writer.lastDefinition.Actor != 17 {
		t.Fatalf("unsafe definition=%+v", writer.lastDefinition)
	}
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(definition, "import", "", "901", targetDigest))
	for _, tableID := range channelTableIDs[1:] {
		row := rows[tableID][0]
		decision := "archive"
		if tableID == "public/automation_channel_assignee" || tableID == "public/automation_channel_contact" {
			decision = "quarantine"
		}
		txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, decision, channelTerminalReason(tableID), "", [sha256.Size]byte{}))
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Imported: 1, Archived: 6, Quarantined: 2, Replayed: 9}) || fake.writerCalls != 1 {
		t.Fatalf("replay result=%+v err=%v writerCalls=%d", result, err, fake.writerCalls)
	}
}

func channelTerminalReason(tableID string) string {
	decision := ""
	switch tableID {
	case "public/automation_channel_assignee":
		decision = "staff_mapping_required"
	case "public/automation_channel_contact":
		decision = "historical_entry_projection_required"
	case "public/automation_channel_entry_effect_log":
		decision = "provider_effect_history_archive_only"
	case "public/automation_channel_entry_runtime":
		decision = "callback_runtime_archive_only"
	case "public/automation_channel_qrcode_asset":
		decision = "provider_asset_legacy_unverified"
	case "public/automation_channel_scene_alias":
		decision = "callback_alias_archive_only"
	case "public/channel_welcome_effect_dependency":
		decision = "welcome_dependency_archive_only"
	case "public/channel_welcome_effect_graph":
		decision = "welcome_execution_archive_only"
	}
	return decision
}

func TestChannelImporterQuarantinesInvalidAndRedactedDefinitions(t *testing.T) {
	for name, mutate := range map[string]func(*v1archive.ArchivedRow){
		"invalid-json": func(row *v1archive.ArchivedRow) {
			row.Payload = []byte("{")
			row.PayloadHMAC = sha256.Sum256(row.Payload)
		},
		"redacted": func(row *v1archive.ArchivedRow) { row.RedactedFields = []string{"channel_name"} },
	} {
		t.Run(name, func(t *testing.T) {
			row := channelArchiveRow(t, channelDefinitionTableID, 49, 1)
			mutate(&row)
			importer, fake, txs, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {row}})
			prepareFirstTerminal(txs[channelDefinitionTableID], row, "quarantine", map[string]string{"invalid-json": "invalid_channel_definition", "redacted": "redacted_channel_definition"}[name], "", [sha256.Size]byte{})
			result, err := importer.Import(context.Background(), "archive-run")
			if err != nil || result != (ChannelImportResult{Quarantined: 1}) || fake.writerCalls != 0 {
				t.Fatalf("result=%+v err=%v writer=%d", result, err, fake.writerCalls)
			}
			txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(row, "quarantine", map[string]string{"invalid-json": "invalid_channel_definition", "redacted": "redacted_channel_definition"}[name], "", [sha256.Size]byte{}))
			result, err = importer.Import(context.Background(), "archive-run")
			if err != nil || result != (ChannelImportResult{Quarantined: 1, Replayed: 1}) {
				t.Fatalf("replay=%+v err=%v", result, err)
			}
		})
	}
}

func TestChannelImporterRejectsInvalidSourceAndChangedTerminalPayload(t *testing.T) {
	invalid := channelArchiveRow(t, channelDefinitionTableID, 49, 0)
	importer, fake, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {invalid}})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || fake.commits != 0 {
		t.Fatalf("invalid source err=%v commits=%d", err, fake.commits)
	}

	row := channelArchiveRow(t, "public/automation_channel_assignee", 7, 1)
	importer, _, txs, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{row.TableID: {row}})
	prepareFirstTerminal(txs[row.TableID], row, "quarantine", "staff_mapping_required", "", [sha256.Size]byte{})
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	changed := row
	changed.Payload = []byte(`{"id":7,"changed":true}`)
	changed.PayloadHMAC = sha256.Sum256(changed.Payload)
	importer.archive.(*channelArchiveFake).rows[row.TableID] = []v1archive.ArchivedRow{changed}
	txs[row.TableID].rows = append(txs[row.TableID].rows, foundChannelTerminal(row, "quarantine", "staff_mapping_required", "", [sha256.Size]byte{}))
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed payload err=%v", err)
	}
}

func TestNewChannelImporterRejectsMismatchedNineTableScope(t *testing.T) {
	importer, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{})
	journals := make(map[string]*Journal, len(importer.journals))
	for table, journal := range importer.journals {
		copy := *journal
		journals[table] = &copy
	}
	journals[channelDefinitionTableID].scope.TargetTable = "customers"
	if _, err := NewChannelImporter(importer.archive, importer.uow, importer.writer, journals, 17); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("scope err=%v", err)
	}
}

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

type channelRelationsFake struct {
	contacts, assignees int
	lastContact         contactport.HistoricalChannelContactDefinition
	lastAssignee        contactport.HistoricalChannelAssigneeDefinition
}

func (fake *channelRelationsFake) ImportContact(_ context.Context, definition contactport.HistoricalChannelContactDefinition) (contactport.HistoricalChannelReceipt, error) {
	fake.contacts++
	fake.lastContact = definition
	return contactport.HistoricalChannelReceipt{SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest,
		TargetID: 1001, TargetDigest: sha256.Sum256([]byte("contact/" + definition.SourceIdentifier)), Replayed: fake.contacts > 1}, nil
}

func (fake *channelRelationsFake) ImportAssignee(_ context.Context, definition contactport.HistoricalChannelAssigneeDefinition) (contactport.HistoricalChannelReceipt, error) {
	fake.assignees++
	fake.lastAssignee = definition
	return contactport.HistoricalChannelReceipt{SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest,
		TargetID: 2001, TargetDigest: sha256.Sum256([]byte("assignee/" + definition.SourceIdentifier)), Replayed: fake.assignees > 1}, nil
}

type channelResolverFake struct {
	calls int
	value *int64
	err   error
}

func (fake *channelResolverFake) ResolveHistoricalChannelCustomer(_ context.Context, _ string) (*int64, error) {
	fake.calls++
	return fake.value, fake.err
}

type channelWriterFake struct {
	fake           *channelImportFake
	lastDefinition contactport.HistoricalChannelDefinition
	failReplay     bool
}

func (writer *channelWriterFake) Import(ctx context.Context, definition contactport.HistoricalChannelDefinition) (contactport.HistoricalChannelReceipt, error) {
	if ctx.Value(channelImportTxKey{}) != writer.fake {
		return contactport.HistoricalChannelReceipt{}, errors.New("missing transaction")
	}
	writer.fake.writerCalls++
	if writer.failReplay && writer.fake.writerCalls > 1 {
		return contactport.HistoricalChannelReceipt{}, contactport.ErrHistoricalChannelConflict
	}
	writer.lastDefinition = definition
	targetDigest := sha256.Sum256([]byte("contact.channels\x00" + definition.Code))
	receipt := contactport.HistoricalChannelReceipt{SourceIdentifier: definition.SourceIdentifier, PayloadDigest: definition.PayloadDigest,
		TargetID: 901, TargetDigest: targetDigest}
	receipt.Replayed = writer.fake.writerCalls > 1
	return receipt, nil
}

func mustChannelSourceDigest(value string) [sha256.Size]byte {
	digest, err := ParseSourceIdentifier(value)
	if err != nil {
		panic(err)
	}
	return digest
}

func channelImporterFixture(t *testing.T, rows map[string][]v1archive.ArchivedRow) (*ChannelImporter, *channelImportFake, map[string]*journalTestTx, *channelWriterFake, *channelRelationsFake, *channelResolverFake) {
	t.Helper()
	fake := &channelImportFake{}
	txs, journals := make(map[string]*journalTestTx, len(channelTableIDs)), make(map[string]*Journal, len(channelTableIDs))
	for _, tableID := range channelTableIDs {
		tx := &journalTestTx{}
		txs[tableID] = tx
		target := "channels"
		if tableID == "public/automation_channel_contact" {
			target = "channel_historical_contacts"
		} else if tableID == "public/automation_channel_assignee" {
			target = "channel_historical_assignees"
		}
		journals[tableID] = &Journal{scope: Scope{ImportVersion: "v1-channel-a1", ArchiveRunID: "archive-run", AdapterID: v1archive.DefaultAdapterID,
			TableID: tableID, TargetDomain: "contact", TargetTable: target}, tx: func(ctx context.Context) (pgx.Tx, error) {
			if ctx.Value(channelImportTxKey{}) != fake {
				return nil, errors.New("missing transaction")
			}
			return tx, nil
		}}
	}
	writer, relations, resolver := &channelWriterFake{fake: fake}, &channelRelationsFake{}, &channelResolverFake{}
	importer, err := NewChannelImporter(&channelArchiveFake{rows: rows}, fake, writer, relations, resolver, journals, 17)
	if err != nil {
		t.Fatal(err)
	}
	return importer, fake, txs, writer, relations, resolver
}

func channelArchiveRow(t *testing.T, table string, id, ordinal int64) v1archive.ArchivedRow {
	t.Helper()
	stamp := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	payloadValue := map[string]any{"id": id, "channel_code": "v1-course", "channel_name": "课程渠道", "channel_type": "qrcode", "carrier_type": "qrcode",
		"created_at": stamp, "updated_at": stamp.Add(time.Hour), "scene_value": "old", "qr_url": "old", "welcome_message": "old"}
	if table == "public/automation_channel_contact" {
		payloadValue = map[string]any{"id": id, "channel_id": 1, "unionid": "unionid-1", "owner_staff_id": "legacy-owner", "first_channel_entered_at": stamp,
			"last_channel_entered_at": stamp.Add(time.Hour), "enter_count": 2, "created_at": stamp, "updated_at": stamp.Add(time.Hour)}
	} else if table == "public/automation_channel_assignee" {
		payloadValue = map[string]any{"id": id, "channel_id": 1, "staff_id": "legacy-staff", "display_name_snapshot": "旧员工", "priority": 1,
			"ratio_percent": 50, "max_scans_24h": 10, "status": "active", "created_at": "2026-08-28T08:00:00.123", "updated_at": "2026-08-28T09:00:00"}
	} else if table != channelDefinitionTableID {
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
				domain, table, id := "contact", channelJournalTarget(row.TableID), targetID
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

func channelJournalTarget(tableID string) string {
	if tableID == "public/automation_channel_contact" {
		return "channel_historical_contacts"
	}
	if tableID == "public/automation_channel_assignee" {
		return "channel_historical_assignees"
	}
	return "channels"
}

func prepareFirstTerminal(tx *journalTestTx, row v1archive.ArchivedRow, disposition, reason, targetID string, targetDigest [sha256.Size]byte) {
	tx.rows = append(tx.rows, missingChannelTerminal(), missingChannelTerminal(), foundChannelTerminal(row, disposition, reason, targetID, targetDigest))
}

func TestChannelImporterImportsOnlyDefinitionAndTerminatesAllNineTables(t *testing.T) {
	rows := make(map[string][]v1archive.ArchivedRow, len(channelTableIDs))
	for index, tableID := range channelTableIDs {
		rows[tableID] = []v1archive.ArchivedRow{channelArchiveRow(t, tableID, int64(index+1), 1)}
	}
	importer, fake, txs, writer, relations, resolver := channelImporterFixture(t, rows)
	customerID := int64(77)
	resolver.value = &customerID
	definition := rows[channelDefinitionTableID][0]
	targetDigest := sha256.Sum256([]byte("contact.channels\x00v1-course"))
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(definition, "import", "", "901", targetDigest))
	for _, tableID := range channelTableIDs[1:] {
		row := rows[tableID][0]
		switch tableID {
		case "public/automation_channel_contact":
			digest := sha256.Sum256([]byte("contact/" + SourceIdentifier(row.SourceKeyHMAC)))
			txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, "import", "", "1001", digest))
		case "public/automation_channel_assignee":
			digest := sha256.Sum256([]byte("assignee/" + SourceIdentifier(row.SourceKeyHMAC)))
			txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, "import", "", "2001", digest))
		default:
			prepareFirstTerminal(txs[tableID], row, "archive", channelTerminalReason(tableID), "", [sha256.Size]byte{})
		}
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Imported: 3, Archived: 6}) || fake.writerCalls != 1 || fake.commits != 9 || relations.contacts != 1 || relations.assignees != 1 || resolver.calls != 1 {
		t.Fatalf("result=%+v err=%v fake=%+v", result, err, fake)
	}
	if writer.lastDefinition.SourceIdentifier != SourceIdentifier(definition.SourceKeyHMAC) || writer.lastDefinition.PayloadDigest != definition.PayloadHMAC ||
		writer.lastDefinition.LegacyConfigDigest != fmt.Sprintf("sha256:%x", sha256.Sum256(definition.Payload)) || writer.lastDefinition.Code != "v1-course" || writer.lastDefinition.Actor != 17 {
		t.Fatalf("unsafe definition=%+v", writer.lastDefinition)
	}
	if relations.lastContact.Contact.ChannelID != 901 || relations.lastContact.Contact.CustomerID == nil || *relations.lastContact.Contact.CustomerID != customerID ||
		relations.lastAssignee.Assignee.ChannelID != 901 || relations.lastAssignee.Assignee.SourceCreatedAt != "2026-08-28T08:00:00.123000" {
		t.Fatalf("unsafe relations: contact=%+v assignee=%+v", relations.lastContact, relations.lastAssignee)
	}
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(definition, "import", "", "901", targetDigest))
	for _, tableID := range channelTableIDs[1:] {
		row := rows[tableID][0]
		switch tableID {
		case "public/automation_channel_contact":
			digest := sha256.Sum256([]byte("contact/" + SourceIdentifier(row.SourceKeyHMAC)))
			txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, "import", "", "1001", digest))
		case "public/automation_channel_assignee":
			digest := sha256.Sum256([]byte("assignee/" + SourceIdentifier(row.SourceKeyHMAC)))
			txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, "import", "", "2001", digest))
		default:
			txs[tableID].rows = append(txs[tableID].rows, foundChannelTerminal(row, "archive", channelTerminalReason(tableID), "", [sha256.Size]byte{}))
		}
	}
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Imported: 3, Archived: 6, Replayed: 9}) || fake.writerCalls != 2 || relations.contacts != 2 || relations.assignees != 2 {
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
			importer, fake, txs, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {row}})
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
	importer, fake, _, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {invalid}})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) || fake.commits != 0 {
		t.Fatalf("invalid source err=%v commits=%d", err, fake.commits)
	}

	row := channelArchiveRow(t, "public/automation_channel_assignee", 7, 1)
	importer, _, txs, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{row.TableID: {row}})
	prepareFirstTerminal(txs[row.TableID], row, "quarantine", "missing_channel_definition", "", [sha256.Size]byte{})
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil {
		t.Fatal(err)
	}
	changed := row
	changed.Payload = []byte(`{"id":7,"changed":true}`)
	changed.PayloadHMAC = sha256.Sum256(changed.Payload)
	importer.archive.(*channelArchiveFake).rows[row.TableID] = []v1archive.ArchivedRow{changed}
	txs[row.TableID].rows = append(txs[row.TableID].rows, foundChannelTerminal(row, "quarantine", "missing_channel_definition", "", [sha256.Size]byte{}))
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed payload err=%v", err)
	}
}

func TestNewChannelImporterRejectsMismatchedNineTableScope(t *testing.T) {
	importer, _, _, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{})
	journals := make(map[string]*Journal, len(importer.journals))
	for table, journal := range importer.journals {
		copy := *journal
		journals[table] = &copy
	}
	journals[channelDefinitionTableID].scope.TargetTable = "customers"
	if _, err := NewChannelImporter(importer.archive, importer.uow, importer.writer, importer.relations, importer.resolver, journals, 17); !errors.Is(err, ErrInvalidScope) {
		t.Fatalf("scope err=%v", err)
	}
}

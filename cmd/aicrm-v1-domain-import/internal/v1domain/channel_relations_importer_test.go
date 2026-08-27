package v1domain

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	contactport "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestChannelImporterPropagatesDefinitionReplayTargetDrift(t *testing.T) {
	row := channelArchiveRow(t, channelDefinitionTableID, 1, 1)
	importer, fake, txs, writer, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {row}})
	targetDigest := sha256.Sum256([]byte("contact.channels\x00v1-course"))
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(row, "import", "", "901", targetDigest))
	if _, err := importer.Import(context.Background(), "archive-run"); err != nil || fake.writerCalls != 1 {
		t.Fatalf("first import err=%v calls=%d", err, fake.writerCalls)
	}
	writer.failReplay = true
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, contactport.ErrHistoricalChannelConflict) || fake.writerCalls != 2 {
		t.Fatalf("target drift err=%v calls=%d", err, fake.writerCalls)
	}
}

func TestChannelImporterAllowsMissingCustomerLineageAndQuarantinesMissingParent(t *testing.T) {
	definition := channelArchiveRow(t, channelDefinitionTableID, 1, 1)
	contact := channelArchiveRow(t, "public/automation_channel_contact", 2, 1)
	rows := map[string][]v1archive.ArchivedRow{channelDefinitionTableID: {definition}, contact.TableID: {contact}}
	importer, _, txs, _, relations, resolver := channelImporterFixture(t, rows)
	targetDigest := sha256.Sum256([]byte("contact.channels\x00v1-course"))
	txs[channelDefinitionTableID].rows = append(txs[channelDefinitionTableID].rows, foundChannelTerminal(definition, "import", "", "901", targetDigest))
	contactDigest := sha256.Sum256([]byte("contact/" + SourceIdentifier(contact.SourceKeyHMAC)))
	txs[contact.TableID].rows = append(txs[contact.TableID].rows, foundChannelTerminal(contact, "import", "", "1001", contactDigest))
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Imported: 2}) || resolver.calls != 1 || relations.lastContact.Contact.CustomerID != nil {
		t.Fatalf("missing lineage result=%+v err=%v resolver=%d contact=%+v", result, err, resolver.calls, relations.lastContact)
	}

	missingParent := channelArchiveRow(t, "public/automation_channel_contact", 3, 1)
	importer, _, txs, _, _, _ = channelImporterFixture(t, map[string][]v1archive.ArchivedRow{missingParent.TableID: {missingParent}})
	prepareFirstTerminal(txs[missingParent.TableID], missingParent, "quarantine", "missing_channel_definition", "", [sha256.Size]byte{})
	result, err = importer.Import(context.Background(), "archive-run")
	if err != nil || result != (ChannelImportResult{Quarantined: 1}) {
		t.Fatalf("missing parent result=%+v err=%v", result, err)
	}
}

func TestChannelRelationDefinitionsFailClosedOnRedactionAndCivilTimeLoss(t *testing.T) {
	contact := channelArchiveRow(t, "public/automation_channel_contact", 2, 1)
	contact.RedactedFields = []string{"unionid"}
	if definition, _, _, reason := channelContactDefinition(contact); definition != nil || reason != "redacted_channel_contact" {
		t.Fatalf("redacted contact definition=%+v reason=%q", definition, reason)
	}
	assignee := channelArchiveRow(t, "public/automation_channel_assignee", 3, 1)
	assignee.Payload = []byte(`{"id":3,"channel_id":1,"staff_id":"staff","display_name_snapshot":"name","priority":1,"ratio_percent":0,"max_scans_24h":0,"status":"active","created_at":"2026-08-28T08:00:00.1234567","updated_at":"2026-08-28T09:00:00"}`)
	assignee.PayloadHMAC = sha256.Sum256(assignee.Payload)
	if definition, _, reason := channelAssigneeDefinition(assignee); definition != nil || reason != "invalid_channel_assignee" {
		t.Fatalf("lossy civil time definition=%+v reason=%q", definition, reason)
	}
	if got, ok := channelCivilTime("2026-08-28T08:00:00.12"); !ok || got != "2026-08-28T08:00:00.120000" {
		t.Fatalf("civil time=%q ok=%v", got, ok)
	}
}

func TestChannelRelationRowsRequireVerifiedArchiveProvenance(t *testing.T) {
	row := channelArchiveRow(t, "public/automation_channel_contact", 2, 1)
	row.SourceKeyHMAC = [sha256.Size]byte{}
	importer, _, _, _, _, _ := channelImporterFixture(t, map[string][]v1archive.ArchivedRow{row.TableID: {row}})
	if _, err := importer.Import(context.Background(), "archive-run"); !errors.Is(err, ErrConflict) {
		t.Fatalf("unverified row err=%v", err)
	}
	if row.AdapterID != v1archive.DefaultAdapterID || row.FieldHMAC == [sha256.Size]byte{} {
		t.Fatal("fixture lost source adapter")
	}
}

package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1candidate"
)

func TestCampaignImporterImportsOnlyDisabledDefinition(t *testing.T) {
	stamp := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	archive := &campaignArchiveFake{rows: map[string][]v1archive.ArchivedRow{
		campaignTableID: {
			archivedJSON(t, 1, campaignJSON{ID: 11, CampaignCode: "safe-v1", DisplayName: "Safe", ReviewStatus: "pending_review", RunStatus: "draft", OwnerUserID: "owner-a", CreatedAt: stamp, UpdatedAt: stamp}),
			archivedJSON(t, 2, campaignJSON{ID: 12, CampaignCode: "active-v1", DisplayName: "Active", ReviewStatus: "approved", RunStatus: "active", OwnerUserID: "owner-a", CreatedAt: stamp, UpdatedAt: stamp}),
		},
		campaignStepTableID: {
			archivedJSON(t, 3, campaignStepJSON{ID: 31, CampaignID: 11, CampaignSegmentID: 21, StepIndex: 0, SendTime: "09:30", Timezone: "Asia/Shanghai", ContentText: "hello"}),
			archivedJSON(t, 4, campaignStepJSON{ID: 32, CampaignID: 12, CampaignSegmentID: 22, StepIndex: 0, SendTime: "10:00", Timezone: "Asia/Shanghai", ContentText: "never resume"}),
		},
	}}
	store := &campaignStoreFake{}
	journal := &campaignJournalFake{historical: map[string]campaign.HistoricalDefinitionReceipt{}}
	writer, err := campaign.NewHistoricalDefinitionWriter(store, journal)
	if err != nil {
		t.Fatal(err)
	}
	importer, err := NewCampaignImporter(archive, immediateUOW{}, writer, journal, journal, v1candidate.ActorIDs{"owner-a": 7})
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.Import(context.Background(), "archive-run")
	if err != nil {
		t.Fatal(err)
	}
	if result.ImportedCampaigns != 1 || result.ImportedSteps != 1 || result.ArchivedRows != 2 || result.QuarantinedRows != 0 {
		t.Fatalf("result = %#v", result)
	}
	if len(store.values) != 1 || store.values[0].Campaign.ApprovalStatus != campaign.ApprovalRejected || store.values[0].Campaign.RuntimeStatus != campaign.RuntimePaused {
		t.Fatalf("stored definitions = %#v", store.values)
	}
	if len(journal.terminals) != 3 || journal.terminals[0].Disposition != "import" || journal.terminals[1].Disposition != "archive" || journal.terminals[2].Disposition != "archive" {
		t.Fatalf("terminal receipts = %#v", journal.terminals)
	}
}

type campaignArchiveFake struct {
	rows map[string][]v1archive.ArchivedRow
}

func (archive *campaignArchiveFake) EachTableRow(_ context.Context, _ string, table string, callback func(v1archive.ArchivedRow) error) error {
	for _, row := range archive.rows[table] {
		if err := callback(row); err != nil {
			return err
		}
	}
	return nil
}

type immediateUOW struct{}

func (immediateUOW) Within(ctx context.Context, callback func(context.Context) error) error {
	return callback(ctx)
}

type campaignStoreFake struct {
	values []campaign.HistoricalDefinition
}

func (store *campaignStoreFake) InsertHistoricalDefinition(_ context.Context, value campaign.HistoricalDefinition) error {
	store.values = append(store.values, value)
	return nil
}

type campaignJournalFake struct {
	historical map[string]campaign.HistoricalDefinitionReceipt
	terminals  []TerminalReceipt
}

func (journal *campaignJournalFake) LoadHistoricalDefinition(_ context.Context, source string) (campaign.HistoricalDefinitionReceipt, bool, error) {
	receipt, found := journal.historical[source]
	return receipt, found, nil
}

func (journal *campaignJournalFake) RecordHistoricalDefinition(_ context.Context, receipt campaign.HistoricalDefinitionReceipt) error {
	journal.historical[receipt.SourceIdentifier] = receipt
	return nil
}

func (journal *campaignJournalFake) Record(_ context.Context, receipt TerminalReceipt) error {
	journal.terminals = append(journal.terminals, receipt)
	return nil
}

func archivedJSON(t *testing.T, ordinal int64, value any) v1archive.ArchivedRow {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	source := sha256.Sum256([]byte{byte(ordinal), 's'})
	digest := sha256.Sum256(payload)
	return v1archive.ArchivedRow{AdapterID: v1archive.DefaultAdapterID, TableID: campaignTableID, SourceOrdinal: ordinal, SourceKeyHMAC: source, PayloadHMAC: digest, Payload: payload}
}

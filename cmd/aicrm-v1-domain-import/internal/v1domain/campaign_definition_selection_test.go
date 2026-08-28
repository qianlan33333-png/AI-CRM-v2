package v1domain

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

func TestCampaignDefinitionSelectorSelectsOnlyPriorNonImportRowsInSourceOrder(t *testing.T) {
	campaignOne := campaignDefinitionSelectionRow(campaignTableID, 2, 1, []byte(`{"state":"old"}`))
	campaignTwo := campaignDefinitionSelectionRow(campaignTableID, 1, 2, []byte(`{"state":"noncurrent"}`))
	stepOne := campaignDefinitionSelectionRow(campaignStepTableID, 3, 3, []byte(`{"step":"a"}`))
	stepTwo := campaignDefinitionSelectionRow(campaignStepTableID, 4, 4, []byte(`{"step":"b"}`))
	archive := campaignDefinitionSelectionArchive{rows: map[string][]v1archive.ArchivedRow{
		campaignTableID:     {campaignOne, campaignTwo},
		campaignStepTableID: {stepOne, stepTwo},
	}}
	receipts := campaignDefinitionSelectionReceipts{rows: map[string][]CampaignDefinitionPriorReceipt{
		campaignTableID: {
			campaignDefinitionSelectionReceipt(campaignOne, "archive", "legacy_status"),
			campaignDefinitionSelectionReceipt(campaignTwo, "quarantine", "legacy_noncurrent"),
		},
		campaignStepTableID: {
			campaignDefinitionSelectionReceipt(stepOne, "quarantine", "step_unresolved"),
			campaignDefinitionSelectionReceipt(stepTwo, "import", ""),
		},
	}}
	selector, err := NewCampaignDefinitionSelector(archive, receipts)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := selector.Select(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Campaigns) != 2 || selected.Campaigns[0].ArchivedRow.SourceKeyHMAC != campaignOne.SourceKeyHMAC ||
		selected.Campaigns[0].PriorDisposition != "archive" || selected.Campaigns[0].PriorReason != "legacy_status" {
		t.Fatalf("campaign selection = %#v", selected.Campaigns)
	}
	if selected.Campaigns[1].ArchivedRow.SourceKeyHMAC != campaignTwo.SourceKeyHMAC || selected.Campaigns[1].PriorDisposition != "quarantine" ||
		selected.Campaigns[1].PriorReason != "legacy_noncurrent" {
		t.Fatalf("campaign source order = %#v", selected.Campaigns)
	}
	if len(selected.Steps) != 1 || selected.Steps[0].ArchivedRow.SourceKeyHMAC != stepOne.SourceKeyHMAC ||
		selected.Steps[0].PriorDisposition != "quarantine" || selected.Steps[0].PriorReason != "step_unresolved" {
		t.Fatalf("step selection = %#v", selected.Steps)
	}
}

func TestCampaignDefinitionSelectorFailsClosedOnArchiveOrReceiptMismatch(t *testing.T) {
	row := campaignDefinitionSelectionRow(campaignTableID, 1, 1, []byte(`{"id":1}`))
	other := campaignDefinitionSelectionRow(campaignTableID, 2, 2, []byte(`{"id":2}`))
	validReceipt := campaignDefinitionSelectionReceipt(row, "archive", "old")
	cases := map[string]struct {
		rows     []v1archive.ArchivedRow
		receipts []CampaignDefinitionPriorReceipt
	}{
		"same key different archive payload": {rows: []v1archive.ArchivedRow{row, func() v1archive.ArchivedRow {
			value := row
			value.PayloadHMAC = other.PayloadHMAC
			value.Payload = other.Payload
			return value
		}()}, receipts: []CampaignDefinitionPriorReceipt{validReceipt}},
		"duplicate receipt": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{validReceipt, validReceipt}},
		"missing receipt":   {rows: []v1archive.ArchivedRow{row}, receipts: nil},
		"extra receipt":     {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{validReceipt, campaignDefinitionSelectionReceipt(other, "archive", "old")}},
		"payload mismatch": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{func() CampaignDefinitionPriorReceipt {
			value := validReceipt
			value.PayloadDigest = other.PayloadHMAC
			return value
		}()}},
		"archive has target scope": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{func() CampaignDefinitionPriorReceipt {
			value := validReceipt
			value.TargetDomain = "campaign"
			value.TargetTable = "wrong"
			return value
		}()}},
		"quarantine missing reason": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{func() CampaignDefinitionPriorReceipt {
			value := validReceipt
			value.Disposition = "quarantine"
			value.Reason = ""
			return value
		}()}},
		"import missing target scope": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{func() CampaignDefinitionPriorReceipt {
			value := campaignDefinitionSelectionReceipt(row, "import", "")
			value.TargetTable = ""
			return value
		}()}},
		"import has reason": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{campaignDefinitionSelectionReceipt(row, "import", "unexpected")}},
		"unknown disposition": {rows: []v1archive.ArchivedRow{row}, receipts: []CampaignDefinitionPriorReceipt{func() CampaignDefinitionPriorReceipt {
			value := validReceipt
			value.Disposition = "pending"
			return value
		}()}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			selector, err := NewCampaignDefinitionSelector(campaignDefinitionSelectionArchive{rows: map[string][]v1archive.ArchivedRow{
				campaignTableID: test.rows, campaignStepTableID: {},
			}}, campaignDefinitionSelectionReceipts{rows: map[string][]CampaignDefinitionPriorReceipt{
				campaignTableID: test.receipts, campaignStepTableID: {},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = selector.Select(context.Background(), "run"); err == nil {
				t.Fatal("Select unexpectedly succeeded")
			}
		})
	}
}

func TestCampaignDefinitionSelectorRejectsCrossTableReceiptAndKeepsEmptyTablesValid(t *testing.T) {
	row := campaignDefinitionSelectionRow(campaignTableID, 1, 1, []byte(`{"id":1}`))
	receipt := campaignDefinitionSelectionReceipt(row, "archive", "old")
	receipt.TableID = campaignStepTableID
	selector, err := NewCampaignDefinitionSelector(campaignDefinitionSelectionArchive{rows: map[string][]v1archive.ArchivedRow{
		campaignTableID: {row}, campaignStepTableID: {},
	}}, campaignDefinitionSelectionReceipts{rows: map[string][]CampaignDefinitionPriorReceipt{
		campaignTableID: {receipt}, campaignStepTableID: {},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = selector.Select(context.Background(), "run"); err == nil {
		t.Fatal("Select unexpectedly accepted a cross-table receipt")
	}
}

type campaignDefinitionSelectionArchive struct {
	rows map[string][]v1archive.ArchivedRow
}

func (source campaignDefinitionSelectionArchive) EachTableRow(_ context.Context, _ string, table string, emit func(v1archive.ArchivedRow) error) error {
	for _, row := range source.rows[table] {
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

type campaignDefinitionSelectionReceipts struct {
	rows map[string][]CampaignDefinitionPriorReceipt
}

func (source campaignDefinitionSelectionReceipts) EachCampaignDefinitionPriorReceipt(_ context.Context, _ string, table string, emit func(CampaignDefinitionPriorReceipt) error) error {
	for _, receipt := range source.rows[table] {
		if err := emit(receipt); err != nil {
			return err
		}
	}
	return nil
}

func campaignDefinitionSelectionRow(table string, ordinal int64, seed byte, payload []byte) v1archive.ArchivedRow {
	return v1archive.ArchivedRow{
		AdapterID: v1archive.DefaultAdapterID, TableID: table, SourceOrdinal: ordinal,
		SourceKeyHMAC: sha256.Sum256([]byte{seed}), PayloadHMAC: sha256.Sum256(payload), FieldHMAC: sha256.Sum256([]byte{seed, 1}), Payload: payload,
	}
}

func campaignDefinitionSelectionReceipt(row v1archive.ArchivedRow, disposition, reason string) CampaignDefinitionPriorReceipt {
	targetTable := "cloud_campaigns"
	if row.TableID == campaignStepTableID {
		targetTable = "cloud_campaign_steps"
	}
	receipt := CampaignDefinitionPriorReceipt{
		ImportVersion: campaignDefinitionSelectionImportVersion, ArchiveRunID: "run", AdapterID: v1archive.DefaultAdapterID,
		TableID: row.TableID, SourceKey: row.SourceKeyHMAC,
		PayloadDigest: row.PayloadHMAC, Disposition: disposition, Reason: reason,
	}
	if disposition == "import" {
		receipt.TargetDomain = "campaign"
		receipt.TargetTable = targetTable
		receipt.Reason = reason
	}
	return receipt
}

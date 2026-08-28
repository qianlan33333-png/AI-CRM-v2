package v1domain

import (
	"context"
	"crypto/sha256"

	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const campaignDefinitionSelectionImportVersion = "v1-domain-a1"

type CampaignDefinitionPriorReceipt struct {
	ImportVersion string
	ArchiveRunID  string
	AdapterID     string
	TableID       string
	TargetDomain  string
	TargetTable   string
	SourceKey     [sha256.Size]byte
	PayloadDigest [sha256.Size]byte
	Disposition   string
	Reason        string
}

// CampaignDefinitionPriorReceiptReader is implemented by the root-owned SQL
// adapter. It streams only receipts; this selector neither writes nor queries
// a target table.
type CampaignDefinitionPriorReceiptReader interface {
	EachCampaignDefinitionPriorReceipt(context.Context, string, string, func(CampaignDefinitionPriorReceipt) error) error
}

type CampaignDefinitionSelectedRow struct {
	ArchivedRow      v1archive.ArchivedRow
	PriorDisposition string
	PriorReason      string
}

type CampaignDefinitionSelection struct {
	Campaigns []CampaignDefinitionSelectedRow
	Steps     []CampaignDefinitionSelectedRow
}

type CampaignDefinitionSelector struct {
	archive  ArchiveSource
	receipts CampaignDefinitionPriorReceiptReader
}

func NewCampaignDefinitionSelector(archive ArchiveSource, receipts CampaignDefinitionPriorReceiptReader) (*CampaignDefinitionSelector, error) {
	if archive == nil || receipts == nil {
		return nil, ErrInvalidScope
	}
	return &CampaignDefinitionSelector{archive: archive, receipts: receipts}, nil
}

// Select returns every old non-import campaign receipt paired with its exact
// archived source material. Current import receipts are intentionally omitted;
// old status, actor, and code fields never participate in this predicate.
func (selector *CampaignDefinitionSelector) Select(ctx context.Context, archiveRunID string) (CampaignDefinitionSelection, error) {
	if selector == nil || selector.archive == nil || selector.receipts == nil || ctx == nil || archiveRunID == "" {
		return CampaignDefinitionSelection{}, ErrInvalidScope
	}
	campaigns, err := selector.selectTable(ctx, archiveRunID, campaignDefinitionSelectionScope{campaignTableID, "campaign", "cloud_campaigns"})
	if err != nil {
		return CampaignDefinitionSelection{}, err
	}
	steps, err := selector.selectTable(ctx, archiveRunID, campaignDefinitionSelectionScope{campaignStepTableID, "campaign", "cloud_campaign_steps"})
	if err != nil {
		return CampaignDefinitionSelection{}, err
	}
	return CampaignDefinitionSelection{Campaigns: campaigns, Steps: steps}, nil
}

type campaignDefinitionSelectionScope struct {
	tableID      string
	targetDomain string
	targetTable  string
}

func (selector *CampaignDefinitionSelector) selectTable(ctx context.Context, archiveRunID string, scope campaignDefinitionSelectionScope) ([]CampaignDefinitionSelectedRow, error) {
	rows := make([]v1archive.ArchivedRow, 0)
	archived := make(map[[sha256.Size]byte]v1archive.ArchivedRow)
	if err := selector.archive.EachTableRow(ctx, archiveRunID, scope.tableID, func(row v1archive.ArchivedRow) error {
		if row.AdapterID != v1archive.DefaultAdapterID || row.TableID != scope.tableID || row.SourceOrdinal < 1 ||
			row.SourceKeyHMAC == ([sha256.Size]byte{}) || row.PayloadHMAC == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if _, found := archived[row.SourceKeyHMAC]; found {
			return ErrConflict
		}
		archived[row.SourceKeyHMAC] = row
		rows = append(rows, row)
		return nil
	}); err != nil {
		return nil, err
	}

	receipts := make(map[[sha256.Size]byte]CampaignDefinitionPriorReceipt, len(archived))
	if err := selector.receipts.EachCampaignDefinitionPriorReceipt(ctx, archiveRunID, scope.tableID, func(receipt CampaignDefinitionPriorReceipt) error {
		if receipt.ImportVersion != campaignDefinitionSelectionImportVersion || receipt.ArchiveRunID != archiveRunID ||
			receipt.AdapterID != v1archive.DefaultAdapterID || receipt.TableID != scope.tableID ||
			receipt.TargetDomain != scope.targetDomain || receipt.TargetTable != scope.targetTable ||
			receipt.SourceKey == ([sha256.Size]byte{}) || receipt.PayloadDigest == ([sha256.Size]byte{}) {
			return ErrConflict
		}
		if receipt.Disposition != "import" && receipt.Disposition != "archive" && receipt.Disposition != "quarantine" {
			return ErrConflict
		}
		if _, found := receipts[receipt.SourceKey]; found {
			return ErrConflict
		}
		receipts[receipt.SourceKey] = receipt
		return nil
	}); err != nil {
		return nil, err
	}

	if len(rows) != len(receipts) {
		return nil, ErrConflict
	}
	selected := make([]CampaignDefinitionSelectedRow, 0)
	for _, row := range rows {
		receipt, found := receipts[row.SourceKeyHMAC]
		if !found || receipt.PayloadDigest != row.PayloadHMAC {
			return nil, ErrConflict
		}
		switch receipt.Disposition {
		case "import":
			continue
		case "archive", "quarantine":
			selected = append(selected, CampaignDefinitionSelectedRow{ArchivedRow: row, PriorDisposition: receipt.Disposition, PriorReason: receipt.Reason})
		default:
			return nil, ErrConflict
		}
	}
	return selected, nil
}

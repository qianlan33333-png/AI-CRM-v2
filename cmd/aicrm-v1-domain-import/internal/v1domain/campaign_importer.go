package v1domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1candidate"
	campaign "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	campaignTableID     = "public/campaigns"
	campaignStepTableID = "public/campaign_steps"
)

type ArchiveSource interface {
	EachTableRow(context.Context, string, string, func(v1archive.ArchivedRow) error) error
}

type UnitOfWork interface {
	Within(context.Context, func(context.Context) error) error
}

type CampaignReceiptJournal interface {
	campaign.HistoricalDefinitionJournal
	Record(context.Context, TerminalReceipt) error
}

type TerminalJournal interface {
	Record(context.Context, TerminalReceipt) error
}

type CampaignImportResult struct {
	ImportedCampaigns int
	ImportedSteps     int
	ArchivedRows      int
	QuarantinedRows   int
	ReplayedCampaigns int
}

type CampaignImporter struct {
	archive         ArchiveSource
	uow             UnitOfWork
	writer          *campaign.HistoricalDefinitionWriter
	campaignJournal CampaignReceiptJournal
	stepJournal     TerminalJournal
	actors          v1candidate.ActorIDs
}

func NewCampaignImporter(archive ArchiveSource, uow UnitOfWork, writer *campaign.HistoricalDefinitionWriter, campaignJournal CampaignReceiptJournal, stepJournal TerminalJournal, actors v1candidate.ActorIDs) (*CampaignImporter, error) {
	if archive == nil || uow == nil || writer == nil || campaignJournal == nil || stepJournal == nil || len(actors) == 0 {
		return nil, ErrInvalidScope
	}
	return &CampaignImporter{archive: archive, uow: uow, writer: writer, campaignJournal: campaignJournal, stepJournal: stepJournal, actors: actors}, nil
}

type campaignArchiveRow struct {
	archive v1archive.ArchivedRow
	source  v1candidate.CampaignRow
}

type campaignStepArchiveRow struct {
	archive v1archive.ArchivedRow
	source  v1candidate.CampaignStepRow
}

type campaignJSON struct {
	ID           int64     `json:"id"`
	CampaignCode string    `json:"campaign_code"`
	DisplayName  string    `json:"display_name"`
	ReviewStatus string    `json:"review_status"`
	RunStatus    string    `json:"run_status"`
	OwnerUserID  string    `json:"owner_userid"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type campaignStepJSON struct {
	ID                int64  `json:"id"`
	CampaignID        int64  `json:"campaign_id"`
	CampaignSegmentID int64  `json:"campaign_segment_id"`
	StepIndex         int    `json:"step_index"`
	DayOffset         int    `json:"day_offset"`
	SendTime          string `json:"send_time"`
	Timezone          string `json:"timezone"`
	ContentText       string `json:"content_text"`
}

func (importer *CampaignImporter) Import(ctx context.Context, archiveRunID string) (CampaignImportResult, error) {
	if importer == nil || archiveRunID == "" {
		return CampaignImportResult{}, ErrInvalidScope
	}
	campaigns := make([]campaignArchiveRow, 0)
	if err := importer.archive.EachTableRow(ctx, archiveRunID, campaignTableID, func(row v1archive.ArchivedRow) error {
		var source campaignJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return fmt.Errorf("decode archived campaign row %d: %w", row.SourceOrdinal, err)
		}
		campaigns = append(campaigns, campaignArchiveRow{archive: row, source: v1candidate.CampaignRow{
			ID: source.ID, CampaignCode: source.CampaignCode, DisplayName: source.DisplayName,
			ReviewStatus: source.ReviewStatus, RunStatus: source.RunStatus, OwnerUserID: source.OwnerUserID,
			CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		}})
		return nil
	}); err != nil {
		return CampaignImportResult{}, err
	}
	stepsByCampaign := make(map[int64][]campaignStepArchiveRow)
	if err := importer.archive.EachTableRow(ctx, archiveRunID, campaignStepTableID, func(row v1archive.ArchivedRow) error {
		var source campaignStepJSON
		if err := json.Unmarshal(row.Payload, &source); err != nil {
			return fmt.Errorf("decode archived campaign step row %d: %w", row.SourceOrdinal, err)
		}
		step := campaignStepArchiveRow{archive: row, source: v1candidate.CampaignStepRow{
			ID: source.ID, CampaignID: source.CampaignID, CampaignSegmentID: source.CampaignSegmentID,
			StepIndex: source.StepIndex, DayOffset: source.DayOffset, SendTime: source.SendTime,
			Timezone: source.Timezone, ContentText: source.ContentText,
		}}
		stepsByCampaign[source.CampaignID] = append(stepsByCampaign[source.CampaignID], step)
		return nil
	}); err != nil {
		return CampaignImportResult{}, err
	}
	for campaignID := range stepsByCampaign {
		sort.Slice(stepsByCampaign[campaignID], func(left, right int) bool {
			return stepsByCampaign[campaignID][left].source.StepIndex < stepsByCampaign[campaignID][right].source.StepIndex
		})
	}
	knownCampaigns := make(map[int64]struct{}, len(campaigns))
	result := CampaignImportResult{}
	for _, archivedCampaign := range campaigns {
		knownCampaigns[archivedCampaign.source.ID] = struct{}{}
		archivedSteps := stepsByCampaign[archivedCampaign.source.ID]
		sourceSteps := make([]v1candidate.CampaignStepRow, len(archivedSteps))
		for index := range archivedSteps {
			sourceSteps[index] = archivedSteps[index].source
		}
		decision := v1candidate.ConvertCampaignDefinition(archivedCampaign.source, sourceSteps, importer.actors)
		switch decision.Disposition {
		case v1candidate.CanonicalCandidate:
			if decision.Candidate == nil {
				return CampaignImportResult{}, ErrConflict
			}
			replayed := false
			err := importer.uow.Within(ctx, func(tx context.Context) error {
				replayed = false
				receipt, err := importer.writer.Import(tx, campaign.HistoricalDefinition{
					SourceIdentifier:       SourceIdentifier(archivedCampaign.archive.SourceKeyHMAC),
					PayloadDigest:          archivedCampaign.archive.PayloadHMAC,
					OriginalApprovalStatus: archivedCampaign.source.ReviewStatus,
					OriginalRuntimeStatus:  archivedCampaign.source.RunStatus,
					Campaign:               decision.Candidate.Campaign, Steps: decision.Candidate.Steps,
				})
				if err != nil {
					return err
				}
				replayed = receipt.Replayed
				for index, step := range archivedSteps {
					targetID := receipt.TargetCampaignCode + ":" + strconv.Itoa(index+1)
					targetDigest := sha256.Sum256([]byte(targetID + "\x00" + hex.EncodeToString(step.archive.PayloadHMAC[:])))
					if err := importer.stepJournal.Record(tx, TerminalReceipt{
						SourceKeyDigest: step.archive.SourceKeyHMAC, PayloadDigest: step.archive.PayloadHMAC,
						Disposition: "import", TargetID: targetID, TargetDigest: targetDigest,
					}); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return CampaignImportResult{}, err
			}
			if replayed {
				result.ReplayedCampaigns++
			}
			result.ImportedCampaigns++
			result.ImportedSteps += len(archivedSteps)
		case v1candidate.Archive, v1candidate.Quarantine:
			disposition := string(decision.Disposition)
			err := importer.uow.Within(ctx, func(tx context.Context) error {
				if err := importer.campaignJournal.Record(tx, TerminalReceipt{
					SourceKeyDigest: archivedCampaign.archive.SourceKeyHMAC, PayloadDigest: archivedCampaign.archive.PayloadHMAC,
					Disposition: disposition, Reason: decision.Reason,
				}); err != nil {
					return err
				}
				for _, step := range archivedSteps {
					if err := importer.stepJournal.Record(tx, TerminalReceipt{
						SourceKeyDigest: step.archive.SourceKeyHMAC, PayloadDigest: step.archive.PayloadHMAC,
						Disposition: disposition, Reason: decision.Reason,
					}); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return CampaignImportResult{}, err
			}
			if decision.Disposition == v1candidate.Archive {
				result.ArchivedRows += 1 + len(archivedSteps)
			} else {
				result.QuarantinedRows += 1 + len(archivedSteps)
			}
		default:
			return CampaignImportResult{}, ErrConflict
		}
	}
	for campaignID, orphanSteps := range stepsByCampaign {
		if _, found := knownCampaigns[campaignID]; found {
			continue
		}
		if err := importer.uow.Within(ctx, func(tx context.Context) error {
			for _, step := range orphanSteps {
				if err := importer.stepJournal.Record(tx, TerminalReceipt{
					SourceKeyDigest: step.archive.SourceKeyHMAC, PayloadDigest: step.archive.PayloadHMAC,
					Disposition: "quarantine", Reason: v1candidate.ReasonStepCampaignUnresolved,
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return CampaignImportResult{}, err
		}
		result.QuarantinedRows += len(orphanSteps)
	}
	return result, nil
}

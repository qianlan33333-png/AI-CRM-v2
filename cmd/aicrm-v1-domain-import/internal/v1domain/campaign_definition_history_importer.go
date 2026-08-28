package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"
	"time"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1campaigndefinitionhistory"
	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

// CampaignDefinitionHistoryWriter is the Campaign-owned non-executable write
// boundary. It writes its target and generic receipt in the caller transaction.
type CampaignDefinitionHistoryWriter interface {
	WriteDefinition(context.Context, string, campaignport.HistoricalCampaignDefinition) (campaignport.CampaignHistoryReceipt, error)
	WriteStep(context.Context, string, campaignport.HistoricalCampaignDefinitionStep) (campaignport.CampaignHistoryReceipt, error)
}

// CampaignDefinitionCurrentParentResolver may return only a current Campaign
// code proven by the previous v1-domain-a1 receipt and its actual target. A
// missing receipt is represented by found=false, never by a guessed code.
type CampaignDefinitionCurrentParentResolver interface {
	ResolveVerifiedCurrentCampaignDefinition(context.Context, int64, [sha256.Size]byte) (currentCode string, found bool, err error)
}

type CampaignDefinitionHistoryImportResult struct {
	ImportedDefinitions int
	ImportedSteps       int
	ReplayedDefinitions int
	ReplayedSteps       int
}

func (result CampaignDefinitionHistoryImportResult) Selected() int {
	return result.ImportedDefinitions + result.ImportedSteps + result.ReplayedDefinitions + result.ReplayedSteps
}

// CampaignDefinitionHistoryImporter writes only definitions and steps selected
// from the old non-import receipts. It never creates or changes current
// Campaigns, plans, commands, events, or Provider effects.
type CampaignDefinitionHistoryImporter struct {
	selector      *CampaignDefinitionSelector
	uow           UnitOfWork
	writer        CampaignDefinitionHistoryWriter
	parent        CampaignDefinitionCurrentParentResolver
	journal       CampaignDefinitionHistoryImportJournal
	sourceHMACKey []byte
}

func NewCampaignDefinitionHistoryImporter(selector *CampaignDefinitionSelector, uow UnitOfWork, writer CampaignDefinitionHistoryWriter, parent CampaignDefinitionCurrentParentResolver, journal CampaignDefinitionHistoryImportJournal, sourceHMACKey []byte) (*CampaignDefinitionHistoryImporter, error) {
	if selector == nil || uow == nil || writer == nil || parent == nil || journal == nil || len(sourceHMACKey) < sha256.Size {
		return nil, ErrInvalidScope
	}
	return &CampaignDefinitionHistoryImporter{
		selector: selector, uow: uow, writer: writer, parent: parent, journal: journal, sourceHMACKey: append([]byte(nil), sourceHMACKey...),
	}, nil
}

type campaignDefinitionHistoryDefinitionRow struct {
	selected CampaignDefinitionSelectedRow
	fact     v1campaigndefinitionhistory.DefinitionFact
}

type campaignDefinitionHistoryStepRow struct {
	selected CampaignDefinitionSelectedRow
	fact     v1campaigndefinitionhistory.StepFact
}

func (importer *CampaignDefinitionHistoryImporter) Import(ctx context.Context, archiveRunID string) (CampaignDefinitionHistoryImportResult, error) {
	if importer == nil || ctx == nil || importer.selector == nil || importer.uow == nil || importer.writer == nil || importer.parent == nil || importer.journal == nil ||
		archiveRunID == "" || len(importer.sourceHMACKey) < sha256.Size || importer.journal.ValidateCampaignDefinitionHistoryImportScope(archiveRunID) != nil {
		return CampaignDefinitionHistoryImportResult{}, ErrInvalidScope
	}
	selection, err := importer.selector.Select(ctx, archiveRunID)
	if err != nil {
		return CampaignDefinitionHistoryImportResult{}, err
	}
	definitions, steps, err := importer.adapt(selection)
	if err != nil {
		return CampaignDefinitionHistoryImportResult{}, err
	}

	result := CampaignDefinitionHistoryImportResult{}
	historyParents := make(map[int64]int64, len(definitions))
	for _, row := range definitions {
		receipt, err := importer.importDefinition(ctx, row)
		if err != nil {
			return CampaignDefinitionHistoryImportResult{}, err
		}
		if _, found := historyParents[row.fact.SourceID]; found || receipt.TargetID < 1 {
			return CampaignDefinitionHistoryImportResult{}, ErrConflict
		}
		historyParents[row.fact.SourceID] = receipt.TargetID
		if receipt.Replayed {
			result.ReplayedDefinitions++
		} else {
			result.ImportedDefinitions++
		}
	}
	for _, row := range steps {
		receipt, err := importer.importStep(ctx, row, historyParents)
		if err != nil {
			return CampaignDefinitionHistoryImportResult{}, err
		}
		if receipt.Replayed {
			result.ReplayedSteps++
		} else {
			result.ImportedSteps++
		}
	}
	if result.Selected() != len(definitions)+len(steps) {
		return CampaignDefinitionHistoryImportResult{}, ErrConflict
	}
	return result, nil
}

// adapt validates every selected archive envelope before the first writer
// transaction. Receipt selection stays separate: it decides source coverage,
// while the adapter decides whether the encrypted source can be preserved.
func (importer *CampaignDefinitionHistoryImporter) adapt(selection CampaignDefinitionSelection) ([]campaignDefinitionHistoryDefinitionRow, []campaignDefinitionHistoryStepRow, error) {
	definitions := make([]campaignDefinitionHistoryDefinitionRow, 0, len(selection.Campaigns))
	for _, selected := range selection.Campaigns {
		if !validCampaignDefinitionSelection(selected, campaignDefinitionHistoryDefinitionTable) {
			return nil, nil, ErrConflict
		}
		fact, err := v1campaigndefinitionhistory.AdaptDefinition(selected.ArchivedRow, importer.sourceHMACKey)
		if err != nil || !campaignDefinitionEnvelopeMatches(selected.ArchivedRow, fact.Source) {
			return nil, nil, ErrConflict
		}
		definitions = append(definitions, campaignDefinitionHistoryDefinitionRow{selected: selected, fact: fact})
	}
	steps := make([]campaignDefinitionHistoryStepRow, 0, len(selection.Steps))
	for _, selected := range selection.Steps {
		if !validCampaignDefinitionSelection(selected, campaignDefinitionHistoryStepTable) {
			return nil, nil, ErrConflict
		}
		fact, err := v1campaigndefinitionhistory.AdaptStep(selected.ArchivedRow, importer.sourceHMACKey)
		if err != nil || !campaignDefinitionEnvelopeMatches(selected.ArchivedRow, fact.Source) {
			return nil, nil, ErrConflict
		}
		steps = append(steps, campaignDefinitionHistoryStepRow{selected: selected, fact: fact})
	}
	return definitions, steps, nil
}

func (importer *CampaignDefinitionHistoryImporter) importDefinition(ctx context.Context, row campaignDefinitionHistoryDefinitionRow) (campaignport.CampaignHistoryReceipt, error) {
	var receipt campaignport.CampaignHistoryReceipt
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		receipt = campaignport.CampaignHistoryReceipt{}
		value := campaignport.HistoricalCampaignDefinition{
			SourceID: row.fact.SourceID, Code: row.fact.Code, DisplayName: row.fact.DisplayName, Intent: row.fact.Intent,
			AnchorMode: row.fact.AnchorMode, AnchorDate: row.fact.AnchorDate, ReviewStatus: row.fact.ReviewStatus, RunStatus: row.fact.RunStatus,
			ApprovedAt: copyCampaignDefinitionTime(row.fact.ApprovedAt), StartedAt: copyCampaignDefinitionTime(row.fact.StartedAt),
			FinishedAt: copyCampaignDefinitionTime(row.fact.FinishedAt), PausedAt: copyCampaignDefinitionTime(row.fact.PausedAt),
			PausedReason: row.fact.PausedReason, CreatedAt: row.fact.CreatedAt, UpdatedAt: row.fact.UpdatedAt,
			OriginalDisposition: row.selected.PriorDisposition, OriginalReason: row.selected.PriorReason,
			PrivateDigest: [sha256.Size]byte(row.fact.PrivateDigest), SourceKeyDigest: [sha256.Size]byte(row.fact.Source.SourceKeyDigest),
			SourcePayloadDigest: [sha256.Size]byte(row.fact.Source.PayloadDigest), SourceFieldDigest: [sha256.Size]byte(row.fact.Source.FieldDigest),
			RedactedRoots: append([]string(nil), row.fact.RedactedRoots...),
		}
		var err error
		receipt, err = importer.writer.WriteDefinition(tx, SourceIdentifier(row.selected.ArchivedRow.SourceKeyHMAC), value)
		if err != nil {
			return err
		}
		return importer.verifyReceipt(tx, campaignDefinitionHistoryDefinitionKind, row.selected.ArchivedRow, receipt)
	})
	return receipt, err
}

func (importer *CampaignDefinitionHistoryImporter) importStep(ctx context.Context, row campaignDefinitionHistoryStepRow, historyParents map[int64]int64) (campaignport.CampaignHistoryReceipt, error) {
	var receipt campaignport.CampaignHistoryReceipt
	err := importer.uow.Within(ctx, func(tx context.Context) error {
		receipt = campaignport.CampaignHistoryReceipt{}
		historyID, currentCode, state, err := importer.resolveParent(tx, row.fact.CampaignSourceID, historyParents)
		if err != nil {
			return err
		}
		value := campaignport.HistoricalCampaignDefinitionStep{
			SourceID: row.fact.SourceID, CampaignSourceID: row.fact.CampaignSourceID, SegmentSourceID: row.fact.SegmentSourceID,
			HistoryDefinitionID: copyCampaignDefinitionID(historyID), CurrentCampaignCode: copyCampaignDefinitionString(currentCode), SourceParentState: state,
			StepIndex: row.fact.StepIndex, DayOffset: row.fact.DayOffset, SendTime: row.fact.SendTime, Timezone: row.fact.Timezone,
			ContentMasked: row.fact.ContentMasked, StopOnReply: row.fact.StopOnReply, SkipRecentDays: row.fact.SkipRecentDays,
			CreatedAt: row.fact.CreatedAt, UpdatedAt: row.fact.UpdatedAt, OriginalDisposition: row.selected.PriorDisposition, OriginalReason: row.selected.PriorReason,
			ContentDigest: [sha256.Size]byte(row.fact.ContentDigest), PrivateDigest: [sha256.Size]byte(row.fact.PrivateDigest),
			SourceKeyDigest: [sha256.Size]byte(row.fact.Source.SourceKeyDigest), SourcePayloadDigest: [sha256.Size]byte(row.fact.Source.PayloadDigest),
			SourceFieldDigest: [sha256.Size]byte(row.fact.Source.FieldDigest), RedactedRoots: append([]string(nil), row.fact.RedactedRoots...),
		}
		receipt, err = importer.writer.WriteStep(tx, SourceIdentifier(row.selected.ArchivedRow.SourceKeyHMAC), value)
		if err != nil {
			return err
		}
		return importer.verifyReceipt(tx, campaignDefinitionHistoryStepKind, row.selected.ArchivedRow, receipt)
	})
	return receipt, err
}

func (importer *CampaignDefinitionHistoryImporter) resolveParent(ctx context.Context, campaignSourceID int64, historyParents map[int64]int64) (*int64, *string, string, error) {
	if historyID, found := historyParents[campaignSourceID]; found {
		if historyID < 1 {
			return nil, nil, "", ErrConflict
		}
		return &historyID, nil, "history_definition", nil
	}
	sourceKey, err := v1archive.SourceKeyHMAC(importer.sourceHMACKey, strings.TrimPrefix(campaignDefinitionHistoryDefinitionTable, "public/"), []byte("["+strconv.FormatInt(campaignSourceID, 10)+"]"))
	if err != nil {
		return nil, nil, "", ErrConflict
	}
	currentCode, found, err := importer.parent.ResolveVerifiedCurrentCampaignDefinition(ctx, campaignSourceID, sourceKey)
	if err != nil {
		return nil, nil, "", err
	}
	if found {
		if currentCode == "" {
			return nil, nil, "", ErrConflict
		}
		return nil, &currentCode, "current_definition", nil
	}
	return nil, nil, "unresolved_definition", nil
}

func (importer *CampaignDefinitionHistoryImporter) verifyReceipt(ctx context.Context, kind string, row v1archive.ArchivedRow, receipt campaignport.CampaignHistoryReceipt) error {
	if !validCampaignDefinitionHistoryKind(kind) || receipt.SourceIdentifier != SourceIdentifier(row.SourceKeyHMAC) || receipt.PayloadDigest != row.PayloadHMAC ||
		receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return ErrConflict
	}
	terminal, found, err := importer.journal.LoadCampaignDefinitionHistoryTerminal(ctx, kind, receipt.SourceIdentifier)
	if err != nil {
		return err
	}
	if !found || terminal.SourceKeyDigest != row.SourceKeyHMAC || terminal.PayloadDigest != row.PayloadHMAC || terminal.Disposition != "import" || terminal.Reason != "" ||
		terminal.TargetID != strconv.FormatInt(receipt.TargetID, 10) || terminal.TargetDigest != receipt.TargetDigest || len(terminal.Metadata) != 0 {
		return ErrConflict
	}
	return nil
}

func validCampaignDefinitionSelection(selected CampaignDefinitionSelectedRow, table string) bool {
	return selected.ArchivedRow.TableID == table && (selected.PriorDisposition == "archive" || selected.PriorDisposition == "quarantine") && strings.TrimSpace(selected.PriorReason) != ""
}

func campaignDefinitionEnvelopeMatches(row v1archive.ArchivedRow, source v1campaigndefinitionhistory.SourceEnvelope) bool {
	return [sha256.Size]byte(source.SourceKeyDigest) == row.SourceKeyHMAC && [sha256.Size]byte(source.PayloadDigest) == row.PayloadHMAC && [sha256.Size]byte(source.FieldDigest) == row.FieldHMAC
}

func copyCampaignDefinitionTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyCampaignDefinitionID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyCampaignDefinitionString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

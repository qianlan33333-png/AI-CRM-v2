package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
)

const (
	campaignDefinitionHistoryKind     = "definitions"
	campaignDefinitionHistoryStepKind = "steps"
)

// CampaignDefinitionHistoryWriter persists V1 observations only. It does not
// use the current Campaign repository, a queue, events, or a Provider.
type CampaignDefinitionHistoryWriter struct {
	store   campaignport.CampaignDefinitionHistoryStore
	journal campaignport.CampaignDefinitionHistoryJournal
}

func NewCampaignDefinitionHistoryWriter(store campaignport.CampaignDefinitionHistoryStore, journal campaignport.CampaignDefinitionHistoryJournal) *CampaignDefinitionHistoryWriter {
	return &CampaignDefinitionHistoryWriter{store: store, journal: journal}
}

func (writer *CampaignDefinitionHistoryWriter) WriteDefinition(ctx context.Context, source string, value campaignport.HistoricalCampaignDefinition) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignDefinitionHistoryReady(writer, ctx) || value.ID != 0 || !validHistoricalCampaignDefinition(value, false) || !validCampaignDefinitionHistorySource(source, value.SourceKeyDigest) {
		return campaignDefinitionHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalCampaignDefinition(value)
	return writeCampaignHistory(ctx, campaignDefinitionHistoryJournalBridge{writer.journal}, campaignDefinitionHistoryKind, source, value.SourcePayloadDigest, value,
		func(v campaignport.HistoricalCampaignDefinition) int64 { return v.ID },
		func(v campaignport.HistoricalCampaignDefinition, id int64) campaignport.HistoricalCampaignDefinition {
			v.ID = id
			return normalizeHistoricalCampaignDefinition(v)
		},
		HistoricalCampaignDefinitionDigest, writer.store.CreateHistoricalCampaignDefinition, writer.store.GetHistoricalCampaignDefinition)
}

func (writer *CampaignDefinitionHistoryWriter) WriteStep(ctx context.Context, source string, value campaignport.HistoricalCampaignDefinitionStep) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignDefinitionHistoryReady(writer, ctx) || value.ID != 0 || !validHistoricalCampaignDefinitionStep(value, false) || !validCampaignDefinitionHistorySource(source, value.SourceKeyDigest) {
		return campaignDefinitionHistoryInvalidOrUnavailable(writer, ctx)
	}
	value = normalizeHistoricalCampaignDefinitionStep(value)
	return writeCampaignHistory(ctx, campaignDefinitionHistoryJournalBridge{writer.journal}, campaignDefinitionHistoryStepKind, source, value.SourcePayloadDigest, value,
		func(v campaignport.HistoricalCampaignDefinitionStep) int64 { return v.ID },
		func(v campaignport.HistoricalCampaignDefinitionStep, id int64) campaignport.HistoricalCampaignDefinitionStep {
			v.ID = id
			return normalizeHistoricalCampaignDefinitionStep(v)
		},
		HistoricalCampaignDefinitionStepDigest, writer.store.CreateHistoricalCampaignDefinitionStep, writer.store.GetHistoricalCampaignDefinitionStep)
}

func HistoricalCampaignDefinitionDigest(value campaignport.HistoricalCampaignDefinition) ([sha256.Size]byte, error) {
	if !validHistoricalCampaignDefinition(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("definition", normalizeHistoricalCampaignDefinition(value))
}

func HistoricalCampaignDefinitionStepDigest(value campaignport.HistoricalCampaignDefinitionStep) ([sha256.Size]byte, error) {
	if !validHistoricalCampaignDefinitionStep(value, true) {
		return [sha256.Size]byte{}, campaignport.ErrCampaignHistoryInvalid
	}
	return campaignHistoryDigest("definition_step", normalizeHistoricalCampaignDefinitionStep(value))
}

type campaignDefinitionHistoryJournalBridge struct {
	journal campaignport.CampaignDefinitionHistoryJournal
}

func (bridge campaignDefinitionHistoryJournalBridge) LoadCampaignHistory(ctx context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	if campaignHistoryNil(bridge.journal) {
		return campaignport.CampaignHistoryReceipt{}, false, campaignport.ErrCampaignHistoryUnavailable
	}
	return bridge.journal.LoadCampaignDefinitionHistory(ctx, kind, source)
}

func (bridge campaignDefinitionHistoryJournalBridge) RecordCampaignHistory(ctx context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	if campaignHistoryNil(bridge.journal) {
		return campaignport.ErrCampaignHistoryUnavailable
	}
	return bridge.journal.RecordCampaignDefinitionHistory(ctx, kind, receipt)
}

func validHistoricalCampaignDefinition(value campaignport.HistoricalCampaignDefinition, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && validCampaignDefinitionDisposition(value.OriginalDisposition, value.OriginalReason) &&
		value.PrivateDigest != ([sha256.Size]byte{}) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && value.SourceFieldDigest != ([sha256.Size]byte{}) &&
		validCampaignHistoryText(value.Code, value.DisplayName, value.Intent, value.AnchorMode, value.AnchorDate, value.ReviewStatus, value.RunStatus, value.PausedReason) &&
		validCampaignHistoryRedactedRoots(value.RedactedRoots) && validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored) &&
		validCampaignHistoryOptionalTime(value.ApprovedAt, stored) && validCampaignHistoryOptionalTime(value.StartedAt, stored) &&
		validCampaignHistoryOptionalTime(value.FinishedAt, stored) && validCampaignHistoryOptionalTime(value.PausedAt, stored)
}

func validHistoricalCampaignDefinitionStep(value campaignport.HistoricalCampaignDefinitionStep, stored bool) bool {
	return validCampaignHistoryID(value.ID, stored) && validCampaignDefinitionDisposition(value.OriginalDisposition, value.OriginalReason) &&
		value.ContentDigest != ([sha256.Size]byte{}) && value.PrivateDigest != ([sha256.Size]byte{}) && value.SourceKeyDigest != ([sha256.Size]byte{}) &&
		value.SourcePayloadDigest != ([sha256.Size]byte{}) && value.SourceFieldDigest != ([sha256.Size]byte{}) &&
		validCampaignHistoryText(value.SendTime, value.Timezone, value.ContentMasked) && validCampaignHistoryRedactedRoots(value.RedactedRoots) &&
		validCampaignHistoryTime(value.CreatedAt, stored) && validCampaignHistoryTime(value.UpdatedAt, stored) && validCampaignDefinitionStepParent(value)
}

func validCampaignDefinitionDisposition(disposition, reason string) bool {
	return (disposition == "archive" || disposition == "quarantine") && strings.TrimSpace(reason) != "" && validCampaignHistoryText(disposition, reason)
}

func validCampaignDefinitionStepParent(value campaignport.HistoricalCampaignDefinitionStep) bool {
	switch value.SourceParentState {
	case "history_definition":
		return value.HistoryDefinitionID != nil && *value.HistoryDefinitionID > 0 && value.CurrentCampaignID == nil
	case "current_definition":
		return value.HistoryDefinitionID == nil && value.CurrentCampaignID != nil && *value.CurrentCampaignID > 0
	case "unresolved_definition":
		return value.HistoryDefinitionID == nil && value.CurrentCampaignID == nil
	default:
		return false
	}
}

func validCampaignDefinitionHistorySource(source string, digest [sha256.Size]byte) bool {
	return digest != ([sha256.Size]byte{}) && validCampaignHistorySourceIdentifier(source) && source == hex.EncodeToString(digest[:])
}

func validCampaignHistoryRedactedRoots(roots []string) bool {
	return validCampaignHistoryText(roots...)
}

func normalizeHistoricalCampaignDefinition(value campaignport.HistoricalCampaignDefinition) campaignport.HistoricalCampaignDefinition {
	value.ApprovedAt = normalizeCampaignHistoryTimePointer(value.ApprovedAt)
	value.StartedAt = normalizeCampaignHistoryTimePointer(value.StartedAt)
	value.FinishedAt = normalizeCampaignHistoryTimePointer(value.FinishedAt)
	value.PausedAt = normalizeCampaignHistoryTimePointer(value.PausedAt)
	value.CreatedAt, value.UpdatedAt = normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt)
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	return value
}

func normalizeHistoricalCampaignDefinitionStep(value campaignport.HistoricalCampaignDefinitionStep) campaignport.HistoricalCampaignDefinitionStep {
	value.HistoryDefinitionID = cloneCampaignHistoryID(value.HistoryDefinitionID)
	value.CurrentCampaignID = cloneCampaignHistoryID(value.CurrentCampaignID)
	value.CreatedAt, value.UpdatedAt = normalizeCampaignHistoryTime(value.CreatedAt), normalizeCampaignHistoryTime(value.UpdatedAt)
	value.RedactedRoots = append([]string{}, value.RedactedRoots...)
	return value
}

func campaignDefinitionHistoryReady(writer *CampaignDefinitionHistoryWriter, ctx context.Context) bool {
	return writer != nil && ctx != nil && ctx.Err() == nil && !campaignHistoryNil(writer.store) && !campaignHistoryNil(writer.journal)
}

func campaignDefinitionHistoryInvalidOrUnavailable(writer *CampaignDefinitionHistoryWriter, ctx context.Context) (campaignport.CampaignHistoryReceipt, error) {
	if !campaignDefinitionHistoryReady(writer, ctx) {
		return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryUnavailable
	}
	return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryInvalid
}

var _ campaignport.CampaignHistoryJournal = campaignDefinitionHistoryJournalBridge{}

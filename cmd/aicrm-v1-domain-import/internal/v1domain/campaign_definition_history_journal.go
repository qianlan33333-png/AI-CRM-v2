package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	campaignDefinitionHistoryImportVersion = "v1-campaign-definition-history-a1"
	campaignDefinitionHistoryDomain        = "campaign"

	campaignDefinitionHistoryDefinitionKind = "definitions"
	campaignDefinitionHistoryStepKind       = "steps"

	campaignDefinitionHistoryDefinitionTable  = "public/campaigns"
	campaignDefinitionHistoryStepTable        = "public/campaign_steps"
	campaignDefinitionHistoryDefinitionTarget = "campaign_v1_definition_history"
	campaignDefinitionHistoryStepTarget       = "campaign_v1_definition_step_history"
)

var campaignDefinitionHistoryScopes = [...]struct{ kind, table, target string }{
	{campaignDefinitionHistoryDefinitionKind, campaignDefinitionHistoryDefinitionTable, campaignDefinitionHistoryDefinitionTarget},
	{campaignDefinitionHistoryStepKind, campaignDefinitionHistoryStepTable, campaignDefinitionHistoryStepTarget},
}

// CampaignDefinitionHistoryImportJournal keeps the owner writer and generic
// V1 receipt in the same caller-bound transaction.
type CampaignDefinitionHistoryImportJournal interface {
	campaignport.CampaignDefinitionHistoryJournal
	LoadCampaignDefinitionHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordCampaignDefinitionHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateCampaignDefinitionHistoryImportScope(string) error
}

type CampaignDefinitionHistoryJournal struct{ journals map[string]*Journal }

var _ CampaignDefinitionHistoryImportJournal = (*CampaignDefinitionHistoryJournal)(nil)

func NewCampaignDefinitionHistoryJournal(definitions, steps *Journal) (*CampaignDefinitionHistoryJournal, error) {
	journals := map[string]*Journal{
		campaignDefinitionHistoryDefinitionKind: definitions,
		campaignDefinitionHistoryStepKind:       steps,
	}
	if !validCampaignDefinitionHistoryJournals(journals) {
		return nil, ErrInvalidScope
	}
	return &CampaignDefinitionHistoryJournal{journals: journals}, nil
}

func (journal *CampaignDefinitionHistoryJournal) ValidateCampaignDefinitionHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validCampaignDefinitionHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range campaignDefinitionHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *CampaignDefinitionHistoryJournal) LoadCampaignDefinitionHistory(ctx context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadCampaignDefinitionHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return campaignport.CampaignHistoryReceipt{}, found, err
	}
	receipt, err := campaignDefinitionHistoryReceiptFromTerminal(kind, source, terminal)
	return receipt, err == nil, err
}

func (journal *CampaignDefinitionHistoryJournal) RecordCampaignDefinitionHistory(ctx context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	terminal, err := campaignDefinitionHistoryTerminalFromReceipt(kind, receipt)
	if err != nil {
		return err
	}
	return journal.RecordCampaignDefinitionHistoryTerminal(ctx, kind, terminal)
}

func (journal *CampaignDefinitionHistoryJournal) LoadCampaignDefinitionHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	key, err := ParseSourceIdentifier(source)
	if err != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, source)
}

func (journal *CampaignDefinitionHistoryJournal) RecordCampaignDefinitionHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.forKind(kind)
	if err != nil || ctx == nil {
		return ErrInvalidScope
	}
	return selected.Record(ctx, receipt)
}

func (journal *CampaignDefinitionHistoryJournal) forKind(kind string) (*Journal, error) {
	if journal == nil || !validCampaignDefinitionHistoryKind(kind) || !validCampaignDefinitionHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func campaignDefinitionHistoryReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (campaignport.CampaignHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !validCampaignDefinitionHistoryKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return campaignport.CampaignHistoryReceipt{}, ErrConflict
	}
	return campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func campaignDefinitionHistoryTerminalFromReceipt(kind string, receipt campaignport.CampaignHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !validCampaignDefinitionHistoryKind(kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func validCampaignDefinitionHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(campaignDefinitionHistoryScopes) {
		return false
	}
	var run string
	for _, scope := range campaignDefinitionHistoryScopes {
		journal := journals[scope.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != campaignDefinitionHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != scope.table ||
			journal.scope.TargetDomain != campaignDefinitionHistoryDomain || journal.scope.TargetTable != scope.target {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return run != ""
}

func validCampaignDefinitionHistoryKind(kind string) bool {
	for _, scope := range campaignDefinitionHistoryScopes {
		if kind == scope.kind {
			return true
		}
	}
	return false
}

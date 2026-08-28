package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	campaignport "github.com/qianlan33333-png/AI-CRM-v2/internal/campaign/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	campaignHistoryImportVersion = "v1-campaign-history-a1"
	campaignHistoryTargetDomain  = "campaign"

	campaignHistorySegmentsKind   = "segments"
	campaignHistoryMembersKind    = "members"
	campaignHistoryPlansKind      = "plans"
	campaignHistoryRecipientsKind = "recipients"
	campaignHistoryMessagesKind   = "messages"

	campaignHistorySegmentsTable   = "public/campaign_segments"
	campaignHistoryMembersTable    = "public/campaign_members"
	campaignHistoryPlansTable      = "public/cloud_broadcast_plans"
	campaignHistoryRecipientsTable = "public/cloud_broadcast_plan_recipients"
	campaignHistoryMessagesTable   = "public/cloud_broadcast_plan_recipient_messages"
)

var campaignHistoryScopes = map[string][2]string{
	campaignHistorySegmentsKind:   {campaignHistorySegmentsTable, "campaign_v1_history_segments"},
	campaignHistoryMembersKind:    {campaignHistoryMembersTable, "campaign_v1_history_members"},
	campaignHistoryPlansKind:      {campaignHistoryPlansTable, "campaign_v1_history_broadcast_plans"},
	campaignHistoryRecipientsKind: {campaignHistoryRecipientsTable, "campaign_v1_history_broadcast_recipients"},
	campaignHistoryMessagesKind:   {campaignHistoryMessagesTable, "campaign_v1_history_broadcast_messages"},
}

type campaignHistoryTerminalJournal interface {
	LoadTerminal(context.Context, string) (TerminalReceipt, bool, error)
	Record(context.Context, TerminalReceipt) error
}

// CampaignHistoryJournal keeps the five owner-owned history streams within
// the migration receipt journal. It has no current Campaign or dispatch role.
type CampaignHistoryJournal struct {
	journals map[string]campaignHistoryTerminalJournal
}

var _ campaignport.CampaignHistoryJournal = (*CampaignHistoryJournal)(nil)

func NewCampaignHistoryJournal(segments, members, plans, recipients, messages *Journal) (*CampaignHistoryJournal, error) {
	values := map[string]*Journal{
		campaignHistorySegmentsKind: segments, campaignHistoryMembersKind: members,
		campaignHistoryPlansKind: plans, campaignHistoryRecipientsKind: recipients,
		campaignHistoryMessagesKind: messages,
	}
	if !validCampaignHistoryJournalKinds(values) {
		return nil, campaignport.ErrCampaignHistoryInvalid
	}
	terminals := make(map[string]campaignHistoryTerminalJournal, len(values))
	for kind, journal := range values {
		terminals[kind] = journal
	}
	return newCampaignHistoryJournal(terminals)
}

func newCampaignHistoryJournal(journals map[string]campaignHistoryTerminalJournal) (*CampaignHistoryJournal, error) {
	if len(journals) != len(campaignHistoryScopes) {
		return nil, campaignport.ErrCampaignHistoryInvalid
	}
	for kind := range campaignHistoryScopes {
		if journals[kind] == nil {
			return nil, campaignport.ErrCampaignHistoryInvalid
		}
	}
	return &CampaignHistoryJournal{journals: journals}, nil
}

// validCampaignHistoryJournalKinds validates the owner Journal constructor,
// whose five arguments are keyed by owner receipt kind.
func validCampaignHistoryJournalKinds(journals map[string]*Journal) bool {
	if len(journals) != len(campaignHistoryScopes) {
		return false
	}
	var run string
	for kind, expected := range campaignHistoryScopes {
		journal := journals[kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() ||
			journal.scope.ImportVersion != campaignHistoryImportVersion || journal.scope.AdapterID != v1archive.DefaultAdapterID ||
			journal.scope.TableID != expected[0] || journal.scope.TargetDomain != campaignHistoryTargetDomain || journal.scope.TargetTable != expected[1] {
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

// validCampaignHistoryImportJournals validates the importer map, whose keys
// are source table IDs so terminal quarantine receipts use the same scope as
// their archived source row.
func validCampaignHistoryImportJournals(journals map[string]*Journal) bool {
	if len(journals) != len(campaignHistoryScopes) {
		return false
	}
	byKind := make(map[string]*Journal, len(campaignHistoryScopes))
	for kind, expected := range campaignHistoryScopes {
		journal := journals[expected[0]]
		if journal == nil {
			return false
		}
		byKind[kind] = journal
	}
	return validCampaignHistoryJournalKinds(byKind)
}

func (journal *CampaignHistoryJournal) LoadCampaignHistory(ctx context.Context, kind, source string) (campaignport.CampaignHistoryReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return campaignport.CampaignHistoryReceipt{}, false, err
	}
	sourceKey, err := ParseSourceIdentifier(source)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || source != SourceIdentifier(sourceKey) {
		return campaignport.CampaignHistoryReceipt{}, false, campaignport.ErrCampaignHistoryInvalid
	}
	terminal, found, err := selected.LoadTerminal(ctx, source)
	if err != nil || !found {
		return campaignport.CampaignHistoryReceipt{}, found, err
	}
	receipt, err := campaignHistoryReceiptFromTerminal(source, terminal)
	if err != nil {
		return campaignport.CampaignHistoryReceipt{}, false, err
	}
	return receipt, true, nil
}

func (journal *CampaignHistoryJournal) RecordCampaignHistory(ctx context.Context, kind string, receipt campaignport.CampaignHistoryReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return err
	}
	terminal, err := campaignHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return selected.Record(ctx, terminal)
}

func (journal *CampaignHistoryJournal) selectJournal(kind string) (campaignHistoryTerminalJournal, error) {
	if journal == nil || journal.journals == nil {
		return nil, campaignport.ErrCampaignHistoryInvalid
	}
	if _, known := campaignHistoryScopes[kind]; !known || journal.journals[kind] == nil {
		return nil, campaignport.ErrCampaignHistoryInvalid
	}
	return journal.journals[kind], nil
}

func campaignHistoryReceiptFromTerminal(source string, terminal TerminalReceipt) (campaignport.CampaignHistoryReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(source)
	targetID, idErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || idErr != nil || sourceKey == ([sha256.Size]byte{}) || source != SourceIdentifier(sourceKey) ||
		targetID < 1 || strconv.FormatInt(targetID, 10) != terminal.TargetID || terminal.SourceKeyDigest != sourceKey ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) ||
		terminal.Disposition != "import" || terminal.Reason != "" || len(terminal.Metadata) != 0 {
		return campaignport.CampaignHistoryReceipt{}, campaignport.ErrCampaignHistoryConflict
	}
	return campaignport.CampaignHistoryReceipt{SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: targetID, TargetDigest: terminal.TargetDigest}, nil
}

func campaignHistoryTerminalFromReceipt(receipt campaignport.CampaignHistoryReceipt) (TerminalReceipt, error) {
	sourceKey, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || sourceKey == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(sourceKey) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.Replayed {
		return TerminalReceipt{}, campaignport.ErrCampaignHistoryInvalid
	}
	return TerminalReceipt{SourceKeyDigest: sourceKey, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func sameCampaignHistoryTerminal(left, right TerminalReceipt) bool {
	return left.SourceKeyDigest == right.SourceKeyDigest && left.PayloadDigest == right.PayloadDigest && left.Disposition == right.Disposition &&
		left.Reason == right.Reason && left.TargetID == right.TargetID && left.TargetDigest == right.TargetDigest && len(left.Metadata) == len(right.Metadata)
}

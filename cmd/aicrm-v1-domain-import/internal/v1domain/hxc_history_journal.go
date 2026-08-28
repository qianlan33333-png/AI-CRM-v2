package v1domain

import (
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1hxchistory"
	hxcport "github.com/qianlan33333-png/AI-CRM-v2/internal/hxc/port"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
)

const (
	hxcHistoryImportVersion = "v1-hxc-history-a1"
	hxcHistoryDomain        = "hxc"

	hxcHistoryMetaTarget       = "hxc_v1_dashboard_refresh_history"
	hxcHistorySnapshotTarget   = "hxc_v1_dashboard_observations"
	hxcHistoryActivationTarget = "hxc_v1_activation_observations"
	hxcHistoryLeadTarget       = "hxc_v1_experience_lead_history"
	hxcHistoryBatchTarget      = "hxc_v1_import_batch_history"
	hxcHistoryArchiveTarget    = "hxc_v1_runtime_archive"

	hxcHistorySendRecordsKind = "send_records_archive"
	hxcHistorySendConfigKind  = "send_config_archive"
)

var hxcHistoryTables = [...]string{
	v1hxchistory.DashboardMetaTableID,
	v1hxchistory.DashboardSnapshotTableID,
	v1hxchistory.ActivationStatusTableID,
	v1hxchistory.HuangxiaocanActivationID,
	v1hxchistory.ExperienceLeadsTableID,
	v1hxchistory.ImportBatchesTableID,
	v1hxchistory.SendRecordsTableID,
	v1hxchistory.SendConfigTableID,
}

type hxcHistoryScope struct{ kind, table, target string }

var hxcHistoryScopes = [...]hxcHistoryScope{
	{hxcport.HXCHistoryMeta, v1hxchistory.DashboardMetaTableID, hxcHistoryMetaTarget},
	{hxcport.HXCHistorySnapshot, v1hxchistory.DashboardSnapshotTableID, hxcHistorySnapshotTarget},
	{hxcport.HXCHistoryActivationStatus, v1hxchistory.ActivationStatusTableID, hxcHistoryActivationTarget},
	{hxcport.HXCHistoryHuangxiaocanActivation, v1hxchistory.HuangxiaocanActivationID, hxcHistoryActivationTarget},
	{hxcport.HXCHistoryLead, v1hxchistory.ExperienceLeadsTableID, hxcHistoryLeadTarget},
	{hxcport.HXCHistoryBatch, v1hxchistory.ImportBatchesTableID, hxcHistoryBatchTarget},
	{hxcHistorySendRecordsKind, v1hxchistory.SendRecordsTableID, hxcHistoryArchiveTarget},
	{hxcHistorySendConfigKind, v1hxchistory.SendConfigTableID, hxcHistoryArchiveTarget},
}

// HXCHistoryImportJournal is private composition glue. Both the owner writer
// and archive-only terminals use the same caller transaction.
type HXCHistoryImportJournal interface {
	hxcport.HXCHistoryJournal
	LoadHXCHistoryTerminal(context.Context, string, string) (TerminalReceipt, bool, error)
	RecordHXCHistoryTerminal(context.Context, string, TerminalReceipt) error
	ValidateHXCHistoryImportScope(string) error
}

// HXCHistoryJournal routes immutable HXC observations to their exact source
// scope. It cannot create current HXC state, sends, queues, or Provider work.
type HXCHistoryJournal struct{ journals map[string]*Journal }

var _ HXCHistoryImportJournal = (*HXCHistoryJournal)(nil)

func NewHXCHistoryJournal(meta, snapshot, activationStatus, huangxiaocan, lead, batch, sendRecords, sendConfig *Journal) (*HXCHistoryJournal, error) {
	values := map[string]*Journal{
		hxcport.HXCHistoryMeta:                   meta,
		hxcport.HXCHistorySnapshot:               snapshot,
		hxcport.HXCHistoryActivationStatus:       activationStatus,
		hxcport.HXCHistoryHuangxiaocanActivation: huangxiaocan,
		hxcport.HXCHistoryLead:                   lead,
		hxcport.HXCHistoryBatch:                  batch,
		hxcHistorySendRecordsKind:                sendRecords,
		hxcHistorySendConfigKind:                 sendConfig,
	}
	if !validHXCHistoryJournals(values) {
		return nil, ErrInvalidScope
	}
	return &HXCHistoryJournal{journals: values}, nil
}

func (journal *HXCHistoryJournal) ValidateHXCHistoryImportScope(run string) error {
	if journal == nil || run == "" || !validHXCHistoryJournals(journal.journals) {
		return ErrInvalidScope
	}
	for _, scope := range hxcHistoryScopes {
		if journal.journals[scope.kind].scope.ArchiveRunID != run {
			return ErrInvalidScope
		}
	}
	return nil
}

func (journal *HXCHistoryJournal) LoadHXCHistory(ctx context.Context, kind, source string) (hxcport.HXCHistoryReceipt, bool, error) {
	terminal, found, err := journal.LoadHXCHistoryTerminal(ctx, kind, source)
	if err != nil || !found {
		return hxcport.HXCHistoryReceipt{}, found, err
	}
	receipt, err := hxcHistoryReceiptFromTerminal(kind, source, terminal)
	if err != nil {
		return hxcport.HXCHistoryReceipt{}, false, err
	}
	return receipt, true, nil
}

func (journal *HXCHistoryJournal) RecordHXCHistory(ctx context.Context, receipt hxcport.HXCHistoryReceipt) error {
	terminal, err := hxcHistoryTerminalFromReceipt(receipt)
	if err != nil {
		return err
	}
	return journal.RecordHXCHistoryTerminal(ctx, receipt.Kind, terminal)
}

func (journal *HXCHistoryJournal) LoadHXCHistoryTerminal(ctx context.Context, kind, source string) (TerminalReceipt, bool, error) {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return TerminalReceipt{}, false, err
	}
	key, err := ParseSourceIdentifier(source)
	if err != nil || key == ([sha256.Size]byte{}) || source != SourceIdentifier(key) {
		return TerminalReceipt{}, false, ErrInvalidScope
	}
	return selected.LoadTerminal(ctx, source)
}

func (journal *HXCHistoryJournal) RecordHXCHistoryTerminal(ctx context.Context, kind string, receipt TerminalReceipt) error {
	selected, err := journal.selectJournal(kind)
	if err != nil {
		return err
	}
	return selected.Record(ctx, receipt)
}

func (journal *HXCHistoryJournal) selectJournal(kind string) (*Journal, error) {
	if journal == nil || !validHXCHistoryKind(kind) || !validHXCHistoryJournals(journal.journals) {
		return nil, ErrInvalidScope
	}
	return journal.journals[kind], nil
}

func hxcHistoryReceiptFromTerminal(kind, source string, terminal TerminalReceipt) (hxcport.HXCHistoryReceipt, error) {
	key, err := ParseSourceIdentifier(source)
	id, idErr := positiveID(terminal.TargetID)
	if err != nil || idErr != nil || !hxcHistoryBusinessKind(kind) || key == ([sha256.Size]byte{}) || key != terminal.SourceKeyDigest ||
		terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.TargetDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" ||
		len(terminal.Metadata) != 0 || strconv.FormatInt(id, 10) != terminal.TargetID {
		return hxcport.HXCHistoryReceipt{}, ErrConflict
	}
	return hxcport.HXCHistoryReceipt{Kind: kind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: id, TargetDigest: terminal.TargetDigest}, nil
}

func hxcHistoryTerminalFromReceipt(receipt hxcport.HXCHistoryReceipt) (TerminalReceipt, error) {
	key, err := ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || !hxcHistoryBusinessKind(receipt.Kind) || key == ([sha256.Size]byte{}) || receipt.SourceIdentifier != SourceIdentifier(key) ||
		receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) || receipt.TargetID < 1 || receipt.Replayed {
		return TerminalReceipt{}, ErrInvalidScope
	}
	return TerminalReceipt{SourceKeyDigest: key, PayloadDigest: receipt.PayloadDigest, Disposition: "import", TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest}, nil
}

func hxcHistoryScopeForKind(kind string) (hxcHistoryScope, bool) {
	for _, scope := range hxcHistoryScopes {
		if scope.kind == kind {
			return scope, true
		}
	}
	return hxcHistoryScope{}, false
}

func validHXCHistoryKind(kind string) bool {
	_, ok := hxcHistoryScopeForKind(kind)
	return ok
}

func hxcHistoryBusinessKind(kind string) bool {
	return kind != hxcHistorySendRecordsKind && kind != hxcHistorySendConfigKind && validHXCHistoryKind(kind)
}

func validHXCHistoryJournals(journals map[string]*Journal) bool {
	if len(journals) != len(hxcHistoryScopes) {
		return false
	}
	var run string
	for _, expected := range hxcHistoryScopes {
		journal := journals[expected.kind]
		if journal == nil || journal.tx == nil || !journal.scope.valid() || journal.scope.ImportVersion != hxcHistoryImportVersion ||
			journal.scope.AdapterID != v1archive.DefaultAdapterID || journal.scope.TableID != expected.table || journal.scope.TargetDomain != hxcHistoryDomain || journal.scope.TargetTable != expected.target {
			return false
		}
		if run == "" {
			run = journal.scope.ArchiveRunID
		} else if run != journal.scope.ArchiveRunID {
			return false
		}
	}
	return true
}

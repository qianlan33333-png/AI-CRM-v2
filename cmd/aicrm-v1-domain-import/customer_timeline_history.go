package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1customertimelinehistory"
	"github.com/qianlan33333-png/AI-CRM-v2/cmd/aicrm-v1-domain-import/internal/v1domain"
	contactapp "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/app"
	contact "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/port"
	contactstore "github.com/qianlan33333-png/AI-CRM-v2/internal/contact/store"
	"github.com/qianlan33333-png/AI-CRM-v2/internal/migration/v1archive"
	platformstore "github.com/qianlan33333-png/AI-CRM-v2/internal/platform/store"
)

const (
	customerTimelineHistoryDomain        = "customer-timeline-history"
	customerTimelineHistoryImportVersion = "v1-customer-timeline-history-a1"
	customerTimelineHistoryKind          = "customer_timeline_event"
	customerTimelineHistoryTargetTable   = "contact_v1_customer_timeline_history"
	customerTimelineHistoryFieldMetadata = "field_hmac"
)

type customerTimelineHistoryReferences struct{ customer *channelCustomerResolver }

func (references *customerTimelineHistoryReferences) ResolveVerifiedCustomerTimelineUnionID(ctx context.Context, unionID string) (*int64, error) {
	if references == nil || references.customer == nil {
		return nil, v1domain.ErrInvalidScope
	}
	return references.customer.ResolveHistoricalChannelCustomer(ctx, unionID)
}

// customerTimelineHistoryJournal uses one generic receipt stream for both the
// Contact app writer and importer terminal checks. field_hmac is retained in
// generic metadata because that generic schema has no field_digest column.
type customerTimelineHistoryJournal struct {
	terminal   *v1domain.Journal
	targets    contact.CustomerTimelineHistoryStore
	archiveRun string
}

var _ contact.CustomerTimelineHistoryJournal = (*customerTimelineHistoryJournal)(nil)
var _ v1domain.CustomerTimelineImportJournal = (*customerTimelineHistoryJournal)(nil)

func newCustomerTimelineHistoryJournal(run string, targets contact.CustomerTimelineHistoryStore) (*customerTimelineHistoryJournal, error) {
	if run == "" || targets == nil {
		return nil, v1domain.ErrInvalidScope
	}
	terminal, err := v1domain.NewJournal(v1domain.Scope{
		ImportVersion: customerTimelineHistoryImportVersion,
		ArchiveRunID:  run,
		AdapterID:     v1archive.DefaultAdapterID,
		TableID:       v1customertimelinehistory.TableID,
		TargetDomain:  "contact",
		TargetTable:   customerTimelineHistoryTargetTable,
	})
	if err != nil {
		return nil, err
	}
	return &customerTimelineHistoryJournal{terminal: terminal, targets: targets, archiveRun: run}, nil
}

func (journal *customerTimelineHistoryJournal) LoadCustomerTimelineHistory(ctx context.Context, kind, source string) (contact.CustomerTimelineHistoryReceipt, bool, error) {
	if journal == nil || journal.terminal == nil || kind != customerTimelineHistoryKind || ctx == nil {
		return contact.CustomerTimelineHistoryReceipt{}, false, v1domain.ErrInvalidScope
	}
	terminal, found, err := journal.terminal.LoadTerminal(ctx, source)
	if err != nil || !found {
		return contact.CustomerTimelineHistoryReceipt{}, found, err
	}
	field, err := customerTimelineHistoryFieldHMAC(terminal.Metadata)
	if err != nil {
		return contact.CustomerTimelineHistoryReceipt{}, false, err
	}
	_ = field // validates the field-level archive proof for app replay too.
	return customerTimelineHistoryReceipt(source, terminal)
}

func (journal *customerTimelineHistoryJournal) RecordCustomerTimelineHistory(ctx context.Context, receipt contact.CustomerTimelineHistoryReceipt) error {
	if journal == nil || journal.terminal == nil || journal.targets == nil || ctx == nil || receipt.Kind != customerTimelineHistoryKind || receipt.Replayed || receipt.TargetID < 1 || receipt.PayloadDigest == ([sha256.Size]byte{}) || receipt.TargetDigest == ([sha256.Size]byte{}) {
		return v1domain.ErrInvalidScope
	}
	source, err := v1domain.ParseSourceIdentifier(receipt.SourceIdentifier)
	if err != nil || source == ([sha256.Size]byte{}) {
		return v1domain.ErrInvalidScope
	}
	target, err := journal.targets.GetHistoricalCustomerTimelineEvent(ctx, receipt.TargetID)
	if err != nil {
		return err
	}
	digest, err := contactapp.HistoricalCustomerTimelineEventDigest(target)
	if err != nil || target.ID != receipt.TargetID || target.SourceKeyDigest != source || target.SourcePayloadDigest != receipt.PayloadDigest || target.SourceFieldDigest == ([sha256.Size]byte{}) || digest != receipt.TargetDigest {
		return v1domain.ErrConflict
	}
	return journal.terminal.Record(ctx, v1domain.TerminalReceipt{
		SourceKeyDigest: source, PayloadDigest: receipt.PayloadDigest, Disposition: "import",
		TargetID: strconv.FormatInt(receipt.TargetID, 10), TargetDigest: receipt.TargetDigest,
		Metadata: customerTimelineHistoryMetadata(target.SourceFieldDigest),
	})
}

func (journal *customerTimelineHistoryJournal) LoadCustomerTimelineTerminal(ctx context.Context, version string, source [sha256.Size]byte) (v1domain.CustomerTimelineTerminal, bool, error) {
	if journal == nil || journal.terminal == nil || version != customerTimelineHistoryImportVersion || ctx == nil || source == ([sha256.Size]byte{}) {
		return v1domain.CustomerTimelineTerminal{}, false, v1domain.ErrInvalidScope
	}
	terminal, found, err := journal.terminal.LoadTerminal(ctx, v1domain.SourceIdentifier(source))
	if err != nil || !found {
		return v1domain.CustomerTimelineTerminal{}, found, err
	}
	return customerTimelineHistoryTerminal(journal.archiveRun, terminal)
}

func (journal *customerTimelineHistoryJournal) RecordCustomerTimelineTerminal(ctx context.Context, value v1domain.CustomerTimelineTerminal) error {
	if journal == nil || journal.terminal == nil || ctx == nil || value.Version != customerTimelineHistoryImportVersion || value.TableID != v1customertimelinehistory.TableID || value.Kind != customerTimelineHistoryKind || value.SourceKeyHMAC == ([sha256.Size]byte{}) || value.PayloadHMAC == ([sha256.Size]byte{}) || value.FieldHMAC == ([sha256.Size]byte{}) || value.Disposition != v1customertimelinehistory.DispositionQuarantine || value.Reason == "" || value.TargetID != 0 || value.TargetDigest != ([sha256.Size]byte{}) {
		return v1domain.ErrInvalidScope
	}
	return journal.terminal.Record(ctx, v1domain.TerminalReceipt{
		SourceKeyDigest: value.SourceKeyHMAC, PayloadDigest: value.PayloadHMAC,
		Disposition: "quarantine", Reason: value.Reason,
		Metadata: customerTimelineHistoryMetadata(value.FieldHMAC),
	})
}

func customerTimelineHistoryReceipt(source string, terminal v1domain.TerminalReceipt) (contact.CustomerTimelineHistoryReceipt, bool, error) {
	key, err := v1domain.ParseSourceIdentifier(source)
	targetID, targetErr := strconv.ParseInt(terminal.TargetID, 10, 64)
	if err != nil || targetErr != nil || targetID < 1 || strconv.FormatInt(targetID, 10) != terminal.TargetID || key == ([sha256.Size]byte{}) || terminal.SourceKeyDigest != key || terminal.PayloadDigest == ([sha256.Size]byte{}) || terminal.Disposition != "import" || terminal.Reason != "" || terminal.TargetDigest == ([sha256.Size]byte{}) {
		return contact.CustomerTimelineHistoryReceipt{}, false, v1domain.ErrConflict
	}
	if _, err = customerTimelineHistoryFieldHMAC(terminal.Metadata); err != nil {
		return contact.CustomerTimelineHistoryReceipt{}, false, err
	}
	return contact.CustomerTimelineHistoryReceipt{Kind: customerTimelineHistoryKind, SourceIdentifier: source, PayloadDigest: terminal.PayloadDigest, TargetID: targetID, TargetDigest: terminal.TargetDigest}, true, nil
}

func customerTimelineHistoryTerminal(archiveRun string, terminal v1domain.TerminalReceipt) (v1domain.CustomerTimelineTerminal, bool, error) {
	if archiveRun == "" {
		return v1domain.CustomerTimelineTerminal{}, false, v1domain.ErrInvalidScope
	}
	field, err := customerTimelineHistoryFieldHMAC(terminal.Metadata)
	if err != nil {
		return v1domain.CustomerTimelineTerminal{}, false, err
	}
	value := v1domain.CustomerTimelineTerminal{
		Version: customerTimelineHistoryImportVersion, ArchiveRunID: archiveRun, TableID: v1customertimelinehistory.TableID, Kind: customerTimelineHistoryKind,
		SourceKeyHMAC: terminal.SourceKeyDigest, PayloadHMAC: terminal.PayloadDigest, FieldHMAC: field,
		Disposition: terminal.Disposition, Reason: terminal.Reason, TargetDigest: terminal.TargetDigest,
	}
	switch terminal.Disposition {
	case "import":
		id, parseErr := strconv.ParseInt(terminal.TargetID, 10, 64)
		if parseErr != nil || id < 1 || strconv.FormatInt(id, 10) != terminal.TargetID {
			return v1domain.CustomerTimelineTerminal{}, false, v1domain.ErrConflict
		}
		value.TargetID = id
	case "quarantine":
		if terminal.TargetID != "" || terminal.TargetDigest != ([sha256.Size]byte{}) {
			return v1domain.CustomerTimelineTerminal{}, false, v1domain.ErrConflict
		}
	default:
		return v1domain.CustomerTimelineTerminal{}, false, v1domain.ErrConflict
	}
	return value, true, nil
}

func customerTimelineHistoryMetadata(field [sha256.Size]byte) map[string]any {
	return map[string]any{customerTimelineHistoryFieldMetadata: hex.EncodeToString(field[:])}
}

func customerTimelineHistoryFieldHMAC(metadata map[string]any) ([sha256.Size]byte, error) {
	if len(metadata) != 1 {
		return [sha256.Size]byte{}, v1domain.ErrConflict
	}
	encoded, found := metadata[customerTimelineHistoryFieldMetadata]
	value, ok := encoded.(string)
	if !found || !ok || len(value) != 2*sha256.Size {
		return [sha256.Size]byte{}, v1domain.ErrConflict
	}
	var digest [sha256.Size]byte
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, v1domain.ErrConflict
	}
	copy(digest[:], decoded)
	if value != hex.EncodeToString(digest[:]) || digest == ([sha256.Size]byte{}) {
		return [sha256.Size]byte{}, v1domain.ErrConflict
	}
	return digest, nil
}

func importCustomerTimelineHistory(ctx context.Context, archive *v1archive.PostgresArchiveReader, uow *platformstore.UnitOfWork, run string, dm01Run int64, dm01Key, sourceKey []byte, reconcile bool) (any, error) {
	if ctx == nil || archive == nil || uow == nil || run == "" || dm01Run < 1 || len(dm01Key) < sha256.Size || len(sourceKey) < sha256.Size {
		return nil, v1domain.ErrInvalidScope
	}
	resolver, err := newChannelCustomerResolver(ctx, uow, dm01Run, dm01Key)
	if err != nil {
		return nil, err
	}
	targets := contactstore.NewCustomerTimelineHistoryStore()
	journal, err := newCustomerTimelineHistoryJournal(run, targets)
	if err != nil {
		return nil, err
	}
	writer, err := contactapp.NewCustomerTimelineHistoryWriter(targets, journal)
	if err != nil {
		return nil, err
	}
	importer, err := v1domain.NewCustomerTimelineHistoryImporter(v1domain.NewCustomerTimelineArchiveReadySQL(), archive, uow, writer,
		&customerTimelineHistoryReferences{customer: resolver}, targets, journal, v1domain.NewCustomerTimelineReconciliationSealStore())
	if err != nil {
		return nil, err
	}
	if reconcile {
		return importer.Reconcile(ctx, run, sourceKey)
	}
	return importer.Import(ctx, run, sourceKey)
}
